package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrFileTooLarge is returned when a file exceeds the download/upload size
// cap (FetchSizeCapMB). Handlers map it to 413; other storage errors stay 502.
var ErrFileTooLarge = errors.New("cloud: file exceeds the size cap")

// ErrNoDrive is returned when a OneDrive account lacks the drive_id stamped
// at connect time.
var ErrNoDrive = errors.New("cloud storage: onedrive account has no drive_id — reconnect the account")

// Re-exports of the native backend sentinel errors for handler mapping.
var (
	ErrNotFound    = storage.ErrNotFound
	ErrDirNotEmpty = storage.ErrDirNotEmpty
	ErrNotDir      = storage.ErrNotDir
)

// StorageService bridges connected accounts to NATIVE provider backends
// (Google Drive v3 / Microsoft Graph — no external binary). Backends are
// cached per account and rebuilt when the account row changes (re-grant,
// reconnect), so authorization always follows the DB tokens.
type StorageService struct {
	manager    *Manager
	transfers  *transferRegistry // async folder copy jobs (in-memory, this process)
	configDir  string            // unused (legacy rclone.conf dir); kept for call-site compat
	backendsMu sync.Mutex
	backends   map[string]*backendEntry
}

type backendEntry struct {
	updatedAt time.Time // account row generation the backend was built from
	backend   storage.Backend
}

// NewStorageService creates the storage service.
// configDir is retained for call-site compatibility (the former rclone.conf
// location); native backends keep no credentials on disk.
func NewStorageService(manager *Manager, configDir string) *StorageService {
	return &StorageService{
		manager:   manager,
		transfers: newTransferRegistry(),
		configDir: configDir,
		backends:  map[string]*backendEntry{},
	}
}

// Shutdown cancels every running async copy job (gateway close). Native
// backends hold no child processes — nothing else to stop.
func (s *StorageService) Shutdown() {
	if s.transfers != nil {
		s.transfers.shutdownAll()
	}
}

// RemoveRemote kept for call-site compatibility (Manager disconnect/re-grant
// paths): the former rclone remotes persisted tokens in a shared config file,
// native backends do not — nothing to clean up.
func (s *StorageService) RemoveRemote(_ context.Context, _ string) {}

// backendFor returns the cached native backend for the account, rebuilding it
// whenever the account row changed (re-grant stamps a new token + updated_at).
func (s *StorageService) backendFor(ctx context.Context, acct *store.CloudAccount) (storage.Backend, error) {
	s.backendsMu.Lock()
	if e, ok := s.backends[acct.ID]; ok && e.updatedAt.Equal(acct.UpdatedAt) {
		s.backendsMu.Unlock()
		return e.backend, nil
	}
	s.backendsMu.Unlock()

	ts, err := s.manager.TokenSource(ctx, acct.ID)
	if err != nil {
		return nil, err
	}
	var backend storage.Backend
	switch acct.Provider {
	case GoogleProvider:
		backend = storage.NewDriveBackend(ctx, ts)
	case MicrosoftProvider:
		var settings struct {
			DriveID string `json:"drive_id"`
		}
		if acct.Settings != "" {
			if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
				return nil, fmt.Errorf("cloud storage: account settings: %w", err)
			}
		}
		if settings.DriveID == "" {
			return nil, ErrNoDrive
		}
		backend = storage.NewGraphBackend(ctx, ts, settings.DriveID)
	default:
		return nil, fmt.Errorf("cloud storage: provider %q is not storage-capable", acct.Provider)
	}

	s.backendsMu.Lock()
	s.backends[acct.ID] = &backendEntry{updatedAt: acct.UpdatedAt, backend: backend}
	s.backendsMu.Unlock()
	return backend, nil
}

// resolveAccount picks by email/ID when given, else per-scope bindings
// (group → user → tenant default) and finally the caller's own accounts —
// including tenant-shared ones (the enterprise "company drive" pattern).
func (s *StorageService) resolveAccount(ctx context.Context, name string) (*store.CloudAccount, error) {
	return s.manager.ResolveAccount(ctx, name, []string{GoogleProvider, MicrosoftProvider}, func(a *store.CloudAccount) bool {
		return isStorageProvider(a.Provider)
	})
}

// AgentAccount resolves the account for an AGENT tool call, additionally
// enforcing the per-account agent access level (admin-configured). The web UI
// is unaffected — this only gates what agents may touch. An explicit account
// that exists but is below the required level fails with the actionable
// denied error (not a misleading "not found").
func (s *StorageService) AgentAccount(ctx context.Context, name string, min AgentAccess) (*store.CloudAccount, error) {
	acct, err := s.manager.ResolveAccount(ctx, name, []string{GoogleProvider, MicrosoftProvider}, func(a *store.CloudAccount) bool {
		return isStorageProvider(a.Provider)
	})
	if err != nil {
		return nil, err
	}
	if level := AgentAccessOf(acct); !level.allows(min) {
		return nil, errAgentAccessDenied(level, acct.Email)
	}
	return acct, nil
}

