package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeGatewayUpgradeRunner struct {
	tags []string
	err  error
}

func (r *fakeGatewayUpgradeRunner) Start(tag string) error {
	r.tags = append(r.tags, tag)
	return r.err
}

func TestValidGatewayUpgradeTag(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{"latest", true},
		{"v3.12.0", true},
		{"v3.12.0-beta.1", true},
		{"v3.12.0-rc.2", true},
		{"", false},
		{"3.12.0", false},
		{"v3.12", false},
		{"v3.12.0-beta", false},
		{"https://example.com/goclaw.tar.gz", false},
		{"v3.12.0;reboot", false},
		{"../v3.12.0", false},
		{"v3.12.0 linux", false},
	}
	for _, tt := range tests {
		if got := validGatewayUpgradeTag(tt.tag); got != tt.want {
			t.Fatalf("validGatewayUpgradeTag(%q) = %v, want %v", tt.tag, got, tt.want)
		}
	}
}

func TestGatewayUpgradeStatusMissingReturnsIdle(t *testing.T) {
	h := &GatewayUpgradeHandler{StatusPath: filepath.Join(t.TempDir(), "missing.json"), TriggerToken: "secret-token"}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/status", nil)
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(ownerCtx(req.Context(), "gateway-status-owner"))
	w := httptest.NewRecorder()

	h.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["state"] != "idle" {
		t.Fatalf("state = %v, want idle", body["state"])
	}
}

