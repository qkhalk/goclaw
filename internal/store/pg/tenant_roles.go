package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGTenantRoleStore implements store.TenantRoleStore backed by PostgreSQL.
// Custom roles live in `roles`; their grants live in `role_permissions`;
// per-member custom-role assignments live in `member_role_assignments`.
type PGTenantRoleStore struct {
	db *sql.DB
}

// NewPGTenantRoleStore creates a PG-backed tenant role store.
func NewPGTenantRoleStore(db *sql.DB) *PGTenantRoleStore {
	return &PGTenantRoleStore{db: db}
}

const tenantRoleCols = `id, tenant_id, name, description, builtin, created_at`

// ListRoles returns all roles for the context tenant.
func (s *PGTenantRoleStore) ListRoles(ctx context.Context) ([]store.TenantRole, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []tenantRoleRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT `+tenantRoleCols+` FROM roles WHERE tenant_id = $1 ORDER BY builtin DESC, name`,
		tid); err != nil {
		return nil, err
	}
	items := make([]store.TenantRole, len(rows))
	for i, row := range rows {
		items[i] = row.toStore()
	}
	return items, nil
}

// GetRole returns one role by id, scoped to the context tenant.
func (s *PGTenantRoleStore) GetRole(ctx context.Context, id uuid.UUID) (*store.TenantRole, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	var row tenantRoleRow
	if err := pkgSqlxDB.GetContext(ctx, &row,
		`SELECT `+tenantRoleCols+` FROM roles WHERE id = $1 AND tenant_id = $2`,
		id, tid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrRoleNotFound
		}
		return nil, err
	}
	got := row.toStore()
	return &got, nil
}

// CreateRole inserts a custom role for the context tenant.
func (s *PGTenantRoleStore) CreateRole(ctx context.Context, role *store.TenantRole) error {
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
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		role.ID, tid, role.Name, role.Description, false, role.CreatedAt)
	if err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

// UpdateRole patches a custom role (name/description), scoped to the context
// tenant. Builtin roles cannot be updated.
func (s *PGTenantRoleStore) UpdateRole(ctx context.Context, id uuid.UUID, updates map[string]any) error {
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
func (s *PGTenantRoleStore) DeleteRole(ctx context.Context, id uuid.UUID) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return err
	}
	var builtin bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT builtin FROM roles WHERE id = $1 AND tenant_id = $2`, id, tid).Scan(&builtin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ErrRoleNotFound
		}
		return err
	}
	if builtin {
		return store.ErrBuiltinRoleProtected
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM roles WHERE id = $1 AND tenant_id = $2`, id, tid)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

// SetRolePermissions replaces the permission rows for one role, scoped to the
// context tenant, inside a transaction.
func (s *PGTenantRoleStore) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissions []store.RolePermission) error {
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	for _, p := range permissions {
		if p.ID == uuid.Nil {
			p.ID = store.GenNewID()
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO role_permissions (id, role_id, permission, effect) VALUES ($1, $2, $3, $4)`,
			p.ID, roleID, p.Permission, p.Effect); err != nil {
			return fmt.Errorf("set role permission %q: %w", p.Permission, err)
		}
	}
	return tx.Commit()
}

// ListRolePermissions returns the permission rows for one role, scoped to the
// context tenant.
func (s *PGTenantRoleStore) ListRolePermissions(ctx context.Context, roleID uuid.UUID) ([]store.RolePermission, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.rolesIsTenant(ctx, roleID, tid); err != nil {
		return nil, err
	}
	var rows []rolePermissionRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT id, role_id, permission, effect FROM role_permissions WHERE role_id = $1 ORDER BY permission`,
		roleID); err != nil {
		return nil, err
	}
	items := make([]store.RolePermission, len(rows))
	for i, row := range rows {
		items[i] = row.toStore()
	}
	return items, nil
}

// AssignRoleToMember grants a custom role to a tenant member.
func (s *PGTenantRoleStore) AssignRoleToMember(ctx context.Context, tenantID uuid.UUID, userID string, roleID uuid.UUID) error {
	if userID == "" {
		return fmt.Errorf("assign role: user_id required")
	}
	if err := s.rolesIsTenant(ctx, roleID, tenantID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO member_role_assignments (tenant_id, user_id, role_id)
		 VALUES ($1, $2, $3) ON CONFLICT (tenant_id, user_id, role_id) DO NOTHING`,
		tenantID, userID, roleID)
	if err != nil {
		return fmt.Errorf("assign role to member: %w", err)
	}
	return nil
}

