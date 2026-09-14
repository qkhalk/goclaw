package gateway

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Typed browser-panel invoke failures. The web_browse tool maps each of these
// to its server-side fallback path, so a missing/slow web client never strands
// an agent turn — and neither can a user closing the tab mid-browse.
var (
	ErrBrowserClientOffline  = errors.New("no connected web client for this user")
	ErrBrowserInvokeTimeout  = errors.New("browser panel invoke timed out")
	ErrBrowserInvokeRejected = errors.New("web client could not render or extract the page")
)

const (
	// DefaultBrowserInvokeTimeout bounds a client browse when the caller passes none.
	DefaultBrowserInvokeTimeout = 45 * time.Second

	// MaxBrowserInvokeTimeout caps caller-supplied timeouts.
	MaxBrowserInvokeTimeout = 2 * time.Minute

	// MaxBrowserResultBytes caps extracted page content carried in one result.
	MaxBrowserResultBytes = 256 * 1024
)

// browserPanelWaiter correlates one invoke with the client connection it was
// targeted at. The channel is buffered(1) so a late/dupe result never blocks
// the posting RPC handler (nodes.Registry.pending precedent).
type browserPanelWaiter struct {
	clientID string
	tenantID uuid.UUID
	userID   string
	ch       chan map[string]any
}

// BrowserPanelBridge pushes a browse request to a connected web client and
// awaits the extracted page content. Mirrors the nodes.Invoke correlation
// pattern (invoke UUID + waiter channel) but targets a gateway WS client
// found by tenant+user instead of a registered node daemon. Events are sent
// directly on the *Client (WhatsApp-QR precedent), never through the bus, so
// no event-filter branch is involved and other users never see the invoke.
type BrowserPanelBridge struct {
	server *Server

	mu      sync.Mutex
	waiters map[string]*browserPanelWaiter
}

// BrowserPanelBridge returns the server-wide bridge, creating it on first use.
func (s *Server) BrowserPanelBridge() *BrowserPanelBridge {
	s.browserBridgeOnce.Do(func() {
		s.browserBridge = &BrowserPanelBridge{
			server:  s,
			waiters: make(map[string]*browserPanelWaiter),
		}
	})
	return s.browserBridge
}

// findClientByUser returns the most recently connected authenticated client
// for the tenant+user pair (multiple tabs resolve to the newest one).
func (s *Server) findClientByUser(tenantID uuid.UUID, userID string) *Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best *Client
	for _, c := range s.clients {
		if !c.authenticated || c.userID != userID || c.tenantID != tenantID {
			continue
		}
		if best == nil || c.connectedAt.After(best.connectedAt) {
			best = c
		}
	}
	return best
}

// InvokeBrowserPanel sends a browser.panel.invoke event to the user's web
// client and blocks until the client posts browser.panel.result, the timeout
// elapses, or ctx is canceled. payload must carry a "browseId"; one is minted
// when absent. The returned map is the client's result payload (content,
// title, finalUrl, error) — a non-empty "error" field yields
// ErrBrowserInvokeRejected.
func (b *BrowserPanelBridge) InvokeBrowserPanel(ctx context.Context, tenantID uuid.UUID, userID string, payload map[string]any, timeout time.Duration) (map[string]any, error) {
	if b == nil || b.server == nil {
		return nil, ErrBrowserClientOffline
	}
	client := b.server.findClientByUser(tenantID, userID)
	if client == nil {
		return nil, ErrBrowserClientOffline
	}
	if timeout <= 0 {
		timeout = DefaultBrowserInvokeTimeout
	}
	if timeout > MaxBrowserInvokeTimeout {
		timeout = MaxBrowserInvokeTimeout
	}

	browseID, _ := payload["browseId"].(string)
	if browseID == "" {
		browseID = uuid.NewString()
		payload["browseId"] = browseID
	}

	w := &browserPanelWaiter{
		clientID: client.id,
		tenantID: tenantID,
		userID:   userID,
		ch:       make(chan map[string]any, 1),
	}
	b.mu.Lock()
	b.waiters[browseID] = w
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.waiters, browseID)
		b.mu.Unlock()
	}()

	client.SendEvent(*protocol.NewEvent(protocol.EventBrowserPanelInvoke, payload))
	slog.Info("browser_panel.invoke_sent", "client", client.id, "browse_id", browseID)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-w.ch:
		if errMsg, _ := res["error"].(string); errMsg != "" {
			return res, ErrBrowserInvokeRejected
		}
		return res, nil
	case <-timer.C:
		return nil, ErrBrowserInvokeTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// DeliverBrowserResult resolves the waiter for browseID with a result posted
// by client. Only the connection the invoke targeted may deliver (sender
// check mirrors nodes.handleResult); anything else is logged as a security
// event and dropped. Returns false for unknown, expired, or mismatched
// invocations.
func (b *BrowserPanelBridge) DeliverBrowserResult(browseID string, client *Client, result map[string]any) bool {
	if b == nil || browseID == "" || client == nil {
		return false
	}
	b.mu.Lock()
	w, ok := b.waiters[browseID]
	if ok {
		if w.clientID != client.id || w.tenantID != client.tenantID || w.userID != client.userID {
			b.mu.Unlock()
			slog.Warn("security.browser_result_wrong_sender",
				"browse_id", browseID, "client", client.id)
			return false
		}
		delete(b.waiters, browseID)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	capBrowserResult(result)
	select {
	case w.ch <- result:
	default:
	}
	return true
}

// capBrowserResult truncates oversized extracted content in place so one
// chatty page cannot blow up a frame or the agent context.
func capBrowserResult(result map[string]any) {
	if content, ok := result["content"].(string); ok && len(content) > MaxBrowserResultBytes {
		result["content"] = content[:MaxBrowserResultBytes]
		result["truncated"] = true
	}
}
