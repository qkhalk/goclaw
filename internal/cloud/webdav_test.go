package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestConnectWebDAVValidation(t *testing.T) {
	m := &Manager{}
	ctx := context.Background()

	cases := []struct {
		name string
		in   WebDAVConnectInput
		want string
	}{
		{"missing fields", WebDAVConnectInput{}, "endpoint, username and password are required"},
		{"missing password", WebDAVConnectInput{Endpoint: "https://x", Username: "u"}, "required"},
		{"bad scheme", WebDAVConnectInput{Endpoint: "ftp://x", Username: "u", Password: "p"}, "http(s)"},
		{"no scheme", WebDAVConnectInput{Endpoint: "cloud.example.com/dav", Username: "u", Password: "p"}, "http(s)"},
		{"no host", WebDAVConnectInput{Endpoint: "https://", Username: "u", Password: "p"}, "http(s)"},
	}
	for _, tc := range cases {
		_, err := m.ConnectWebDAV(ctx, "", "", tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

func TestWebDAVAccountCredsRoundTrip(t *testing.T) {
	settings, _ := json.Marshal(map[string]string{"endpoint": "https://cloud.example.com/dav"})
	acct := &store.CloudAccount{
		Settings:     string(settings),
		AccessToken:  "alice",
		RefreshToken: "s3cret",
	}
	creds, err := webdavAccountCreds(acct)
	if err != nil {
		t.Fatalf("creds: %v", err)
	}
	if creds.Endpoint != "https://cloud.example.com/dav" || creds.Username != "alice" || creds.Password != "s3cret" {
		t.Fatalf("creds = %+v", creds)
	}

	// Missing endpoint fails with an actionable reconnect error.
	acct.Settings = "{}"
	if _, err := webdavAccountCreds(acct); err == nil || !strings.Contains(err.Error(), "reconnect") {
		t.Fatalf("err = %v, want reconnect hint", err)
	}
}