func TestGatewayUpgradeStartAcceptsValidTag(t *testing.T) {
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{
		StatusPath:   filepath.Join(t.TempDir(), "status.json"),
		TriggerToken: "secret-token",
		Runner:       runner,
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"v3.12.0"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(ownerCtx(req.Context(), "gateway-start-owner"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 1 || runner.tags[0] != "v3.12.0" {
		t.Fatalf("runner tags = %#v, want [v3.12.0]", runner.tags)
	}
}

func TestGatewayUpgradeStartRejectsInvalidTag(t *testing.T) {
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{
		StatusPath:   filepath.Join(t.TempDir(), "status.json"),
		TriggerToken: "secret-token",
		Runner:       runner,
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"https://example.com/x"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(ownerCtx(req.Context(), "gateway-invalid-owner"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 0 {
		t.Fatalf("runner should not be called, got %#v", runner.tags)
	}
}

func TestGatewayUpgradeStartRejectsRunningJob(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	if err := os.WriteFile(statusPath, []byte(`{"state":"running"}`), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{StatusPath: statusPath, TriggerToken: "secret-token", Runner: runner}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"latest"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(ownerCtx(req.Context(), "gateway-running-owner"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 0 {
		t.Fatalf("runner should not be called, got %#v", runner.tags)
	}
}

func TestGatewayUpgradeStartAllowsStaleRunningJob(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	staleStartedAt := time.Now().UTC().Add(-gatewayUpgradeRunningMaxAge - time.Minute).Format(time.RFC3339)
	if err := os.WriteFile(statusPath, []byte(`{"state":"running","startedAt":"`+staleStartedAt+`"}`), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{StatusPath: statusPath, TriggerToken: "secret-token", Runner: runner}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"latest"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(ownerCtx(req.Context(), "gateway-stale-running-owner"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 1 || runner.tags[0] != "latest" {
		t.Fatalf("runner tags = %#v, want [latest]", runner.tags)
	}
}

func TestGatewayUpgradeTriggerTokenGuard(t *testing.T) {
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{
		StatusPath:   filepath.Join(t.TempDir(), "status.json"),
		TriggerToken: "secret-token",
		Runner:       runner,
	}
	// Non-owner with a WRONG token is rejected even when a token is configured.
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"latest"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "wrong-token")
	req = req.WithContext(tenantAdminCtx(req.Context(), "gateway-tenant-admin"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 0 {
		t.Fatalf("runner should not be called, got %#v", runner.tags)
	}
}

func TestGatewayUpgradeOwnerSessionTriggersWithoutToken(t *testing.T) {
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{
		StatusPath:   filepath.Join(t.TempDir(), "status.json"),
		TriggerToken: "", // no automation token configured
		Runner:       runner,
	}
	// The web UI flow: an owner session triggers the upgrade directly.
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"v4.9.0"}`))
	req = req.WithContext(ownerCtx(req.Context(), "gateway-ui-owner"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 1 || runner.tags[0] != "v4.9.0" {
		t.Fatalf("runner tags = %#v, want [v4.9.0]", runner.tags)
	}
}

func TestGatewayUpgradeRejectsNonMasterScope(t *testing.T) {
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{
		StatusPath:   filepath.Join(t.TempDir(), "status.json"),
		TriggerToken: "secret-token",
		Runner:       runner,
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"latest"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(tenantAdminCtx(req.Context(), "gateway-tenant-admin"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 0 {
		t.Fatalf("runner should not be called, got %#v", runner.tags)
	}
}

func TestGatewayUpgradeStartRunnerError(t *testing.T) {
	h := &GatewayUpgradeHandler{
		StatusPath:   filepath.Join(t.TempDir(), "status.json"),
		TriggerToken: "secret-token",
		Runner:       &fakeGatewayUpgradeRunner{err: errors.New("boom")},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"latest"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(ownerCtx(req.Context(), "gateway-runner-error-owner"))
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGatewayUpgradeFailsClosedWithoutConfiguredTriggerToken(t *testing.T) {
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{
		StatusPath: filepath.Join(t.TempDir(), "status.json"),
		Runner:     runner,
	}
	// Master scope (uuid.Nil tenant) but NOT owner — the token policy is what
	// must reject this caller (503 when unconfigured).
	masterAdminCtx := store.WithRole(store.WithTenantID(context.Background(), uuid.Nil), "admin")
	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"latest"}`))
	req = req.WithContext(masterAdminCtx)
	w := httptest.NewRecorder()

	h.handleStart(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 0 {
		t.Fatalf("runner should not be called, got %#v", runner.tags)
	}
}

func TestGatewayUpgradeRegisterRoutes(t *testing.T) {
	runner := &fakeGatewayUpgradeRunner{}
	h := &GatewayUpgradeHandler{
		StatusPath:   filepath.Join(t.TempDir(), "status.json"),
		TriggerToken: "secret-token",
		Runner:       runner,
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/system/gateway/upgrade", bytes.NewBufferString(`{"tag":"latest"}`))
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", w.Code, w.Body.String())
	}
	if len(runner.tags) != 1 || runner.tags[0] != "latest" {
		t.Fatalf("runner tags = %#v, want [latest]", runner.tags)
	}
}

func TestNewGatewayUpgradeHandlerFromEnvDefaults(t *testing.T) {
	t.Setenv("GOCLAW_UPGRADE_SCRIPT", "")
	t.Setenv("GOCLAW_UPGRADE_STATUS_PATH", "")
	t.Setenv("GOCLAW_UPGRADE_TRIGGER_TOKEN", "secret-token")

	h := NewGatewayUpgradeHandlerFromEnv()

	if h.ScriptPath != defaultGatewayUpgradeScript {
		t.Fatalf("ScriptPath = %q, want %q", h.ScriptPath, defaultGatewayUpgradeScript)
	}
	if h.StatusPath != defaultGatewayUpgradeStatus {
		t.Fatalf("StatusPath = %q, want %q", h.StatusPath, defaultGatewayUpgradeStatus)
	}
	if h.TriggerToken != "secret-token" {
		t.Fatalf("TriggerToken was not loaded from env")
	}
}

func TestGatewayUpgradeStatusRejectsInvalidJSON(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(statusPath, []byte(`{bad json`), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
	h := &GatewayUpgradeHandler{StatusPath: statusPath, TriggerToken: "secret-token"}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/status", nil)
	req.Header.Set(gatewayUpgradeTokenHeader, "secret-token")
	req = req.WithContext(ownerCtx(req.Context(), "gateway-invalid-status-owner"))
	w := httptest.NewRecorder()

	h.handleStatus(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), statusPath) || strings.Contains(w.Body.String(), "decode upgrade status") {
		t.Fatalf("response leaked internal status details: %s", w.Body.String())
	}
}

// --- GET /v1/system/gateway/upgrade/check ---

// stubReleaseTransport serves canned GitHub release listings for the check
// endpoint without touching the network.
type stubReleaseTransport struct {
	body string
}

func (s stubReleaseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

func withStubReleases(t *testing.T, body string) {
	t.Helper()
	orig := upgradeCheckClient
	upgradeCheckClient = &http.Client{Transport: stubReleaseTransport{body: body}}
	t.Cleanup(func() { upgradeCheckClient = orig })
}

const twoStableReleasesJSON = `[
  {"tag_name":"v4.9.0","html_url":"https://github.com/qkhalk/goclaw/releases/tag/v4.9.0","draft":false,"prerelease":false,
   "assets":[{"name":"goclaw-v4.9.0-linux-amd64.tar.gz","browser_download_url":"https://x/1"},
             {"name":"goclaw-v4.9.0-windows-amd64.zip","browser_download_url":"https://x/2"}]},
  {"tag_name":"v4.8.0","html_url":"https://github.com/qkhalk/goclaw/releases/tag/v4.8.0","draft":false,"prerelease":false,
   "assets":[{"name":"goclaw-v4.8.0-linux-amd64.tar.gz","browser_download_url":"https://x/3"}]}
]`

func TestUpgradeCheckUpdateAvailable(t *testing.T) {
	withStubReleases(t, twoStableReleasesJSON)
	h := &GatewayUpgradeHandler{Repo: "qkhalk/goclaw", Version: "4.6.2+444"}

	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/check", nil)
	req = req.WithContext(ownerCtx(req.Context(), "check-owner"))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	wantAsset := "goclaw-v4.9.0-linux-amd64.tar.gz"
	if runtime.GOOS == "windows" {
		wantAsset = "goclaw-v4.9.0-windows-amd64.zip"
	}
	for _, want := range []string{`"available":true`, `"latest":"v4.9.0"`, wantAsset} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s:\n%s", want, body)
		}
	}
}

func TestUpgradeCheckBuildMetadataStillDetectsNewer(t *testing.T) {
	withStubReleases(t, twoStableReleasesJSON)
	// "4.6.2+444" must compare as 4.6.2 — not 4.6.0, which would also be
	// newer than every release and break the IsNewer short-circuit.
	h := &GatewayUpgradeHandler{Repo: "qkhalk/goclaw", Version: "4.6.2+444"}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/check", nil)
	req = req.WithContext(ownerCtx(req.Context(), "check-owner"))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), `"latest":"v4.9.0"`) {
		t.Errorf("build metadata mishandled: %s", w.Body.String())
	}
}

func TestUpgradeCheckUpToDate(t *testing.T) {
	withStubReleases(t, twoStableReleasesJSON)
	h := &GatewayUpgradeHandler{Repo: "qkhalk/goclaw", Version: "v4.9.0"}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/check", nil)
	req = req.WithContext(ownerCtx(req.Context(), "check-owner"))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), `"available":false`) {
		t.Errorf("expected available=false, got %s", w.Body.String())
	}
}

func TestUpgradeCheckSkipsPrereleaseAndForkTags(t *testing.T) {
	withStubReleases(t, `[
      {"tag_name":"v4.10.0-beta.1","draft":false,"prerelease":true,"assets":[]},
      {"tag_name":"v4.9.0-fork.2","draft":false,"prerelease":false,"assets":[]},
      {"tag_name":"v4.8.0","draft":false,"prerelease":false,
       "assets":[{"name":"goclaw-v4.8.0-linux-amd64.tar.gz","browser_download_url":"https://x/3"}]}
    ]`)
	h := &GatewayUpgradeHandler{Repo: "qkhalk/goclaw", Version: "4.6.2"}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/check", nil)
	req = req.WithContext(ownerCtx(req.Context(), "check-owner"))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), `"latest":"v4.8.0"`) {
		t.Errorf("prerelease/fork tags must be skipped: %s", w.Body.String())
	}
}

func TestUpgradeCheckNoMatchingAsset(t *testing.T) {
	withStubReleases(t, `[
      {"tag_name":"v4.9.0","draft":false,"prerelease":false,
       "assets":[{"name":"goclaw-v4.9.0-solaris-amd64.tar.gz","browser_download_url":"https://x/2"}]}
    ]`)
	h := &GatewayUpgradeHandler{Repo: "qkhalk/goclaw", Version: "4.6.2"}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/check", nil)
	req = req.WithContext(ownerCtx(req.Context(), "check-owner"))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"available":false`) ||
		!strings.Contains(body, fmt.Sprintf("no %s/%s binary archive", runtime.GOOS, runtime.GOARCH)) {
		t.Errorf("missing-asset case not surfaced: %s", body)
	}
}

func TestUpgradeCheckUnknownVersion(t *testing.T) {
	h := &GatewayUpgradeHandler{Repo: "qkhalk/goclaw", Version: "dev"}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/check", nil)
	req = req.WithContext(ownerCtx(req.Context(), "check-owner"))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"available":false`) {
		t.Errorf("dev version: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUpgradeCheckOwnerRoleAllowedByTokenPolicy(t *testing.T) {
	// The check is read-only — an owner session passes the token policy even
	// with no automation token configured.
	h := &GatewayUpgradeHandler{Repo: "qkhalk/goclaw", Version: "dev", TriggerToken: ""}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/gateway/upgrade/check", nil)
	req = req.WithContext(ownerCtx(req.Context(), "check-owner"))
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden || w.Code == http.StatusServiceUnavailable {
		t.Fatalf("owner check blocked by token policy: %d %s", w.Code, w.Body.String())
	}
}
