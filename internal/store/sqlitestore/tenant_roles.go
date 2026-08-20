//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteTenantRoleStore implements store.TenantRoleStore backed by SQLite.
// Custom roles live in `roles`; role_permissions and member_role_assignments
// mirror the PG schema (see migrations 000108).
type SQLiteTenantRoleStore struct {
	db *sql.DB
}

// NewSQLiteTenantRoleStore creates a SQLite-backed tenant role store.
func NewSQLiteTenantRoleStore(db *sql.DB) *SQLiteTenantRoleStore {
	return &SQLiteTenantRoleStore{db: db}
}

const sqliteTenantRoleCols = `id, tenant_id, name, description, builtin, created_at`

// ListRoles returns all roles for the context tenant.
func (s *SQLiteTenantRoleStore) ListRoles(ctx context.Context) ([]store.TenantRole, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []tenantRoleRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT `+sqliteTenantRoleCols+` FROM roles WHERE tenant_id = ? ORDER BY builtin DESC, name`,
		tid.String()); err != nil {
		return nil, err
	}
	items := make([]store.TenantRole, len(rows))
	for i, row := range rows {
		items[i] = row.toStore()
	}
	return items, nil
}

// GetRole returns one role by id, scoped to the context tenant.
func (s *SQLiteTenantRoleStore) GetRole(ctx context.Context, id uuid.UUID) (*store.TenantRole, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	var row tenantRoleRow
	if err := pkgSqlxDB.GetContext(ctx, &row,
		`SELECT `+sqliteTenantRoleCols+` FROM roles WHERE id = ? AND tenant_id = ?`,
		id.String(), tid.String()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrRoleNotFound
		}
		return nil, err
	}
	got := row.toStore()
	return &got, nil
}

// CreateRole inserts a custom role for the context tenant.
func (s *SQLiteTenantRoleStore) CreateRole(ctx context.Context, role *store.TenantRole) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if role.ID == uuid.Nil {
		role.ID = store.GenNewID()
	}
	if role.Name == "" {
		return fmt.Errorf("create role: name required")
	}
	if store.IsBuiltinTenantRole(role.Name) {
		return store.ErrBuiltinRoleProtected
	}
	role.TenantID = tid
	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now()
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO roles (id, tenant_id, name, description, builtin, created_at)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		role.ID.String(), tid.String(), role.Name, role.Description,
		role.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

// UpdateRole patches a custom role (name/description), scoped to the context
// tenant. Builtin roles cannot be updated.
func (s *SQLiteTenantRoleStore) UpdateRole(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if name, ok := updates["name"].(string); ok && store.IsBuiltinTenantRole(name) {
		return store.ErrBuiltinRoleProtected
	}
	if err := execMapUpdateWhereTenant(ctx, s.db, "roles", updates, id, tid); err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

// DeleteRole removes a custom role (cascades to its role_permissions and
// member assignments). Builtin roles cannot be deleted.
func (s *SQLiteTenantRoleStore) DeleteRole(ctx context.Context, id uuid.UUID) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	var builtin int
	if err := s.db.QueryRowContext(ctx,
		`SELECT builtin FROM roles WHERE id = ? AND tenant_id = ?`, id.String(), tid.String()).Scan(&builtin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ErrRoleNotFound
		}
		return err
	}
	if builtin != 0 {
		return store.ErrBuiltinRoleProtected
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM roles WHERE id = ? AND tenant_id = ?`, id.String(), tid.String())
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

// SetRolePermissions replaces the permission rows for one role, scoped to the
// context tenant, inside a transaction.
func (s *SQLiteTenantRoleStore) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissions []store.RolePermission) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	if err := s.rolesIsTenant(ctx, roleID, tid); err != nil {
		return err
	}
	for _, p := range permissions {
		if !store.ValidPermissionEffect(p.Effect) {
			return fmt.Errorf("%w: effect %q", store.ErrRolePermissionDenied, p.Effect)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = ?`, roleID.String()); err != nil {
		return err
	}
	for _, p := range permissions {
		if p.ID == uuid.Nil {
			p.ID = store.GenNewID()
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO role_permissions (id, role_id, permission, effect) VALUES (?, ?, ?, ?)`,
			p.ID.String(), roleID.String(), p.Permission, p.Effect); err != nil {
			return fmt.Errorf("set role permission %q: %w", p.Permission, err)
		}
	}
	return tx.Commit()
}

