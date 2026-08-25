package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGTerminalStore implements store.TerminalStore backed by PostgreSQL.
type PGTerminalStore struct {
	db *sql.DB
}

func NewPGTerminalStore(db *sql.DB) *PGTerminalStore {
	return &PGTerminalStore{db: db}
}

const terminalColumns = `id, tenant_id, user_id, workspace_id, cwd, shell,
 status, exit_code, created_at, updated_at`

func scanTerminalSession(row interface{ Scan(...any) error }) (*store.TerminalSession, error) {
	var s store.TerminalSession
	var tenantID sql.NullString
	var exitCode sql.NullInt64
	if err := row.Scan(&s.ID, &tenantID, &s.UserID, &s.WorkspaceID,
		&s.CWD, &s.Shell, &s.Status, &exitCode,
		&s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.TenantID = nilStrPtr(tenantID)
	if exitCode.Valid {
		code := int(exitCode.Int64)
		s.ExitCode = &code
	}
	return &s, nil
}

// CreateSession inserts a new terminal session row. ID defaults to a fresh
// UUIDv7; timestamps and status are defaulted when zero-valued.
func (s *PGTerminalStore) CreateSession(ctx context.Context, sess *store.TerminalSession) error {
	if strings.TrimSpace(sess.ID) == "" {
		sess.ID = store.GenNewID().String()
	}
	now := time.Now()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = now
	}
	if !store.ValidTerminalSessionStatus(sess.Status) {
		sess.Status = store.TerminalStatusRunning
	}
	var tenantID any
	if sess.TenantID != nil && *sess.TenantID != "" {
		tid, err := uuid.Parse(*sess.TenantID)
		if err != nil {
			return fmt.Errorf("terminal tenant_id: %w", err)
		}
		tenantID = tid
	}
	wid, err := uuid.Parse(sess.WorkspaceID)
	if err != nil {
		return fmt.Errorf("terminal workspace_id %q: %w", sess.WorkspaceID, err)
	}
	var exitCode any
	if sess.ExitCode != nil {
		exitCode = *sess.ExitCode
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO terminal_sessions
		 (id, tenant_id, user_id, workspace_id, cwd, shell, status, exit_code,
		  created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		uuid.Must(uuid.Parse(sess.ID)), tenantID, sess.UserID, wid,
		sess.CWD, sess.Shell, sess.Status, exitCode,
		sess.CreatedAt, sess.UpdatedAt)
	return err
}

func (s *PGTerminalStore) GetSession(ctx context.Context, id string) (*store.TerminalSession, error) {
	tid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("terminal session id %q: %w", id, err)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+terminalColumns+` FROM terminal_sessions WHERE id = $1`, tid)
	return scanTerminalSession(row)
}

func (s *PGTerminalStore) ListSessions(ctx context.Context, tenantID *string, userID string, workspaceID string) ([]*store.TerminalSession, error) {
	where := "user_id = $1"
	args := []any{userID}
	if tenantID != nil && *tenantID != "" {
		tid, err := uuid.Parse(*tenantID)
		if err != nil {
			return nil, fmt.Errorf("terminal tenant_id %q: %w", *tenantID, err)
		}
		args = append(args, tid)
		where += fmt.Sprintf(" AND tenant_id = $%d", len(args))
	} else {
		where += " AND tenant_id IS NULL"
	}
	if workspaceID != "" {
		wid, err := uuid.Parse(workspaceID)
		if err != nil {
			return nil, fmt.Errorf("terminal workspace_id %q: %w", workspaceID, err)
		}
		args = append(args, wid)
		where += fmt.Sprintf(" AND workspace_id = $%d", len(args))
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+terminalColumns+` FROM terminal_sessions WHERE `+where+
			` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*store.TerminalSession
	for rows.Next() {
		sess, err := scanTerminalSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// UpdateStatus transitions the lifecycle status with an explicit set of
// status + exit_code + updated_at (no COALESCE: callers always know both).
// Zero affected rows → wrapped sql.ErrNoRows, matching UpdateWorkspace.
func (s *PGTerminalStore) UpdateStatus(ctx context.Context, id, status string, exitCode *int) error {
	tid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("terminal session id %q: %w", id, err)
	}
	now := time.Now()
	var code any
	if exitCode != nil {
		code = *exitCode
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE terminal_sessions SET status = $2, exit_code = $3, updated_at = $4
		 WHERE id = $1`,
		tid, status, code, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("terminal session not found: %w", sql.ErrNoRows)
	}
	return nil
}
