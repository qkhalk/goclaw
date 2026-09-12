package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"golang.org/x/oauth2"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Manager wires the OAuth web flow (authorize URL → callback exchange) and
// hands out TokenSources for connected accounts. One instance per gateway;
// safe for concurrent use.
type Manager struct {
	cfg     CloudProviderConfig
	store   store.CloudAccountStore
	secrets store.ConfigSecretsStore // optional dynamic provider credentials (saved from the web UI)
	encKey  string
	storage *StorageService // optional; deletes rclone remotes on disconnect
}

// CloudProviderConfig carries the static provider credentials for the
// manager (env/config). Dynamic credentials saved from the web UI take
// precedence (see googleCredentials/microsoftCredentials).
type CloudProviderConfig struct {
	GoogleClientID        string
	GoogleClientSecret    string
	MicrosoftClientID     string
	MicrosoftClientSecret string
}

// config_secrets keys for credentials saved from the web UI (first-run setup).
const (
	SecretKeyGoogleClientID     = "cloud.google.client_id"
	SecretKeyGoogleClientSecret = "cloud.google.client_secret"

	SecretKeyMicrosoftClientID     = "cloud.microsoft.client_id"
	SecretKeyMicrosoftClientSecret = "cloud.microsoft.client_secret"
)

// SupportedProviders lists the storage providers the manager can connect,
// in UI display order.
var SupportedProviders = []string{GoogleProvider, MicrosoftProvider}

// IsSupportedProvider reports whether the provider id can be connected.
func IsSupportedProvider(provider string) bool {
	return slices.Contains(SupportedProviders, provider)
}

// NewManager creates a cloud Manager.
func NewManager(cfg CloudProviderConfig, accounts store.CloudAccountStore, encryptionKey string) *Manager {
	return &Manager{cfg: cfg, store: accounts, encKey: encryptionKey}
}

// SetSecretsStore enables dynamic (web-UI-saved) provider credentials.
func (m *Manager) SetSecretsStore(s store.ConfigSecretsStore) { m.secrets = s }

// googleCredentials resolves the Google OAuth client at call time: web-UI
// saved credentials (config_secrets) win over env/config — so a public
// install can be set up entirely from the browser without a restart.
func (m *Manager) googleCredentials(ctx context.Context) (clientID, clientSecret string) {
	if m.secrets != nil {
		sctx := store.WithTenantID(ctx, store.MasterTenantID)
		if id, err := m.secrets.Get(sctx, SecretKeyGoogleClientID); err == nil && id != "" {
			secret, serr := m.secrets.Get(sctx, SecretKeyGoogleClientSecret)
			if serr == nil && secret != "" {
				return id, secret
			}
		}
	}
	return m.cfg.GoogleClientID, m.cfg.GoogleClientSecret
}

// GoogleConfigured reports whether the Google OAuth client is present.
func (m *Manager) GoogleConfigured(ctx context.Context) bool {
	id, secret := m.googleCredentials(ctx)
	return id != "" && secret != ""
}

// microsoftCredentials resolves the Microsoft OAuth client at call time
// (web-UI saved credentials win over env/config, same as Google).
func (m *Manager) microsoftCredentials(ctx context.Context) (clientID, clientSecret string) {
	if m.secrets != nil {
		sctx := store.WithTenantID(ctx, store.MasterTenantID)
		if id, err := m.secrets.Get(sctx, SecretKeyMicrosoftClientID); err == nil && id != "" {
			secret, serr := m.secrets.Get(sctx, SecretKeyMicrosoftClientSecret)
			if serr == nil && secret != "" {
				return id, secret
			}
		}
	}
	return m.cfg.MicrosoftClientID, m.cfg.MicrosoftClientSecret
}

// MicrosoftConfigured reports whether the Microsoft OAuth client is present.
func (m *Manager) MicrosoftConfigured(ctx context.Context) bool {
	id, secret := m.microsoftCredentials(ctx)
	return id != "" && secret != ""
}

// ProviderConfigured dispatches the per-provider OAuth client check.
func (m *Manager) ProviderConfigured(ctx context.Context, provider string) bool {
	switch provider {
	case GoogleProvider:
		return m.GoogleConfigured(ctx)
	case MicrosoftProvider:
		return m.MicrosoftConfigured(ctx)
	default:
		return false
	}
}

