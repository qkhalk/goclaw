package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrRCloneMissing is returned when the rclone binary is not installed.
// Storage tools surface it verbatim; mail tools are unaffected.
var ErrRCloneMissing = errors.New("rclone is not installed on the server — install it (see docs/30-cloud-accounts.md) or set cloud.rclone_path")

// ErrFileTooLarge is returned when a file exceeds the download/upload size
// cap (FetchSizeCapMB). Handlers map it to 413; other storage errors stay 502.
var ErrFileTooLarge = errors.New("cloud: file exceeds the size cap")

// StorageService bridges connected accounts to rclone remotes:
// on demand it (re)injects the account's token into the rcd config so tools
// can address the account as `goclaw-<id8>:` remote paths.
type StorageService struct {
	manager    *Manager
	supervisor *storage.Supervisor
	transfers  *transferRegistry // async cross-account transfer jobs (in-memory)
}

// NewStorageService creates the storage service.
// configDir persists rclone.conf (rclone refreshes tokens there at runtime —
// the DB row is the bootstrap, the conf is the runtime truth; reconnecting
// via the Cloud page re-injects a fresh token).
func NewStorageService(manager *Manager, configDir string) *StorageService {
	return &StorageService{
		manager:    manager,
		supervisor: storage.NewSupervisor("", configDir),
		transfers:  newTransferRegistry(),
	}
}

// Shutdown stops the rcd child (gateway close).
func (s *StorageService) Shutdown() { s.supervisor.Shutdown() }

// RemoveRemote deletes the account's rclone remote (disconnect path). Best
// effort: when rcd is not running there is no conf to clean. Called from
// Manager.Delete so tokens never linger on disk after a disconnect.
func (s *StorageService) RemoveRemote(ctx context.Context, accountID string) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return // rcd never started → nothing persisted in any rclone.conf
	}
	if err := rc.ConfigDelete(ctx, storage.RemoteName(accountID)); err != nil {
		// The row is already gone from the DB; a stale remote degrades to
		// "tool errors until reconnect", never a security regression.
		slog.Warn("cloud storage: remove remote failed", "error", err)
	}
}

// resolveAccount picks by email/ID when given, else per-scope bindings
// (group → user → tenant default) and finally the caller's own accounts —
// including tenant-shared ones (the enterprise "company drive" pattern).
func (s *StorageService) resolveAccount(ctx context.Context, name string) (*store.CloudAccount, error) {
	return s.manager.ResolveAccount(ctx, name, []string{GoogleProvider, MicrosoftProvider}, func(a *store.CloudAccount) bool {
		return isStorageProvider(a.Provider)
	})
}

// isStorageProvider gates which connected accounts the rclone layer may use.
func isStorageProvider(provider string) bool {
	switch provider {
	case GoogleProvider, MicrosoftProvider:
		return true
	default:
		return false
	}
}

// IsStorageProvider reports whether the provider is usable by the rclone
// storage tools. Exported for the agent-facing cloud_accounts tool.
func IsStorageProvider(provider string) bool { return isStorageProvider(provider) }

