package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- registry shape ---

func TestCredentialRegistrySpecs(t *testing.T) {
	for _, id := range CredentialProviderIDs() {
		spec, ok := CredentialProviderByID(id)
		if !ok {
			t.Fatalf("provider %q listed but missing from the registry", id)
		}
		if spec.ID != id {
			t.Fatalf("spec.ID %q != registry key %q", spec.ID, id)
		}
		if spec.RemoteType == "" {
			t.Fatalf("provider %q has no rclone remote type", id)
		}
		if len(spec.Fields) == 0 {
			t.Fatalf("provider %q has no fields", id)
		}
		// Whitelist must be EXACTLY the field keys (no extras, no misses) —
		// that equality is the security boundary against rclone-option
		// injection.
		whitelist := map[string]bool{}
		for _, k := range spec.ParamWhitelist {
			if whitelist[k] {
				t.Fatalf("provider %q: whitelist key %q duplicated", id, k)
			}
			whitelist[k] = true
		}
		seen := map[string]bool{}
		for _, f := range spec.Fields {
			if f.Key == "" {
				t.Fatalf("provider %q has a field with an empty key", id)
			}
			if seen[f.Key] {
				t.Fatalf("provider %q: field key %q duplicated", id, f.Key)
			}
			seen[f.Key] = true
			if f.Type != FieldTypeText && f.Type != FieldTypePassword {
				t.Fatalf("provider %q field %q has invalid type %q", id, f.Key, f.Type)
			}
		}
		for k := range whitelist {
			if !seen[k] {
				t.Fatalf("provider %q: whitelist key %q is not a field key", id, k)
			}
		}
		for k := range seen {
			if !whitelist[k] {
				t.Fatalf("provider %q: field key %q missing from the whitelist", id, k)
			}
		}
		// Every provider needs at least one required secret.
		hasSecret := false
		for _, f := range spec.Fields {
			if f.Required && f.Type == FieldTypePassword {
				hasSecret = true
			}
		}
		if !hasSecret {
			t.Fatalf("provider %q has no required password field", id)
		}
	}
}

func TestSupportedProvidersIncludeCredentialProviders(t *testing.T) {
	for _, id := range CredentialProviderIDs() {
		if !IsSupportedProvider(id) {
			t.Fatalf("credential provider %q missing from SupportedProviders", id)
		}
		if !IsCredentialProvider(id) {
			t.Fatalf("IsCredentialProvider(%q) = false", id)
		}
		if !isStorageProvider(id) {
			t.Fatalf("isStorageProvider(%q) = false — tools could not use the account", id)
		}
	}
	if IsCredentialProvider("google") || IsCredentialProvider("onedrive") {
		t.Fatal("OAuth providers must not be credential providers")
	}
}

// --- validation ---

func TestSanitizeValidateParamsRequiredAndUnknownKeys(t *testing.T) {
	spec := credentialProviders["s3"]

	// Unknown key → rejected even when everything else is valid (whitelist
	// enforcement: "type"/"token"/any extra rclone option must never pass).
	_, err := spec.SanitizeAndValidateParams(map[string]string{
		"access_key_id":     "AKIA…",
		"secret_access_key": "secret",
		"type":              "sftp", // injection attempt
	})
	if err == nil || !strings.Contains(err.Error(), "unknown parameter") {
		t.Fatalf("unknown key must be rejected, got err=%v", err)
	}

	// Missing required field.
	if _, err := spec.SanitizeAndValidateParams(map[string]string{"access_key_id": "AKIA…"}); err == nil {
		t.Fatal("missing secret_access_key must fail")
	}

	// Valid minimal set: optionals dropped, values trimmed.
	got, err := spec.SanitizeAndValidateParams(map[string]string{
		"access_key_id":     "  AKIA…  ",
		"secret_access_key": "secret",
		"region":            "  ",
	})
	if err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
	if got["access_key_id"] != "AKIA…" {
		t.Fatalf("value not trimmed: %q", got["access_key_id"])
	}
	if _, ok := got["region"]; ok {
		t.Fatal("blank optional must be dropped")
	}

	// Length sanity.
	if _, err := spec.SanitizeAndValidateParams(map[string]string{
		"access_key_id":     strings.Repeat("x", credentialValueMaxLen+1),
		"secret_access_key": "secret",
	}); err == nil {
		t.Fatal("oversized value must fail")
	}
}