// AnyProviderConfigured reports whether at least one storage provider has an
// OAuth client (drives the surface-wide "enabled" signal).
func (m *Manager) AnyProviderConfigured(ctx context.Context) bool {
	for _, p := range SupportedProviders {
		if m.ProviderConfigured(ctx, p) {
			return true
		}
	}
	return false
}

// SaveMicrosoftCredentials stores the OAuth client from the web-UI setup
// form (encrypted at rest). An empty secret keeps the one already saved.
func (m *Manager) SaveMicrosoftCredentials(ctx context.Context, clientID, clientSecret string) error {
	return m.saveProviderCredentials(ctx, SecretKeyMicrosoftClientID, SecretKeyMicrosoftClientSecret, clientID, clientSecret)
}

// SaveGoogleCredentials stores the OAuth client from the web-UI setup form
// (encrypted at rest by the secrets store). An empty secret keeps the one
// already saved (client-ID-only updates).
func (m *Manager) SaveGoogleCredentials(ctx context.Context, clientID, clientSecret string) error {
	return m.saveProviderCredentials(ctx, SecretKeyGoogleClientID, SecretKeyGoogleClientSecret, clientID, clientSecret)
}

func (m *Manager) saveProviderCredentials(ctx context.Context, idKey, secretKey, clientID, clientSecret string) error {
	if m.secrets == nil {
		return errors.New("cloud: secrets store unavailable — configure via env instead")
	}
	sctx := store.WithTenantID(ctx, store.MasterTenantID)
	if err := m.secrets.Set(sctx, idKey, clientID); err != nil {
		return err
	}
	if clientSecret != "" {
		return m.secrets.Set(sctx, secretKey, clientSecret)
	}
	return nil
}

// GoogleCredentialsStatus returns the configured client ID (for the admin
// settings view; the secret is never returned) and whether a secret is set.
func (m *Manager) GoogleCredentialsStatus(ctx context.Context) (clientID string, secretSet bool) {
	return m.credentialsStatus(ctx, SecretKeyGoogleClientID, SecretKeyGoogleClientSecret, m.cfg.GoogleClientID, m.cfg.GoogleClientSecret)
}

// MicrosoftCredentialsStatus returns the configured Microsoft client ID and
// whether a secret is set (same shape as GoogleCredentialsStatus).
func (m *Manager) MicrosoftCredentialsStatus(ctx context.Context) (clientID string, secretSet bool) {
	return m.credentialsStatus(ctx, SecretKeyMicrosoftClientID, SecretKeyMicrosoftClientSecret, m.cfg.MicrosoftClientID, m.cfg.MicrosoftClientSecret)
}

func (m *Manager) credentialsStatus(ctx context.Context, idKey, secretKey, envID, envSecret string) (clientID string, secretSet bool) {
	if m.secrets != nil {
		sctx := store.WithTenantID(ctx, store.MasterTenantID)
		if id, err := m.secrets.Get(sctx, idKey); err == nil && id != "" {
			secret, serr := m.secrets.Get(sctx, secretKey)
			return id, serr == nil && secret != ""
		}
	}
	return envID, envSecret != ""
}

// BuildAuthURL returns the consent URL for the given storage provider
// ("google" | "onedrive") for a (tenant, user), plus the redirect URI that
// must be registered in the provider's console.
func (m *Manager) BuildAuthURL(ctx context.Context, provider, baseURL, tenantID, userID string) (authURL, redirectURI string, err error) {
	switch provider {
	case GoogleProvider:
		return m.buildGoogleAuthURL(ctx, baseURL, tenantID, userID)
	case MicrosoftProvider:
		return m.buildMicrosoftAuthURL(ctx, baseURL, tenantID, userID)
	default:
		return "", "", fmt.Errorf("cloud: unsupported provider %q", provider)
	}
}

