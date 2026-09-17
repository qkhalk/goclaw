package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/browse"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// browseMaxDocumentBytes mirrors browse.MaxDocumentBytes for the raw fetch
// read cap in web_fetch.go.
const browseMaxDocumentBytes = browse.MaxDocumentBytes

// DefaultBrowseClientTimeout bounds the wait for the web client's extraction
// (mirrors gateway.DefaultBrowserInvokeTimeout).
const DefaultBrowseClientTimeout = 45 * time.Second

// ClientBrowserInvoker pushes a browse request to the user's connected web
// client and awaits its extracted page content. Implemented by the gateway's
// BrowserPanelBridge; declared here so the tools package never imports the
// gateway (which imports tools).
type ClientBrowserInvoker interface {
	InvokeBrowserPanel(ctx context.Context, tenantID uuid.UUID, userID string, payload map[string]any, timeout time.Duration) (map[string]any, error)
}

// RelayTokenSigner mints a short-lived signed token (?ft=) authorizing an
// iframe to load a relay path without an Authorization header. Implemented by
// cmd wiring over httpapi.SignFileToken (tools cannot import internal/http —
// cycle).
type RelayTokenSigner func(path string) string

// RelayInfo describes one sanitized document prepared for the browser panel.
type RelayInfo struct {
	BrowseID string
	RelayURL string // /v1/browse/{id}?ft=... (signed, iframe-loadable)
	FinalURL string // post-redirect page URL
	Title    string
}

// WebBrowseTool implements web_browse: opens URLs in the user's browser panel
// and operates the page on their behalf (click/type/navigate) while it renders
// visually in their web client. The gateway fetches ONE sanitized HTML
// document per navigation (SSRF-checked, scripts stripped) and relays it
// same-origin; the user's browser loads every image/CSS/font directly from the
// origin site and extracts the page text client-side. Falls back to plain
// server-side extraction (the web_fetch pipeline) for the open action when no
// web client is available — Telegram, dashboards, timeouts — so the tool never
// hangs.
type WebBrowseTool struct {
	fetch   *WebFetchTool
	store   *browse.Store
	invoker ClientBrowserInvoker
	signer  RelayTokenSigner
}

// NewWebBrowseTool builds the tool around the shared web_fetch pipeline and
// the browse relay store.
func NewWebBrowseTool(fetch *WebFetchTool, store *browse.Store) *WebBrowseTool {
	return &WebBrowseTool{fetch: fetch, store: store}
}

// SetClientInvoker wires the gateway browser-panel bridge (cmd wiring).
func (t *WebBrowseTool) SetClientInvoker(inv ClientBrowserInvoker) { t.invoker = inv }

// SetRelayTokenSigner wires the ?ft= signer over httpapi (cmd wiring).
func (t *WebBrowseTool) SetRelayTokenSigner(signer RelayTokenSigner) { t.signer = signer }

func (t *WebBrowseTool) Name() string { return "web_browse" }

func (t *WebBrowseTool) Description() string {
	return "Open a URL in the user's browser panel, read it, and operate it (click links/buttons, fill inputs) " +
		"while the user watches the page live in their panel. The page renders in the user's web client — heavy " +
		"assets load on the user's machine, not the server. The returned content tags interactive elements with " +
		"[eN] refs: pass action=\"click\"/\"type\" with a ref to act, then the fresh page content comes back. " +
		"Prefer this over web_fetch on the web channel (the user sees what you are doing) and over the browser " +
		"tool (which runs heavy headless Chrome on the server). Falls back to a plain server-side fetch for the " +
		"open action when no web client is connected. Limitation: the relayed page never executes scripts, so " +
		"JS-only sites return thin content — switch to web_search or an API in that case."
}

func (t *WebBrowseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "HTTP or HTTPS URL to open (omit when action is set; keep opening with url to navigate).",
			},
			"action": map[string]any{
				"type":        "string",
				"description": "Operate the page currently shown in the panel instead of opening a new URL.",
				"enum":        []string{"click", "type", "extract", "back", "reload"},
			},
			"ref": map[string]any{
				"type":        "string",
				"description": "Element ref from the [eN] tags (required for click/type).",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Text to type (required for type).",
			},
			"maxChars": map[string]any{
				"type":        "number",
				"description": "Maximum characters of extracted content to return. Default: 60000.",
				"minimum":     100.0,
			},
			"timeoutMs": map[string]any{
				"type":        "number",
				"description": "How long to wait for the user's browser (ms). Default: 45000, max 120000.",
				"minimum":     5000.0,
				"maximum":     120000.0,
			},
		},
	}
}

