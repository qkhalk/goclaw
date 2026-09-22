package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Credential-based providers: rclone backends connected with keys/passwords
// the user types directly — NO OAuth app registration needed (the rclone
// model). Each provider is described by a strict spec: the rclone remote
// type, the credential fields, and a per-provider PARAMETER WHITELIST. Only
// whitelisted keys may ever flow into the rclone config — that is the
// security boundary preventing arbitrary rclone-option injection (e.g. a
// crafted "type" or "token" parameter hijacking the remote).

// Field types for FieldSpec.Type.
const (
	FieldTypeText = "text"
	// FieldTypePassword — single-line secret: masked input in the UI, stored
	// in the encrypted column.
	FieldTypePassword = "password"
	// FieldTypeSecretTextarea — multi-line secret (PEM private key,
	// service-account key JSON): textarea in the UI, stored in the encrypted
	// column exactly like a password.
	FieldTypeSecretTextarea = "secret_textarea"
)

// credentialValueMaxLen bounds every user-supplied credential value — enough
// for any real key/token, small enough to stop absurd payloads.
const credentialValueMaxLen = 1024

// Per-field length bounds for multi-line secrets: real-world service-account
// key JSON is ~2-3 KB and an RSA-4096 PEM key ~3.2 KB — both far exceed the
// default bound above.
const (
	serviceAccountJSONMaxLen = 16384
	pemPrivateKeyMaxLen      = 16384
)

// FieldSpec describes one credential field of a CredentialProvider.
type FieldSpec struct {
	// Key is BOTH the JSON field name the API accepts AND the rclone config
	// parameter name (whitelisted via ParamWhitelist).
	Key string
	// Label is the English fallback label; the web UI has its own i18n copy
	// (keep ui/web credentials-connect-dialog.tsx in sync with these specs).
	Label string
	// Type is FieldTypeText, FieldTypePassword or FieldTypeSecretTextarea
	// (secret types → encrypted column).
	Type string
	// Required fields must be non-empty after trimming.
	Required bool
	// Hint is optional English help text for the field.
	Hint string
	// MaxLen overrides credentialValueMaxLen for oversized values (multi-line
	// secrets); 0 means the default.
	MaxLen int
}

// CredentialProvider describes one rclone backend connected via typed
// credentials instead of the OAuth web flow.
type CredentialProvider struct {
	// ID is the provider identifier ("s3", "b2", …) stored on the account row.
	ID string
	// RemoteType is the rclone backend type for rc ConfigCreate.
	RemoteType string
	// Label is the English display label.
	Label string
	// Fields are the credential form fields, in UI display order.
	Fields []FieldSpec
	// ParamWhitelist is the exhaustive set of parameter keys allowed to reach
	// the rclone config for this provider. Derived from Fields; asserted by
	// tests to stay exactly in sync.
	ParamWhitelist []string
	// ProbeViaList marks backends WITHOUT rclone operations/about (quota)
	// support — the connect probe proves auth with a root listing instead.
	// Declared per provider (azureblob, gcs, ftp report "not implemented")
	// so the HTTP layer never depends on rclone error-string matching.
	ProbeViaList bool
}

// s3ProviderHints are the accepted values for the s3 "provider" hint. rclone
// auto-detects most S3-compatible services; these cover the common ones
// where the hint genuinely matters ("other" lets rclone detect).
var s3ProviderHints = map[string]bool{
	"aws":           true,
	"cloudflare_r2": true,
	"digitalocean":  true,
	"other":         true,
}

// webdavVendors are the accepted values for the webdav "vendor" hint.
var webdavVendors = map[string]bool{
	"nextcloud": true,
	"owncloud":  true,
	"other":     true,
}