// ensureRemote makes sure the rcd process is running and the account's remote
// exists in its config with a non-stale bootstrap token.
func (s *StorageService) ensureRemote(ctx context.Context, acct *store.CloudAccount) (string, error) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") ||
			strings.Contains(err.Error(), "start rcd") {
			return "", ErrRCloneMissing
		}
		return "", err
	}
	remote := storage.RemoteName(acct.ID)

	remotes, err := rc.ConfigListRemotes(ctx)
	if err != nil {
		return "", fmt.Errorf("cloud storage: list remotes: %w", err)
	}
	if slices.Contains(remotes, remote) {
		return remote, nil // rclone owns token refresh from here
	}
	// Inject (bootstrap) the remote from a live token: the TokenSource
	// auto-refreshes (this also proves the grant works for this account's
	// pinned OAuth client). Falls back to the stored token if refresh fails.
	accessToken, refreshToken := acct.AccessToken, acct.RefreshToken
	expiry := time.Now().Add(time.Hour).UTC()
	if ts, terr := s.manager.TokenSource(ctx, acct.ID); terr == nil {
		if tok, terr := ts.Token(); terr == nil && tok.AccessToken != "" {
			accessToken = tok.AccessToken
			if tok.RefreshToken != "" {
				refreshToken = tok.RefreshToken
			}
			if !tok.Expiry.IsZero() {
				expiry = tok.Expiry
			}
		}
	} else {
		slog.Warn("cloud storage: live token refresh failed, using stored token", "account", acct.ID, "error", terr)
	}
	token := map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		// A real expiry makes rclone refresh proactively — with the pinned
		// scopes below — instead of discovering expiry via a Graph 401.
		"expiry": expiry.Format(time.RFC3339),
	}
	tokenJSON, _ := json.Marshal(token)
	params := map[string]any{"token": string(tokenJSON)}
	remoteType := "drive"
	if acct.Provider == MicrosoftProvider {
		// The onedrive backend refuses to auto-pick a drive non-interactively:
		// drive_id (resolved at connect time via Graph /me/drives) is required.
		remoteType = "onedrive"
		var settings struct {
			DriveID   string `json:"drive_id"`
			DriveType string `json:"drive_type"`
		}
		if acct.Settings != "" {
			if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
				return "", fmt.Errorf("cloud storage: account settings: %w", err)
			}
		}
		if settings.DriveID == "" {
			return "", errors.New("cloud storage: onedrive account has no drive_id — reconnect the account")
		}
		params["drive_id"] = settings.DriveID
		if settings.DriveType != "" {
			params["drive_type"] = settings.DriveType
		}
		// MSA refresh MUST repeat the original grant scopes, otherwise
		// Microsoft returns a compact (non-JWT) token Graph rejects with
		// IDX14100. The onedrive backend reads the `access_scopes` option
		// (comma-separated) — plain `scope` is silently ignored, which made
		// rclone refresh with ITS defaults and poison the persisted config.
		// Pin the account's STORED grant (not the current constants): after a
		// re-grant with wider scopes the stored grant is authoritative, and a
		// pre-write-upgrade grant keeps repeating its original read-only
		// scopes until the owner re-grants.
		params["access_scopes"] = strings.Join(accountMicrosoftScopes(acct), ",")
	}
	// Refresh tokens are bound to the issuing OAuth client: pin the client the
	// account consented to, or rclone refreshes with ITS own defaults and
	// Google/Microsoft reject the grant once the access token expires.
	if creds := s.manager.credentialsForAccount(ctx, acct); creds.ClientID != "" {
		params["client_id"] = creds.ClientID
		if creds.ClientSecret != "" {
			params["client_secret"] = creds.ClientSecret
		}
	}
	if err := rc.ConfigCreate(ctx, remote, remoteType, params); err != nil {
		return "", fmt.Errorf("cloud storage: create remote: %w", err)
	}
	return remote, nil
}

// authPoisoned reports whether an rc error means rclone's persisted token is
// unusable (e.g. a compact MSA token from a wrong-scope refresh): the remote
// must be rebuilt from a live DB token before the operation can succeed.
func authPoisoned(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "InvalidAuthentication") ||
		strings.Contains(msg, "JWT is not well formed")
}

// rebuildRemote drops the account's remote so ensureRemote re-bootstraps it
// from a fresh TokenSource token — the DB row is the token source of truth.
func (s *StorageService) rebuildRemote(ctx context.Context, acct *store.CloudAccount) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return
	}
	if derr := rc.ConfigDelete(ctx, storage.RemoteName(acct.ID)); derr != nil {
		slog.Warn("cloud storage: rebuild: delete remote failed", "error", derr)
	}
	if _, berr := s.ensureRemote(ctx, acct); berr != nil {
		slog.Warn("cloud storage: rebuild: re-bootstrap failed", "account", acct.ID, "error", berr)
	}
}