func (t *WebBrowseTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	switch action {
	case "", "open":
		return t.executeOpen(ctx, args)
	case "click", "type", "extract", "back", "reload":
		return t.executeClientAction(ctx, action, args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action %q (use open/click/type/extract/back/reload, or pass url to open)", action))
	}
}

// executeOpen opens a URL: relay it to the client panel (agent sees the page
// live) or fall back to server-side extraction when no web client exists.
func (t *WebBrowseTool) executeOpen(ctx context.Context, args map[string]any) *Result {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return ErrorResult("url is required (or pass action to operate the currently open page)")
	}
	pol, errResult := t.validateURL(ctx, rawURL)
	if errResult != nil {
		return errResult
	}
	maxChars := browseMaxChars(args, t.fetch.maxChars)
	timeout := browseTimeout(args)

	// Single document fetch shared by both paths: relayed to the client when
	// one is reachable, otherwise extracted server-side.
	doc, err := t.fetch.fetchRawHTML(ctx, rawURL, pol)
	if err != nil {
		return ErrorResult(fmt.Sprintf("fetch failed: %s", truncateStr(err.Error(), defaultErrorMaxChars)))
	}

	channel := ToolChannelFromCtx(ctx)
	chatID := ToolChatIDFromCtx(ctx)
	tenantID := store.TenantIDFromContext(ctx)

	canDelegate := t.invoker != nil && t.store != nil && t.signer != nil &&
		channel == ChannelWeb && chatID != "" && tenantID != uuid.Nil
	if canDelegate {
		if res := t.delegateToClient(ctx, doc, rawURL, tenantID, chatID, timeout); res != nil {
			content, _ := res["content"].(string)
			title, _ := res["title"].(string)
			finalURL, _ := res["finalUrl"].(string)
			if finalURL == "" {
				finalURL = doc.finalURL
			}
			return NewResult(formatBrowseResult(content, title, finalURL, maxChars, true, true))
		}
	}

	// Fallback (or delegation declined): server-side extraction from the
	// already-fetched document — no second request.
	text := extractDocumentText(doc)
	title := browse.ExtractTitle(doc.content)
	return NewResult(formatBrowseResult(text, title, doc.finalURL, maxChars, false, false))
}

// executeClientAction runs click/type/extract/back/reload on the page the
// user's panel is currently showing. These require the live panel — there is
// no server-side equivalent (the server holds no page state).
func (t *WebBrowseTool) executeClientAction(ctx context.Context, action string, args map[string]any) *Result {
	channel := ToolChannelFromCtx(ctx)
	chatID := ToolChatIDFromCtx(ctx)
	tenantID := store.TenantIDFromContext(ctx)
	if t.invoker == nil || channel != ChannelWeb || chatID == "" || tenantID == uuid.Nil {
		return ErrorResult(fmt.Sprintf("action %q requires the user's browser panel (web channel, panel open)", action))
	}
	ref, _ := args["ref"].(string)
	text, _ := args["text"].(string)
	if (action == "click" || action == "type") && ref == "" {
		return ErrorResult(fmt.Sprintf("action %q requires ref (an [eN] tag from the last page content)", action))
	}
	if action == "type" && strings.TrimSpace(text) == "" {
		return ErrorResult("action type requires text")
	}
	maxChars := browseMaxChars(args, t.fetch.maxChars)
	timeout := browseTimeout(args)

	payload := map[string]any{
		"browseId":   uuid.NewString(), // correlation id for the action's result
		"action":     action,
		"maxChars":   maxChars,
		"deadlineMs": timeout.Milliseconds(),
	}
	if ref != "" {
		payload["ref"] = ref
	}
	if text != "" {
		payload["text"] = text
	}
	res, err := t.invoker.InvokeBrowserPanel(ctx, tenantID, chatID, payload, timeout)
	if err != nil {
		return ErrorResult(fmt.Sprintf("browser panel action failed: %s (the user may have closed the panel)", truncateStr(err.Error(), 300)))
	}
	content, _ := res["content"].(string)
	title, _ := res["title"].(string)
	finalURL, _ := res["finalUrl"].(string)
	note, _ := res["note"].(string)
	out := formatBrowseResult(content, title, finalURL, maxChars, true, false)
	if note != "" {
		out += "\n\nNote: " + note
	}
	return NewResult(out)
}

// validateURL applies scheme/host/SSRF/tenant-domain checks shared by the
// open action and OpenRelay.
func (t *WebBrowseTool) validateURL(ctx context.Context, rawURL string) (webFetchPolicy, *Result) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return webFetchPolicy{}, ErrorResult(fmt.Sprintf("invalid URL: %v", err))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return webFetchPolicy{}, ErrorResult("only http and https URLs are supported")
	}
	if parsed.Host == "" {
		return webFetchPolicy{}, ErrorResult("missing hostname in URL")
	}
	if err := CheckSSRF(rawURL); err != nil {
		return webFetchPolicy{}, ErrorResult(fmt.Sprintf("SSRF protection: %v", err))
	}
	pol := t.fetch.resolvePolicy(ctx)
	hostname := parsed.Hostname()
	if matchDomainList(hostname, pol.blockedDomains) {
		return webFetchPolicy{}, ErrorResult(fmt.Sprintf("domain %q is blocked by policy", hostname))
	}
	if pol.mode == "allowlist" && !matchDomainList(hostname, pol.allowedDomains) {
		return webFetchPolicy{}, ErrorResult(fmt.Sprintf("domain %q is not in the allowed domains list", hostname))
	}
	return pol, nil
}