// RevokeRoleFromMember removes a custom role from a tenant member.
func (s *PGTenantRoleStore) RevokeRoleFromMember(ctx context.Context, tenantID uuid.UUID, userID string, roleID uuid.UUID) error {
	if userID == "" {
		return fmt.Errorf("revoke role: user_id required")
	}
	if err := s.rolesIsTenant(ctx, roleID, tenantID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM member_role_assignments WHERE tenant_id = $1 AND user_id = $2 AND role_id = $3`,
		tenantID, userID, roleID)
	if err != nil {
		return fmt.Errorf("revoke role from member: %w", err)
	}
	return nil
}

// ListMemberRoles returns all custom role IDs assigned to a tenant member.
func (s *PGTenantRoleStore) ListMemberRoles(ctx context.Context, tenantID uuid.UUID, userID string) ([]uuid.UUID, error) {
	var roleIDs []uuid.UUID
	if err := pkgSqlxDB.SelectContext(ctx, &roleIDs,
		`SELECT role_id FROM member_role_assignments WHERE tenant_id = $1 AND user_id = $2 ORDER BY role_id`,
		tenantID, userID); err != nil {
		return nil, err
	}
	return roleIDs, nil
}

// ListTenantMemberRoles returns all custom-role assignments for the tenant.
func (s *PGTenantRoleStore) ListTenantMemberRoles(ctx context.Context, tenantID uuid.UUID) ([]store.MemberRoleAssignment, error) {
	var rows []memberRoleAssignmentRow
	if err := pkgSqlxDB.SelectContext(ctx, &rows,
		`SELECT user_id, role_id FROM member_role_assignments WHERE tenant_id = $1 ORDER BY user_id`,
		tenantID); err != nil {
		return nil, err
	}
	items := make([]store.MemberRoleAssignment, len(rows))
	for i, row := range rows {
		items[i] = store.MemberRoleAssignment{UserID: row.UserID, RoleID: row.RoleID}
	}
	return items, nil
}

// rolesIsTenant verifies the role row belongs to tenantID, failing closed.
func (s *PGTenantRoleStore) rolesIsTenant(ctx context.Context, roleID uuid.UUID, tenantID uuid.UUID) error {
	var exists bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM roles WHERE id = $1 AND tenant_id = $2)`,
		roleID, tenantID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return store.ErrRoleNotFound
	}
	return nil
}

type tenantRoleRow struct {
	ID          uuid.UUID  `db:"id"`
	TenantID    uuid.UUID  `db:"tenant_id"`
	Name        string     `db:"name"`
	Description *string    `db:"description"`
	Builtin     bool       `db:"builtin"`
	CreatedAt   time.Time  `db:"created_at"`
}

func (r tenantRoleRow) toStore() store.TenantRole {
	return store.TenantRole{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Description: r.Description,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
	}
}

type rolePermissionRow struct {
	ID         uuid.UUID `db:"id"`
	RoleID     uuid.UUID `db:"role_id"`
	Permission string    `db:"permission"`
	Effect     string    `db:"effect"`
}

func (r rolePermissionRow) toStore() store.RolePermission {
	return store.RolePermission{
		ID:         r.ID,
		RoleID:     r.RoleID,
		Permission: r.Permission,
		Effect:     r.Effect,
	}
}

type memberRoleAssignmentRow struct {
	UserID string    `db:"user_id"`
	RoleID uuid.UUID `db:"role_id"`
}