// runWithRemote resolves nothing: it ensures acct's remote, runs op, and on a
// poisoned-token error rebuilds the remote from the DB and retries once.
func (s *StorageService) runWithRemote(ctx context.Context, acct *store.CloudAccount, op func(fs string) error) error {
	fs, err := s.ensureRemote(ctx, acct)
	if err != nil {
		return err
	}
	if err = op(fs); err != nil && authPoisoned(err) {
		slog.Warn("cloud storage: rclone token poisoned — rebuilding remote", "account", acct.ID)
		s.rebuildRemote(ctx, acct)
		fs, err = s.ensureRemote(ctx, acct)
		if err != nil {
			return err
		}
		err = op(fs)
	}
	return err
}

// FS returns the rclone filesystem spec ("goclaw-xxxx:") for the account.
func (s *StorageService) FS(ctx context.Context, account string) (string, error) {
	acct, err := s.resolveAccount(ctx, account)
	if err != nil {
		return "", err
	}
	return s.ensureRemote(ctx, acct)
}

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
	rc, rerr := s.supervisor.RC(ctx)
	if rerr != nil {
		return nil, rerr
	}
	var info *storage.StatInfo
	err = s.runWithRemote(ctx, acct, func(fs string) error {
		got, opErr := rc.OperationsStat(ctx, fs, path)
		if opErr == nil {
			info = got
		}
		return opErr
	})
	return info, err
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
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	var info *storage.AboutInfo
	err = s.runWithRemote(ctx, acct, func(fs string) error {
		got, opErr := rc.OperationsAbout(ctx, fs)
		if opErr == nil {
			info = got
		}
		return opErr
	})
	return info, err
}

// AboutAccount returns quota info for a specific connected account (no
// re-resolution — the caller already proved accessibility).
func (s *StorageService) AboutAccount(ctx context.Context, acct *store.CloudAccount) (*storage.AboutInfo, error) {
	return s.aboutFor(ctx, acct)
}

// ListAccount lists one account's remote path (caller proved accessibility).
func (s *StorageService) ListAccount(ctx context.Context, acct *store.CloudAccount, path string, max int) ([]storage.ListEntry, error) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	var out []storage.ListEntry
	err = s.runWithRemote(ctx, acct, func(fs string) error {
		entries, opErr := rc.OperationsList(ctx, fs, path, max)
		if opErr == nil {
			out = entries
		}
		return opErr
	})
	return out, err
}

// runWithTwoRemotes is runWithRemote across two accounts (cross-account
// transfer): ensures both remotes, runs op, and on a poisoned-token error
// rebuilds BOTH remotes from the DB and retries once (an rc error does not
// say which side's token went bad).
func (s *StorageService) runWithTwoRemotes(ctx context.Context, src, dst *store.CloudAccount, op func(srcFS, dstFS string) error) error {
	srcFS, err := s.ensureRemote(ctx, src)
	if err != nil {
		return err
	}
	dstFS, err := s.ensureRemote(ctx, dst)
	if err != nil {
		return err
	}
	if err = op(srcFS, dstFS); err != nil && authPoisoned(err) {
		slog.Warn("cloud storage: rclone token poisoned during transfer — rebuilding remotes", "source", src.ID, "target", dst.ID)
		s.rebuildRemote(ctx, src)
		s.rebuildRemote(ctx, dst)
		if srcFS, err = s.ensureRemote(ctx, src); err != nil {
			return err
		}
		if dstFS, err = s.ensureRemote(ctx, dst); err != nil {
			return err
		}
		err = op(srcFS, dstFS)
	}
	return err
}

// --- file operations (Phase 4): every method takes a pre-authorized account
// (caller proved accessibility) and self-heals via runWithRemote. Paths are
// re-validated here as defense in depth — handlers validate for 400s first.

// MkdirAccount creates a directory in the account's remote.
func (s *StorageService) MkdirAccount(ctx context.Context, acct *store.CloudAccount, dir string) error {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return err
	}
	if dir, err = CleanRemotePath(dir); err != nil {
		return err
	}
	return s.runWithRemote(ctx, acct, func(fs string) error {
		return rc.OperationsMkdir(ctx, fs, dir)
	})
}