func TestSanitizeValidateParamsS3EndpointAndHint(t *testing.T) {
	spec := credentialProviders["s3"]
	base := map[string]string{"access_key_id": "AKIA…", "secret_access_key": "s"}

	// s3 endpoint must be an absolute https URL when set.
	for _, bad := range []string{"http://storage.example.com", "storage.example.com", "ftp://x", "https://"} {
		p := map[string]string{"access_key_id": "AKIA…", "secret_access_key": "s", "endpoint": bad}
		if _, err := spec.SanitizeAndValidateParams(p); err == nil {
			t.Fatalf("s3 endpoint %q must be rejected", bad)
		}
	}
	p := map[string]string{"access_key_id": "AKIA…", "secret_access_key": "s", "endpoint": "https://<account>.r2.cloudflarestorage.com"}
	if _, err := spec.SanitizeAndValidateParams(p); err != nil {
		t.Fatalf("https endpoint must pass: %v", err)
	}

	// provider hint is an enum.
	p = map[string]string{"access_key_id": "AKIA…", "secret_access_key": "s", "provider": "cloudflare_r2"}
	if _, err := spec.SanitizeAndValidateParams(p); err != nil {
		t.Fatalf("cloudflare_r2 hint must pass: %v", err)
	}
	p["provider"] = "dropbox"
	if _, err := spec.SanitizeAndValidateParams(p); err == nil {
		t.Fatal("unknown provider hint must be rejected")
	}
	_ = base
}

func TestSanitizeValidateParamsWebdavAndB2AndPCloud(t *testing.T) {
	webdav := credentialProviders["webdav"]
	good := map[string]string{
		"url":    "https://nextcloud.example.com/remote.php/dav",
		"vendor": "nextcloud",
		"user":   "alice",
		"pass":   "pw",
	}
	if _, err := webdav.SanitizeAndValidateParams(good); err != nil {
		t.Fatalf("valid webdav params rejected: %v", err)
	}
	bad := map[string]string{"url": "not a url", "user": "alice", "pass": "pw"}
	if _, err := webdav.SanitizeAndValidateParams(bad); err == nil {
		t.Fatal("webdav url must be a URL")
	}
	bad = map[string]string{"url": "https://x.example.com", "vendor": "sharepoint", "user": "a", "pass": "p"}
	if _, err := webdav.SanitizeAndValidateParams(bad); err == nil {
		t.Fatal("webdav vendor outside the enum must be rejected")
	}
	// http URL is allowed for self-hosted LAN servers.
	lan := map[string]string{"url": "http://192.168.1.10/remote.php/dav", "user": "a", "pass": "p"}
	if _, err := webdav.SanitizeAndValidateParams(lan); err != nil {
		t.Fatalf("webdav http url for LAN must pass: %v", err)
	}

	b2 := credentialProviders["b2"]
	if _, err := b2.SanitizeAndValidateParams(map[string]string{"account": "keyID", "key": "appKey"}); err != nil {
		t.Fatalf("valid b2 params rejected: %v", err)
	}
	if _, err := b2.SanitizeAndValidateParams(map[string]string{"account": "keyID"}); err == nil {
		t.Fatal("b2 without application key must fail")
	}
	pcloud := credentialProviders["pcloud"]
	if _, err := pcloud.SanitizeAndValidateParams(map[string]string{"username": "u", "password": "p"}); err != nil {
		t.Fatalf("valid pcloud params rejected: %v", err)
	}
}

// --- can_write semantics ---

