package cloud

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- token config shape ---

func TestDropboxTokenConfigShape(t *testing.T) {
	cfg := NewDropboxTokenConfig("id", "secret", "https://cb")
	if cfg.Endpoint.AuthURL != DropboxAuthURL || cfg.Endpoint.TokenURL != DropboxTokenURL {
		t.Fatalf("dropbox endpoints = %s / %s", cfg.Endpoint.AuthURL, cfg.Endpoint.TokenURL)
	}
	if cfg.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
		t.Fatal("dropbox token endpoint needs form-style client credentials (no Basic auth)")
	}
	// The grant must cover browsing, writes and links — and must NOT grow
	// team/admin scopes by accident.
	for _, want := range []string{"files.metadata.write", "files.content.write", "files.content.read", "sharing.write", "account_info.read"} {
		if !slices.Contains(DropboxScopes, want) {
			t.Fatalf("dropbox grant missing %q", want)
		}
	}
	for _, banned := range []string{"members.read", "team_data.member", "file_requests.write"} {
		if slices.Contains(DropboxScopes, banned) {
			t.Fatalf("dropbox grant must not include %q", banned)
		}
	}
}

func TestYandexTokenConfigShape(t *testing.T) {
	cfg := NewYandexTokenConfig("id", "secret", "https://cb")
	if cfg.Endpoint.AuthURL != YandexAuthURL || cfg.Endpoint.TokenURL != YandexTokenURL {
		t.Fatalf("yandex endpoints = %s / %s", cfg.Endpoint.AuthURL, cfg.Endpoint.TokenURL)
	}
	if cfg.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
		t.Fatal("yandex token endpoint needs form-style client credentials")
	}
	// Yandex grants permissions at APP level — a scope param must never be
	// sent (Yandex rejects unknown scope values).
	if len(cfg.Scopes) != 0 {
		t.Fatalf("yandex config must carry no scopes, got %v", cfg.Scopes)
	}
}

// --- authorize URL building ---

func TestBuildDropboxAuthURL(t *testing.T) {
	m := NewManager(CloudProviderConfig{}, &fakeAccountStore{}, "test-key")
	authURL, redirectURI, mode, err := m.BuildAuthURL(context.Background(), DropboxProvider, "https://gw.example.com", "t-1", "u-1")
	if err != nil {
		t.Fatalf("BuildAuthURL(dropbox): %v", err)
	}
	if mode != "paste" || redirectURI != LoopbackRedirectDropbox {
		t.Fatalf("embedded client must use the paste-back flow, got mode=%q redirect=%q", mode, redirectURI)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("auth url: %v", err)
	}
	q := u.Query()
	// Dropbox issues NO refresh token without token_access_type=offline.
	if q.Get("token_access_type") != "offline" {
		t.Fatalf("authorize must carry token_access_type=offline, got %v", q)
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatal("authorize must carry PKCE S256")
	}
	if got := q.Get("scope"); !strings.Contains(got, "files.content.write") {
		t.Fatalf("authorize scope missing files.content.write: %q", got)
	}
	if q.Get("response_type") != "code" || q.Get("client_id") != EmbeddedDropboxClientID {
		t.Fatalf("authorize shape wrong: client_id=%q", q.Get("client_id"))
	}
}

func TestBuildYandexAuthURL(t *testing.T) {
	m := NewManager(CloudProviderConfig{}, &fakeAccountStore{}, "test-key")
	authURL, redirectURI, mode, err := m.BuildAuthURL(context.Background(), YandexProvider, "https://gw.example.com", "t-1", "u-1")
	if err != nil {
		t.Fatalf("BuildAuthURL(yandex): %v", err)
	}
	if mode != "paste" || redirectURI != LoopbackRedirectYandex {
		t.Fatalf("embedded client must use the paste-back flow, got mode=%q redirect=%q", mode, redirectURI)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("auth url: %v", err)
	}
	q := u.Query()
	// Yandex OAuth has no PKCE and no scope parameter.
	if q.Get("code_challenge") != "" || q.Get("code_challenge_method") != "" {
		t.Fatalf("yandex must not send PKCE params, got %v", q)
	}
	if q.Get("scope") != "" {
		t.Fatalf("yandex must not send a scope param, got %q", q.Get("scope"))
	}
	if q.Get("response_type") != "code" || q.Get("client_id") != EmbeddedYandexClientID {
		t.Fatalf("authorize shape wrong: client_id=%q", q.Get("client_id"))
	}
}

