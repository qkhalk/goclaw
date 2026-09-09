package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGApprovalStore implements store.ApprovalStore backed by PostgreSQL.
// payload is stored as JSONB; nullable decision fields use nil pointers.
type PGApprovalStore struct {
	db *sql.DB
}

// NewPGApprovalStore creates a PG-backed approval store.
func NewPGApprovalStore(db *sql.DB) *PGApprovalStore {
	return &PGApprovalStore{db: db}
}

// nilBool converts a Go bool to a nullable pointer for nullable boolean
// columns. False maps to NULL (zero value) to keep the column unset until a
// resolve actually happens.
func nilBool(v bool) *bool {
	if !v {
		return nil
	}
	return &v
}

// derefBool safely dereferences a nullable boolean pointer.
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// CreateRequest inserts a new approval request. Status defaults to pending;
// the tenant comes from the context tenant (master fallback).
func (s *PGApprovalStore) CreateRequest(ctx context.Context, req *store.ApprovalRequest) error {
	if req.ID == uuid.Nil {
		req.ID = store.GenNewID()
	}
	now := time.Now()
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}
	if req.Status == "" {
		req.Status = store.ApprovalStatusPending
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 120
	}
	req.TenantID = tenantIDForInsert(ctx)

	// payload is a NOT NULL column; normalize nil to "{}".
	payload := jsonOrEmpty(req.Payload)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO approval_requests (id, tenant_id, agent_id, requester_id, requester_type, action_type, payload, command, status, decision, decided_by, allow_once, allow_always, session_key, args_digest, grant_expires_at, created_at, decided_at, expired_at, timeout_seconds)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`,
		req.ID, req.TenantID, nilUUID(req.AgentID), nilUUID(req.RequesterID), req.RequesterType,
		req.ActionType, payload, nilStr(req.Command), req.Status, nilStr(req.Decision),
		nilUUID(req.DecidedBy), nilBool(req.AllowOnce), nilBool(req.AllowAlways),
		req.SessionKey, req.ArgsDigest, nilTime(req.GrantExpiresAt),
		req.CreatedAt, nilTime(req.DecidedAt), nilTime(req.ExpiredAt), req.TimeoutSeconds,
	)
	if err != nil {
		return fmt.Errorf("create approval request: %w", err)
	}
	return nil
}

// approvalSelectCols is the shared SELECT column list for approval_requests
// reads. Order must match approvalRow.scan targets exactly.
const approvalSelectCols = `id, tenant_id, agent_id, requester_id, requester_type, action_type, payload::text, command, status, decision, decided_by, allow_once, allow_always, session_key, args_digest, grant_expires_at, created_at, decided_at, expired_at, timeout_seconds`

// ListPending returns pending (unresolved) requests for the given tenant,
// oldest first.
func (s *PGApprovalStore) ListPending(ctx context.Context, tenantID uuid.UUID) ([]store.ApprovalRequest, error) {
	if tenantID == uuid.Nil {
		tenantID = tenantIDForInsert(ctx)
	}
	q := `SELECT ` + approvalSelectCols + `
		 FROM approval_requests
		 WHERE tenant_id = $1 AND status = $2
		 ORDER BY created_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, q, tenantID, store.ApprovalStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []store.ApprovalRequest
	for rows.Next() {
		var r approvalRow
		if err := scanApprovalRow(rows.Scan, &r); err != nil {
			return nil, err
		}
		items = append(items, r.toStore())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []store.ApprovalRequest{}
	}
	return items, nil
}

// ListPendingAll returns pending requests across ALL tenants, oldest first.
// Cross-tenant by design (startup reinstatement); callers must carry each
// row's TenantID forward.
func (s *PGApprovalStore) ListPendingAll(ctx context.Context) ([]store.ApprovalRequest, error) {
	q := `SELECT ` + approvalSelectCols + `
		 FROM approval_requests
		 WHERE status = $1
		 ORDER BY created_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, q, store.ApprovalStatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []store.ApprovalRequest
	for rows.Next() {
		var r approvalRow
		if err := scanApprovalRow(rows.Scan, &r); err != nil {
			return nil, err
		}
		items = append(items, r.toStore())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []store.ApprovalRequest{}
	}
	return items, nil
}

// ResolveWithScope transitions a pending request to a terminal state, scoped
// to the context tenant. A second resolve of the same row returns
// ErrApprovalAlreadyResolved.
func (s *PGApprovalStore) ResolveWithScope(ctx context.Context, id uuid.UUID, decision string, decidedBy *uuid.UUID, grant store.ApprovalGrant) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	status := store.ApprovalStatusApproved
	if decision == store.ApprovalDecisionDeny {
		status = store.ApprovalStatusDenied
	}
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE approval_requests
		 SET status = $3, decision = $4, decided_by = $5, allow_once = $6, allow_always = $7, session_key = $8, grant_expires_at = $9, decided_at = $10
		 WHERE id = $1 AND tenant_id = $2 AND status = $11`,
		id, tid, status, decision, nilUUID(decidedBy), nilBool(grant.AllowOnce), nilBool(grant.AllowAlways),
		grant.SessionKey, nilTime(grant.ExpiresAt), now, store.ApprovalStatusPending)
	if err != nil {
		return fmt.Errorf("resolve approval request: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("resolve approval request: rows affected: %w", err)
	}
	if affected == 0 {
		return store.ErrApprovalAlreadyResolved
	}
	return nil
}