func TestAccountCanWriteCredentialProviders(t *testing.T) {
	for _, id := range CredentialProviderIDs() {
		acct := &store.CloudAccount{Provider: id, Scopes: `["` + CredentialScopesMarker + `"]`}
		if !AccountCanWrite(acct) {
			t.Fatalf("credential provider %q must be write-capable", id)
		}
		// Registry-based, not scopes-based: even a legacy/empty scopes value
		// stays full-access for these providers.
		if !AccountCanWrite(&store.CloudAccount{Provider: id, Scopes: ""}) {
			t.Fatalf("credential provider %q with empty scopes must stay writable", id)
		}
	}
	if AccountCanWrite(&store.CloudAccount{Provider: "dropbox"}) {
		t.Fatal("unknown provider must not be writable")
	}
}

// --- account construction (secret/plaintext split) ---

func TestNewCredentialAccount(t *testing.T) {
	m := NewManager(CloudProviderConfig{}, nil, "test-key")
	acct, err := m.NewCredentialAccount(context.Background(), "s3", "  My R2 bucket  ", map[string]string{
		"access_key_id":     "AKIA…",
		"secret_access_key": "top-secret",
		"region":            "auto",
		"endpoint":          "https://acct.r2.cloudflarestorage.com",
		"provider":          "cloudflare_r2",
	}, "11111111-1111-1111-1111-111111111111", "user-1")
	if err != nil {
		t.Fatalf("NewCredentialAccount: %v", err)
	}
	if acct.ID == "" {
		t.Fatal("account ID must be pre-generated (the probe remote derives from it)")
	}
	if acct.Email != "My R2 bucket" || acct.DisplayName != "My R2 bucket" {
		t.Fatalf("display name not trimmed/used: email=%q display=%q", acct.Email, acct.DisplayName)
	}

	// Settings must carry ONLY non-secret params plus the marker — the
	// settings column is plaintext and returned by the accounts API.
	var settings struct {
		ConnectedVia string            `json:"connected_via"`
		RcloneParams map[string]string `json:"rclone_params"`
	}
	if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	if settings.ConnectedVia != CredentialScopesMarker {
		t.Fatalf("connected_via = %q", settings.ConnectedVia)
	}
	if v, ok := settings.RcloneParams["secret_access_key"]; ok {
		t.Fatalf("secret leaked into plaintext settings: %q", v)
	}
	if settings.RcloneParams["access_key_id"] != "AKIA…" || settings.RcloneParams["endpoint"] != "https://acct.r2.cloudflarestorage.com" {
		t.Fatalf("non-secret params missing from settings: %#v", settings.RcloneParams)
	}

	// Secrets ride in the refresh-token column (AES-256-GCM at rest).
	var secret map[string]string
	if err := json.Unmarshal([]byte(acct.RefreshToken), &secret); err != nil {
		t.Fatalf("secret JSON: %v", err)
	}
	if secret["secret_access_key"] != "top-secret" {
		t.Fatalf("secret param not in the encrypted column: %#v", secret)
	}
	if _, ok := secret["access_key_id"]; ok {
		t.Fatal("non-secret param must not be duplicated into the secret column")
	}

	// Scopes marker + status.
	if !strings.Contains(acct.Scopes, CredentialScopesMarker) {
		t.Fatalf("scopes marker missing: %q", acct.Scopes)
	}
	if acct.Status != "active" {
		t.Fatalf("status = %q", acct.Status)
	}

	// Unsupported provider / missing identity rejected.
	if _, err := m.NewCredentialAccount(context.Background(), "google", "x", nil, "t", "u"); err == nil {
		t.Fatal("OAuth provider must be rejected by NewCredentialAccount")
	}
	if _, err := m.NewCredentialAccount(context.Background(), "s3", "x", map[string]string{"access_key_id": "a", "secret_access_key": "b"}, "", "u"); err == nil {
		t.Fatal("missing tenant must be rejected")
	}

	// Empty display name falls back to the provider label.
	acct, err = m.NewCredentialAccount(context.Background(), "b2", "   ", map[string]string{"account": "k", "key": "v"}, "t", "u")
	if err != nil {
		t.Fatalf("NewCredentialAccount(b2): %v", err)
	}
	if acct.DisplayName != "Backblaze B2" {
		t.Fatalf("display name fallback = %q", acct.DisplayName)
	}
}