// --- write semantics ---

func TestAccountCanWriteDropboxYandex(t *testing.T) {
	// Dropbox: scope-based, with the scope-less-row fallback (older token
	// responses may omit the scope field — the app permission set governs).
	if !AccountCanWrite(&store.CloudAccount{Provider: DropboxProvider, Scopes: `["files.metadata.write","files.content.write"]`}) {
		t.Fatal("dropbox with the content-write scope must be writable")
	}
	if AccountCanWrite(&store.CloudAccount{Provider: DropboxProvider, Scopes: `["files.content.read","account_info.read"]`}) {
		t.Fatal("dropbox read-only grant must NOT be writable")
	}
	if !AccountCanWrite(&store.CloudAccount{Provider: DropboxProvider, Scopes: ""}) {
		t.Fatal("dropbox scope-less row must stay writable")
	}
	// Yandex: registry-based (app-level Disk permissions).
	if !AccountCanWrite(&store.CloudAccount{Provider: YandexProvider, Scopes: `["` + YandexScopesMarker + `"]`}) {
		t.Fatal("yandex must be writable")
	}
	if !AccountCanWrite(&store.CloudAccount{Provider: YandexProvider, Scopes: ""}) {
		t.Fatal("yandex with empty scopes must stay writable")
	}
}

// --- issuer binding for refresh ---

func TestCredentialsForAccountDropboxYandex(t *testing.T) {
	m := NewManager(CloudProviderConfig{}, &fakeAccountStore{}, "test-enc-key")
	m.SetSecretsStore(&fakeSecretsStore{})

	// Embedded-stamped rows keep the shared client.
	dacct := &store.CloudAccount{Provider: DropboxProvider, Settings: `{"client_id":"` + EmbeddedDropboxClientID + `"}`}
	if creds := m.credentialsForAccount(context.Background(), dacct); !creds.Embedded || creds.ClientID != EmbeddedDropboxClientID {
		t.Fatalf("embedded dropbox = %+v", creds)
	}
	yacct := &store.CloudAccount{Provider: YandexProvider, Settings: `{"client_id":"` + EmbeddedYandexClientID + `"}`}
	if creds := m.credentialsForAccount(context.Background(), yacct); !creds.Embedded || creds.ClientID != EmbeddedYandexClientID {
		t.Fatalf("embedded yandex = %+v", creds)
	}

	// A BYO save flips unstamped rows to BYO while the stamped embedded rows
	// stay on the shared client (issuer-bound refresh tokens).
	if err := m.SaveDropboxCredentials(context.Background(), "byo-db", "byo-db-secret"); err != nil {
		t.Fatalf("SaveDropboxCredentials: %v", err)
	}
	if creds := m.credentialsForAccount(context.Background(), dacct); !creds.Embedded {
		t.Fatalf("stamped dropbox must stay embedded after BYO save, got %+v", creds)
	}
	if creds := m.credentialsForAccount(context.Background(), &store.CloudAccount{Provider: DropboxProvider}); creds.Embedded || creds.ClientID != "byo-db" {
		t.Fatalf("unstamped dropbox should resolve to BYO, got %+v", creds)
	}

	if err := m.SaveYandexCredentials(context.Background(), "byo-yd", "byo-yd-secret"); err != nil {
		t.Fatalf("SaveYandexCredentials: %v", err)
	}
	if creds := m.credentialsForAccount(context.Background(), &store.CloudAccount{Provider: YandexProvider}); creds.Embedded || creds.ClientID != "byo-yd" {
		t.Fatalf("unstamped yandex should resolve to BYO, got %+v", creds)
	}

	// ProviderConfigured dispatches (embedded clients always configured).
	if !m.ProviderConfigured(context.Background(), DropboxProvider) || !m.ProviderConfigured(context.Background(), YandexProvider) {
		t.Fatal("dropbox/yandex must report configured (embedded shared client)")
	}
}