// OpenRelay validates, fetches, sanitizes, stores, and signs one document for
// the browser panel — without invoking the client. Shared by the panel's own
// navigation RPC (browser.panel.open): URL-bar entries and link clicks made in
// the panel come through here. The sanitized document is exactly what the
// panel iframe already renders, so the RPC response carries everything the
// client needs to load it.
func (t *WebBrowseTool) OpenRelay(ctx context.Context, rawURL string) (*RelayInfo, error) {
	if t.store == nil || t.signer == nil || t.fetch == nil {
		return nil, fmt.Errorf("browse relay not wired")
	}
	if _, errResult := t.validateURL(ctx, rawURL); errResult != nil {
		return nil, fmt.Errorf("%s", errResult.ForLLM)
	}
	pol := t.fetch.resolvePolicy(ctx)
	doc, err := t.fetch.fetchRawHTML(ctx, rawURL, pol)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	return t.storeRelay(doc, rawURL)
}

// storeRelay sanitizes + stores a fetched document and signs its relay URL.
func (t *WebBrowseTool) storeRelay(doc fetchRawResult, rawURL string) (*RelayInfo, error) {
	sanitized, title := browse.Sanitize(doc.content, doc.finalURL)
	contentType := doc.contentType
	if !strings.Contains(contentType, "html") {
		contentType = "text/html; charset=utf-8" // sanitized output is always an HTML document
	}
	id, err := t.store.Put(&browse.Entry{
		HTML:        sanitized,
		Title:       title,
		URL:         rawURL,
		FinalURL:    doc.finalURL,
		ContentType: contentType,
	})
	if err != nil {
		return nil, err
	}
	relayPath := "/v1/browse/" + id
	return &RelayInfo{
		BrowseID: id,
		RelayURL: relayPath + "?ft=" + t.signer(relayPath),
		FinalURL: doc.finalURL,
		Title:    title,
	}, nil
}

// delegateToClient relays the sanitized document and waits for the client's
// extraction. Returns nil when the client path is unavailable or failed —
// the caller then falls back to server-side extraction.
func (t *WebBrowseTool) delegateToClient(ctx context.Context, doc fetchRawResult, rawURL string, tenantID uuid.UUID, chatID string, timeout time.Duration) map[string]any {
	info, err := t.storeRelay(doc, rawURL)
	if err != nil {
		return nil
	}
	payload := map[string]any{
		"browseId":   info.BrowseID,
		"url":        rawURL,
		"finalUrl":   info.FinalURL,
		"relayUrl":   info.RelayURL,
		"deadlineMs": timeout.Milliseconds(),
	}
	res, err := t.invoker.InvokeBrowserPanel(ctx, tenantID, chatID, payload, timeout)
	if err != nil || res == nil {
		return nil
	}
	if content, _ := res["content"].(string); strings.TrimSpace(content) != "" {
		return res
	}
	return nil
}

// extractDocumentText routes the raw document by content type, mirroring
// web_fetch's routing but on the already-fetched body.
func extractDocumentText(doc fetchRawResult) string {
	switch {
	case strings.Contains(doc.contentType, "application/json"):
		text, _ := extractJSON([]byte(doc.content))
		return text
	case strings.Contains(doc.contentType, "text/html"),
		strings.Contains(doc.contentType, "application/xhtml"),
		doc.contentType == "": // assume HTML
		text := htmlToMarkdown(doc.content)
		if strings.TrimSpace(text) == "" {
			return "[No content extracted. The page may require JavaScript to render or returned a " +
				"bot-protection challenge — the relayed page never executes scripts. Try web_search or an API instead.]"
		}
		return text
	default:
		return doc.content
	}
}

// formatBrowseResult shapes the tool output for the LLM.
func formatBrowseResult(content, title, finalURL string, maxChars int, viaClient, withRefsHint bool) string {
	source := "server-fetch"
	if viaClient {
		source = "user-browser (page rendered in the user's browser panel)"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "URL: %s\n", finalURL)
	if title != "" {
		fmt.Fprintf(&sb, "Title: %s\n", title)
	}
	fmt.Fprintf(&sb, "Source: %s\n", source)
	if withRefsHint && strings.Contains(content, "[e") {
		sb.WriteString("Interactive elements are tagged [eN] — act on them with {\"action\":\"click\"|\"type\", \"ref\":\"eN\"}. " +
			"Clicking a link navigates; the user sees every step.\n")
	}
	sb.WriteString("\n")
	if len(content) > maxChars {
		content = content[:maxChars] + "\n[... truncated]"
	}
	sb.WriteString(content)
	return sb.String()
}

func browseMaxChars(args map[string]any, def int) int {
	if mc, ok := args["maxChars"].(float64); ok && int(mc) >= 100 {
		return int(mc)
	}
	return def
}

func browseTimeout(args map[string]any) time.Duration {
	if tm, ok := args["timeoutMs"].(float64); ok && tm > 0 {
		return time.Duration(tm) * time.Millisecond
	}
	return DefaultBrowseClientTimeout
}
