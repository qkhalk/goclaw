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

// WebBrowseTool implements web_browse: opens a URL in the user's browser
// panel. The gateway fetches ONE sanitized HTML document (SSRF-checked,
// scripts stripped) and relays it same-origin; the user's browser then loads
// every image/CSS/font directly from the origin site and extracts the page
// text client-side. Falls back to plain server-side extraction (the web_fetch
// pipeline) when no web client is available — Telegram, dashboards, timeouts
// — so the tool works on every channel and never hangs.
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
	return "Open a URL in the user's browser panel and get the page content as markdown. " +
		"The page renders visually in the user's web client while you read it — heavy assets load on " +
		"the user's machine, not the server. Prefer this over web_fetch when reading articles/docs on " +
		"the web channel (the user sees what you are reading), and over the browser tool (which runs a " +
		"heavy headless Chrome on the server). Falls back to a plain server-side fetch when no web " +
		"client is connected. Note: pages that render purely via JavaScript return little content — " +
		"the relayed page never executes scripts; if the result looks thin, switch to web_search or an API."
}

func (t *WebBrowseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "HTTP or HTTPS URL to open in the user's browser panel.",
			},
			"maxChars": map[string]any{
				"type":        "number",
				"description": "Maximum characters of extracted content to return. Default: 60000.",
				"minimum":     100.0,
			},
			"timeoutMs": map[string]any{
				"type":        "number",
				"description": "How long to wait for the user's browser to render and extract (ms). Default: 45000, max 120000.",
				"minimum":     5000.0,
				"maximum":     120000.0,
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebBrowseTool) Execute(ctx context.Context, args map[string]any) *Result {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return ErrorResult("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid URL: %v", err))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrorResult("only http and https URLs are supported")
	}
	if parsed.Host == "" {
		return ErrorResult("missing hostname in URL")
	}
	if err := CheckSSRF(rawURL); err != nil {
		return ErrorResult(fmt.Sprintf("SSRF protection: %v", err))
	}
	pol := t.fetch.resolvePolicy(ctx)
	hostname := parsed.Hostname()
	if matchDomainList(hostname, pol.blockedDomains) {
		return ErrorResult(fmt.Sprintf("domain %q is blocked by policy", hostname))
	}
	if pol.mode == "allowlist" && !matchDomainList(hostname, pol.allowedDomains) {
		return ErrorResult(fmt.Sprintf("domain %q is not in the allowed domains list", hostname))
	}

	maxChars := t.fetch.maxChars
	if mc, ok := args["maxChars"].(float64); ok && int(mc) >= 100 {
		maxChars = int(mc)
	}
	timeout := DefaultBrowseClientTimeout
	if tm, ok := args["timeoutMs"].(float64); ok && tm > 0 {
		timeout = time.Duration(tm) * time.Millisecond
	}

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
			return NewResult(formatBrowseResult(content, title, finalURL, maxChars, true))
		}
	}

	// Fallback (or delegation declined): server-side extraction from the
	// already-fetched document — no second request.
	text := extractDocumentText(doc)
	title := browse.ExtractTitle(doc.content)
	return NewResult(formatBrowseResult(text, title, doc.finalURL, maxChars, false))
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

// delegateToClient relays the sanitized document and waits for the client's
// extraction. Returns nil when the client path is unavailable or failed —
// the caller then falls back to server-side extraction.
func (t *WebBrowseTool) delegateToClient(ctx context.Context, doc fetchRawResult, rawURL string, tenantID uuid.UUID, chatID string, timeout time.Duration) map[string]any {
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
		return nil
	}
	relayPath := "/v1/browse/" + id
	payload := map[string]any{
		"browseId":   id,
		"url":        rawURL,
		"finalUrl":   doc.finalURL,
		"relayUrl":   relayPath + "?ft=" + t.signer(relayPath),
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

// formatBrowseResult shapes the tool output for the LLM.
func formatBrowseResult(content, title, finalURL string, maxChars int, viaClient bool) string {
	source := "server-fetch"
	if viaClient {
		source = "user-browser (page rendered in the user's browser panel)"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "URL: %s\n", finalURL)
	if title != "" {
		fmt.Fprintf(&sb, "Title: %s\n", title)
	}
	fmt.Fprintf(&sb, "Source: %s\n\n", source)
	if len(content) > maxChars {
		content = content[:maxChars] + "\n[... truncated]"
	}
	sb.WriteString(content)
	return sb.String()
}