// GetByID returns one request by id, scoped to the context tenant. Returns
// (nil, nil) when the row is missing or belongs to another tenant.
func (s *PGApprovalStore) GetByID(ctx context.Context, id uuid.UUID) (*store.ApprovalRequest, error) {
	where, args := buildApprovalGetWhere(ctx, id)
	q := `SELECT ` + approvalSelectCols + `
		 FROM approval_requests` + where
	var r approvalRow
	if err := scanApprovalRow(s.db.QueryRowContext(ctx, q, args...).Scan, &r); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	got := r.toStore()
	return &got, nil
}

// MarkExpired transitions a pending request to expired, scoped to the context
// tenant. Already-terminal rows are left untouched.
func (s *PGApprovalStore) MarkExpired(ctx context.Context, id uuid.UUID) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE approval_requests SET status = $3, decided_at = $4 WHERE id = $1 AND tenant_id = $2 AND status = $5`,
		id, tid, store.ApprovalStatusExpired, time.Now(), store.ApprovalStatusPending)
	if err != nil {
		return fmt.Errorf("mark approval expired: %w", err)
	}
	return nil
}

// ListHistory returns resolved requests for the given tenant, newest first,
// optionally filtered by status.
func (s *PGApprovalStore) ListHistory(ctx context.Context, tenantID uuid.UUID, opts store.ApprovalListOpts) ([]store.ApprovalRequest, error) {
	if tenantID == uuid.Nil {
		tenantID = tenantIDForInsert(ctx)
	}
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var conditions []string
	var args []any
	argIdx := 1
	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, tenantID)
	argIdx++
	if opts.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, opts.Status)
		argIdx++
	}
	args = append(args, limit, opts.Offset)
	q := `SELECT ` + approvalSelectCols + `
		 FROM approval_requests
		 WHERE ` + strings.Join(conditions, " AND ") +
		` ORDER BY created_at DESC, id DESC` +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []store.ApprovalRequest
	for rows.Next() {
		var r approvalRow
		if err := scanApprovalRow(rows.Scan, &r); err != nil {
			return nil, err
		}
		items = append(items, r.toStore())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []store.ApprovalRequest{}
	}
	return items, nil
}

// buildApprovalGetWhere scopes a single-row read. Fails closed (WHERE 1=0)
// when a tenant ID is required but absent from the context.
func buildApprovalGetWhere(ctx context.Context, id uuid.UUID) (string, []any) {
	if !store.IsCrossTenant(ctx) {
		tenantID := store.TenantIDFromContext(ctx)
		if tenantID == uuid.Nil {
			return " WHERE 1=0", nil
		}
		return " WHERE id = $1 AND tenant_id = $2", []any{id, tenantID}
	}
	return " WHERE id = $1", []any{id}
}

type approvalRow struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	AgentID        *uuid.UUID
	RequesterID    *uuid.UUID
	RequesterType  *string
	ActionType     string
	Payload        *string
	Command        *string
	Status         string
	Decision       *string
	DecidedBy      *uuid.UUID
	AllowOnce      *bool
	AllowAlways    *bool
	SessionKey     *string
	ArgsDigest     *string
	GrantExpiresAt *time.Time
	CreatedAt      time.Time
	DecidedAt      *time.Time
	ExpiredAt      *time.Time
	TimeoutSeconds int
}

func scanApprovalRow(scan func(dest ...any) error, r *approvalRow) error {
	return scan(
		&r.ID, &r.TenantID, &r.AgentID, &r.RequesterID, &r.RequesterType, &r.ActionType,
		&r.Payload, &r.Command, &r.Status, &r.Decision, &r.DecidedBy,
		&r.AllowOnce, &r.AllowAlways, &r.SessionKey, &r.ArgsDigest, &r.GrantExpiresAt,
		&r.CreatedAt, &r.DecidedAt, &r.ExpiredAt, &r.TimeoutSeconds,
	)
}

func (r approvalRow) toStore() store.ApprovalRequest {
	req := store.ApprovalRequest{
		ID:             r.ID,
		TenantID:       r.TenantID,
		AgentID:        r.AgentID,
		RequesterID:    r.RequesterID,
		RequesterType:  derefStr(r.RequesterType),
		ActionType:     r.ActionType,
		Payload:        nil,
		Command:        derefStr(r.Command),
		Status:         r.Status,
		Decision:       derefStr(r.Decision),
		DecidedBy:      r.DecidedBy,
		AllowOnce:      derefBool(r.AllowOnce),
		AllowAlways:    derefBool(r.AllowAlways),
		SessionKey:     derefStr(r.SessionKey),
		ArgsDigest:     derefStr(r.ArgsDigest),
		GrantExpiresAt: r.GrantExpiresAt,
		CreatedAt:      r.CreatedAt,
		DecidedAt:      r.DecidedAt,
		ExpiredAt:      r.ExpiredAt,
		TimeoutSeconds: r.TimeoutSeconds,
	}
	if r.Payload != nil {
		req.Payload = []byte(*r.Payload)
	}
	return req
}