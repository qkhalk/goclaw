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

// Microsoft provider constants (OneDrive via Microsoft Graph). BYO client
// like Google: each install registers its own Azure "App registration"
// (Web platform, redirect URI = <base>/v1/cloud/oauth/callback). The
// "common" tenant covers both personal (outlook) and work/school accounts.
// offline_access is mandatory — without it Azure issues no refresh token.
const (
	MicrosoftProvider    = "onedrive"
	MicrosoftAuthURL     = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	MicrosoftTokenURL    = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	MicrosoftGraphMeURL  = "https://graph.microsoft.com/v1.0/me"
	MicrosoftGraphDrivesURL = "https://graph.microsoft.com/v1.0/me/drives"
)

// MicrosoftScopes is the v1 scope set. Order is stable: tests assert it to
// catch accidental scope creep (security-relevant). Files.Read.All covers
// the rclone onedrive backend's browse/read; there is no write scope —
// cloud_fetch copies out only.
var MicrosoftScopes = []string{
	"offline_access",
	"User.Read",
	"Files.Read.All",
}

// MicrosoftUserInfo is the subset of the Graph /me response we persist.
type MicrosoftUserInfo struct {
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
	DisplayName       string `json:"displayName"`
}

// Email prefers the SMTP mail field; personal accounts may only expose the
// UPN (which for consumer accounts is the email address anyway).
func (u *MicrosoftUserInfo) Email() string {
	if u.Mail != "" {
		return u.Mail
	}
	return u.UserPrincipalName
}

// NewMicrosoftTokenConfig builds the x/oauth2 config for the Microsoft
// provider (confidential client + PKCE S256, same shape as Google).
func NewMicrosoftTokenConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes:       MicrosoftScopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  MicrosoftAuthURL,
			TokenURL: MicrosoftTokenURL,
		},
	}
}

// ExchangeMicrosoftCode swaps an authorization code for tokens using the
// PKCE verifier (RFC 7636 §4.5).
func ExchangeMicrosoftCode(ctx context.Context, cfg *oauth2.Config, code, verifier string) (*oauth2.Token, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("cloud: microsoft oauth client not configured")
	}
	return cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
}

// fetchMicrosoftGraph is the shared one-shot Graph GET (limit-bounded body).
func fetchMicrosoftGraph(ctx context.Context, url, accessToken string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
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
		return nil, fmt.Errorf("graph status %d", resp.StatusCode)
	}
	return json.RawMessage(body), nil
}

// fetchMicrosoftProfile resolves the account identity via Graph /me.
func fetchMicrosoftProfile(ctx context.Context, accessToken string) (*MicrosoftUserInfo, error) {
	body, err := fetchMicrosoftGraph(ctx, MicrosoftGraphMeURL, accessToken)
	if err != nil {
		return nil, fmt.Errorf("graph me: %w", err)
	}
	var info MicrosoftUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// MicrosoftDrive is one OneDrive drive from Graph /me/drives.
type MicrosoftDrive struct {
	ID        string `json:"id"`
	DriveType string `json:"driveType"` // "personal" | "business" | "documentLibrary"
}

// fetchMicrosoftDefaultDrive picks the drive rclone should mount. Personal
// accounts return one drive; business accounts may have several — prefer the
// first business drive, else the first entry (rclone's own heuristic).
func fetchMicrosoftDefaultDrive(ctx context.Context, accessToken string) (*MicrosoftDrive, error) {
	body, err := fetchMicrosoftGraph(ctx, MicrosoftGraphDrivesURL, accessToken)
	if err != nil {
		return nil, fmt.Errorf("graph drives: %w", err)
	}
	var list struct {
		Value []MicrosoftDrive `json:"value"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	if len(list.Value) == 0 {
		return nil, errors.New("graph drives: account has no drives")
	}
	for i := range list.Value {
		if list.Value[i].DriveType == "business" {
			return &list.Value[i], nil
		}
	}
	return &list.Value[0], nil
}