// credentialProviders is the credential-provider registry, keyed by provider
// id. Order-insensitive; CredentialProviderIDs returns the display order.
var credentialProviders = map[string]CredentialProvider{
	"s3": {
		ID:         "s3",
		RemoteType: "s3",
		Label:      "Amazon S3 / compatible",
		Fields: []FieldSpec{
			{Key: "access_key_id", Label: "Access key ID", Type: FieldTypeText, Required: true},
			{Key: "secret_access_key", Label: "Secret access key", Type: FieldTypePassword, Required: true},
			{Key: "region", Label: "Region", Type: FieldTypeText, Required: false, Hint: "e.g. us-east-1 (optional for R2/MinIO)"},
			{Key: "endpoint", Label: "Endpoint", Type: FieldTypeText, Required: false, Hint: "https://… for R2, Spaces, MinIO (optional for AWS)"},
			{Key: "provider", Label: "Service type", Type: FieldTypeText, Required: false, Hint: "aws | cloudflare_r2 | digitalocean | other"},
		},
		ParamWhitelist: []string{"access_key_id", "secret_access_key", "region", "endpoint", "provider"},
	},
	"b2": {
		ID:         "b2",
		RemoteType: "b2",
		Label:      "Backblaze B2",
		Fields: []FieldSpec{
			{Key: "account", Label: "Key ID", Type: FieldTypeText, Required: true},
			{Key: "key", Label: "Application key", Type: FieldTypePassword, Required: true},
		},
		ParamWhitelist: []string{"account", "key"},
	},
	"pcloud": {
		ID:         "pcloud",
		RemoteType: "pcloud",
		Label:      "pCloud",
		Fields: []FieldSpec{
			{Key: "username", Label: "Username", Type: FieldTypeText, Required: true},
			{Key: "password", Label: "Password", Type: FieldTypePassword, Required: true},
		},
		ParamWhitelist: []string{"username", "password"},
	},
	"webdav": {
		ID:         "webdav",
		RemoteType: "webdav",
		Label:      "WebDAV",
		Fields: []FieldSpec{
			{Key: "url", Label: "URL", Type: FieldTypeText, Required: true, Hint: "https://nextcloud.example.com/remote.php/dav"},
			{Key: "vendor", Label: "Server type", Type: FieldTypeText, Required: false, Hint: "nextcloud | owncloud | other"},
			{Key: "user", Label: "Username", Type: FieldTypeText, Required: true},
			{Key: "pass", Label: "Password", Type: FieldTypePassword, Required: true},
		},
		ParamWhitelist: []string{"url", "vendor", "user", "pass"},
	},
	"azureblob": {
		ID:         "azureblob",
		RemoteType: "azureblob",
		Label:      "Azure Blob Storage",
		// No operations/about in rclone — probe with a container listing.
		ProbeViaList: true,
		Fields: []FieldSpec{
			{Key: "account", Label: "Storage account name", Type: FieldTypeText, Required: true},
			{Key: "key", Label: "Storage account key", Type: FieldTypePassword, Required: true},
			{Key: "endpoint", Label: "Endpoint", Type: FieldTypeText, Required: false, Hint: "https://<account>.blob.core.windows.net (optional; plain http for Azurite)"},
		},
		ParamWhitelist: []string{"account", "key", "endpoint"},
	},
	"gcs": {
		ID:         "gcs",
		RemoteType: "googlecloudstorage",
		Label:      "Google Cloud Storage",
		// No operations/about in rclone — probe with a bucket listing, which
		// is also why project_number is required.
		ProbeViaList: true,
		Fields: []FieldSpec{
			{Key: "service_account_credentials", Label: "Service account JSON", Type: FieldTypeSecretTextarea, Required: true, MaxLen: serviceAccountJSONMaxLen, Hint: "Paste the whole service-account key JSON file"},
			{Key: "project_number", Label: "Project number", Type: FieldTypeText, Required: true, Hint: "Google Cloud project number (IAM & Admin → Project info)"},
		},
		ParamWhitelist: []string{"service_account_credentials", "project_number"},
	},
	"ftp": {
		ID:           "ftp",
		RemoteType:   "ftp",
		Label:        "FTP",
		ProbeViaList: true, // no operations/about in rclone
		Fields: []FieldSpec{
			{Key: "host", Label: "Host", Type: FieldTypeText, Required: true, Hint: "Hostname or IP, no scheme"},
			{Key: "port", Label: "Port", Type: FieldTypeText, Required: false, Hint: "Default 21"},
			{Key: "user", Label: "Username", Type: FieldTypeText, Required: true, Hint: "\"anonymous\" for public FTP servers"},
			{Key: "pass", Label: "Password", Type: FieldTypePassword, Required: true},
			{Key: "explicit_tls", Label: "Explicit TLS (FTPS)", Type: FieldTypeText, Required: false, Hint: "true | false (default false)"},
		},
		ParamWhitelist: []string{"host", "port", "user", "pass", "explicit_tls"},
	},
	"sftp": {
		ID:         "sftp",
		RemoteType: "sftp",
		Label:      "SFTP / SSH",
		Fields: []FieldSpec{
			{Key: "host", Label: "Host", Type: FieldTypeText, Required: true, Hint: "Hostname or IP, no scheme"},
			{Key: "port", Label: "Port", Type: FieldTypeText, Required: false, Hint: "Default 22"},
			{Key: "user", Label: "Username", Type: FieldTypeText, Required: true},
			{Key: "pass", Label: "Password", Type: FieldTypePassword, Required: false},
			{Key: "key_pem", Label: "Private key (PEM)", Type: FieldTypeSecretTextarea, Required: false, MaxLen: pemPrivateKeyMaxLen, Hint: "Paste an UNENCRYPTED private key (newlines are handled for you)"},
			{Key: "key_file_pass", Label: "Key passphrase", Type: FieldTypePassword, Required: false, Hint: "Only for passphrase-protected PEM keys"},
		},
		ParamWhitelist: []string{"host", "port", "user", "pass", "key_pem", "key_file_pass"},
	},
	"smb": {
		ID:         "smb",
		RemoteType: "smb",
		Label:      "SMB / Samba share",
		Fields: []FieldSpec{
			{Key: "host", Label: "Host", Type: FieldTypeText, Required: true, Hint: "Hostname or IP, no scheme"},
			{Key: "user", Label: "Username", Type: FieldTypeText, Required: true, Hint: "\"guest\" for guest shares"},
			{Key: "pass", Label: "Password", Type: FieldTypePassword, Required: true},
			{Key: "domain", Label: "Domain", Type: FieldTypeText, Required: false, Hint: "Default WORKGROUP"},
			{Key: "port", Label: "Port", Type: FieldTypeText, Required: false, Hint: "Default 445"},
		},
		ParamWhitelist: []string{"host", "user", "pass", "domain", "port"},
	},
}

