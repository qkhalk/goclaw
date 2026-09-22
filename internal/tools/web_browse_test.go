package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/browse"
	"github.com/nextlevelbuilder/goclaw/internal/store"
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
	out := formatBrowseResult("hello", "T", "https://example.com", 10, true, false)
	if !strings.Contains(out, "URL: https://example.com") || !strings.Contains(out, "user-browser") {
		t.Fatalf("formatBrowseResult missing metadata:\n%s", out)
	}
	long := formatBrowseResult(strings.Repeat("x", 50), "T", "u", 10, false, false)
	if !strings.Contains(long, "[... truncated]") {
		t.Fatal("truncation marker missing")
	}
	// Refs hint appears only when requested and the content carries [eN] tags.
	withRefs := formatBrowseResult("link [e3] here", "T", "u", 5000, true, true)
	if !strings.Contains(withRefs, "action") {
		t.Fatal("refs hint missing for tagged content")
	}
	noTags := formatBrowseResult("plain", "T", "u", 5000, true, true)
	if strings.Contains(noTags, "action") {
		t.Fatal("refs hint leaked for untagged content")
	}
}

func TestWebBrowseActionRouter(t *testing.T) {
	inv := &mockBrowserInvoker{reply: map[string]any{
		"content":  "page [e1] [e2]",
		"title":    "T",
		"finalUrl": "https://example.com/x",
	}}
	tool := newTestWebBrowseTool(inv)
	ctx := store.WithTenantID(WithToolChannel(WithToolChatID(context.Background(), "user-1"), ChannelWeb), uuid.New())

	// Unknown action.
	if r := tool.Execute(ctx, map[string]any{"action": "sniff"}); r == nil || !strings.Contains(r.ForLLM, "unknown action") {
		t.Fatalf("unknown action: %+v", r)
	}
	// click/type without ref.
	if r := tool.Execute(ctx, map[string]any{"action": "click"}); r == nil || !strings.Contains(r.ForLLM, "requires ref") {
		t.Fatalf("click no ref: %+v", r)
	}
	if r := tool.Execute(ctx, map[string]any{"action": "type", "ref": "e1"}); r == nil || !strings.Contains(r.ForLLM, "requires text") {
		t.Fatalf("type no text: %+v", r)
	}
	// Valid type action reaches the invoker with action/ref/text payload.
	r := tool.Execute(ctx, map[string]any{"action": "type", "ref": "e1", "text": "hello world"})
	if r == nil || r.IsError {
		t.Fatalf("type action failed: %+v", r)
	}
	if inv.calls != 1 {
		t.Fatalf("invoker calls = %d, want 1", inv.calls)
	}
	if inv.captured["action"] != "type" || inv.captured["ref"] != "e1" || inv.captured["text"] != "hello world" {
		t.Fatalf("payload mismatch: %+v", inv.captured)
	}
	if !strings.Contains(r.ForLLM, "page [e1] [e2]") {
		t.Fatalf("content missing from result: %s", r.ForLLM)
	}
	// Actions require the web channel.
	telegramCtx := WithToolChannel(context.Background(), "telegram")
	if r := tool.Execute(telegramCtx, map[string]any{"action": "extract"}); r == nil || !strings.Contains(r.ForLLM, "requires the user's browser panel") {
		t.Fatalf("extract on telegram: %+v", r)
	}
}

// mockPageRenderer stubs the headless render fallback for unit tests.
type mockPageRenderer struct {
	result *PageRenderResult
	err    error
	calls  int
}

func (m *mockPageRenderer) RenderHTML(ctx context.Context, rawURL string) (*PageRenderResult, error) {
	m.calls++
	return m.result, m.err
}

