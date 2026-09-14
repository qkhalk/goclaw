package methods

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// BrowserPanelMethods implements the browser.panel.* WS surface for
// client-side browsing: the web client posts the extracted content of a page
// the gateway asked it to render (browser.panel.invoke event). Distinct from
// browser.act/snapshot/screenshot, which drive the server-side Rod browser.
type BrowserPanelMethods struct {
	bridge *gateway.BrowserPanelBridge
}

// NewBrowserPanelMethods wires the surface. A nil bridge yields a stub whose
// handler replies unavailable, keeping registration nil-safe.
func NewBrowserPanelMethods(bridge *gateway.BrowserPanelBridge) *BrowserPanelMethods {
	return &BrowserPanelMethods{bridge: bridge}
}

// Register wires the browser.panel.* methods into the method router.
func (m *BrowserPanelMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodBrowserPanelResult, m.handleResult)
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
		"truncated": params.Truncated,
	})
	if !delivered {
		slog.Debug("browser_panel.result_unmatched", "browse_id", params.BrowseID, "client", client.ID())
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"delivered": delivered}))
}
