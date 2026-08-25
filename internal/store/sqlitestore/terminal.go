//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteTerminalStore implements store.TerminalStore backed by SQLite
// (desktop/lite edition).
type SQLiteTerminalStore struct {
	db *sql.DB
}

func NewSQLiteTerminalStore(db *sql.DB) *SQLiteTerminalStore {
	return &SQLiteTerminalStore{db: db}
}

const terminalColumns = `id, tenant_id, user_id, workspace_id, cwd, shell,
 status, exit_code, created_at, updated_at`

func scanTerminalSession(row interface{ Scan(...any) error }) (*store.TerminalSession, error) {
	var s store.TerminalSession
	var tenantID sql.NullString
	var exitCode sql.NullInt64
	// Timestamps are TEXT in SQLite (modernc driver returns strings); scan
	// through sqliteTime so RFC3339 text round-trips into time.Time.
	var createdAt, updatedAt sqliteTime
	if err := row.Scan(&s.ID, &tenantID, &s.UserID, &s.WorkspaceID,
		&s.CWD, &s.Shell, &s.Status, &exitCode,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	s.TenantID = nilStrPtr(tenantID)
	if exitCode.Valid {
		code := int(exitCode.Int64)
		s.ExitCode = &code
	}
	s.CreatedAt = createdAt.Time
	s.UpdatedAt = updatedAt.Time
	return &s, nil
}

// CreateSession inserts a new terminal session row. ID defaults to a fresh
// UUID; timestamps and status are defaulted when zero-valued.
func (s *SQLiteTerminalStore) CreateSession(ctx context.Context, sess *store.TerminalSession) error {
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
	var tenantArg any
	if sess.TenantID != nil && *sess.TenantID != "" {
		tenantArg = *sess.TenantID
	}
	var exitCode any
	if sess.ExitCode != nil {
		exitCode = *sess.ExitCode
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO terminal_sessions
		 (id, tenant_id, user_id, workspace_id, cwd, shell, status, exit_code,
		  created_at, updated_at)
		 VALUES (?1,?2,?3,?4,?5,?6,?7,?8,?9,?10)`,
		sess.ID, tenantArg, sess.UserID, sess.WorkspaceID,
		sess.CWD, sess.Shell, sess.Status, exitCode,
		sess.CreatedAt.UTC(), sess.UpdatedAt.UTC())
	return err
}

func (s *SQLiteTerminalStore) GetSession(ctx context.Context, id string) (*store.TerminalSession, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+terminalColumns+` FROM terminal_sessions WHERE id = ?1`, id)
	return scanTerminalSession(row)
}

func (s *SQLiteTerminalStore) ListSessions(ctx context.Context, tenantID *string, userID string, workspaceID string) ([]*store.TerminalSession, error) {
	where := "user_id = ?1"
	args := []any{userID}
	if tenantID != nil && *tenantID != "" {
		args = append(args, *tenantID)
		where += fmt.Sprintf(" AND tenant_id = ?%d", len(args))
	} else {
		where += " AND tenant_id IS NULL"
	}
	if workspaceID != "" {
		args = append(args, workspaceID)
		where += fmt.Sprintf(" AND workspace_id = ?%d", len(args))
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
// status + exit_code + updated_at. Zero affected rows → wrapped
// sql.ErrNoRows, matching UpdateWorkspace.
func (s *SQLiteTerminalStore) UpdateStatus(ctx context.Context, id, status string, exitCode *int) error {
	now := time.Now().UTC()
	var code any
	if exitCode != nil {
		code = *exitCode
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE terminal_sessions SET status = ?2, exit_code = ?3, updated_at = ?4
		 WHERE id = ?1`,
		id, status, code, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("terminal session not found")
	}
	return nil
}