func TestWebBrowseRelayNeedsRender(t *testing.T) {
	shell := fetchRawResult{
		content:     "<html><head><title>9Router</title></head><body><div>Loading...</div></body></html>",
		contentType: "text/html; charset=utf-8",
		statusCode:  200,
	}
	if !relayNeedsRender(shell) {
		t.Fatal("JS-only shell should need render")
	}
	rich := fetchRawResult{
		content:     "<html><body>" + strings.Repeat("<p>meaningful body text here</p>", 20) + "</body></html>",
		contentType: "text/html",
		statusCode:  200,
	}
	if relayNeedsRender(rich) {
		t.Fatal("rich static page should not need render")
	}
	challenge := fetchRawResult{content: "<html></html>", contentType: "text/html", statusCode: 403}
	if !relayNeedsRender(challenge) {
		t.Fatal("403 challenge should need render")
	}
	rateLimited := fetchRawResult{content: "", contentType: "text/html", statusCode: 429}
	if !relayNeedsRender(rateLimited) {
		t.Fatal("429 should need render")
	}
	serverError := fetchRawResult{content: "", contentType: "text/html", statusCode: 503}
	if !relayNeedsRender(serverError) {
		t.Fatal("503 should need render")
	}
	notFound := fetchRawResult{content: "<html><body>page not found</body></html>", contentType: "text/html", statusCode: 404}
	if relayNeedsRender(notFound) {
		t.Fatal("404 should relay as-is")
	}
	jsonDoc := fetchRawResult{content: `{"ok":true}`, contentType: "application/json", statusCode: 200}
	if relayNeedsRender(jsonDoc) {
		t.Fatal("JSON documents never need render")
	}
}

func TestWebBrowseMaybeRender(t *testing.T) {
	shell := fetchRawResult{
		content:     "<html><body><div>Loading...</div></body></html>",
		contentType: "text/html",
		finalURL:    "https://example.com/login",
		statusCode:  200,
	}
	rendered := &PageRenderResult{
		HTML:     "<html><body>" + strings.Repeat("<p>rendered login form text</p>", 30) + "</body></html>",
		FinalURL: "https://example.com/login",
		Title:    "Sign in",
	}

	// No renderer wired: doc unchanged, no render flag.
	tool := newTestWebBrowseTool(&mockBrowserInvoker{})
	got, renderedFlag := tool.maybeRender(context.Background(), shell, "https://example.com/login")
	if renderedFlag || got.statusCode != shell.statusCode {
		t.Fatal("nil renderer must keep the fetched document")
	}

	// Successful render: adopted, flagged, content-type forced to HTML.
	renderer := &mockPageRenderer{result: rendered}
	tool.renderer = renderer
	got, renderedFlag = tool.maybeRender(context.Background(), shell, "https://example.com/login")
	if !renderedFlag {
		t.Fatal("improved render should be adopted")
	}
	if renderer.calls != 1 {
		t.Fatalf("renderer calls = %d, want 1", renderer.calls)
	}
	if got.extractor != "headless-render" || !strings.Contains(got.content, "rendered login form text") {
		t.Fatalf("rendered doc not adopted: %+v", got)
	}
	if got.contentType != "text/html; charset=utf-8" || got.finalURL != "https://example.com/login" {
		t.Fatalf("rendered doc metadata wrong: %+v", got)
	}

	// Renderer failure: original document kept.
	tool.renderer = &mockPageRenderer{err: context.DeadlineExceeded}
	got, renderedFlag = tool.maybeRender(context.Background(), shell, "https://example.com/login")
	if renderedFlag || got.extractor == "headless-render" {
		t.Fatal("failed render must keep the fetched document")
	}

	// Render no better than the fetch: not adopted.
	tool.renderer = &mockPageRenderer{result: &PageRenderResult{HTML: "<html><body><div>still thin</div></body></html>"}}
	got, renderedFlag = tool.maybeRender(context.Background(), shell, "https://example.com/login")
	if renderedFlag || got.extractor == "headless-render" {
		t.Fatal("no-better render must keep the fetched document")
	}

	// Rich page skips the renderer entirely.
	renderer2 := &mockPageRenderer{result: rendered}
	tool.renderer = renderer2
	rich := fetchRawResult{
		content:     "<html><body>" + strings.Repeat("<p>meaningful body text here</p>", 20) + "</body></html>",
		contentType: "text/html",
		statusCode:  200,
	}
	if _, flag := tool.maybeRender(context.Background(), rich, "https://example.com"); flag || renderer2.calls != 0 {
		t.Fatal("rich fetch must not invoke the renderer")
	}
}
