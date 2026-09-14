package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/browse"
)

// mockBrowserInvoker stubs the gateway bridge for tool-level tests.
type mockBrowserInvoker struct {
	captured map[string]any
	reply    map[string]any
	err      error
	calls    int
}

func (m *mockBrowserInvoker) InvokeBrowserPanel(ctx context.Context, tenantID uuid.UUID, userID string, payload map[string]any, timeout time.Duration) (map[string]any, error) {
	m.calls++
	m.captured = payload
	return m.reply, m.err
}

func newTestWebBrowseTool(inv ClientBrowserInvoker) *WebBrowseTool {
	tool := NewWebBrowseTool(NewWebFetchTool(WebFetchConfig{}), browse.NewStore())
	tool.SetClientInvoker(inv)
	tool.SetRelayTokenSigner(func(path string) string { return "signed:" + path })
	return tool
}

func TestWebBrowseDelegateToClientSuccess(t *testing.T) {
	inv := &mockBrowserInvoker{reply: map[string]any{
		"content": "# extracted", "title": "Example", "finalUrl": "https://example.com/final",
	}}
	tool := newTestWebBrowseTool(inv)

	doc := fetchRawResult{
		content:     "<html><head><title>Example</title></head><body><p>text</p></body></html>",
		finalURL:    "https://example.com/final",
		contentType: "text/html",
	}
	res := tool.delegateToClient(context.Background(), doc, "https://example.com", uuid.New(), "user-1", time.Second)
	if res == nil {
		t.Fatal("delegateToClient = nil, want result")
	}
	if res["content"] != "# extracted" {
		t.Fatalf("content = %v", res["content"])
	}

	// The relayed document must be sanitized and carry a signed URL.
	relayURL, _ := inv.captured["relayUrl"].(string)
	if !strings.HasPrefix(relayURL, "/v1/browse/") || !strings.Contains(relayURL, "?ft=signed:") {
		t.Fatalf("relayUrl = %q", relayURL)
	}
	browseID, _ := inv.captured["browseId"].(string)
	entry, ok := tool.store.Get(browseID)
	if !ok {
		t.Fatal("relay store missing entry")
	}
	lower := strings.ToLower(entry.HTML)
	if strings.Contains(lower, "<script") {
		t.Fatalf("relay document not sanitized")
	}
	if entry.FinalURL != "https://example.com/final" {
		t.Fatalf("entry FinalURL = %q", entry.FinalURL)
	}
}

func TestWebBrowseDelegateReturnsNilOnFailure(t *testing.T) {
	inv := &mockBrowserInvoker{err: context.DeadlineExceeded}
	tool := newTestWebBrowseTool(inv)

	doc := fetchRawResult{content: "<html><body><p>text</p></body></html>", finalURL: "https://example.com"}
	if res := tool.delegateToClient(context.Background(), doc, "https://example.com", uuid.New(), "u", time.Millisecond); res != nil {
		t.Fatalf("delegateToClient = %v, want nil on error", res)
	}

	// Client reachable but empty content also declines (falls back).
	inv2 := &mockBrowserInvoker{reply: map[string]any{"content": "   "}}
	tool2 := newTestWebBrowseTool(inv2)
	if res := tool2.delegateToClient(context.Background(), doc, "https://example.com", uuid.New(), "u", time.Second); res != nil {
		t.Fatalf("delegateToClient = %v, want nil on empty content", res)
	}
}

func TestWebBrowseExecuteValidation(t *testing.T) {
	tool := newTestWebBrowseTool(&mockBrowserInvoker{})
	if r := tool.Execute(context.Background(), map[string]any{}); r == nil || !strings.Contains(r.ForLLM, "url is required") {
		t.Fatalf("missing url: %+v", r)
	}
	if r := tool.Execute(context.Background(), map[string]any{"url": "ftp://example.com"}); r == nil || !strings.Contains(r.ForLLM, "only http and https") {
		t.Fatalf("bad scheme: %+v", r)
	}
	// SSRF: loopback literal IP must be rejected before any fetch.
	if r := tool.Execute(context.Background(), map[string]any{"url": "http://127.0.0.1:9600/"}); r == nil || !strings.Contains(r.ForLLM, "SSRF") {
		t.Fatalf("ssrf: %+v", r)
	}
}

func TestWebBrowseFallbackRouting(t *testing.T) {
	htmlDoc := fetchRawResult{
		content:     "<html><body><article><h1>H</h1><p>Body text</p></article></body></html>",
		contentType: "text/html; charset=utf-8",
	}
	if got := extractDocumentText(htmlDoc); !strings.Contains(got, "Body text") {
		t.Fatalf("html routing lost content: %q", got)
	}

	jsonDoc := fetchRawResult{content: `{"a":1}`, contentType: "application/json"}
	if got := extractDocumentText(jsonDoc); !strings.Contains(got, `"a": 1`) {
		t.Fatalf("json routing: %q", got)
	}

	emptyDoc := fetchRawResult{content: "<html><body></body></html>", contentType: "text/html"}
	if got := extractDocumentText(emptyDoc); !strings.Contains(got, "No content extracted") {
		t.Fatalf("empty html hint missing: %q", got)
	}
}

func TestWebBrowseFormatResult(t *testing.T) {
	out := formatBrowseResult("hello", "T", "https://example.com", 10, true)
	if !strings.Contains(out, "URL: https://example.com") || !strings.Contains(out, "user-browser") {
		t.Fatalf("formatBrowseResult missing metadata:\n%s", out)
	}
	long := formatBrowseResult(strings.Repeat("x", 50), "T", "u", 10, false)
	if !strings.Contains(long, "[... truncated]") {
		t.Fatal("truncation marker missing")
	}
}