// credentialProviderOrder is the stable display/registration order: object
// storage, personal/self-hosted cloud, then transfer protocols.
var credentialProviderOrder = []string{"s3", "b2", "azureblob", "gcs", "pcloud", "webdav", "ftp", "sftp", "smb"}

// CredentialProviderIDs returns the credential provider ids in display order.
func CredentialProviderIDs() []string {
	out := make([]string, 0, len(credentialProviderOrder))
	out = append(out, credentialProviderOrder...)
	return out
}

// CredentialProviderByID looks up a credential provider by id.
func CredentialProviderByID(id string) (CredentialProvider, bool) {
	p, ok := credentialProviders[id]
	return p, ok
}

// IsCredentialProvider reports whether the provider id connects via typed
// credentials (no OAuth flow).
func IsCredentialProvider(provider string) bool {
	_, ok := credentialProviders[provider]
	return ok
}

// CredentialScopesMarker is the scopes value stored on credential-connected
// accounts: `["credentials"]`. These providers have no OAuth grant — the
// typed key IS full access — so AccountCanWrite treats the marker (more
// precisely: the provider registry) as inherently write-capable.
const CredentialScopesMarker = "credentials"

// SanitizeAndValidateParams trims every value, REJECTS unknown keys
// (whitelist enforcement — a key outside ParamWhitelist can never reach the
// rclone config), checks required fields and value sanity, and returns the
// cleaned params (empty optional values dropped).
func (p CredentialProvider) SanitizeAndValidateParams(params map[string]string) (map[string]string, error) {
	allowed := make(map[string]bool, len(p.ParamWhitelist))
	for _, k := range p.ParamWhitelist {
		allowed[k] = true
	}
	clean := make(map[string]string, len(params))
	for key, raw := range params {
		if !allowed[key] {
			return nil, fmt.Errorf("cloud: unknown parameter %q for provider %q", key, p.ID)
		}
		v := strings.TrimSpace(raw)
		if v == "" {
			continue // optional fields may be omitted; required checked below
		}
		v = p.normalizeFieldValue(key, v)
		if max := p.fieldMaxLen(key); len(v) > max {
			return nil, fmt.Errorf("cloud: parameter %q exceeds %d characters", key, max)
		}
		clean[key] = v
	}
	for _, f := range p.Fields {
		if f.Required && clean[f.Key] == "" {
			return nil, fmt.Errorf("cloud: %s: %q is required", p.ID, f.Key)
		}
	}
	if err := p.validateFieldValueHints(clean); err != nil {
		return nil, err
	}
	return clean, nil
}

