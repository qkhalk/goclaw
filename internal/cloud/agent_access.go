package cloud

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Per-account agent access levels (enterprise governance): the web UI is
// unaffected — these levels gate ONLY what agents may do through the
// cloud_* / mail_* tools. Levels are a strict ladder:
//
//	none  — the account is invisible to agents entirely
//	read  — browse/read/fetch files, read mail (legacy default)
//	write — read + create/update files (cloud_write, cloud_mkdir), mutating
//	        mail actions (archive, unsubscribe)
//	full  — write + destructive/structuring operations (delete, move,
//	        public share links)
type AgentAccess string

const (
	AgentAccessNone  AgentAccess = "none"
	AgentAccessRead  AgentAccess = "read"
	AgentAccessWrite AgentAccess = "write"
	AgentAccessFull  AgentAccess = "full"
)

// DefaultAgentAccess applies to legacy rows (pre-column accounts): agents
// could read before the permission system existed, so read stays the default.
const DefaultAgentAccess = AgentAccessRead

// agentAccessRank orders the ladder for comparisons.
var agentAccessRank = map[AgentAccess]int{
	AgentAccessNone: 0, AgentAccessRead: 1, AgentAccessWrite: 2, AgentAccessFull: 3,
}

// ErrAgentAccessDenied is returned when an agent tool needs a higher access
// level than the account grants. The message names the level to request from
// an admin so the failure is actionable.
func errAgentAccessDenied(level AgentAccess, email string) error {
	switch level {
	case AgentAccessNone:
		return fmt.Errorf("cloud: agent access is disabled for %s — an admin can enable it on the Clouds page", email)
	default:
		return fmt.Errorf("cloud: agent access to %s does not allow %q — an admin can raise it on the Clouds page", email, level)
	}
}

// NormalizeAgentAccess validates a user-supplied level string.
func NormalizeAgentAccess(s string) (AgentAccess, error) {
	switch AgentAccess(strings.TrimSpace(s)) {
	case AgentAccessNone:
		return AgentAccessNone, nil
	case AgentAccessRead:
		return AgentAccessRead, nil
	case AgentAccessWrite:
		return AgentAccessWrite, nil
	case AgentAccessFull:
		return AgentAccessFull, nil
	case "":
		return DefaultAgentAccess, nil
	default:
		return "", errors.New("agent access must be one of: none, read, write, full")
	}
}

// AgentAccessOf returns the account's configured agent access level, falling
// back to the legacy default for empty/unknown values.
func AgentAccessOf(acct *store.CloudAccount) AgentAccess {
	if acct == nil {
		return AgentAccessNone
	}
	level := AgentAccess(strings.TrimSpace(acct.AgentAccess))
	if _, ok := agentAccessRank[level]; !ok {
		return DefaultAgentAccess
	}
	return level
}

// allows reports whether the granted level covers the required minimum.
func (a AgentAccess) allows(min AgentAccess) bool {
	return agentAccessRank[a] >= agentAccessRank[min]
}
