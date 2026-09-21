package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/version"
)

// systemUpdateIsNewer mirrors the handler's compare (v prefix tolerated).
func systemUpdateIsNewer(latest, current string) bool {
	return version.IsNewer(latest, current)
}

// ---- helpers ----

// newTestSystemUpdateHandler builds a handler pointed at a fake GitHub API.
func newTestSystemUpdateHandler(github *httptest.Server, version string) *SystemUpdateHandler {
	h := NewSystemUpdateHandler(func() string { return version })
	h.GitHubAPIBase = github.URL
	h.checkClient = github.Client()
	h.downloadClient = github.Client()
	return h
}

// fakeGitHubLatest serves a minimal /repos/:owner/:name/releases/latest.
func fakeGitHubLatest(t *testing.T, tag, assetName string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag,
			"html_url": "https://github.com/qkhalk/goclaw/releases/tag/" + tag,
			"assets": []map[string]string{
				{
					"name":                 assetName,
					"browser_download_url": "http://" + r.Host + "/assets/" + assetName,
				},
			},
		})
	}))
}

// ---- semver compare (via internal/version used by the handler) ----

func TestSystemUpdateSemverCompare(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.4", "v1.2.3", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.2", "v1.2.3", false},
		{"v1.10.0", "v1.9.0", true}, // numeric, not lexicographic
		{"v1.2.3", "dev", false},
		{"", "v1.2.3", false},
	}
	for _, tt := range tests {
		if got := systemUpdateIsNewer(tt.latest, tt.current); got != tt.want {
			t.Errorf("systemUpdateIsNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

// ---- GET /v1/system/update ----

func TestSystemUpdateCheckReturnsUpdateAvailable(t *testing.T) {
	github := fakeGitHubLatest(t, "v9.9.9", "goclaw-v9.9.9-linux-amd64.tar.gz")
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/v1/system/update", nil)
	req = req.WithContext(ownerCtx(req.Context(), "sysupdate-check-owner"))
	w := httptest.NewRecorder()
	h.handleCheck(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body systemUpdateStatus
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Current != "v1.0.0" || body.Latest != "v9.9.9" || !body.UpdateAvailable {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.URL == "" {
		t.Fatalf("expected release url in response")
	}
}

func TestSystemUpdateCheckGitHubUnreachableReturns200(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/v1/system/update", nil)
	req = req.WithContext(ownerCtx(req.Context(), "sysupdate-down-owner"))
	w := httptest.NewRecorder()
	h.handleCheck(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 on github failure, got %d: %s", w.Code, w.Body.String())
	}
	var body systemUpdateStatus
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Latest != "" || body.UpdateAvailable {
		t.Fatalf("expected empty latest + no update, got %+v", body)
	}
	if body.Error == "" {
		t.Fatalf("expected error field set on github failure")
	}
}

func TestSystemUpdateCheckCachesResult(t *testing.T) {
	calls := 0
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v2.0.0", "html_url": "https://example.com/rel"})
	}))
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/system/update", nil)
		req = req.WithContext(ownerCtx(req.Context(), "sysupdate-cache-owner"))
		w := httptest.NewRecorder()
		h.handleCheck(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: want 200, got %d", i, w.Code)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 upstream call after 3 checks (15min cache), got %d", calls)
	}
}

// ---- POST /v1/system/update/install ----

func TestSystemUpdateInstallRejectsNonLinux(t *testing.T) {
	github := fakeGitHubLatest(t, "v9.9.9", "goclaw-v9.9.9-linux-amd64.tar.gz")
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")
	h.goos, h.goarch = "windows", "amd64"

	req := httptest.NewRequest(http.MethodPost, "/v1/system/update/install", nil)
	req = req.WithContext(ownerCtx(req.Context(), "sysupdate-win-owner"))
	w := httptest.NewRecorder()
	h.handleInstall(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on non-linux, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "linux/amd64") {
		t.Fatalf("expected platform error message, got: %s", w.Body.String())
	}
}

func TestSystemUpdateInstallRejectsNonAmd64(t *testing.T) {
	github := fakeGitHubLatest(t, "v9.9.9", "goclaw-v9.9.9-linux-amd64.tar.gz")
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")
	h.goos, h.goarch = "linux", "arm64"

	req := httptest.NewRequest(http.MethodPost, "/v1/system/update/install", nil)
	req = req.WithContext(ownerCtx(req.Context(), "sysupdate-arm-owner"))
	w := httptest.NewRecorder()
	h.handleInstall(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 on non-amd64, got %d", w.Code)
	}
}

func TestSystemUpdateInstallRejectsNonMasterScope(t *testing.T) {
	github := fakeGitHubLatest(t, "v9.9.9", "goclaw-v9.9.9-linux-amd64.tar.gz")
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")

	tid := mustParseUUID("aaaabbbb-cccc-dddd-eeee-ffffaaaabbbb")
	ctx := store.WithRole(store.WithTenantID(context.Background(), tid), "admin")
	req := httptest.NewRequest(http.MethodPost, "/v1/system/update/install", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleInstall(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for non-master tenant admin, got %d", w.Code)
	}
}

func TestSystemUpdateCheckRejectsNonMasterScope(t *testing.T) {
	github := fakeGitHubLatest(t, "v9.9.9", "goclaw-v9.9.9-linux-amd64.tar.gz")
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")

	tid := mustParseUUID("aaaabbbb-cccc-dddd-eeee-ffffaaaabbbb")
	ctx := store.WithRole(store.WithTenantID(context.Background(), tid), "admin")
	req := httptest.NewRequest(http.MethodGet, "/v1/system/update", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleCheck(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for non-master tenant admin, got %d", w.Code)
	}
}

// ---- ELF sanity ----

func TestCheckELFMagicRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("this is definitely not an ELF binary, just garbage bytes"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := checkELFMagic(path)
	if err == nil {
		t.Fatal("expected error for garbage bytes, got nil")
	}
	if !strings.Contains(err.Error(), "not an ELF") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckELFMagicAcceptsELFHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	// ELF magic + rest of a plausible (but not runnable here) header.
	buf := append([]byte("\x7fELF"), make([]byte, 64)...)
	if err := os.WriteFile(path, buf, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := checkELFMagic(path); err != nil {
		t.Fatalf("expected ELF magic accepted, got: %v", err)
	}
}

func TestCheckELFMagicRejectsShortFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("\x7f"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := checkELFMagic(path); err == nil {
		t.Fatal("expected error for truncated file, got nil")
	}
}

// ---- misc helpers ----

func TestSanitizeTagForPath(t *testing.T) {
	if got := sanitizeTagForPath("v1.2.3"); got != "v1.2.3" {
		t.Errorf("sanitizeTagForPath(v1.2.3) = %q", got)
	}
	if got := sanitizeTagForPath("v1.2.3/../evil"); got != "v1.2.3..evil" {
		t.Errorf("sanitizeTagForPath traversal = %q", got)
	}
	if got := sanitizeTagForPath("../../etc"); strings.ContainsAny(got, "/\\") {
		t.Errorf("sanitizeTagForPath must strip path separators, got %q", got)
	}
	if got := sanitizeTagForPath(""); got != "unknown" {
		t.Errorf("sanitizeTagForPath(empty) = %q", got)
	}
}

// ---- routes ----

func TestSystemUpdateRegisterRoutes(t *testing.T) {
	github := fakeGitHubLatest(t, "v9.9.9", "goclaw-v9.9.9-linux-amd64.tar.gz")
	defer github.Close()
	h := newTestSystemUpdateHandler(github, "v1.0.0")

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/system/update", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// No gateway token configured (test env) → requireAuth lets the request
	// through as dev-mode auth; the route must be mounted and serve the check.
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"update_available":true`) {
		t.Fatalf("expected update_available true, got: %s", w.Body.String())
	}
}