func (m *Manager) buildGoogleAuthURL(ctx context.Context, baseURL, tenantID, userID string) (authURL, redirectURI string, err error) {
	clientID, clientSecret := m.googleCredentials(ctx)
	if clientID == "" || clientSecret == "" {
		return "", "", errors.New("cloud: google oauth client not configured")
	}
	redirectURI = RedirectURI(baseURL)
	cfg := NewGoogleTokenConfig(clientID, clientSecret, redirectURI)

	verifier, err := NewVerifier()
	if err != nil {
		return "", "", err
	}
	state, err := EncodeState(StatePayload{
		Provider: GoogleProvider,
		TenantID: tenantID,
		UserID:   userID,
		Verifier: verifier,
		Redirect: redirectURI,
	}, m.encKey)
	if err != nil {
		return "", "", err
	}

	// access_type=offline is mandatory for a refresh token; prompt=consent
	// re-issues one when the user previously granted (Google only sends the
	// refresh token on first grant otherwise).
	url := cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("access_type", "offline"),
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("code_challenge", VerifierChallenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	return url, redirectURI, nil
}

func (m *Manager) buildMicrosoftAuthURL(ctx context.Context, baseURL, tenantID, userID string) (authURL, redirectURI string, err error) {
	clientID, clientSecret := m.microsoftCredentials(ctx)
	if clientID == "" || clientSecret == "" {
		return "", "", errors.New("cloud: microsoft oauth client not configured")
	}
	redirectURI = RedirectURI(baseURL)
	cfg := NewMicrosoftTokenConfig(clientID, clientSecret, redirectURI)

	verifier, err := NewVerifier()
	if err != nil {
		return "", "", err
	}
	state, err := EncodeState(StatePayload{
		Provider: MicrosoftProvider,
		TenantID: tenantID,
		UserID:   userID,
		Verifier: verifier,
		Redirect: redirectURI,
	}, m.encKey)
	if err != nil {
		return "", "", err
	}

	// response_mode=query puts the code in the query string (the callback
	// handler reads r.URL.Query()) — Azure's default for code flow is already
	// query, but being explicit keeps the contract stable.
	url := cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", VerifierChallenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("response_mode", "query"),
	)
	return url, redirectURI, nil
}

// HandleCallback verifies the signed state, exchanges the code with the
// state-named provider, fetches the provider profile and upserts the
// encrypted account row. Returns the account for the post-connect redirect.
func (m *Manager) HandleCallback(ctx context.Context, code, state string) (*store.CloudAccount, error) {
	payload, err := DecodeState(state, m.encKey)
	if err != nil {
		return nil, err
	}
	if !IsSupportedProvider(payload.Provider) {
		return nil, fmt.Errorf("cloud: unsupported provider %q", payload.Provider)
	}
	if payload.TenantID == "" || payload.UserID == "" {
		return nil, errors.New("cloud: state missing identity")
	}

	sctx, err := scopeContext(ctx, payload.TenantID, payload.UserID)
	if err != nil {
		return nil, err
	}

	var acct *store.CloudAccount
	switch payload.Provider {
	case GoogleProvider:
		acct, err = m.handleGoogleCallback(ctx, code, *payload)
	case MicrosoftProvider:
		acct, err = m.handleMicrosoftCallback(ctx, code, *payload)
	default:
		err = fmt.Errorf("cloud: unsupported provider %q", payload.Provider)
	}
	if err != nil {
		return nil, err
	}

	if err := m.store.Upsert(sctx, acct); err != nil {
		return nil, fmt.Errorf("cloud: persist account: %w", err)
	}
	return acct, nil
}

func (m *Manager) handleGoogleCallback(ctx context.Context, code string, payload StatePayload) (*store.CloudAccount, error) {
	// RFC 6749 §4.1.3: the token exchange MUST repeat the exact redirect_uri
	// used in the authorization request — Google rejects the exchange otherwise.
	clientID, clientSecret := m.googleCredentials(ctx)
	cfg := NewGoogleTokenConfig(clientID, clientSecret, payload.Redirect)
	tok, err := ExchangeGoogleCode(ctx, cfg, code, payload.Verifier)
	if err != nil {
		return nil, fmt.Errorf("cloud: token exchange: %w", err)
	}

	profile, err := fetchUserinfo(ctx, tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("cloud: userinfo: %w", err)
	}
	if profile.Email == "" {
		return nil, errors.New("cloud: userinfo returned no email")
	}

	scopes, _ := json.Marshal(tok.Extra("scope"))
	expires := tok.Expiry
	acct := &store.CloudAccount{
		Provider:       GoogleProvider,
		Email:          profile.Email,
		DisplayName:    profile.Name,
		Scopes:         string(scopes),
		AccessToken:    tok.AccessToken,
		RefreshToken:   tok.RefreshToken,
		TokenExpiresAt: &expires,
		Status:         "active",
	}
	if acct.Scopes == "null" || acct.Scopes == "" {
		scopes, _ = json.Marshal(GoogleScopes)
		acct.Scopes = string(scopes)
	}
	return acct, nil
}