// isStorageProvider gates which connected accounts the storage layer may use.
func isStorageProvider(provider string) bool {
	switch provider {
	case GoogleProvider, MicrosoftProvider:
		return true
	default:
		return false
	}
}

// IsStorageProvider reports whether the provider is usable by the storage
// tools. Exported for the agent-facing cloud_accounts tool.
func IsStorageProvider(provider string) bool { return isStorageProvider(provider) }

// List lists remote path entries (non-recursive).
func (s *StorageService) List(ctx context.Context, account, path string, max int) ([]storage.ListEntry, error) {
	acct, err := s.resolveAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return s.ListAccount(ctx, acct, path, max)
}

// Stat stats one remote path.
func (s *StorageService) Stat(ctx context.Context, account, path string) (*storage.StatInfo, error) {
	acct, err := s.resolveAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return s.StatAccount(ctx, acct, path)
}

// StatAccount stats one path on a pre-authorized account.
func (s *StorageService) StatAccount(ctx context.Context, acct *store.CloudAccount, path string) (*storage.StatInfo, error) {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return nil, err
	}
	if path, err = CleanRemotePath(path); err != nil {
		return nil, err
	}
	return backend.Stat(ctx, path)
}

// About returns quota info for the account's Drive.
func (s *StorageService) About(ctx context.Context, account string) (*storage.AboutInfo, error) {
	acct, err := s.resolveAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return s.aboutFor(ctx, acct)
}

func (s *StorageService) aboutFor(ctx context.Context, acct *store.CloudAccount) (*storage.AboutInfo, error) {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return nil, err
	}
	return backend.About(ctx)
}

// AboutAccount returns quota info for a specific connected account (no
// re-resolution — the caller already proved accessibility).
func (s *StorageService) AboutAccount(ctx context.Context, acct *store.CloudAccount) (*storage.AboutInfo, error) {
	return s.aboutFor(ctx, acct)
}

// ListAccount lists one account's remote path (caller proved accessibility).
// CleanRemoteDir (not CleanRemotePath): the drive root "/" is a valid listing
// target and normalizes to "".
func (s *StorageService) ListAccount(ctx context.Context, acct *store.CloudAccount, path string, max int) ([]storage.ListEntry, error) {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return nil, err
	}
	if path, err = CleanRemoteDir(path); err != nil {
		return nil, err
	}
	return backend.List(ctx, path, max)
}

// --- file operations: every method takes a pre-authorized account (caller
// proved accessibility). Paths are re-validated here as defense in depth —
// handlers validate for 400s first.

// MkdirAccount creates a directory in the account's remote.
func (s *StorageService) MkdirAccount(ctx context.Context, acct *store.CloudAccount, dir string) error {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return err
	}
	if dir, err = CleanRemotePath(dir); err != nil {
		return err
	}
	return backend.Mkdir(ctx, dir)
}

// DeleteAccount removes one file (isDir=false) or one EMPTY directory
// (isDir=true — providers refuse non-empty dirs, so wiping a populated tree
// is deliberately not exposed). Deletion is PERMANENT on Drive — no trash.
func (s *StorageService) DeleteAccount(ctx context.Context, acct *store.CloudAccount, path string, isDir bool) error {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return err
	}
	if path, err = CleanRemotePath(path); err != nil {
		return err
	}
	return backend.Delete(ctx, path, isDir)
}

// MoveAccount renames/moves one file within the account's remote.
func (s *StorageService) MoveAccount(ctx context.Context, acct *store.CloudAccount, from, to string) error {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return err
	}
	if from, err = CleanRemotePath(from); err != nil {
		return err
	}
	if to, err = CleanRemotePath(to); err != nil {
		return err
	}
	return backend.Move(ctx, from, to)
}

// CopyAccount copies one file to another path within the account's remote
// (server-side on both providers).
func (s *StorageService) CopyAccount(ctx context.Context, acct *store.CloudAccount, from, to string) error {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return err
	}
	if from, err = CleanRemotePath(from); err != nil {
		return err
	}
	if to, err = CleanRemotePath(to); err != nil {
		return err
	}
	return backend.Copy(ctx, from, to)
}

// CopyURLAccount uploads-by-URL: the server fetches url into the file at path
// (full destination path incl. name).
func (s *StorageService) CopyURLAccount(ctx context.Context, acct *store.CloudAccount, rawURL, path string) error {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return err
	}
	if path, err = CleanRemotePath(path); err != nil {
		return err
	}
	return backend.CopyURL(ctx, rawURL, path)
}

// PublicLinkAccount creates/retrieves the public share link for one path.
func (s *StorageService) PublicLinkAccount(ctx context.Context, acct *store.CloudAccount, path string) (*storage.PublicLinkInfo, error) {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return nil, err
	}
	if path, err = CleanRemotePath(path); err != nil {
		return nil, err
	}
	return backend.PublicLink(ctx, path)
}