// DeleteAccount removes one file (isDir=false → deletefile) or one EMPTY
// directory (isDir=true → rmdir; rclone refuses non-empty dirs, so wiping a
// populated tree is deliberately not exposed via rc). Deletion is PERMANENT
// on Drive — no trash.
func (s *StorageService) DeleteAccount(ctx context.Context, acct *store.CloudAccount, path string, isDir bool) error {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return err
	}
	if path, err = CleanRemotePath(path); err != nil {
		return err
	}
	return s.runWithRemote(ctx, acct, func(fs string) error {
		if isDir {
			return rc.OperationsRmdir(ctx, fs, path)
		}
		return rc.OperationsDeleteFile(ctx, fs, path)
	})
}

// MoveAccount renames/moves one file within the account's remote (both paths
// on the same fs — rclone movefile covers rename and cross-folder moves).
func (s *StorageService) MoveAccount(ctx context.Context, acct *store.CloudAccount, from, to string) error {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return err
	}
	if from, err = CleanRemotePath(from); err != nil {
		return err
	}
	if to, err = CleanRemotePath(to); err != nil {
		return err
	}
	return s.runWithRemote(ctx, acct, func(fs string) error {
		return rc.OperationsMoveFile(ctx, fs+":", from, fs+":", to)
	})
}

// CopyAccount copies one file to another path within the account's remote.
func (s *StorageService) CopyAccount(ctx context.Context, acct *store.CloudAccount, from, to string) error {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return err
	}
	if from, err = CleanRemotePath(from); err != nil {
		return err
	}
	if to, err = CleanRemotePath(to); err != nil {
		return err
	}
	return s.runWithRemote(ctx, acct, func(fs string) error {
		return rc.OperationsCopyFile(ctx, fs+":", from, fs+":", to)
	})
}

// CopyURLAccount uploads-by-URL: rclone fetches url (server side) into the
// file at path (full destination path incl. name).
func (s *StorageService) CopyURLAccount(ctx context.Context, acct *store.CloudAccount, url, path string) error {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return err
	}
	if path, err = CleanRemotePath(path); err != nil {
		return err
	}
	return s.runWithRemote(ctx, acct, func(fs string) error {
		return rc.OperationsCopyURL(ctx, url, fs, path)
	})
}

// PublicLinkAccount creates/retrieves the public share link for one path.
func (s *StorageService) PublicLinkAccount(ctx context.Context, acct *store.CloudAccount, path string) (*storage.PublicLinkInfo, error) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	if path, err = CleanRemotePath(path); err != nil {
		return nil, err
	}
	var out *storage.PublicLinkInfo
	err = s.runWithRemote(ctx, acct, func(fs string) error {
		link, opErr := rc.OperationsPublicLink(ctx, fs, path)
		if opErr == nil {
			out = link
		}
		return opErr
	})
	return out, err
}

// DownloadedFile is a remote file copied to a local temp path, ready to be
// streamed to the client. The caller owns Dir and must remove it after
// serving.
type DownloadedFile struct {
	Dir  string             // temp dir — caller removes it
	Name string             // file name inside Dir
	Stat *storage.StatInfo  // remote stat (size, mime) captured before the copy
}

// DownloadAccount copies one remote file into a fresh temp dir (after stat +
// size-cap checks) so the handler can stream it and delete it afterwards.
func (s *StorageService) DownloadAccount(ctx context.Context, acct *store.CloudAccount, remotePath string, sizeCapMB int64) (*DownloadedFile, error) {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	if remotePath, err = CleanRemotePath(remotePath); err != nil {
		return nil, err
	}
	var out *DownloadedFile
	err = s.runWithRemote(ctx, acct, func(fs string) error {
		dl, opErr := s.downloadVia(ctx, rc, fs, remotePath, sizeCapMB)
		if opErr == nil {
			out = dl
		}
		return opErr
	})
	return out, err
}

