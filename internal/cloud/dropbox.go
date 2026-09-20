package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
)

// Dropbox provider constants (OAuth2 + Dropbox RPC API). Granular scopes are
// mandatory (every Dropbox app since 2020); the authorization request MUST
// carry token_access_type=offline — Dropbox otherwise issues an online-only
// token (~4h, no refresh_token). Same param the MCP OAuth flow uses
// (internal/http/mcp_oauth.go dropboxOfflineAuthParam).
const (
	DropboxProvider   = "dropbox"
	DropboxAuthURL    = "https://www.dropbox.com/oauth2/authorize"
	DropboxTokenURL   = "https://api.dropboxapi.com/oauth2/token"
	DropboxProfileURL = "https://api.dropboxapi.com/2/users/get_current_account"
)

// DropboxScopes is the scope set, identical to rclone's own dropbox client.
// Order is stable: tests assert it to catch accidental scope creep
// (security-relevant). files.content.write is THE write scope (scopes.go);
// files.metadata.write covers rename/move/delete.
var DropboxScopes = []string{
	"files.metadata.write",
	"files.content.write",
	"files.content.read",
	"sharing.write",
	"account_info.read", // profile + About (quota)
}

// DropboxUserInfo is the subset of get_current_account we persist.
type DropboxUserInfo struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
	Name      struct {
		DisplayName string `json:"display_name"`
	} `json:"name"`
}

// NewDropboxTokenConfig builds the x/oauth2 config for the Dropbox provider.
// AuthStyleInParams: Dropbox's token endpoint wants the client credentials in
// the POST body (HTTP Basic is not supported). PKCE S256 rides alongside the
// confidential-client secret, same shape as Google/Microsoft.
func NewDropboxTokenConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes:       DropboxScopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   DropboxAuthURL,
			TokenURL:  DropboxTokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

// ExchangeDropboxCode swaps an authorization code for tokens using the PKCE
// verifier. The offline refresh token was requested at authorize time
// (token_access_type=offline — see buildDropboxAuthURL).
func ExchangeDropboxCode(ctx context.Context, cfg *oauth2.Config, code, verifier string) (*oauth2.Token, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("cloud: dropbox oauth client not configured")
	}
	return cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
}

// fetchDropboxProfile resolves the account identity via get_current_account
// (an RPC-style endpoint: POST with a literal "null" JSON body).
func fetchDropboxProfile(ctx context.Context, accessToken string) (*DropboxUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DropboxProfileURL, bytes.NewReader([]byte("null")))
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
		return nil, fmt.Errorf("dropbox profile status %d", resp.StatusCode)
	}
	var info DropboxUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
