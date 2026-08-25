package store

import (
	"context"
	"time"
)

// TerminalStatus values for the terminal session lifecycle. running marks a
// live tab; exited means the shell process died on its own; closed means the
// session was closed explicitly (by the owner or an admin).
const (
	TerminalStatusRunning = "running"
	TerminalStatusExited  = "exited"
	TerminalStatusClosed  = "closed"
)

// TerminalSession is one web-terminal tab (Paseo plan Phase 4 / §25). Only
// metadata + lifecycle is durable; the PTY itself and its output live in the
// gateway's in-memory ring buffer and die with the process.
type TerminalSession struct {
	ID          string    `json:"id"`
	TenantID    *string   `json:"tenantId,omitempty"`
	UserID      string    `json:"userId"`
	WorkspaceID string    `json:"workspaceId"`
	CWD         string    `json:"cwd"`
	Shell       string    `json:"shell"`
	Status      string    `json:"status"`
	ExitCode    *int      `json:"exitCode,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ValidTerminalSessionStatus reports whether s is a known terminal status.
func ValidTerminalSessionStatus(s string) bool {
	switch s {
	case TerminalStatusRunning, TerminalStatusExited, TerminalStatusClosed:
		return true
	}
	return false
}

// TerminalStore persists terminal tab metadata so the UI can list and
// reconnect tabs after a gateway restart (the PTY is gone; attach then
// reports the session ended). Implementations must scope writes to the
// tenant from context where a tenant column exists.
type TerminalStore interface {
	// CreateSession inserts a new terminal session row. The ID and
	// timestamps are defaulted by the implementation when zero-valued.
	CreateSession(ctx context.Context, s *TerminalSession) error
	// GetSession resolves one session by id.
	GetSession(ctx context.Context, id string) (*TerminalSession, error)
	// ListSessions returns the user's sessions ordered by updated_at DESC.
	// workspaceID filters to a single workspace when non-empty.
	ListSessions(ctx context.Context, tenantID *string, userID string, workspaceID string) ([]*TerminalSession, error)
	// UpdateStatus transitions the lifecycle status, stamping exit_code (nil
	// keeps it NULL) and refreshing updated_at.
	UpdateStatus(ctx context.Context, id, status string, exitCode *int) error
}
