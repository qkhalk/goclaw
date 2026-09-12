package oauth

// GitHub Copilot subscription OAuth via the GitHub device flow (public client
// of the Copilot CLI). Endpoints and behavior verified against OpenClaw's
// implementation (extensions/github-copilot, Sep 2026): after the device grant,
// the ORIGINAL GitHub token is sent to the Copilot API — the retired
// copilot_internal/v2/token ephemeral exchange is not used.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CopilotClientID       = "Iv1.b507a08c87ecfe98"
	CopilotScope          = "read:user"
	CopilotDeviceCodeURL  = "https://github.com/login/device/code"
	CopilotAccessTokenURL = "https://github.com/login/oauth/access_token"
	CopilotUserURL        = "https://api.github.com/copilot_internal/user"
	CopilotAPIBaseDefault = "https://api.individual.githubcopilot.com"
	CopilotPollInterval   = 5 * time.Second
	CopilotSlowDownStep   = 5 * time.Second
	CopilotFlowTimeout    = 15 * time.Minute
)

// Overridable endpoints (test hooks); production values are the constants above.
var (
	copilotDeviceCodeURL  = CopilotDeviceCodeURL
	copilotAccessTokenURL = CopilotAccessTokenURL
	copilotUserURL        = CopilotUserURL
)

// CopilotTokenResponse is the GitHub device-flow access token.
type CopilotTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// PendingCopilotLogin is an in-progress device flow. Wait() blocks until the
// user authorizes at VerificationURI or the device code expires.
type PendingCopilotLogin struct {
	UserCode        string
	VerificationURI string

	deviceCode string
	interval   time.Duration
	expiresAt  time.Time

	tokenCh chan *CopilotTokenResponse
	errCh   chan error
	cancel  context.CancelFunc
}

// Wait blocks until the device flow completes or ctx is cancelled.
func (p *PendingCopilotLogin) Wait(ctx context.Context) (*CopilotTokenResponse, error) {
	select {
	case t := <-p.tokenCh:
		return t, nil
	case err := <-p.errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("device flow canceled: %w", ctx.Err())
	}
}

// Shutdown cancels the background poller without waiting.
func (p *PendingCopilotLogin) Shutdown() {
	if p.cancel != nil {
		p.cancel()
	}
}

// StartLoginCopilot requests a device code and starts the background poller.
func StartLoginCopilot() (*PendingCopilotLogin, error) {
	data := url.Values{
		"client_id": {CopilotClientID},
		"scope":     {CopilotScope},
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, copilotDeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var dc struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.Unmarshal(body, &dc); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" || dc.VerificationURI == "" {
		return nil, fmt.Errorf("device code response missing required fields")
	}

	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = CopilotPollInterval
	}
	expiresIn := time.Duration(dc.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = CopilotFlowTimeout
	}

	pollCtx, cancel := context.WithCancel(context.Background())
	p := &PendingCopilotLogin{
		UserCode:        dc.UserCode,
		VerificationURI: dc.VerificationURI,
		deviceCode:      dc.DeviceCode,
		interval:        interval,
		expiresAt:       time.Now().Add(expiresIn),
		tokenCh:         make(chan *CopilotTokenResponse, 1),
		errCh:           make(chan error, 1),
		cancel:          cancel,
	}
	go p.poll(pollCtx)
	return p, nil
}

// poll loops the token endpoint until the user authorizes, GitHub errors out,
// the device code expires, or the poll context is canceled.
func (p *PendingCopilotLogin) poll(ctx context.Context) {
	interval := p.interval
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
		if time.Now().After(p.expiresAt) {
			p.errCh <- fmt.Errorf("device code expired")
			return
		}

		data := url.Values{
			"client_id":   {CopilotClientID},
			"device_code": {p.deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, copilotAccessTokenURL, strings.NewReader(data.Encode()))
		if err != nil {
			p.errCh <- fmt.Errorf("token poll request: %w", err)
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			p.errCh <- fmt.Errorf("token poll request: %w", err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var tr struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			Scope       string `json:"scope"`
			Error       string `json:"error"`
		}
		if err := json.Unmarshal(body, &tr); err != nil {
			p.errCh <- fmt.Errorf("parse token poll response: %w", err)
			return
		}
		switch tr.Error {
		case "":
			if tr.AccessToken == "" {
				p.errCh <- fmt.Errorf("token poll response missing access_token")
				return
			}
			p.tokenCh <- &CopilotTokenResponse{AccessToken: tr.AccessToken, TokenType: tr.TokenType, Scope: tr.Scope}
			return
		case "authorization_pending":
			// keep polling at the current interval
		case "slow_down":
			interval += CopilotSlowDownStep
		default:
			p.errCh <- fmt.Errorf("device flow error: %s", tr.Error)
			return
		}
	}
}

// isTrustedCopilotAPIHost ports OpenClaw's allowlist: https only, no
// userinfo/query/fragment, and the host must be a Copilot API host.
func isTrustedCopilotAPIHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "copilot-proxy.githubusercontent.com" || strings.HasSuffix(host, ".githubcopilot.com")
}

// CopilotValidationResult carries the validated runtime auth info.
type CopilotValidationResult struct {
	APIBase string
	Login   string
}

// ValidateCopilotToken checks Copilot access with the GitHub token and
// resolves the account-specific Copilot API base. An untrusted or missing
// endpoints.api falls back to the default base (never followed — SSRF guard).
func ValidateCopilotToken(ctx context.Context, ghToken string) (*CopilotValidationResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotUserURL, nil)
	if err != nil {
		return nil, fmt.Errorf("copilot user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+ghToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot user request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("github token has no Copilot access (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot user request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse copilot user response: %w", err)
	}
	res := &CopilotValidationResult{APIBase: CopilotAPIBaseDefault}
	if login, ok := raw["login"].(string); ok {
		res.Login = login
	}
	if endpoints, ok := raw["endpoints"].(map[string]any); ok {
		if api, ok := endpoints["api"].(string); ok && api != "" {
			if isTrustedCopilotAPIHost(api) {
				res.APIBase = strings.TrimRight(api, "/")
			} else {
				slog.Warn("security.copilot_untrusted_api_endpoint", "endpoint", api)
			}
		}
	}
	return res, nil
}
