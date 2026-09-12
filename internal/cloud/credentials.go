package cloud

import (
	"context"
	"encoding/json"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Embedded shared OAuth clients, identical to what rclone itself ships
// (backend/drive/drive.go + backend/onedrive/onedrive.go). They are public
// credentials by design: every rclone user consents to these clients, so a
// fresh GoClaw install can connect Google Drive / OneDrive with ZERO
// configuration — the user just signs in.
//
// Trade-offs (same as rclone users have): the consent screen names "rclone",
// the shared quota is rate-limited, and rclone may rotate the secret in a
// future release. Admins who want their own branding / quota / Gmail access
// save a BYO client in the web UI (or env) — that always wins over the
// embedded defaults.
const (
	EmbeddedGoogleClientID        = "202264815644.apps.googleusercontent.com"
	EmbeddedGoogleClientSecret    = "X4Z3ca8xfWDb1Voo-F9a7ZxJ"
	EmbeddedMicrosoftClientID     = "b15665d9-eda6-4092-8539-0eec376afd59"
	EmbeddedMicrosoftClientSecret = "qtyfaBBYA403=unZUP40~_#"

	// rclone's registered loopback redirect targets (oauthutil.RedirectURL /
	// RedirectLocalhostURL). The browser lands on the user's own machine
	// where nothing is listening — the UI asks the user to paste the
	// address-bar URL back (rclone's well-known copy/paste flow).
	LoopbackRedirectGoogle   = "http://127.0.0.1:53682/"
	LoopbackRedirectMicrosoft = "http://localhost:53682/"
)

// EmbeddedGoogleScopes is the zero-config scope set. Deliberately narrower
// than the BYO set: NO Gmail scopes — rclone's verified Google client covers
// the Drive scopes, and mail tools therefore require a BYO account (see
// MailService.resolveAccount).
var EmbeddedGoogleScopes = []string{
	"openid",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/drive.readonly",
}

// providerCredentials is the resolved OAuth client for one provider plus a
// flag telling whether it is the embedded shared client.
type providerCredentials struct {
	ClientID     string
	ClientSecret string
	Embedded     bool
}

// googleCredentialsAll returns BYO credentials (web-UI secrets > env/config)
// and reports whether they exist.
func (m *Manager) googleCredentialsAll(ctx context.Context) (providerCredentials, bool) {
	id, secret := m.googleCredentials(ctx)
	if id != "" && secret != "" {
		return providerCredentials{ClientID: id, ClientSecret: secret}, true
	}
	return providerCredentials{
		ClientID:     EmbeddedGoogleClientID,
		ClientSecret: EmbeddedGoogleClientSecret,
		Embedded:     true,
	}, false
}

// microsoftCredentialsAll mirrors googleCredentialsAll for OneDrive.
func (m *Manager) microsoftCredentialsAll(ctx context.Context) (providerCredentials, bool) {
	id, secret := m.microsoftCredentials(ctx)
	if id != "" && secret != "" {
		return providerCredentials{ClientID: id, ClientSecret: secret}, true
	}
	return providerCredentials{
		ClientID:     EmbeddedMicrosoftClientID,
		ClientSecret: EmbeddedMicrosoftClientSecret,
		Embedded:     true,
	}, false
}

// credentialsForAccount resolves the OAuth client that must be used to
// refresh the account's tokens: refresh tokens are bound to the issuing
// client, so the account remembers its client_id in Settings. The same
// credentials must also be handed to rclone when it refreshes at runtime.
func (m *Manager) credentialsForAccount(ctx context.Context, acct *store.CloudAccount) providerCredentials {
	embeddedG := providerCredentials{ClientID: EmbeddedGoogleClientID, ClientSecret: EmbeddedGoogleClientSecret, Embedded: true}
	embeddedM := providerCredentials{ClientID: EmbeddedMicrosoftClientID, ClientSecret: EmbeddedMicrosoftClientSecret, Embedded: true}

	var settings struct {
		ClientID string `json:"client_id"`
	}
	if acct.Settings != "" {
		if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
			settings.ClientID = ""
		}
	}

	switch acct.Provider {
	case GoogleProvider:
		byo, isByo := m.googleCredentialsAll(ctx)
		byo.Embedded = false
		switch {
		case settings.ClientID == EmbeddedGoogleClientID:
			return embeddedG // token was issued by the shared client
		case settings.ClientID == "" && !isByo:
			return embeddedG // legacy row, no BYO configured
		case settings.ClientID == byo.ClientID, settings.ClientID == "":
			return byo
		default:
			// Unknown issuer (BYO client rotated away): the refresh token no
			// longer matches any configured client — reconnect required.
			return providerCredentials{ClientID: settings.ClientID}
		}
	case MicrosoftProvider:
		byo, isByo := m.microsoftCredentialsAll(ctx)
		byo.Embedded = false
		switch {
		case settings.ClientID == EmbeddedMicrosoftClientID:
			return embeddedM
		case settings.ClientID == "" && !isByo:
			return embeddedM
		case settings.ClientID == byo.ClientID, settings.ClientID == "":
			return byo
		default:
			return providerCredentials{ClientID: settings.ClientID}
		}
	default:
		return embeddedG
	}
}

// stampSettings merges key/value pairs into the account's settings JSON at
// connect time (client_id issuer stamp + OneDrive drive info).
func stampSettings(base string, kv map[string]string) string {
	var m map[string]string
	if base != "" {
		_ = json.Unmarshal([]byte(base), &m)
	}
	if m == nil {
		m = map[string]string{}
	}
	for k, v := range kv {
		m[k] = v
	}
	out, _ := json.Marshal(m)
	return string(out)
}
