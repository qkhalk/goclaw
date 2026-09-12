package cloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
)

// Google provider constants. Scopes are deliberately minimal for v1:
// gmail.readonly (search/read), gmail.labels + gmail.modify (archive/trash/
// label ops), drive.readonly (storage browse/read). The full-access
// https://mail.google.com/ scope is intentionally NOT requested — v1 has no
// permanent-delete path, and narrower scopes mean a smaller blast radius.
const (
	GoogleProvider   = "google"
	GoogleAuthURL    = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenURL   = "https://oauth2.googleapis.com/token"
	GoogleUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

// GoogleScopes is the v1 scope set. Order is stable: unit tests assert it to
// catch accidental scope creep (security-relevant).
var GoogleScopes = []string{
	"openid",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/gmail.labels",
	"https://www.googleapis.com/auth/gmail.modify",
	"https://www.googleapis.com/auth/drive.readonly",
}

// GoogleUserInfo is the subset of the OIDC userinfo response we persist.
type GoogleUserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// NewGoogleTokenConfig builds the x/oauth2 config for the Google provider.
// clientSecret is the BYO GCP client secret (confidential client — the
// browser flow additionally uses PKCE S256).
func NewGoogleTokenConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes:       GoogleScopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  GoogleAuthURL,
			TokenURL: GoogleTokenURL,
		},
	}
}

// NewVerifier generates a PKCE code_verifier (RFC 7636 §4.1): 43-char
// base64url of 32 random bytes.
func NewVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("cloud: pkce verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// VerifierChallenge computes the S256 code_challenge for a verifier.
func VerifierChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ExchangeGoogleCode swaps an authorization code for tokens using the PKCE
// verifier. AuthCodeOptions passes the code_verifier per RFC 7636 §4.5.
func ExchangeGoogleCode(ctx context.Context, cfg *oauth2.Config, code, verifier string) (*oauth2.Token, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("cloud: google oauth client not configured")
	}
	return cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
}
