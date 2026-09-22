package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/mcp/installer"
)

// The installer routes must register cleanly on a mux (no pattern conflicts
// with the existing /v1/mcp/* surface) and stay auth-gated: an anonymous
// request must never reach the handlers. Pinning a gateway token disables
// the local/dev no-auth fallback so anonymous really is anonymous here.
func TestMCPInstallRoutes_AuthGated(t *testing.T) {
	old := pkgGatewayToken
	pkgGatewayToken = "test-token-mcp-install"
	t.Cleanup(func() { pkgGatewayToken = old })

	h := NewMCPInstallHandler(nil, nil, nil, nil, t.TempDir(), "v0.0.0-test")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, tc := range []struct {
		method, path string
		wantStatus   int
	}{
		{"GET", "/v1/mcp/catalog", http.StatusUnauthorized},
		{"GET", "/v1/mcp/installed", http.StatusUnauthorized},
		{"GET", "/v1/mcp/install/some-job", http.StatusUnauthorized},
		{"POST", "/v1/mcp/install", http.StatusUnauthorized},
		{"DELETE", "/v1/mcp/installed/media-probe", http.StatusUnauthorized},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.wantStatus {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.wantStatus)
		}
	}
}

// Job state snapshots must be safe copies: mutating the live job after a
// snapshot must not leak into the snapshot (polling reads copies).
func TestMCPInstallJob_SnapshotIsolation(t *testing.T) {
	j := &mcpInstallJob{state: mcpInstallJobState{ID: "j1", Status: "running"}}
	j.update(installer.StepClone, 30, "cloning")
	snap := j.snapshot()
	j.update(installer.StepSmoke, 80, "smoke")
	j.finish("done", "")
	if snap.Status != "running" || snap.Step != installer.StepClone || snap.Progress != 30 {
		t.Errorf("snapshot mutated: %+v", snap)
	}
	final := j.snapshot()
	if final.Status != "done" || final.Progress != 80 || len(final.Log) != 2 {
		t.Errorf("final state wrong: %+v", final)
	}
}
