package cloud

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestParseRedirectedURL(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		code  string
		state string
		fails bool
	}{
		{
			name:  "google loopback query",
			raw:   "http://127.0.0.1:53682/?state=abc-123&code=xyz_code&scope=drive.readonly",
			code:  "xyz_code",
			state: "abc-123",
		},
		{
			name:  "microsoft localhost query",
			raw:   "http://localhost:53682/?code=ms_code&state=s2#",
			code:  "ms_code",
			state: "s2",
		},
		{
			name:  "fragment form",
			raw:   "http://localhost:53682/#code=frag_code&state=s3",
			code:  "frag_code",
			state: "s3",
		},
		{name: "whitespace wrapped", raw: "  http://127.0.0.1/?code=c&state=s\n", code: "c", state: "s"},
		{name: "missing code", raw: "http://127.0.0.1/?state=s", fails: true},
		{name: "missing state", raw: "http://127.0.0.1/?code=c", fails: true},
		{name: "not a url", raw: "garbage", fails: true},
		{name: "empty", raw: "   ", fails: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, state, err := ParseRedirectedURL(tc.raw)
			if tc.fails {
				if err == nil {
					t.Fatalf("expected error, got code=%q state=%q", code, state)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tc.code || state != tc.state {
				t.Fatalf("code/state = %q/%q, want %q/%q", code, state, tc.code, tc.state)
			}
		})
	}
}

func TestCredentialsForAccountEmbeddedVsByo(t *testing.T) {
	m := NewManager(CloudProviderConfig{}, &fakeAccountStore{}, "test-enc-key")
	m.SetSecretsStore(&fakeSecretsStore{})

	// No BYO anywhere: every account refreshes with the embedded shared client.
	acct := &store.CloudAccount{Provider: GoogleProvider, Settings: `{"client_id":"` + EmbeddedGoogleClientID + `"}`}
	if creds := m.credentialsForAccount(context.Background(), acct); !creds.Embedded || creds.ClientID != EmbeddedGoogleClientID {
		t.Fatalf("embedded google = %+v", creds)
	}

	// A legacy Google row without the stamp: no BYO → embedded.
	legacy := &store.CloudAccount{Provider: GoogleProvider}
	if creds := m.credentialsForAccount(context.Background(), legacy); !creds.Embedded {
		t.Fatalf("legacy google should fall back to embedded, got %+v", creds)
	}

	// Save a BYO Google client: the stamped account keeps the embedded client
	// (refresh tokens are issuer-bound), new resolution returns BYO.
	if err := m.SaveGoogleCredentials(context.Background(), "byo-id", "byo-secret"); err != nil {
		t.Fatalf("SaveGoogleCredentials: %v", err)
	}
	if creds := m.credentialsForAccount(context.Background(), acct); !creds.Embedded || creds.ClientID != EmbeddedGoogleClientID {
		t.Fatalf("stamped embedded account must stay on the embedded client even after BYO save, got %+v", creds)
	}
	// New accounts (no stamp) resolve to the BYO client now that it exists.
	if creds := m.credentialsForAccount(context.Background(), legacy); creds.Embedded || creds.ClientID != "byo-id" {
		t.Fatalf("legacy account without stamp should resolve to BYO, got %+v", creds)
	}
	if _, byo := m.googleCredentialsAll(context.Background()); !byo {
		t.Fatal("googleCredentialsAll should report BYO after save")
	}

	// OneDrive: same issuer-binding semantics.
	oacct := &store.CloudAccount{Provider: MicrosoftProvider, Settings: `{"client_id":"` + EmbeddedMicrosoftClientID + `","drive_id":"d1"}`}
	if creds := m.credentialsForAccount(context.Background(), oacct); !creds.Embedded || creds.ClientID != EmbeddedMicrosoftClientID {
		t.Fatalf("embedded onedrive = %+v", creds)
	}
}
