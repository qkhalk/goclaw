package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/browse"
)

func newTestBrowseRelay(t *testing.T) (*BrowseRelayHandler, string) {
	t.Helper()
	store := browse.NewStore()
	id, err := store.Put(&browse.Entry{
		HTML:        "<html><head><title>T</title></head><body>sanitized body</body></html>",
		Title:       "T",
		FinalURL:    "https://example.com/page",
		ContentType: "text/html; charset=utf-8",
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	return NewBrowseRelayHandler(store), id
}

func TestBrowseRelayServesWithSignedToken(t *testing.T) {
	h, id := newTestBrowseRelay(t)
	path := "/v1/browse/" + id
	ft := SignFileToken(path, FileSigningKey(), FileTokenTTL)

	rec := httptest.NewRecorder()
	// Serve through the mux so path values are populated.
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?ft="+ft, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" || got != "script-src 'none'; object-src 'none'; frame-ancestors 'self'" {
		t.Fatalf("CSP = %q", got)
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Body.String() != "<html><head><title>T</title></head><body>sanitized body</body></html>" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestBrowseRelayRejectsBadToken(t *testing.T) {
	h, id := newTestBrowseRelay(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Token signed for a different path must not authorize this one.
	ft := SignFileToken("/v1/browse/other", FileSigningKey(), FileTokenTTL)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/browse/"+id+"?ft="+ft, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestBrowseRelayUnknownID(t *testing.T) {
	h, _ := newTestBrowseRelay(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	path := "/v1/browse/deadbeefdeadbeef"
	ft := SignFileToken(path, FileSigningKey(), FileTokenTTL)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?ft="+ft, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