// ListRolePermissions returns the permission rows for one role, scoped to the
// context tenant.
func (s *SQLiteTenantRoleStore) ListRolePermissions(ctx context.Context, roleID uuid.UUID) ([]store.RolePermission, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.rolesIsTenant(ctx, roleID, tid); err != nil {
		return nil, err
	}
	var rows []rolePermissionRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT id, role_id, permission, effect FROM role_permissions WHERE role_id = ? ORDER BY permission`,
		roleID.String()); err != nil {
		return nil, err
	}
	items := make([]store.RolePermission, len(rows))
	for i, row := range rows {
		items[i] = row.toStore()
	}
	return items, nil
}

// AssignRoleToMember grants a custom role to a tenant member.
func (s *SQLiteTenantRoleStore) AssignRoleToMember(ctx context.Context, tenantID uuid.UUID, userID string, roleID uuid.UUID) error {
	if userID == "" {
		return fmt.Errorf("assign role: user_id required")
	}
	if err := s.rolesIsTenant(ctx, roleID, tenantID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO member_role_assignments (tenant_id, user_id, role_id)
		 VALUES (?, ?, ?) ON CONFLICT (tenant_id, user_id, role_id) DO NOTHING`,
		tenantID.String(), userID, roleID.String())
	if err != nil {
		return fmt.Errorf("assign role to member: %w", err)
	}
	return nil
}

// RevokeRoleFromMember removes a custom role from a tenant member.
func (s *SQLiteTenantRoleStore) RevokeRoleFromMember(ctx context.Context, tenantID uuid.UUID, userID string, roleID uuid.UUID) error {
	if userID == "" {
		return fmt.Errorf("revoke role: user_id required")
	}
	if err := s.rolesIsTenant(ctx, roleID, tenantID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM member_role_assignments WHERE tenant_id = ? AND user_id = ? AND role_id = ?`,
		tenantID.String(), userID, roleID.String())
	if err != nil {
		return fmt.Errorf("revoke role from member: %w", err)
	}
	return nil
}

// ListMemberRoles returns all custom role IDs assigned to a tenant member.
func (s *SQLiteTenantRoleStore) ListMemberRoles(ctx context.Context, tenantID uuid.UUID, userID string) ([]uuid.UUID, error) {
	var roleIDs []uuid.UUID
	rows, err := s.db.QueryContext(ctx,
		`SELECT role_id FROM member_role_assignments WHERE tenant_id = ? AND user_id = ? ORDER BY role_id`,
		tenantID.String(), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, err
		}
		if parsed, err := uuid.Parse(idStr); err == nil {
			roleIDs = append(roleIDs, parsed)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roleIDs, nil
}

// ListTenantMemberRoles returns all custom-role assignments for the tenant.
func (s *SQLiteTenantRoleStore) ListTenantMemberRoles(ctx context.Context, tenantID uuid.UUID) ([]store.MemberRoleAssignment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, role_id FROM member_role_assignments WHERE tenant_id = ? ORDER BY user_id`,
		tenantID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []store.MemberRoleAssignment
	for rows.Next() {
		var userID, roleIDStr string
		if err := rows.Scan(&userID, &roleIDStr); err != nil {
			return nil, err
		}
		if parsed, err := uuid.Parse(roleIDStr); err == nil {
			items = append(items, store.MemberRoleAssignment{UserID: userID, RoleID: parsed})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []store.MemberRoleAssignment{}
	}
	return items, nil
}

// rolesIsTenant verifies the role row belongs to tenantID, failing closed.
func (s *SQLiteTenantRoleStore) rolesIsTenant(ctx context.Context, roleID uuid.UUID, tenantID uuid.UUID) error {
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM roles WHERE id = ? AND tenant_id = ?)`,
		roleID.String(), tenantID.String()).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return store.ErrRoleNotFound
	}
	return nil
}

type tenantRoleRow struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Name        string
	Description sql.NullString
	Builtin     int
	CreatedAt   sqliteTime
}

func (r tenantRoleRow) toStore() store.TenantRole {
	var desc *string
	if r.Description.Valid {
		desc = &r.Description.String
	}
	return store.TenantRole{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Description: desc,
		Builtin:     r.Builtin != 0,
		CreatedAt:   r.CreatedAt.Time,
	}
}

type rolePermissionRow struct {
	ID         uuid.UUID
	RoleID     uuid.UUID
	Permission string
	Effect     string
}

func (r rolePermissionRow) toStore() store.RolePermission {
	return store.RolePermission{
		ID:         r.ID,
		RoleID:     r.RoleID,
		Permission: r.Permission,
		Effect:     r.Effect,
	}
}