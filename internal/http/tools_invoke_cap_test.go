package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// echoTool is a minimal read-only Tool for exercising the invoke handler
// without touching exec/network surfaces.
type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echoes its input" }
func (echoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (echoTool) Execute(_ context.Context, args map[string]any) *tools.Result {
	out, _ := json.Marshal(args)
	return &tools.Result{ForLLM: string(out), ForUser: string(out)}
}

// invokeCapHarness builds a handler with a registry containing the echo tool,
// authenticates via the gateway token (set once per test), and returns
// send(body) -> recorder.
func invokeCapHarness(t *testing.T, maxBody int64) func(string) *httptest.ResponseRecorder {
	t.Helper()
	if pkgGatewayToken == "" {
		InitGatewayToken("test-gw-token")
		t.Cleanup(func() { InitGatewayToken("") })
	}
	reg := tools.NewRegistry()
	reg.Register(echoTool{})

	h := NewToolsInvokeHandler(reg, nil)
	h.SetMaxBodyBytes(maxBody)

	return func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/tools/invoke", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-gw-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
}

// A1: a body over the configured cap must be rejected before the tool runs.
func TestToolsInvoke_BodyOverConfiguredCapRejected(t *testing.T) {
	send := invokeCapHarness(t, 64)

	// Valid JSON with tool name — would execute if the cap were not enforced.
	body := `{"tool":"echo","args":{"x":"` + strings.Repeat("a", 200) + `"}}`
	rec := send(body)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body over 64-byte cap: got %d (%s), want 413", rec.Code, rec.Body.String())
	}
}

// A1: default cap (1 MiB) applies when no override is set — an oversized body
// is rejected even though the JSON is valid and the tool exists.
func TestToolsInvoke_BodyOverDefaultCapRejected(t *testing.T) {
	send := invokeCapHarness(t, 0) // no override: DefaultInvokeMaxBodyBytes

	big := strings.Repeat("a", DefaultInvokeMaxBodyBytes+1024)
	rec := send(`{"tool":"echo","args":{"pad":"` + big + `"}}`)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body over default 1 MiB cap: got %d (%s), want 413", rec.Code, rec.Body.String())
	}
}

// Under-cap bodies still reach the handler normally (no false positives from
// the MaxBytesReader wrap).
func TestToolsInvoke_BodyUnderCapPasses(t *testing.T) {
	send := invokeCapHarness(t, 4096)

	rec := send(`{"tool":"echo","dry_run":true}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("under-cap dry-run invoke: got %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp["tool"] != "echo" {
		t.Fatalf("dry-run response missing tool field: %v", resp)
	}
}

// Non-positive overrides are ignored — the guard cannot be disabled by config.
func TestToolsInvoke_SetMaxBodyBytes_IgnoresNonPositive(t *testing.T) {
	h := NewToolsInvokeHandler(nil, nil)
	h.SetMaxBodyBytes(0)
	h.SetMaxBodyBytes(-1)
	if h.maxBodyBytes != 0 {
		t.Fatalf("maxBodyBytes = %d, want 0 (unset)", h.maxBodyBytes)
	}
}
