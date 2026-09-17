package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
)

// Dropbox provider (referencing rclone's backend/dropbox model): a standard
// OAuth2 code flow, one namespace (paths ARE the API — no drive id), and
// REST endpoints split between api.dropboxapi.com (RPC) and
// content.dropboxapi.com (upload/download).
//
// Dropbox is BYO-client only: unlike Google/Microsoft there is no embedded
// shared client (rclone's Dropbox app is folder-scoped and its consent names
// rclone), so the admin must save a Dropbox app's client id + secret in the
// web UI first — until then the provider reports "not configured".
const (
	DropboxProvider = "dropbox"
	DropboxAuthURL  = "https://www.dropbox.com/oauth2/authorize"
	DropboxTokenURL = "https://api.dropboxapi.com/oauth2/token"

	DropboxAPIBase     = "https://api.dropboxapi.com/2"
	DropboxContentBase = "https://content.dropboxapi.com/2"
)

// DropboxUserInfo is the subset of /2/users/get_current_account we persist.
type DropboxUserInfo struct {
	Email string `json:"email"`
	Name  struct {
		DisplayName string `json:"display_name"`
	} `json:"name"`
}

// Mail returns the account email (method shape mirrors the other providers).
func (u *DropboxUserInfo) Mail() string { return u.Email }

// DisplayName returns the display name.
func (u *DropboxUserInfo) DisplayName() string { return u.Name.DisplayName }

// NewDropboxTokenConfig builds the x/oauth2 config for Dropbox (confidential
// client; redirect URI is the same /v1/cloud/oauth/callback as the others).
func NewDropboxTokenConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		// Dropbox apps carry their permission set in the app console; the
		// authorize request takes no scope list.
		Endpoint: oauth2.Endpoint{
			AuthURL:  DropboxAuthURL,
			TokenURL: DropboxTokenURL,
		},
	}
}

// ExchangeDropboxCode swaps an authorization code for tokens (PKCE verifier
// threaded like the other providers for symmetry).
func ExchangeDropboxCode(ctx context.Context, cfg *oauth2.Config, code, verifier string) (*oauth2.Token, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("cloud: dropbox oauth client not configured")
	}
	return cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
}

// FetchDropboxProfile resolves the account identity via
// /2/users/get_current_account (an RPC POST with a literal null body).
func FetchDropboxProfile(ctx context.Context, accessToken string) (*DropboxUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DropboxAPIBase+"/users/get_current_account", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dropbox account status %d", resp.StatusCode)
	}
	var info DropboxUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
