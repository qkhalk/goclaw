package methods

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// BrowserPanelMethods implements the browser.panel.* WS surface for
// client-side browsing:
//
//   - browser.panel.result: the web client posts extracted content of a page
//     the gateway asked it to render/operate (browser.panel.invoke event).
//   - browser.panel.open: the panel navigates itself (URL bar, link clicks,
//     agent-driven navigation) — the gateway fetches + sanitizes + relays one
//     URL and responds with the loadable relay URL directly.
//
// Distinct from browser.act/snapshot/screenshot, which drive the server-side
// Rod browser.
type BrowserPanelMethods struct {
	bridge *gateway.BrowserPanelBridge
	browse *tools.WebBrowseTool
}

// NewBrowserPanelMethods wires the surface. Nil dependencies yield handlers
// that reply unavailable, keeping registration nil-safe.
func NewBrowserPanelMethods(bridge *gateway.BrowserPanelBridge, browse *tools.WebBrowseTool) *BrowserPanelMethods {
	return &BrowserPanelMethods{bridge: bridge, browse: browse}
}

// Register wires the browser.panel.* methods into the method router.
func (m *BrowserPanelMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodBrowserPanelResult, m.handleResult)
	router.Register(protocol.MethodBrowserPanelOpen, m.handleOpen)
}

// handleResult receives a web client's extracted page content and routes it
// to the in-memory waiter matching the correlation id. Only the connection
// the invoke targeted may post results (enforced inside the bridge).
func (m *BrowserPanelMethods) handleResult(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if m.bridge == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "browser panel bridge not wired"))
		return
	}
	var params struct {
		BrowseID  string `json:"browseId"`
		Content   string `json:"content"`
		Title     string `json:"title"`
		FinalURL  string `json:"finalUrl"`
		Error     string `json:"error"`
		Note      string `json:"note"`
		Truncated bool   `json:"truncated"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.BrowseID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "browseId")))
		return
	}

	delivered := m.bridge.DeliverBrowserResult(params.BrowseID, client, map[string]any{
		"content":   params.Content,
		"title":     params.Title,
		"finalUrl":  params.FinalURL,
		"error":     params.Error,
		"note":      params.Note,
		"truncated": params.Truncated,
	})
	if !delivered {
		slog.Debug("browser_panel.result_unmatched", "browse_id", params.BrowseID, "client", client.ID())
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"delivered": delivered}))
}

// handleOpen navigates the panel: the client sends a URL (URL bar entry, link
// click, or agent-driven navigation hop) and the gateway prepares the
// sanitized relay document, responding directly with the loadable relay URL.
// No waiter — the agent learns the page content through the correlated
// browser.panel.result of whichever invoke started the interaction.
func (m *BrowserPanelMethods) handleOpen(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if m.browse == nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "browse relay not wired"))
		return
	}
	var params struct {
		URL string `json:"url"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.URL == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "url")))
		return
	}

	info, err := m.browse.OpenRelay(ctx, params.URL)
	if err != nil {
		slog.Debug("browser_panel.open_failed", "url", params.URL, "client", client.ID(), "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInternalError, err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"browseId": info.BrowseID,
		"relayUrl": info.RelayURL,
		"finalUrl": info.FinalURL,
		"title":    info.Title,
	}))
}
