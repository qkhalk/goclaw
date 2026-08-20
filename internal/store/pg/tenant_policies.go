package pg

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

// PGTenantPolicyStore implements store.TenantPolicyStore backed by PostgreSQL.
// One row per tenant; max_* NULL means "no cap"; empty text[] means "no
// allowlist restriction".
type PGTenantPolicyStore struct {
	db *sql.DB
}

// NewPGTenantPolicyStore creates a PG-backed tenant policy store.
func NewPGTenantPolicyStore(db *sql.DB) *PGTenantPolicyStore {
	return &PGTenantPolicyStore{db: db}
}

const tenantPolicyCols = `id, tenant_id, quota::text AS quota, allowed_providers, allowed_models, max_agents, max_sessions, max_teams, status, updated_at`

// GetTenantPolicy returns the policy for tenantID, or ErrTenantPolicyNotFound
// when none is configured.
func (s *PGTenantPolicyStore) GetTenantPolicy(ctx context.Context, tenantID uuid.UUID) (*store.TenantPolicy, error) {
	var row tenantPolicyRow
	err := pkgSqlxDB.GetContext(ctx, &row,
		`SELECT `+tenantPolicyCols+` FROM tenant_policies WHERE tenant_id = $1`,
		tenantID)
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
func (s *PGTenantPolicyStore) UpsertTenantPolicy(ctx context.Context, policy *store.TenantPolicy) error {
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
		 VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   quota = EXCLUDED.quota,
		   allowed_providers = EXCLUDED.allowed_providers,
		   allowed_models = EXCLUDED.allowed_models,
		   max_agents = EXCLUDED.max_agents,
		   max_sessions = EXCLUDED.max_sessions,
		   max_teams = EXCLUDED.max_teams,
		   status = EXCLUDED.status,
		   updated_at = EXCLUDED.updated_at`,
		policy.ID, policy.TenantID, []byte(policy.Quota),
		pqStringArray(policy.AllowedProviders), pqStringArray(policy.AllowedModels),
		policy.MaxAgents, policy.MaxSessions, policy.MaxTeams,
		policy.Status, policy.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert tenant policy: %w", err)
	}
	return nil
}

// DeleteTenantPolicy removes the policy for tenantID.
func (s *PGTenantPolicyStore) DeleteTenantPolicy(ctx context.Context, tenantID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tenant_policies WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return fmt.Errorf("delete tenant policy: %w", err)
	}
	return nil
}

// ListTenantPolicies returns all tenant policies (system/owner scope).
func (s *PGTenantPolicyStore) ListTenantPolicies(ctx context.Context) ([]store.TenantPolicy, error) {
	var rows []tenantPolicyRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT `+tenantPolicyCols+` FROM tenant_policies ORDER BY tenant_id`); err != nil {
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
func (s *PGTenantPolicyStore) CheckTenantActive(ctx context.Context, tenantID uuid.UUID) error {
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

// CheckCanCreateAgent/Session/Team enforce the max_* caps. They return nil when
// no cap is configured or the existing count is below the limit, and a
// *store.TenantLimitError when the cap is already reached.
func (s *PGTenantPolicyStore) CheckCanCreateAgent(ctx context.Context, tenantID uuid.UUID) error {
	return s.checkCap(ctx, tenantID, store.CapAgents, `SELECT COUNT(*) FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL`)
}

func (s *PGTenantPolicyStore) CheckCanCreateSession(ctx context.Context, tenantID uuid.UUID) error {
	return s.checkCap(ctx, tenantID, store.CapSessions, `SELECT COUNT(*) FROM sessions WHERE tenant_id = $1`)
}

func (s *PGTenantPolicyStore) CheckCanCreateTeam(ctx context.Context, tenantID uuid.UUID) error {
	return s.checkCap(ctx, tenantID, store.CapTeams, `SELECT COUNT(*) FROM agent_teams WHERE tenant_id = $1`)
}

// checkCap loads the policy and, when the cap is configured, compares the
// existing count scoped to tenantID against the limit.
func (s *PGTenantPolicyStore) checkCap(ctx context.Context, tenantID uuid.UUID, cap store.CapName, countQuery string) error {
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
	if err := s.db.QueryRowContext(ctx, countQuery, tenantID).Scan(&existing); err != nil {
		return fmt.Errorf("tenant policy count %s: %w", cap, err)
	}
	if existing >= limit {
		return &store.TenantLimitError{Cap: cap, Limit: limit, Existing: existing}
	}
	return nil
}

type tenantPolicyRow struct {
	ID               uuid.UUID  `db:"id"`
	TenantID         uuid.UUID  `db:"tenant_id"`
	Quota            []byte     `db:"quota"`
	AllowedProviders []byte     `db:"allowed_providers"`
	AllowedModels    []byte     `db:"allowed_models"`
	MaxAgents        *int       `db:"max_agents"`
	MaxSessions      *int       `db:"max_sessions"`
	MaxTeams         *int       `db:"max_teams"`
	Status           string     `db:"status"`
	UpdatedAt        time.Time  `db:"updated_at"`
}

func (r tenantPolicyRow) toStore() store.TenantPolicy {
	var providers, models []string
	scanStringArray(r.AllowedProviders, &providers)
	scanStringArray(r.AllowedModels, &models)
	if providers == nil {
		providers = []string{}
	}
	if models == nil {
		models = []string{}
	}
	return store.TenantPolicy{
		ID:               r.ID,
		TenantID:         r.TenantID,
		Quota:            json.RawMessage(r.Quota),
		AllowedProviders: providers,
		AllowedModels:    models,
		MaxAgents:        r.MaxAgents,
		MaxSessions:      r.MaxSessions,
		MaxTeams:         r.MaxTeams,
		Status:           r.Status,
		UpdatedAt:        r.UpdatedAt,
	}
}