func (m *Manager) handleMicrosoftCallback(ctx context.Context, code string, payload StatePayload) (*store.CloudAccount, error) {
	clientID, clientSecret := m.microsoftCredentials(ctx)
	cfg := NewMicrosoftTokenConfig(clientID, clientSecret, payload.Redirect)
	tok, err := ExchangeMicrosoftCode(ctx, cfg, code, payload.Verifier)
	if err != nil {
		return nil, fmt.Errorf("cloud: token exchange: %w", err)
	}

	profile, err := fetchMicrosoftProfile(ctx, tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("cloud: profile: %w", err)
	}
	email := profile.Email()
	if email == "" {
		return nil, errors.New("cloud: profile returned no email")
	}

	// Resolve the default drive up front: the rclone onedrive backend needs
	// drive_id/drive_type for a non-interactive config create.
	drive, err := fetchMicrosoftDefaultDrive(ctx, tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("cloud: drives: %w", err)
	}
	settings, _ := json.Marshal(map[string]string{
		"drive_id":   drive.ID,
		"drive_type": drive.DriveType,
	})

	scopes, _ := json.Marshal(tok.Extra("scope"))
	if string(scopes) == "null" || string(scopes) == "" {
		scopes, _ = json.Marshal(MicrosoftScopes)
	}
	expires := tok.Expiry
	acct := &store.CloudAccount{
		Provider:       MicrosoftProvider,
		Email:          email,
		DisplayName:    profile.DisplayName,
		Scopes:         string(scopes),
		AccessToken:    tok.AccessToken,
		RefreshToken:   tok.RefreshToken,
		TokenExpiresAt: &expires,
		Status:         "active",
		Settings:       string(settings),
	}
	return acct, nil
}

// Delete removes a connected account (scoped by ctx identity). When a storage
// service is attached, the account's rclone remote is also deleted so its
// tokens do not linger in the shared rclone.conf on disk.
func (m *Manager) Delete(ctx context.Context, id string) error {
	if err := m.store.Delete(ctx, id); err != nil {
		return err
	}
	if m.storage != nil {
		m.storage.RemoveRemote(ctx, id)
	}
	return nil
}

// SetStorageService attaches the rclone-backed storage service (optional).
func (m *Manager) SetStorageService(s *StorageService) { m.storage = s }

// TokenSource returns a live OAuth token source for the given account ID
// (scoped by ctx tenant+user). The source auto-refreshes via singleflight so
// concurrent tool calls share one refresh.
func (m *Manager) TokenSource(ctx context.Context, accountID string) (oauth2.TokenSource, error) {
	acct, err := m.store.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acct.RefreshToken == "" {
		return nil, errors.New("cloud: account has no refresh token (reconnect required)")
	}
	var cfg *oauth2.Config
	switch acct.Provider {
	case GoogleProvider:
		// x/oauth2's refresh only needs client credentials — no redirect URI.
		clientID, clientSecret := m.googleCredentials(ctx)
		cfg = NewGoogleTokenConfig(clientID, clientSecret, "")
	case MicrosoftProvider:
		clientID, clientSecret := m.microsoftCredentials(ctx)
		cfg = NewMicrosoftTokenConfig(clientID, clientSecret, "")
	default:
		return nil, fmt.Errorf("cloud: unsupported provider %q", acct.Provider)
	}
	ts := newAccountTokenSource(m.store, acct, cfg.TokenSource(ctx, refreshOnlyToken(acct)))
	return ts, nil
}

// RedirectURI resolves the OAuth redirect URI from an explicit base or the
// request-derived origin (caller applies X-Forwarded-* precedence).
func RedirectURI(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	return trimRight(baseURL, "/") + "/v1/cloud/oauth/callback"
}

func trimRight(s, cut string) string {
	for len(s) >= len(cut) && s[len(s)-len(cut):] == cut {
		s = s[:len(s)-len(cut)]
	}
	return s
}

func refreshOnlyToken(acct *store.CloudAccount) *oauth2.Token {
	t := &oauth2.Token{
		AccessToken:  acct.AccessToken,
		RefreshToken: acct.RefreshToken,
		Expiry:       time.Now().Add(-time.Minute), // force refresh on first use
	}
	if acct.TokenExpiresAt != nil && acct.TokenExpiresAt.After(time.Now().Add(2*time.Minute)) {
		t.Expiry = *acct.TokenExpiresAt
	}
	return t
}

func fetchUserinfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GoogleUserinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo status %d", resp.StatusCode)
	}
	var info GoogleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
