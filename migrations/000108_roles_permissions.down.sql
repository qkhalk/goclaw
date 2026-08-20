-- Rollback fine-grained RBAC tables (custom roles + role_permissions + member
-- role assignments, W2 Phase 4).
DROP TABLE IF EXISTS member_role_assignments;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS roles;