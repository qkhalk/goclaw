package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// WebDAVProvider connects a generic WebDAV server (Nextcloud, ownCloud,
// Synology, ...). Like s3 it authenticates with static credentials — there
// is no consent flow: the connect form validates the login with one PROPFIND
// and stores the credentials (encrypted at rest) directly.
const WebDAVProvider = "webdav"

// WebDAVConnectInput is the credential connect payload.
type WebDAVConnectInput struct {
	Label    string `json:"label"` // display name (defaults to the host)
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// ConnectWebDAV validates the credentials against the endpoint (one cheap
// PROPFIND) and upserts the account row. Username/password ride the same
// AES-256-GCM-encrypted columns as the OAuth tokens; the endpoint is a
// client-visible Setting. Reconnecting the same label replaces the stored
// credentials (the upsert dedupes on tenant+user+provider+label).
func (m *Manager) ConnectWebDAV(ctx context.Context, tenantID, userID string, in WebDAVConnectInput) (*store.CloudAccount, error) {
	in.Label = strings.TrimSpace(in.Label)
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	in.Username = strings.TrimSpace(in.Username)

	if in.Endpoint == "" || in.Username == "" || in.Password == "" {
		return nil, errors.New("cloud: endpoint, username and password are required")
	}
	u, err := url.Parse(in.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("cloud: endpoint must be an http(s) URL of the WebDAV collection")
	}

	// Validate by exercising the endpoint — a typo in any field fails here,
	// before anything is persisted.
	backend := storage.NewWebDAVBackend(ctx, storage.WebDAVCreds{
		Endpoint: in.Endpoint, Username: in.Username, Password: in.Password,
	})
	if _, err := backend.List(ctx, "/", 1); err != nil {
		return nil, fmt.Errorf("cloud: WebDAV validation failed — check endpoint/credentials: %w", err)
	}

	label := in.Label
	if label == "" {
		label = u.Host
	}
	settings, _ := json.Marshal(map[string]string{
		"endpoint": in.Endpoint,
	})

	sctx, err := scopeContext(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	acct := &store.CloudAccount{
		Provider:    WebDAVProvider,
		Email:       label,
		DisplayName: fmt.Sprintf("WebDAV · %s", u.Host),
		Scopes:      `["readwrite"]`,
		// Static credentials reuse the encrypted token columns; RefreshToken
		// is not a token here but the password (TokenSource is never used).
		AccessToken:  in.Username,
		RefreshToken: in.Password,
		Status:       "active",
		Settings:     string(settings),
	}
	if err := m.store.Upsert(sctx, acct); err != nil {
		return nil, fmt.Errorf("cloud: persist account: %w", err)
	}
	return acct, nil
}

// webdavAccountCreds rebuilds the storage.WebDAVCreds of a connected account.
func webdavAccountCreds(acct *store.CloudAccount) (storage.WebDAVCreds, error) {
	var settings struct {
		Endpoint string `json:"endpoint"`
	}
	if acct.Settings != "" {
		if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
			return storage.WebDAVCreds{}, fmt.Errorf("cloud storage: account settings: %w", err)
		}
	}
	if settings.Endpoint == "" || acct.AccessToken == "" || acct.RefreshToken == "" {
		return storage.WebDAVCreds{}, errors.New("cloud storage: webdav account is missing endpoint or credentials — reconnect it")
	}
	return storage.WebDAVCreds{
		Endpoint: settings.Endpoint, Username: acct.AccessToken, Password: acct.RefreshToken,
	}, nil
}
