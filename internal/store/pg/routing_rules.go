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

// PGRoutingRulesStore implements store.RoutingRulesStore backed by
// PostgreSQL. All queries are tenant-scoped and parameterized; the eval
// index (tenant_id, enabled, priority, created_at) covers ListRules.
type PGRoutingRulesStore struct {
	db *sql.DB
}

func NewPGRoutingRulesStore(db *sql.DB) *PGRoutingRulesStore {
	return &PGRoutingRulesStore{db: db}
}

const routingRuleColumns = `r.id, r.tenant_id, r.priority, r.match_config,
 r.target_agent_id, r.enabled, r.created_at, r.updated_at, a.agent_key`

func scanRoutingRule(row interface{ Scan(...any) error }) (*store.RoutingRule, error) {
	var r store.RoutingRule
	var id, tenantID, agentID uuid.UUID
	var match []byte
	var agentKey sql.NullString
	if err := row.Scan(&id, &tenantID, &r.Priority, &match,
		&agentID, &r.Enabled, &r.CreatedAt, &r.UpdatedAt, &agentKey); err != nil {
		return nil, err
	}
	r.ID = id.String()
	r.TenantID = tenantID.String()
	r.TargetAgentID = agentID.String()
	r.TargetAgentKey = agentKey.String
	if len(match) > 0 {
		if err := json.Unmarshal(match, &r.Match); err != nil {
			return nil, fmt.Errorf("routing rule match_config: %w", err)
		}
	}
	return &r, nil
}

// ListRules returns the tenant's rules in evaluation order (priority ASC,
// created_at ASC). The agents JOIN resolves the target agent_key so the
// inbound resolution path can route without a second lookup.
func (s *PGRoutingRulesStore) ListRules(ctx context.Context, tenantID string) ([]*store.RoutingRule, error) {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("routing rules tenant_id %q: %w", tenantID, err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+routingRuleColumns+`
		 FROM routing_rules r
		 LEFT JOIN agents a ON a.id = r.target_agent_id
		 WHERE r.tenant_id = $1
		 ORDER BY r.priority ASC, r.created_at ASC`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*store.RoutingRule, 0, 8)
	for rows.Next() {
		rule, err := scanRoutingRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// UpsertRule inserts a rule (ID defaulted to a fresh UUIDv7 when empty) or
// updates an existing row scoped to the same tenant.
func (s *PGRoutingRulesStore) UpsertRule(ctx context.Context, rule *store.RoutingRule) error {
	if rule == nil {
		return errors.New("routing rule is nil")
	}
	tid, err := uuid.Parse(rule.TenantID)
	if err != nil {
		return fmt.Errorf("routing rule tenant_id %q: %w", rule.TenantID, err)
	}
	agentID, err := uuid.Parse(rule.TargetAgentID)
	if err != nil {
		return fmt.Errorf("routing rule target_agent_id %q: %w", rule.TargetAgentID, err)
	}
	now := time.Now()
	matchJSON, err := json.Marshal(rule.Match)
	if err != nil {
		return fmt.Errorf("routing rule match marshal: %w", err)
	}

	var id uuid.UUID
	if rule.ID == "" {
		id = store.GenNewID()
	} else {
		id, err = uuid.Parse(rule.ID)
		if err != nil {
			return fmt.Errorf("routing rule id %q: %w", rule.ID, err)
		}
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO routing_rules
		 (id, tenant_id, priority, match_config, target_agent_id, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $7)
		 ON CONFLICT (id) DO UPDATE SET
		   priority        = EXCLUDED.priority,
		   match_config    = EXCLUDED.match_config,
		   target_agent_id = EXCLUDED.target_agent_id,
		   enabled         = EXCLUDED.enabled,
		   updated_at      = EXCLUDED.updated_at
		 WHERE routing_rules.tenant_id = EXCLUDED.tenant_id`,
		id, tid, rule.Priority, string(matchJSON), agentID, rule.Enabled, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Existing row belongs to another tenant — never cross the boundary.
		return store.ErrRoutingRuleNotFound
	}
	rule.ID = id.String()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	return nil
}

// DeleteRule removes one rule scoped to the tenant. Unknown IDs return
// store.ErrRoutingRuleNotFound.
func (s *PGRoutingRulesStore) DeleteRule(ctx context.Context, tenantID, id string) error {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("routing rules tenant_id %q: %w", tenantID, err)
	}
	rid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("routing rule id %q: %w", id, err)
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM routing_rules WHERE id = $1 AND tenant_id = $2`, rid, tid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrRoutingRuleNotFound
	}
	return nil
}
