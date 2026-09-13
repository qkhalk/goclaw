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

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrRCloneMissing is returned when the rclone binary is not installed.
// Storage tools surface it verbatim; mail tools are unaffected.
var ErrRCloneMissing = errors.New("rclone is not installed on the server — install it (see docs/30-cloud-accounts.md) or set cloud.rclone_path")

// StorageService bridges connected accounts to rclone remotes:
// on demand it (re)injects the account's token into the rcd config so tools
// can address the account as `goclaw-<id8>:` remote paths.
type StorageService struct {
	manager    *Manager
	supervisor *storage.Supervisor
}

// NewStorageService creates the storage service.
// configDir persists rclone.conf (rclone refreshes tokens there at runtime —
// the DB row is the bootstrap, the conf is the runtime truth; reconnecting
// via the Cloud page re-injects a fresh token).
func NewStorageService(manager *Manager, configDir string) *StorageService {
	return &StorageService{
		manager:    manager,
		supervisor: storage.NewSupervisor("", configDir),
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
	// Inject (bootstrap) the remote from the DB token.
	token := map[string]any{
		"access_token":  acct.AccessToken,
		"refresh_token": acct.RefreshToken,
		"token_type":    "Bearer",
		"expiry":        "0001-01-01T00:00:00Z", // force refresh via refresh_token
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
	fs, err := s.FS(ctx, account)
	if err != nil {
		return nil, err
	}
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	return rc.OperationsList(ctx, fs, path, max)
}

// Stat stats one remote path.
func (s *StorageService) Stat(ctx context.Context, account, path string) (*storage.StatInfo, error) {
	fs, err := s.FS(ctx, account)
	if err != nil {
		return nil, err
	}
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return nil, err
	}
	return rc.OperationsStat(ctx, fs, path)
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
	fs, err := s.ensureRemote(ctx, acct)
	if err != nil {
		return nil, err
	}
	return rc.OperationsAbout(ctx, fs)
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
	fs, err := s.ensureRemote(ctx, acct)
	if err != nil {
		return nil, err
	}
	return rc.OperationsList(ctx, fs, path, max)
}

// Fetch copies a remote file into the workspace (workspace/cloud/<name>),
// returning the workspace-relative logical path. sizeCapMB bounds the copy
// by stat first — larger files are rejected before transfer.
func (s *StorageService) Fetch(ctx context.Context, account, remotePath, workspaceDir string, sizeCapMB int64) (string, error) {
	fs, err := s.FS(ctx, account)
	if err != nil {
		return "", err
	}
	rc, err := s.supervisor.RC(ctx)
	if err != nil {
		return "", err
	}
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
	if err := rc.OperationsCopyFile(ctx, fs, remotePath, dstDir, name); err != nil {
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
