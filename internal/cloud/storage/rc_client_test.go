package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRCClientURLJoin guards the rc path join: do() must always request
// <base>/<path> — a missing separator produced
// "http://127.0.0.1:36123core/version" and broke the whole storage layer.
func TestRCClientURLJoin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/core/version") {
			t.Errorf("path = %q, want /core/version prefix", r.URL.Path)
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer srv.Close()

	// Deliberately pass a base without a trailing slash (the rcd supervisor
	// constructs it as "http://127.0.0.1:<port>").
	rc := NewRCClient(srv.URL, "u", "p")
	if _, err := rc.CoreVersion(context.Background()); err != nil {
		t.Fatalf("CoreVersion: %v", err)
	}
}

// TestRCClientConfigListRemotes guards the response shape: rclone wraps the
// names in {"remotes": [...]} — a bare []string decode fails on every call.
func TestRCClientConfigListRemotes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remotes":["goclaw-abc:","goclaw-def:"]}`))
	}))
	defer srv.Close()

	rc := NewRCClient(srv.URL, "u", "p")
	remotes, err := rc.ConfigListRemotes(context.Background())
	if err != nil {
		t.Fatalf("ConfigListRemotes: %v", err)
	}
	if len(remotes) != 2 || remotes[0] != "goclaw-abc:" {
		t.Fatalf("remotes = %v, want [goclaw-abc: goclaw-def:]", remotes)
	}
}
