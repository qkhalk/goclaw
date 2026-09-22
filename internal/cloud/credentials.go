package cloud

import (
	"context"
	"encoding/json"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Embedded shared OAuth clients, identical to what rclone itself ships
// (backend/drive/drive.go, backend/onedrive/onedrive.go,
// backend/dropbox/dropbox.go, backend/yandex/yandex.go). They are public
// credentials by design: every rclone user consents to these clients, so a
// fresh GoClaw install can connect Google Drive / OneDrive / Dropbox /
// Yandex Disk with ZERO configuration — the user just signs in.
//
// Trade-offs (same as rclone users have): the consent screen names "rclone",
// the shared quota is rate-limited (Dropbox most aggressively), and rclone
// may rotate the secret in a future release. Admins who want their own
// branding / quota / Gmail access save a BYO client in the web UI (or env) —
// that always wins over the embedded defaults.
const (
	EmbeddedGoogleClientID        = "202264815644.apps.googleusercontent.com"
	EmbeddedGoogleClientSecret    = "X4Z3ca8xfWDb1Voo-F9a7ZxJ"
	EmbeddedMicrosoftClientID     = "b15665d9-eda6-4092-8539-0eec376afd59"
	EmbeddedMicrosoftClientSecret = "qtyfaBBYA403=unZUP40~_#"
	EmbeddedDropboxClientID       = "5jcck7diasz0rqy"
	EmbeddedDropboxClientSecret   = "1n9m04y2zx7bf26"
	EmbeddedYandexClientID        = "ac39b43b9eba4cae8ffb788c06d816a8"
	EmbeddedYandexClientSecret    = "9cbd7c4b4d8a4ddd848007c86a3bd67e"

	// rclone's registered loopback redirect targets (oauthutil.RedirectURL /
	// RedirectLocalhostURL). The browser lands on the user's own machine
	// where nothing is listening — the UI asks the user to paste the
	// address-bar URL back (rclone's well-known copy/paste flow). Dropbox and
	// Yandex use the same 127.0.0.1:53682 target rclone registers for its
	// shared apps; Microsoft's app is pinned to the localhost spelling.
	LoopbackRedirectGoogle   = "http://127.0.0.1:53682/"
	LoopbackRedirectMicrosoft = "http://localhost:53682/"
	LoopbackRedirectDropbox  = "http://127.0.0.1:53682/"
	LoopbackRedirectYandex   = "http://127.0.0.1:53682/"
)

// EmbeddedGoogleScopes is the zero-config scope set. Deliberately narrower
// than the BYO set: NO Gmail scopes — rclone's verified Google client covers
// the Drive scopes, and mail tools therefore require a BYO account (see
// MailService.resolveAccount). Drive is the full scope (matches rclone's own
// default) so embedded accounts can write, not just read.
var EmbeddedGoogleScopes = []string{
	"openid",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/drive",
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

// dropboxCredentialsAll mirrors googleCredentialsAll for Dropbox.
func (m *Manager) dropboxCredentialsAll(ctx context.Context) (providerCredentials, bool) {
	id, secret := m.dropboxCredentials(ctx)
	if id != "" && secret != "" {
		return providerCredentials{ClientID: id, ClientSecret: secret}, true
	}
	return providerCredentials{
		ClientID:     EmbeddedDropboxClientID,
		ClientSecret: EmbeddedDropboxClientSecret,
		Embedded:     true,
	}, false
}

// yandexCredentialsAll mirrors googleCredentialsAll for Yandex Disk.
func (m *Manager) yandexCredentialsAll(ctx context.Context) (providerCredentials, bool) {
	id, secret := m.yandexCredentials(ctx)
	if id != "" && secret != "" {
		return providerCredentials{ClientID: id, ClientSecret: secret}, true
	}
	return providerCredentials{
		ClientID:     EmbeddedYandexClientID,
		ClientSecret: EmbeddedYandexClientSecret,
		Embedded:     true,
	}, false
}

// embeddedClientFor returns the shared client of one provider (used by
// credentialsForAccount below).
func embeddedClientFor(provider string) providerCredentials {
	switch provider {
	case GoogleProvider:
		return providerCredentials{ClientID: EmbeddedGoogleClientID, ClientSecret: EmbeddedGoogleClientSecret, Embedded: true}
	case MicrosoftProvider:
		return providerCredentials{ClientID: EmbeddedMicrosoftClientID, ClientSecret: EmbeddedMicrosoftClientSecret, Embedded: true}
	case DropboxProvider:
		return providerCredentials{ClientID: EmbeddedDropboxClientID, ClientSecret: EmbeddedDropboxClientSecret, Embedded: true}
	case YandexProvider:
		return providerCredentials{ClientID: EmbeddedYandexClientID, ClientSecret: EmbeddedYandexClientSecret, Embedded: true}
	}
	return providerCredentials{}
}

// issuerCredentials resolves the OAuth client that must be used for an
// account's token refresh: refresh tokens are bound to the issuing client.
//   - the row was stamped with the shared client → embedded client (BYO never
//     applies to tokens it did not issue);
//   - unstamped row with no BYO configured → embedded client (legacy row);
//   - otherwise the current BYO client (or the unstamped row's grant when BYO
//     exists);
//   - an unknown issuer (BYO client rotated away) → the stamped ID with no
//     secret: refresh fails loudly → reconnect required, never a silent
//     downgrade.
func issuerCredentials(settingsClientID string, byo providerCredentials, byoExists bool, embedded providerCredentials) providerCredentials {
	switch {
	case settingsClientID == embedded.ClientID:
		return embedded // token was issued by the shared client
	case settingsClientID == "" && !byoExists:
		return embedded // legacy row, no BYO configured
	case settingsClientID == byo.ClientID, settingsClientID == "":
		return byo
	default:
		return providerCredentials{ClientID: settingsClientID}
	}
}

// credentialsForAccount resolves the OAuth client that must be used to
// refresh the account's tokens (see issuerCredentials). The same credentials
// must also be handed to rclone when it refreshes at runtime.
func (m *Manager) credentialsForAccount(ctx context.Context, acct *store.CloudAccount) providerCredentials {
	embedded := embeddedClientFor(acct.Provider)
	if embedded.ClientID == "" {
		return embedded
	}

	var settings struct {
		ClientID string `json:"client_id"`
	}
	if acct.Settings != "" {
		if err := json.Unmarshal([]byte(acct.Settings), &settings); err != nil {
			settings.ClientID = ""
		}
	}

	var byo providerCredentials
	var byoExists bool
	switch acct.Provider {
	case GoogleProvider:
		byo, byoExists = m.googleCredentialsAll(ctx)
	case MicrosoftProvider:
		byo, byoExists = m.microsoftCredentialsAll(ctx)
	case DropboxProvider:
		byo, byoExists = m.dropboxCredentialsAll(ctx)
	case YandexProvider:
		byo, byoExists = m.yandexCredentialsAll(ctx)
	}
	byo.Embedded = false
	return issuerCredentials(settings.ClientID, byo, byoExists, embedded)
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
