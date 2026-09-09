package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// maxCommandLen bounds the human-readable preview persisted with an approval
// request (the command column / PendingApproval.Command).
const maxCommandLen = 200

// grantRecord is an in-memory approval grant with an optional expiry. A zero
// expiresAt means the grant never expires.
type grantRecord struct {
	expiresAt time.Time
}

// expired reports whether the grant's expiry has passed (false when no expiry).
func (g grantRecord) expired(now time.Time) bool {
	return !g.expiresAt.IsZero() && now.After(g.expiresAt)
}

// grantStore is the dynamic allowlist mechanism for tool-class approvals.
// Three scopes are tracked, each keyed differently:
//
//	once:    class|tool|argsDigest — consumed on first use (allow-once)
//	session: class|tool|sessionKey  — lives for the requesting session
//	always:  class|tool             — permanent until process restart
//
// Grants are the in-memory fast path (mirrored best-effort onto
// approval_requests rows for audit); they are not rebuilt from the store on
// restart, matching the legacy alwaysAllow behavior.
type grantStore struct {
	mu      sync.Mutex
	once    map[string]grantRecord
	session map[string]grantRecord
	always  map[string]grantRecord
}

func newGrantStore() *grantStore {
	return &grantStore{
		once:    make(map[string]grantRecord),
		session: make(map[string]grantRecord),
		always:  make(map[string]grantRecord),
	}
}

func grantKey(parts ...string) string {
	return strings.Join(parts, "|")
}

func derefExpiry(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func (g *grantStore) addOnce(class ToolClass, tool, digest string, expiresAt *time.Time) {
	if digest == "" {
		return // a grant without a digest can never be matched back
	}
	g.mu.Lock()
	g.once[grantKey(string(class), tool, digest)] = grantRecord{expiresAt: derefExpiry(expiresAt)}
	g.mu.Unlock()
}

func (g *grantStore) addSession(class ToolClass, tool, sessionKey string, expiresAt *time.Time) {
	if sessionKey == "" {
		return // no session scope known: the grant would be unmatchable
	}
	g.mu.Lock()
	g.session[grantKey(string(class), tool, sessionKey)] = grantRecord{expiresAt: derefExpiry(expiresAt)}
	g.mu.Unlock()
}

func (g *grantStore) addAlways(class ToolClass, tool string, expiresAt *time.Time) {
	g.mu.Lock()
	g.always[grantKey(string(class), tool)] = grantRecord{expiresAt: derefExpiry(expiresAt)}
	g.mu.Unlock()
}

// checkOnce reports whether an unused allow-once grant exists for the
// (class, tool, digest) triple, consuming it on hit. Expired grants are
// dropped lazily.
func (g *grantStore) checkOnce(class ToolClass, tool, digest string) bool {
	if digest == "" {
		return false
	}
	key := grantKey(string(class), tool, digest)
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	rec, ok := g.once[key]
	if !ok {
		return false
	}
	delete(g.once, key) // allow-once is single-use regardless of expiry
	if rec.expired(now) {
		return false
	}
	return true
}

// checkSession reports whether a live allow-for-session grant exists for the
// (class, tool, session) triple.
func (g *grantStore) checkSession(class ToolClass, tool, sessionKey string) bool {
	if sessionKey == "" {
		return false
	}
	key := grantKey(string(class), tool, sessionKey)
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	rec, ok := g.session[key]
	if !ok {
		return false
	}
	if rec.expired(now) {
		delete(g.session, key)
		return false
	}
	return true
}

// checkAlways reports whether a live allow-always grant exists for (class, tool).
func (g *grantStore) checkAlways(class ToolClass, tool string) bool {
	if tool == "" {
		return false
	}
	key := grantKey(string(class), tool)
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	rec, ok := g.always[key]
	if !ok {
		return false
	}
	if rec.expired(now) {
		delete(g.always, key)
		return false
	}
	return true
}

// commandDigest returns the sha256 hex digest of a shell command, used as the
// allow-once grant key for the legacy exec path.
func commandDigest(command string) string {
	sum := sha256.Sum256([]byte(command))
	return hex.EncodeToString(sum[:])
}

// toolCallPreview renders a human-readable single-line preview of a tool call
// for the approval queue, truncated to maxCommandLen.
func toolCallPreview(toolName string, args map[string]any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return toolName
	}
	return truncateCmd(toolName+" "+string(b), maxCommandLen)
}
