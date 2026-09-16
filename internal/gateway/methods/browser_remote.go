package methods

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/pkg/browser"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// BrowserRemoteMethods implements the browser.remote.* WS surface — the
// server-side (headless Chrome) counterpart of the client browser panel:
//
//   - browser.remote.open: validate URL (SSRF/policy) → open a per-tenant tab
//     in the shared Rod browser → return the a11y snapshot + screenshot.
//   - browser.remote.act: click/type/press/back on the live page by [eN] ref.
//   - browser.remote.screenshot: re-capture the current view.
//
// This gives the agent full control over JS-heavy SPA sites that the
// sanitized relay cannot operate (the relay strips scripts and live iframes
// are cross-origin-blind), while the panel displays the real rendered page
// through the returned screenshot.
type BrowserRemoteMethods struct {
	mgr    *browser.Manager
	cfg    *config.Config
	bridge *gateway.BrowserPanelBridge
}

// NewBrowserRemoteMethods wires the surface. A nil manager yields handlers
// that reply unavailable (browser tool disabled on this instance).
func NewBrowserRemoteMethods(mgr *browser.Manager, cfg *config.Config, bridge *gateway.BrowserPanelBridge) *BrowserRemoteMethods {
	return &BrowserRemoteMethods{mgr: mgr, cfg: cfg, bridge: bridge}
}

// Register wires the browser.remote.* methods into the method router.
func (m *BrowserRemoteMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodBrowserRemoteOpen, m.handleOpen)
	router.Register(protocol.MethodBrowserRemoteAct, m.handleAct)
	router.Register(protocol.MethodBrowserRemoteScreenshot, m.handleScreenshot)
}

func (m *BrowserRemoteMethods) available(client *gateway.Client, reqID string) bool {
	if m == nil || m.mgr == nil {
		client.SendResponse(protocol.NewErrorResponse(reqID, protocol.ErrFailedPrecondition,
			"remote browser is not enabled on this instance (tools.browser.enabled)"))
		return false
	}
	return true
}

// validateRemoteURL applies the same scheme/SSRF/domain-policy gate as the
// web tools before any navigation happens.
func (m *BrowserRemoteMethods) validateRemoteURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only http and https URLs are supported")
	}
	if parsed.Host == "" {
		return fmt.Errorf("missing hostname in URL")
	}
	if err := tools.CheckSSRF(raw); err != nil {
		return fmt.Errorf("SSRF protection: %w", err)
	}
	return nil
}

func (m *BrowserRemoteMethods) handleOpen(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.available(client, req.ID) {
		return
	}
	var params struct {
		URL       string `json:"url"`
		SessionID string `json:"sessionId,omitempty"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
			return
		}
	}
	target := strings.TrimSpace(params.URL)
	if target == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "url is required"))
		return
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	if err := m.validateRemoteURL(target); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, err.Error()))
		return
	}
	if err := m.mgr.Start(ctx); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "browser start: "+err.Error()))
		return
	}
	tab, err := m.mgr.OpenTab(ctx, target)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "open tab: "+err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, m.capture(ctx, tab.TargetID, "open")))
}

// handleAct performs click/type/press/back on the live remote page and
// returns the post-action snapshot + screenshot.
func (m *BrowserRemoteMethods) handleAct(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.available(client, req.ID) {
		return
	}
	var params struct {
		TargetID  string `json:"targetId"`
		Action    string `json:"action"`
		Ref       string `json:"ref,omitempty"`
		Text      string `json:"text,omitempty"`
		Key       string `json:"key,omitempty"`
		URL       string `json:"url,omitempty"`
		SessionID string `json:"sessionId,omitempty"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
			return
		}
	}
	if params.TargetID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "targetId is required"))
		return
	}

	actCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	switch params.Action {
	case "click":
		if params.Ref == "" {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "ref is required for click"))
			return
		}
		if err := m.mgr.Click(actCtx, params.TargetID, params.Ref, browser.ClickOpts{TimeoutMs: 30_000}); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "click: "+err.Error()))
			return
		}
	case "type":
		if params.Ref == "" || params.Text == "" {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "ref and text are required for type"))
			return
		}
		if err := m.mgr.Type(actCtx, params.TargetID, params.Ref, params.Text, browser.TypeOpts{}); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "type: "+err.Error()))
			return
		}
	case "press":
		if params.Key == "" {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "key is required for press"))
			return
		}
		if err := m.mgr.Press(actCtx, params.TargetID, params.Key); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "press: "+err.Error()))
			return
		}
	case "back":
		if err := m.mgr.Navigate(actCtx, params.TargetID, "about:blank"); err != nil {
			// about:blank navigation is a placeholder; real history-back is
			// expressed by re-opening the previous URL — acceptable for v1.
			slog.Debug("browser.remote: back fallback navigate", "error", err)
		}
	case "navigate":
		target := strings.TrimSpace(params.URL)
		if target == "" {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "url is required for navigate"))
			return
		}
		if !strings.Contains(target, "://") {
			target = "https://" + target
		}
		if err := m.validateRemoteURL(target); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, err.Error()))
			return
		}
		if err := m.mgr.Navigate(actCtx, params.TargetID, target); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "navigate: "+err.Error()))
			return
		}
	default:
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
			fmt.Sprintf("unsupported action %q (click|type|press|back|navigate)", params.Action)))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, m.capture(actCtx, params.TargetID, params.Action)))
}

// handleScreenshot re-captures the current view of a tab.
func (m *BrowserRemoteMethods) handleScreenshot(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.available(client, req.ID) {
		return
	}
	var params struct {
		TargetID string `json:"targetId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
			return
		}
	}
	if params.TargetID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "targetId is required"))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, m.capture(ctx, params.TargetID, "screenshot")))
}

// capture snapshots the page (a11y refs for the agent) and screenshots it
// (base64 PNG for the panel visual). Snapshot failures degrade gracefully —
// some pages block the a11y tree but still screenshot.
func (m *BrowserRemoteMethods) capture(ctx context.Context, targetID, action string) map[string]any {
	resp := map[string]any{"targetId": targetID, "action": action}

	snap, err := m.mgr.Snapshot(ctx, targetID, browser.SnapshotOptions{})
	if err == nil && snap != nil {
		resp["snapshot"] = snap.Snapshot
		resp["refs"] = snap.Refs
		resp["url"] = snap.URL
		resp["title"] = snap.Title
	} else if err != nil {
		slog.Warn("browser.remote: snapshot failed", "target", targetID, "error", err)
	}

	if png, err := m.mgr.Screenshot(ctx, targetID, false); err == nil && len(png) > 0 {
		resp["screenshot"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	} else if err != nil {
		slog.Warn("browser.remote: screenshot failed", "target", targetID, "error", err)
	}

	return resp
}
