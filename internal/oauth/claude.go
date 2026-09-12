package oauth

// Claude Pro/Max subscription OAuth (public PKCE client — no client secret).
// Endpoints cross-checked against public implementations (Sep 2026). Anthropic
// has rotated the client_id and token host before, so every value is
// overridable at startup via SetClaudeOAuthConfig (config: providers.claude_oauth).

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	ClaudeAuthorizeURL = "https://claude.ai/oauth/authorize"
	ClaudeTokenURL     = "https://platform.claude.com/v1/oauth/token"
	ClaudeClientID     = "9d1c250a-e61b-44e9-96ed-4d3ce8e8c1af"
	ClaudeScopes       = "org:create_api_key user:profile user:inference"
	ClaudeRedirectURI  = "https://console.anthropic.com/oauth/code/callback"
	ClaudeAPIBase      = "https://api.anthropic.com"
)

// ClaudeOAuthConfig holds the resolved Claude OAuth endpoints. Zero-value
// fields fall back to the constants above.
type ClaudeOAuthConfig struct {
	AuthorizeURL string
	TokenURL     string
	ClientID     string
	Scopes       string
	RedirectURI  string
	APIBase      string
}

func (c ClaudeOAuthConfig) withDefaults() ClaudeOAuthConfig {
	if c.AuthorizeURL == "" {
		c.AuthorizeURL = ClaudeAuthorizeURL
	}
	if c.TokenURL == "" {
		c.TokenURL = ClaudeTokenURL
	}
	if c.ClientID == "" {
		c.ClientID = ClaudeClientID
	}
	if c.Scopes == "" {
		c.Scopes = ClaudeScopes
	}
	if c.RedirectURI == "" {
		c.RedirectURI = ClaudeRedirectURI
	}
	if c.APIBase == "" {
		c.APIBase = ClaudeAPIBase
	}
	return c
}

// claudeOAuthCfg is the process-wide override sink wired from config at startup.
var claudeOAuthCfg = ClaudeOAuthConfig{}

// SetClaudeOAuthConfig applies non-empty endpoint overrides (called once at
// gateway startup; safe to call again in tests with t.Cleanup restoration).
func SetClaudeOAuthConfig(cfg ClaudeOAuthConfig) {
	claudeOAuthCfg = cfg.withDefaults()
}

func claudeOAuthConfig() ClaudeOAuthConfig {
	return claudeOAuthCfg.withDefaults()
}

// ClaudeTokenResponse is the response from the Claude OAuth token endpoint.
type ClaudeTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// PendingClaudeLogin represents an in-progress Claude OAuth flow. Unlike the
// OpenAI flow there is no loopback server: the console redirect page displays
// the code for paste-back, which is the only reachable path for remote/VPS
// web deployments.
type PendingClaudeLogin struct {
	AuthURL  string
	verifier string
	state    string
}

// State exposes the CSRF state for handler-side validation.
func (p *PendingClaudeLogin) State() string { return p.state }

// ExchangeRedirectURL parses a pasted callback (URL or bare code#state),
// validates the state against this flow, and exchanges the code for tokens.
func (p *PendingClaudeLogin) ExchangeRedirectURL(raw string) (*ClaudeTokenResponse, error) {
	code, state, err := ParseClaudeRedirectURL(raw)
	if err != nil {
		return nil, err
	}
	if state == "" || state != p.state {
		return nil, fmt.Errorf("invalid state parameter (possible CSRF)")
	}
	return ExchangeClaudeCode(context.Background(), code, p.verifier)
}

// ParseClaudeRedirectURL extracts the authorization code and state from any of
// the shapes the console callback produces:
//   - full URL with query params:  https://console.anthropic.com/...?code=X&state=Y
//   - full URL with fragment:      https://console.anthropic.com/...#code=X&state=Y
//   - bare code#state text:        X#Y   (what the console page displays)
//
// Fragments never reach a server, so paste-back is the only way to receive
// them; the bare form covers users copying the displayed string instead of
// the address bar.
func ParseClaudeRedirectURL(raw string) (code, state string, err error) {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "%23", "#")
	if raw == "" {
		return "", "", fmt.Errorf("empty redirect input")
	}

	// Bare "code#state" text (no scheme, no slash): url.Parse puts the code in
	// Path and the state in Fragment.
	if !strings.Contains(raw, "://") && !strings.Contains(raw, "?") && strings.Count(raw, "#") == 1 {
		code, state, _ = strings.Cut(raw, "#")
		if code == "" || state == "" {
			return "", "", fmt.Errorf("invalid code#state input")
		}
		return code, state, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid redirect URL: %w", err)
	}

	if code = u.Query().Get("code"); code != "" {
		return code, u.Query().Get("state"), nil
	}
	if u.Fragment != "" {
		if fragVals, fragErr := url.ParseQuery(u.Fragment); fragErr == nil {
			if c := fragVals.Get("code"); c != "" {
				return c, fragVals.Get("state"), nil
			}
		}
	}

	if errParam := u.Query().Get("error"); errParam != "" {
		return "", "", fmt.Errorf("OAuth error: %s", errParam)
	}
	return "", "", fmt.Errorf("no authorization code in redirect input")
}

// StartLoginClaude builds the claude.ai authorize URL (PKCE S256 + CSRF state).
// Does NOT open a browser; the caller directs the user to AuthURL.
func StartLoginClaude() (*PendingClaudeLogin, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	stateBuf := make([]byte, 16)
	if _, err := rand.Read(stateBuf); err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBuf)

	cfg := claudeOAuthConfig()
	params := url.Values{
		"code":                  {"true"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {cfg.RedirectURI},
		"response_type":         {"code"},
		"scope":                 {cfg.Scopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	return &PendingClaudeLogin{
		AuthURL:  cfg.AuthorizeURL + "?" + params.Encode(),
		verifier: verifier,
		state:    state,
	}, nil
}

// ExchangeClaudeCode exchanges an authorization code for tokens.
func ExchangeClaudeCode(ctx context.Context, code, verifier string) (*ClaudeTokenResponse, error) {
	cfg := claudeOAuthConfig()
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURI},
		"code_verifier": {verifier},
	}
	return postClaudeTokenForm(ctx, cfg.TokenURL, data)
}

// RefreshClaudeToken refreshes an expired access token.
func RefreshClaudeToken(ctx context.Context, refreshToken string) (*ClaudeTokenResponse, error) {
	cfg := claudeOAuthConfig()
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {cfg.ClientID},
		"refresh_token": {refreshToken},
	}
	return postClaudeTokenForm(ctx, cfg.TokenURL, data)
}

func postClaudeTokenForm(ctx context.Context, tokenURL string, data url.Values) (*ClaudeTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp ClaudeTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	return &tokenResp, nil
}

// FetchClaudeAccountEmail best-effort fetches the account email for display.
// Never fails the login flow — returns "" on any error.
func FetchClaudeAccountEmail(ctx context.Context, accessToken string) string {
	cfg := claudeOAuthConfig()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.APIBase+"/v1/users/me", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var me struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return ""
	}
	return me.Email
}