func (s *StorageService) downloadVia(ctx context.Context, rc *storage.RCClient, fs, remotePath string, sizeCapMB int64) (*DownloadedFile, error) {
	info, err := rc.OperationsStat(ctx, fs, remotePath)
	if err != nil {
		return nil, err
	}
	if info.IsDir {
		return nil, errors.New("cloud_download: path is a directory")
	}
	capBytes := sizeCapMB << 20
	if info.Size > capBytes {
		return nil, fmt.Errorf("%w: %s is %d MB (cap %d MB)", ErrFileTooLarge, info.Name, info.Size>>20, sizeCapMB)
	}
	name := filepath.Base(remotePath)
	if name == "" || name == "." || name == "/" {
		return nil, errors.New("cloud_download: cannot determine file name")
	}
	dir, err := os.MkdirTemp("", "goclaw-cloud-dl-")
	if err != nil {
		return nil, fmt.Errorf("cloud_download: temp dir: %w", err)
	}
	if err := rc.OperationsCopyFile(ctx, fs+":", remotePath, dir, name); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("cloud_download: copy: %w", err)
	}
	return &DownloadedFile{Dir: dir, Name: name, Stat: info}, nil
}

// UploadAccount copies a local file (localDir/localName — e.g. a multipart
// temp file) into the account's remote at remotePath (full destination path
// incl. name). The caller owns the temp file lifecycle.
func (s *StorageService) UploadAccount(ctx context.Context, acct *store.CloudAccount, localDir, localName, remotePath string) error {
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return err
	}
	if remotePath, err = CleanRemotePath(remotePath); err != nil {
		return err
	}
	return s.runWithRemote(ctx, acct, func(fs string) error {
		return rc.OperationsCopyFile(ctx, localDir, localName, fs+":", remotePath)
	})
}

// Fetch copies a remote file into the workspace (workspace/cloud/<name>),
// returning the workspace-relative logical path. sizeCapMB bounds the copy
// by stat first — larger files are rejected before transfer.
func (s *StorageService) Fetch(ctx context.Context, account, remotePath, workspaceDir string, sizeCapMB int64) (string, error) {
	acct, err := s.resolveAccount(ctx, account)
	if err != nil {
		return "", err
	}
	rc, rerr := s.supervisor.RC(ctx)
	if rerr != nil {
		return "", rerr
	}
	var out string
	err = s.runWithRemote(ctx, acct, func(fs string) error {
		p, opErr := s.fetchVia(ctx, rc, fs, remotePath, workspaceDir, sizeCapMB)
		if opErr == nil {
			out = p
		}
		return opErr
	})
	return out, err
}

func (s *StorageService) fetchVia(ctx context.Context, rc *storage.RCClient, fs, remotePath, workspaceDir string, sizeCapMB int64) (string, error) {
	info, err := rc.OperationsStat(ctx, fs, remotePath)
	if err != nil {
		return "", err
	}
	if info.IsDir {
		return "", errors.New("cloud_fetch: path is a directory — fetch files one by one")
	}
	capBytes := sizeCapMB << 20
	if info.Size > capBytes {
		return "", fmt.Errorf("cloud_fetch: file is %d MB (cap %d MB)", info.Size>>20, sizeCapMB)
	}

	name := filepath.Base(strings.TrimSuffix(remotePath, "/"))
	if name == "" || name == "." || name == "/" {
		return "", errors.New("cloud_fetch: cannot determine file name")
	}
	dstDir := filepath.Join(workspaceDir, "cloud")
	if err := ensureDir(dstDir); err != nil {
		return "", err
	}
	srcFs := fs + ":"
	srcRemote := strings.Trim(remotePath, "/")
	if err := rc.OperationsCopyFile(ctx, srcFs, srcRemote, dstDir, name); err != nil {
		return "", fmt.Errorf("cloud_fetch: copy: %w", err)
	}
	return filepath.Join("cloud", name), nil
}

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cloud storage: mkdir %s: %w", dir, err)
	}
	return nil
}