// --- rclone parameter building (whitelist enforcement at build time) ---

func TestCredentialRemoteParamsS3(t *testing.T) {
	// Tampered settings row: an attacker-managed value tries to inject rclone
	// options ("type", "token") alongside the legit params. Only whitelisted
	// keys may reach the config create call.
	settings, _ := json.Marshal(map[string]any{
		"connected_via": "credentials",
		"rclone_params": map[string]string{
			"access_key_id": "AKIA…",
			"region":        "us-east-1",
			"endpoint":      "https://s3.example.com",
			"type":          "sftp",           // injection attempt (not a field key)
			"token":         "eyJhbGciOiJ...", // injection attempt
		},
	})
	acct := &store.CloudAccount{
		Provider:     "s3",
		Settings:     string(settings),
		RefreshToken: `{"secret_access_key":"top-secret"}`,
	}
	params, err := credentialRemoteParams(acct)
	if err != nil {
		t.Fatalf("credentialRemoteParams: %v", err)
	}
	want := map[string]any{
		"access_key_id":     "AKIA…",
		"region":            "us-east-1",
		"endpoint":          "https://s3.example.com",
		"secret_access_key": "top-secret",
	}
	if len(params) != len(want) {
		t.Fatalf("params = %#v, want exactly %#v", params, want)
	}
	for k, v := range want {
		if params[k] != v {
			t.Fatalf("params[%q] = %v, want %v", k, params[k], v)
		}
	}
}

func TestCredentialRemoteParamsOtherProvidersAndErrors(t *testing.T) {
	// webdav: plain params from settings + secret from the encrypted column.
	acct := &store.CloudAccount{
		Provider:     "webdav",
		Settings:     `{"connected_via":"credentials","rclone_params":{"url":"https://nc.example.com/dav","vendor":"nextcloud","user":"alice"}}`,
		RefreshToken: `{"pass":"hunter2"}`,
	}
	params, err := credentialRemoteParams(acct)
	if err != nil {
		t.Fatalf("credentialRemoteParams(webdav): %v", err)
	}
	if params["url"] != "https://nc.example.com/dav" || params["vendor"] != "nextcloud" || params["user"] != "alice" || params["pass"] != "hunter2" {
		t.Fatalf("webdav params = %#v", params)
	}

	// No secrets stored (older row) still builds the plain params.
	plainOnly := &store.CloudAccount{Provider: "b2", Settings: `{"rclone_params":{"account":"k"}}`}
	params, err = credentialRemoteParams(plainOnly)
	if err != nil {
		t.Fatalf("credentialRemoteParams(b2 plain-only): %v", err)
	}
	if len(params) != 1 || params["account"] != "k" {
		t.Fatalf("b2 params = %#v", params)
	}

	// Malformed secret JSON → explicit reconnect error, never silent param loss.
	bad := &store.CloudAccount{Provider: "s3", Settings: `{}`, RefreshToken: "not-json"}
	if _, err := credentialRemoteParams(bad); err == nil {
		t.Fatal("malformed secret JSON must fail")
	}

	// Malformed settings JSON.
	badSettings := &store.CloudAccount{Provider: "s3", Settings: "not-json"}
	if _, err := credentialRemoteParams(badSettings); err == nil {
		t.Fatal("malformed settings JSON must fail")
	}

	// Non-credential provider.
	if _, err := credentialRemoteParams(&store.CloudAccount{Provider: GoogleProvider}); err == nil {
		t.Fatal("OAuth provider must be rejected")
	}
}
