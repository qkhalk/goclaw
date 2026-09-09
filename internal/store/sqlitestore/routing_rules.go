//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteRoutingRulesStore implements store.RoutingRulesStore backed by
// SQLite (desktop/lite edition). All queries are tenant-scoped and
// parameterized; the eval index covers ListRules.
type SQLiteRoutingRulesStore struct {
	db *sql.DB
}

func NewSQLiteRoutingRulesStore(db *sql.DB) *SQLiteRoutingRulesStore {
	return &SQLiteRoutingRulesStore{db: db}
}

const routingRuleColumns = `r.id, r.tenant_id, r.priority, r.match_config,
 r.target_agent_id, r.enabled, r.created_at, r.updated_at, a.agent_key`

func scanRoutingRule(row interface{ Scan(...any) error }) (*store.RoutingRule, error) {
	var r store.RoutingRule
	var match string
	var agentKey sql.NullString
	// Timestamps are TEXT in SQLite (modernc driver returns strings); scan
	// through sqliteTime so RFC3339 text round-trips into time.Time.
	var createdAt, updatedAt sqliteTime
	if err := row.Scan(&r.ID, &r.TenantID, &r.Priority, &match,
		&r.TargetAgentID, &r.Enabled, &createdAt, &updatedAt, &agentKey); err != nil {
		return nil, err
	}
	r.TargetAgentKey = agentKey.String
	r.CreatedAt = createdAt.Time
	r.UpdatedAt = updatedAt.Time
	if match != "" {
		if err := json.Unmarshal([]byte(match), &r.Match); err != nil {
			return nil, fmt.Errorf("routing rule match_config: %w", err)
		}
	}
	return &r, nil
}

// ListRules returns the tenant's rules in evaluation order (priority ASC,
// created_at ASC) with the target agent_key resolved via LEFT JOIN.
func (s *SQLiteRoutingRulesStore) ListRules(ctx context.Context, tenantID string) ([]*store.RoutingRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+routingRuleColumns+`
		 FROM routing_rules r
		 LEFT JOIN agents a ON a.id = r.target_agent_id
		 WHERE r.tenant_id = ?1
		 ORDER BY r.priority ASC, r.created_at ASC`, tenantID)
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

// UpsertRule inserts a rule (ID defaulted when empty) or updates an existing
// row scoped to the same tenant.
func (s *SQLiteRoutingRulesStore) UpsertRule(ctx context.Context, rule *store.RoutingRule) error {
	if rule == nil {
		return errors.New("routing rule is nil")
	}
	if rule.TenantID == "" {
		return errors.New("routing rule tenant_id is required")
	}
	if rule.TargetAgentID == "" {
		return errors.New("routing rule target_agent_id is required")
	}
	if rule.ID == "" {
		rule.ID = store.GenNewID().String()
	}
	now := time.Now().UTC()
	matchJSON, err := json.Marshal(rule.Match)
	if err != nil {
		return fmt.Errorf("routing rule match marshal: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO routing_rules
		 (id, tenant_id, priority, match_config, target_agent_id, enabled, created_at, updated_at)
		 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?7)
		 ON CONFLICT(id) DO UPDATE SET
		   priority        = excluded.priority,
		   match_config    = excluded.match_config,
		   target_agent_id = excluded.target_agent_id,
		   enabled         = excluded.enabled,
		   updated_at      = excluded.updated_at
		 WHERE routing_rules.tenant_id = excluded.tenant_id`,
		rule.ID, rule.TenantID, rule.Priority, string(matchJSON),
		rule.TargetAgentID, rule.Enabled, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Existing row belongs to another tenant — never cross the boundary.
		return store.ErrRoutingRuleNotFound
	}
	rule.CreatedAt = now
	rule.UpdatedAt = now
	return nil
}

// DeleteRule removes one rule scoped to the tenant. Unknown IDs return
// store.ErrRoutingRuleNotFound.
func (s *SQLiteRoutingRulesStore) DeleteRule(ctx context.Context, tenantID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM routing_rules WHERE id = ?1 AND tenant_id = ?2`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrRoutingRuleNotFound
	}
	return nil
}