// WriteAccount creates/overwrites the file at remotePath with content (the
// agent-facing write tool path).
func (s *StorageService) WriteAccount(ctx context.Context, acct *store.CloudAccount, remotePath, content string) error {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return err
	}
	if remotePath, err = CleanRemotePath(remotePath); err != nil {
		return err
	}
	return backend.Upload(ctx, remotePath, strings.NewReader(content), int64(len(content)), "text/plain; charset=utf-8", time.Time{})
}

// OpenAccount streams one remote file body, relaying rangeHeader ("Range: …")
// when set so HTTP byte-range requests (video previews, seeking) never copy
// the whole file. The size cap is enforced via Stat BEFORE opening — the
// caller maps ErrFileTooLarge to 413.
func (s *StorageService) OpenAccount(ctx context.Context, acct *store.CloudAccount, remotePath string, rangeHeader string, sizeCapMB int64) (*storage.Content, *storage.StatInfo, error) {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return nil, nil, err
	}
	if remotePath, err = CleanRemotePath(remotePath); err != nil {
		return nil, nil, err
	}
	info, err := backend.Stat(ctx, remotePath)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir {
		return nil, nil, errors.New("cloud_download: path is a directory")
	}
	capBytes := sizeCapMB << 20
	if info.Size > capBytes {
		return nil, nil, fmt.Errorf("%w: %s is %d MB (cap %d MB)", ErrFileTooLarge, info.Name, info.Size>>20, sizeCapMB)
	}
	content, err := backend.Open(ctx, remotePath, storage.RangeHeader(rangeHeader))
	if err != nil {
		return nil, nil, err
	}
	return content, info, nil
}

// UploadAccount copies a local file (localDir/localName — e.g. a multipart
// temp file) into the account's remote at remotePath (full destination path
// incl. name). The caller owns the temp file lifecycle.
func (s *StorageService) UploadAccount(ctx context.Context, acct *store.CloudAccount, localDir, localName, remotePath string) error {
	backend, err := s.backendFor(ctx, acct)
	if err != nil {
		return err
	}
	if remotePath, err = CleanRemotePath(remotePath); err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(localDir, localName))
	if err != nil {
		return fmt.Errorf("cloud storage: staged upload missing: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	contentType := ""
	if n := localName; n != "" {
		contentType = mimeByExt(n)
	}
	return backend.Upload(ctx, remotePath, f, info.Size(), contentType, info.ModTime())
}

// Fetch copies a remote file into the workspace (workspace/cloud/<name>),
// returning the workspace-relative logical path. sizeCapMB bounds the copy
// by stat first — larger files are rejected before transfer.
func (s *StorageService) Fetch(ctx context.Context, account, remotePath, workspaceDir string, sizeCapMB int64) (string, error) {
	acct, err := s.resolveAccount(ctx, account)
	if err != nil {
		return "", err
	}
	return s.FetchAccount(ctx, acct, remotePath, workspaceDir, sizeCapMB)
}

// FetchAccount is Fetch for a pre-authorized account (agent tools resolve +
// permission-check once, then call this).
func (s *StorageService) FetchAccount(ctx context.Context, acct *store.CloudAccount, remotePath, workspaceDir string, sizeCapMB int64) (string, error) {
	content, info, err := s.OpenAccount(ctx, acct, remotePath, "", sizeCapMB)
	if err != nil {
		return "", err
	}
	defer content.Close()

	name := info.Name
	if name == "" {
		name = filepath.Base(strings.TrimSuffix(remotePath, "/"))
	}
	if name == "" || name == "." || name == "/" {
		return "", errors.New("cloud_fetch: cannot determine file name")
	}
	dstDir := filepath.Join(workspaceDir, "cloud")
	if err := ensureDir(dstDir); err != nil {
		return "", err
	}
	dst, err := os.Create(filepath.Join(dstDir, name))
	if err != nil {
		return "", fmt.Errorf("cloud_fetch: create: %w", err)
	}
	if _, err := io.Copy(dst, content); err != nil {
		dst.Close()
		return "", fmt.Errorf("cloud_fetch: copy: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	return filepath.Join("cloud", name), nil
}

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cloud storage: mkdir %s: %w", dir, err)
	}
	return nil
}

// mimeByExt returns a conservative content type for upload streaming (the
// providers store it as file metadata; previews derive types client-side).
func mimeByExt(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	switch strings.ToLower(name[i+1:]) {
	case "txt", "md", "log":
		return "text/plain; charset=utf-8"
	case "json":
		return "application/json"
	case "csv":
		return "text/csv"
	case "html":
		return "text/html"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "pdf":
		return "application/pdf"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "zip":
		return "application/zip"
	default:
		return ""
	}
}
