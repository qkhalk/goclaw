//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteTenantPolicyStore implements store.TenantPolicyStore backed by SQLite.
// Arrays are stored as JSON text; nullable max_* as NULL.
type SQLiteTenantPolicyStore struct {
	db *sql.DB
}

// NewSQLiteTenantPolicyStore creates a SQLite-backed tenant policy store.
func NewSQLiteTenantPolicyStore(db *sql.DB) *SQLiteTenantPolicyStore {
	return &SQLiteTenantPolicyStore{db: db}
}

const sqliteTenantPolicyCols = `id, tenant_id, quota, allowed_providers, allowed_models, max_agents, max_sessions, max_teams, status, updated_at`

// GetTenantPolicy returns the policy for tenantID, or ErrTenantPolicyNotFound
// when none is configured.
func (s *SQLiteTenantPolicyStore) GetTenantPolicy(ctx context.Context, tenantID uuid.UUID) (*store.TenantPolicy, error) {
	var row tenantPolicyRow
	err := pkgSqlxDB.GetContext(ctx, &row,
		`SELECT `+sqliteTenantPolicyCols+` FROM tenant_policies WHERE tenant_id = ?`,
		tenantID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrTenantPolicyNotFound
		}
		return nil, err
	}
	got := row.toStore()
	return &got, nil
}

// UpsertTenantPolicy inserts or replaces the policy for its tenant.
func (s *SQLiteTenantPolicyStore) UpsertTenantPolicy(ctx context.Context, policy *store.TenantPolicy) error {
	if policy.ID == uuid.Nil {
		policy.ID = store.GenNewID()
	}
	if policy.Status == "" {
		policy.Status = store.TenantPolicyStatusActive
	}
	if !store.ValidTenantPolicyStatus(policy.Status) {
		return fmt.Errorf("upsert tenant policy: unknown status %q", policy.Status)
	}
	if policy.AllowedProviders == nil {
		policy.AllowedProviders = []string{}
	}
	if policy.AllowedModels == nil {
		policy.AllowedModels = []string{}
	}
	if len(policy.Quota) == 0 {
		policy.Quota = json.RawMessage(`{}`)
	}
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tenant_policies (id, tenant_id, quota, allowed_providers, allowed_models, max_agents, max_sessions, max_teams, status, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   quota = excluded.quota,
		   allowed_providers = excluded.allowed_providers,
		   allowed_models = excluded.allowed_models,
		   max_agents = excluded.max_agents,
		   max_sessions = excluded.max_sessions,
		   max_teams = excluded.max_teams,
		   status = excluded.status,
		   updated_at = excluded.updated_at`,
		policy.ID.String(), policy.TenantID.String(), string(policy.Quota),
		jsonStringArray(policy.AllowedProviders), jsonStringArray(policy.AllowedModels),
		policy.MaxAgents, policy.MaxSessions, policy.MaxTeams,
		policy.Status, policy.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert tenant policy: %w", err)
	}
	return nil
}

// DeleteTenantPolicy removes the policy for tenantID.
func (s *SQLiteTenantPolicyStore) DeleteTenantPolicy(ctx context.Context, tenantID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tenant_policies WHERE tenant_id = ?`, tenantID.String())
	if err != nil {
		return fmt.Errorf("delete tenant policy: %w", err)
	}
	return nil
}

// ListTenantPolicies returns all tenant policies (system/owner scope).
func (s *SQLiteTenantPolicyStore) ListTenantPolicies(ctx context.Context) ([]store.TenantPolicy, error) {
	var rows []tenantPolicyRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT `+sqliteTenantPolicyCols+` FROM tenant_policies ORDER BY tenant_id`); err != nil {
		return nil, err
	}
	items := make([]store.TenantPolicy, len(rows))
	for i, row := range rows {
		items[i] = row.toStore()
	}
	return items, nil
}

// CheckTenantActive returns ErrTenantSuspended when a policy row exists and its
// status is not active. No policy row is treated as active (fast path).
func (s *SQLiteTenantPolicyStore) CheckTenantActive(ctx context.Context, tenantID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return nil
	}
	policy, err := s.GetTenantPolicy(ctx, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrTenantPolicyNotFound) {
			return nil
		}
		return err
	}
	if policy.Suspended() {
		return store.ErrTenantSuspended
	}
	return nil
}

// CheckCanCreateAgent/Session/Team enforce the max_* caps.
func (s *SQLiteTenantPolicyStore) CheckCanCreateAgent(ctx context.Context, tenantID uuid.UUID) error {
	return s.checkCap(ctx, tenantID, store.CapAgents, `SELECT COUNT(*) FROM agents WHERE tenant_id = ? AND deleted_at IS NULL`)
}

func (s *SQLiteTenantPolicyStore) CheckCanCreateSession(ctx context.Context, tenantID uuid.UUID) error {
	return s.checkCap(ctx, tenantID, store.CapSessions, `SELECT COUNT(*) FROM sessions WHERE tenant_id = ?`)
}

func (s *SQLiteTenantPolicyStore) CheckCanCreateTeam(ctx context.Context, tenantID uuid.UUID) error {
	return s.checkCap(ctx, tenantID, store.CapTeams, `SELECT COUNT(*) FROM agent_teams WHERE tenant_id = ?`)
}

// checkCap loads the policy and, when the cap is configured, compares the
// existing count scoped to tenantID against the limit.
func (s *SQLiteTenantPolicyStore) checkCap(ctx context.Context, tenantID uuid.UUID, cap store.CapName, countQuery string) error {
	if tenantID == uuid.Nil {
		return nil
	}
	policy, err := s.GetTenantPolicy(ctx, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrTenantPolicyNotFound) {
			return nil
		}
		return err
	}
	limit := -1
	switch cap {
	case store.CapAgents:
		if policy.MaxAgents != nil {
			limit = *policy.MaxAgents
		}
	case store.CapSessions:
		if policy.MaxSessions != nil {
			limit = *policy.MaxSessions
		}
	case store.CapTeams:
		if policy.MaxTeams != nil {
			limit = *policy.MaxTeams
		}
	}
	if limit < 0 {
		return nil // no cap configured
	}
	var existing int
	if err := s.db.QueryRowContext(ctx, countQuery, tenantID.String()).Scan(&existing); err != nil {
		return fmt.Errorf("tenant policy count %s: %w", cap, err)
	}
	if existing >= limit {
		return &store.TenantLimitError{Cap: cap, Limit: limit, Existing: existing}
	}
	return nil
}

type tenantPolicyRow struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	Quota            sql.NullString
	AllowedProviders sql.NullString
	AllowedModels    sql.NullString
	MaxAgents        *int
	MaxSessions      *int
	MaxTeams         *int
	Status           string
	UpdatedAt        sqliteTime
}

func (r tenantPolicyRow) toStore() store.TenantPolicy {
	var providers, models []string
	scanJSONStringArray([]byte(r.AllowedProviders.String), &providers)
	scanJSONStringArray([]byte(r.AllowedModels.String), &models)
	if providers == nil {
		providers = []string{}
	}
	if models == nil {
		models = []string{}
	}
	return store.TenantPolicy{
		ID:               r.ID,
		TenantID:         r.TenantID,
		Quota:            json.RawMessage(r.Quota.String),
		AllowedProviders: providers,
		AllowedModels:    models,
		MaxAgents:        r.MaxAgents,
		MaxSessions:      r.MaxSessions,
		MaxTeams:         r.MaxTeams,
		Status:           r.Status,
		UpdatedAt:        r.UpdatedAt.Time,
	}
}