// fieldMaxLen returns the value length bound for one field (FieldSpec.MaxLen
// when set, else the shared credentialValueMaxLen).
func (p CredentialProvider) fieldMaxLen(key string) int {
	for _, f := range p.Fields {
		if f.Key == key && f.MaxLen > 0 {
			return f.MaxLen
		}
	}
	return credentialValueMaxLen
}

// normalizeFieldValue adapts pasted multi-line secrets to the exact form the
// rclone backend expects; it runs BEFORE the length check. rclone requires:
//   - sftp key_pem as ONE line with literal "\n" separators (rclone docs:
//     "should be on a single line with line endings replaced with '\n'"), and
//   - gcs service_account_credentials without raw newlines (the rclone config
//     file cannot hold them; compacting JSON is lossless).
func (p CredentialProvider) normalizeFieldValue(key, v string) string {
	switch {
	case p.ID == "sftp" && key == "key_pem":
		v = strings.ReplaceAll(v, "\r\n", "\n")
		v = strings.ReplaceAll(v, "\r", "\n")
		return strings.ReplaceAll(v, "\n", "\\n")
	case p.ID == "gcs" && key == "service_account_credentials":
		var buf bytes.Buffer
		if err := json.Compact(&buf, []byte(v)); err == nil {
			return buf.String()
		}
		// Invalid JSON is caught by validateFieldValueHints below.
	}
	return v
}

// validateFieldValueHints checks per-field semantic constraints beyond
// presence/length: URL shape for endpoint/url fields, enum membership for the
// s3 provider / webdav vendor hints, and cross-field auth requirements for
// the protocol backends.
func (p CredentialProvider) validateFieldValueHints(params map[string]string) error {
	// Any provider exposing a "port" option must receive a TCP port number.
	if v := params["port"]; v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("cloud: %s: port must be a TCP port number (1-65535)", p.ID)
		}
	}
	switch p.ID {
	case "s3":
		if v := params["endpoint"]; v != "" {
			if err := validateHTTPSURL(v); err != nil {
				return fmt.Errorf("cloud: s3: endpoint: %w", err)
			}
		}
		if v := params["provider"]; v != "" && !s3ProviderHints[v] {
			return fmt.Errorf("cloud: s3: provider must be one of aws, cloudflare_r2, digitalocean, other")
		}
	case "webdav":
		if v := params["url"]; v != "" {
			// http allowed: self-hosted Nextcloud instances on LAN are common.
			if err := validateHTTPURL(v); err != nil {
				return fmt.Errorf("cloud: webdav: url: %w", err)
			}
		}
		if v := params["vendor"]; v != "" && !webdavVendors[v] {
			return fmt.Errorf("cloud: webdav: vendor must be one of nextcloud, owncloud, other")
		}
	case "azureblob":
		if v := params["endpoint"]; v != "" {
			// http allowed: Azurite / on-prem emulators are plain http.
			if err := validateHTTPURL(v); err != nil {
				return fmt.Errorf("cloud: azureblob: endpoint: %w", err)
			}
		}
	case "gcs":
		if v := params["service_account_credentials"]; v != "" && !json.Valid([]byte(v)) {
			return errors.New(`cloud: gcs: service_account_credentials must be the service-account key JSON`)
		}
	case "ftp":
		if v := params["explicit_tls"]; v != "" && v != "true" && v != "false" {
			return errors.New("cloud: ftp: explicit_tls must be true or false")
		}
	case "sftp":
		// Exactly one auth secret is required: password or private key.
		if params["pass"] == "" && params["key_pem"] == "" {
			return errors.New(`cloud: sftp: either "pass" or "key_pem" is required`)
		}
	}
	return nil
}

// validateHTTPSURL requires an absolute https:// URL with a host (cloud
// storage endpoints must not send credentials over plaintext http).
func validateHTTPSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("must be a valid https:// URL")
	}
	if u.Scheme != "https" || u.Host == "" {
		return errors.New("must be an absolute https:// URL")
	}
	return nil
}

// validateHTTPURL requires an absolute http(s):// URL with a host.
func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("must be a valid http(s):// URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("must be an absolute http(s):// URL")
	}
	return nil
}

// isSecretFieldType reports whether a field's values must live in the
// encrypted column rather than the plaintext settings JSON.
func isSecretFieldType(t string) bool {
	return t == FieldTypePassword || t == FieldTypeSecretTextarea
}

