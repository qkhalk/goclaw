// Package nodes provides the gateway-side runtime for registered compute
// nodes (inheritance plan Phase 2): key material, the in-memory registry of
// live node connections, and the Invoke helper that routes allowlisted exec
// requests to a node daemon over a targeted node.invoke event and correlates
// the nodes.result outcome.
//
// Distinct surfaces, by design:
//   - workstations (internal/workstation) — SSH remote exec, deferred from
//     this phase's unification scope;
//   - node_leases (internal/gateway/methods/node.go) — UI-tab presence;
//   - pairing — channel sender trust.
//
// Nodes are UNTRUSTED by default (plan Rule 5): nothing executes on a node
// unless its stored trust is `trusted` AND the daemon-side command allowlist
// admits the request.
package nodes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// NodeKeyPrefix marks a node bearer key ("gnk_" + 32 bytes hex = 292 bits of
// entropy). The plaintext is revealed exactly once at key creation; only the
// SHA-256 hex is persisted.
const NodeKeyPrefix = "gnk_"

// GenerateKey returns a fresh node bearer key (plaintext).
func GenerateKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("node key entropy: %w", err)
	}
	return NodeKeyPrefix + hex.EncodeToString(raw), nil
}

// HashKey returns the lowercase SHA-256 hex of a bearer key — the only form
// the registry stores. Constant-time comparison is not needed on the hash
// (the preimage is a 256-bit random value; hashing is done server-side), but
// callers must compare the full digest, never a prefix.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

// ValidateKeyFormat performs cheap structural validation before the DB
// lookup, so garbage keys never reach the store.
func ValidateKeyFormat(key string) bool {
	k := strings.TrimSpace(key)
	if !strings.HasPrefix(k, NodeKeyPrefix) {
		return false
	}
	body := strings.TrimPrefix(k, NodeKeyPrefix)
	if len(body) != 64 {
		return false
	}
	for _, c := range body {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
