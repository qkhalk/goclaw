package nodes

import (
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Sender is the minimal surface the registry needs from a live node
// connection. *gateway.Client satisfies it; the interface keeps this package
// free of gateway imports (and import cycles).
type Sender interface {
	SendEvent(event protocol.EventFrame)
}

// Closeable is implemented by senders that can be force-disconnected
// (gateway.Client.Close). Optional: the registry falls back to dropping the
// entry when the sender cannot be closed.
type Closeable interface {
	Close()
}

// DefaultOnlineTTL is how long a registry entry stays "online" without a
// re-registration heartbeat. Daemons re-register at roughly half this
// interval, so one missed heartbeat still reads online while two in a row
// reads offline. Entry expiry is enforced lazily on read — no sweep goroutine.
const DefaultOnlineTTL = 90 * time.Second

// connEntry is one live node connection with its liveness bookkeeping.
type connEntry struct {
	sender   Sender
	tenantID string
	lastSeen time.Time
}

// Registry tracks the live node daemon connections and the in-flight invoke
// waiters. It is the gateway-side source of truth for "online" state; the DB
// row (last_seen_at) is the durable echo.
type Registry struct {
	mu   sync.RWMutex
	cons map[string]*connEntry // nodeID -> live connection

	pendingMu sync.Mutex
	pending   map[string]chan *InvokeResult // invokeID -> waiter
	onlineTTL time.Duration
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		cons:      make(map[string]*connEntry),
		pending:   make(map[string]chan *InvokeResult),
		onlineTTL: DefaultOnlineTTL,
	}
}

// Set registers (or refreshes) the live connection for a node. Called from
// the nodes.register WS handler; a daemon refreshes its entry periodically so
// the entry never outlives its heartbeat by more than the online TTL.
func (r *Registry) Set(nodeID string, sender Sender, tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cons[nodeID] = &connEntry{sender: sender, tenantID: tenantID, lastSeen: time.Now()}
}

// Remove drops the node's connection entry. When the stored sender matches
// the stale one (reconnect replaced it), nothing is removed — the fresher
// connection stays authoritative. Returns the removed sender, if any.
func (r *Registry) Remove(nodeID string, stale Sender) (Sender, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.cons[nodeID]
	if !ok {
		return nil, false
	}
	if stale != nil && cur.sender != stale {
		// A newer connection already replaced this one; keep it.
		return nil, false
	}
	delete(r.cons, nodeID)
	return cur.sender, true
}

// Touch refreshes the liveness timestamp of the node's entry.
func (r *Registry) Touch(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cons[nodeID]; ok {
		c.lastSeen = time.Now()
	}
}

// Get returns the node's live sender when the entry exists and is within the
// online TTL. Stale entries are evicted lazily (daemon disconnected without
// an explicit unregister).
func (r *Registry) Get(nodeID string) (Sender, bool) {
	r.mu.RLock()
	c, ok := r.cons[nodeID]
	if ok && time.Since(c.lastSeen) > r.onlineTTL {
		ok = false // stale: treat (and report) as offline
	}
	var sender Sender
	if ok {
		sender = c.sender
	}
	r.mu.RUnlock()
	if !ok && c != nil {
		// Evict outside the read lock; Remove keeps the freshest entry.
		r.Remove(nodeID, c.sender)
	}
	return sender, ok
}

// TenantID returns the tenant of the node's registration entry.
func (r *Registry) TenantID(nodeID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.cons[nodeID]; ok {
		return c.tenantID
	}
	return ""
}

// Online reports whether the node has a fresh connection entry.
func (r *Registry) Online(nodeID string) bool {
	_, ok := r.Get(nodeID)
	return ok
}

// Disconnect force-closes the node's live connection (revoke path) and drops
// the entry. Safe to call for unknown nodes.
func (r *Registry) Disconnect(nodeID string) {
	sender, ok := r.Remove(nodeID, nil)
	if !ok {
		return
	}
	if c, ok := sender.(Closeable); ok {
		c.Close()
	}
}

// --- invoke correlation ---

// registerWaiter creates the result channel for an invoke correlation id.
func (r *Registry) registerWaiter(invokeID string) chan *InvokeResult {
	ch := make(chan *InvokeResult, 1)
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	r.pending[invokeID] = ch
	return ch
}

// resolveWaiter removes and returns the waiter for a correlation id. The
// channel stays buffered (capacity 1) so a racing result is never lost
// between resolve and timeout cleanup.
func (r *Registry) resolveWaiter(invokeID string) (chan *InvokeResult, bool) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	ch, ok := r.pending[invokeID]
	if ok {
		delete(r.pending, invokeID)
	}
	return ch, ok
}

// DeliverResult routes a daemon-posted outcome to the pending waiter, if one
// is still waiting. Returns false when no waiter matches (late, unknown, or
// already-timed-out invocation).
func (r *Registry) DeliverResult(res *InvokeResult) bool {
	if res == nil || res.InvokeID == "" {
		return false
	}
	ch, ok := r.resolveWaiter(res.InvokeID)
	if !ok {
		return false
	}
	select {
	case ch <- res:
		return true
	default:
		return false
	}
}