// splitCredentialParams splits validated params into non-secret (stored in
// the plaintext settings JSON) and secret values (stored in the encrypted
// refresh-token column — see NewCredentialAccount).
func splitCredentialParams(p CredentialProvider, params map[string]string) (plain, secret map[string]string) {
	plain, secret = map[string]string{}, map[string]string{}
	for _, f := range p.Fields {
		v, ok := params[f.Key]
		if !ok || v == "" {
			continue
		}
		if isSecretFieldType(f.Type) {
			secret[f.Key] = v
		} else {
			plain[f.Key] = v
		}
	}
	return plain, secret
}

// NewCredentialAccount builds (but does NOT persist) a credential-connected
// account row. The HTTP layer probes the credentials with rclone FIRST and
// only calls the store Upsert when the probe succeeds.
//
// Storage layout, by sensitivity:
//   - Settings (plaintext settings column): {"connected_via":"credentials",
//     "rclone_params":{NON-SECRET whitelisted params only}}.
//   - RefreshToken (AES-256-GCM encrypted at rest by the store — same scheme
//     as OAuth tokens): JSON of the secret params (password-type fields).
//     The settings column is plaintext and returned by the accounts API, so
//     secrets must never land there.
//
// Email doubles as the per-provider unique key (Upsert conflicts on
// tenant+user+provider+email): reconnecting with the same display name
// replaces the previous credentials.
func (m *Manager) NewCredentialAccount(ctx context.Context, provider, displayName string, params map[string]string, tenantID, userID string) (*store.CloudAccount, error) {
	spec, ok := CredentialProviderByID(provider)
	if !ok {
		return nil, fmt.Errorf("cloud: unsupported credential provider %q", provider)
	}
	if tenantID == "" || userID == "" {
		return nil, errors.New("cloud: missing tenant/user identity")
	}
	clean, err := spec.SanitizeAndValidateParams(params)
	if err != nil {
		return nil, err
	}
	plain, secret := splitCredentialParams(spec, clean)

	settingsJSON, err := json.Marshal(map[string]any{
		"connected_via": CredentialScopesMarker,
		"rclone_params": plain,
	})
	if err != nil {
		return nil, err
	}
	secretJSON, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	scopesJSON, err := json.Marshal([]string{CredentialScopesMarker})
	if err != nil {
		return nil, err
	}

	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = spec.Label
	}
	return &store.CloudAccount{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		UserID:       userID,
		Provider:     spec.ID,
		Email:        displayName,
		DisplayName:  displayName,
		Scopes:       string(scopesJSON),
		RefreshToken: string(secretJSON), // encrypted at rest by the store
		Status:       "active",
		Settings:     string(settingsJSON),
	}, nil
}

// credentialRemoteParams builds the rclone config parameters for a
// credential-connected account: non-secret params from settings.rclone_params
// plus secret params from the encrypted token column. Defense in depth: the
// values are re-filtered through the provider's ParamWhitelist here, so even
// a tampered settings row can never inject unexpected rclone options.
func credentialRemoteParams(acct *store.CloudAccount) (map[string]any, error) {
	spec, ok := CredentialProviderByID(acct.Provider)
	if !ok {
		return nil, fmt.Errorf("cloud storage: %q is not a credential provider", acct.Provider)
	}
	whitelist := make(map[string]bool, len(spec.ParamWhitelist))
	for _, k := range spec.ParamWhitelist {
		whitelist[k] = true
	}

	var settings struct {
		RcloneParams map[string]string `json:"rclone_params"`
	}
	if acct.Settings != "" {
		if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
			return nil, fmt.Errorf("cloud storage: account settings: %w", err)
		}
	}
	params := make(map[string]any, len(spec.Fields))
	for key, v := range settings.RcloneParams {
		if whitelist[key] && strings.TrimSpace(v) != "" {
			params[key] = strings.TrimSpace(v)
		}
	}

	// Secrets ride in the encrypted refresh-token column (see
	// NewCredentialAccount); merge them over the plaintext set.
	if acct.RefreshToken != "" {
		var secret map[string]string
		if err := json.Unmarshal([]byte(acct.RefreshToken), &secret); err != nil {
			return nil, fmt.Errorf("cloud storage: account credentials unreadable — reconnect the account")
		}
		for key, v := range secret {
			if whitelist[key] && v != "" {
				params[key] = v
			}
		}
	}
	return params, nil
}
