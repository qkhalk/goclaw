package permissions

import "strings"

// Resource-action permission catalog (Phase 4 W2 — fine-grained RBAC).
//
// This catalog is the additive extension layer on top of the tier-based
// method classification. Every resource:action pair maps to a default minimum
// tier (back-compat: when no per-tenant custom role overrides exist, the tier
// fallback behaves exactly like today). A custom role's role_permissions rows
// can raise or lower a specific resource:action for that role.
//
// The catalog is deliberately independent of storage: it uses only Role and
// primitive strings so this package does not grow a store dependency (store
// imports permissions transitively, so importing store here would cycle).
// Effective role resolution lives in internal/store/rbac_eval.go.
type CatalogPermission struct {
	Resource       string // "agent", "channel", "skill", "provider", "config", "user"
	Action         string // "create", "update", "delete", "deploy", "publish", "invite", ...
	DefaultMinRole Role   // tier fallback when no custom role override exists
	Description    string
}

// PermissionCatalog is the canonical resource:action → default tier map.
// Key format: "<resource>:<action>". Unknown resource:action pairs fail closed
// (denied for every role), matching the method-classification policy.
var PermissionCatalog = map[string]CatalogPermission{
	// Agents
	"agent:create":  {Resource: "agent", Action: "create", DefaultMinRole: RoleOperator, Description: "Create agents"},
	"agent:update":  {Resource: "agent", Action: "update", DefaultMinRole: RoleOperator, Description: "Edit agent configuration"},
	"agent:delete":  {Resource: "agent", Action: "delete", DefaultMinRole: RoleAdmin, Description: "Delete agents"},
	"agent:deploy":  {Resource: "agent", Action: "deploy", DefaultMinRole: RoleOperator, Description: "Deploy/publish an agent"},
	"agent:link":    {Resource: "agent", Action: "link", DefaultMinRole: RoleOperator, Description: "Link agents (multi-agent)"},
	"agent:share":   {Resource: "agent", Action: "share", DefaultMinRole: RoleAdmin, Description: "Change agent visibility/sharing"},
	"agent:members": {Resource: "agent", Action: "members", DefaultMinRole: RoleAdmin, Description: "Manage agent membership"},

	// Channels
	"channel:create": {Resource: "channel", Action: "create", DefaultMinRole: RoleAdmin, Description: "Provision channels"},
	"channel:update": {Resource: "channel", Action: "update", DefaultMinRole: RoleOperator, Description: "Edit channel configuration"},
	"channel:delete": {Resource: "channel", Action: "delete", DefaultMinRole: RoleAdmin, Description: "Delete channels"},
	"channel:toggle": {Resource: "channel", Action: "toggle", DefaultMinRole: RoleOperator, Description: "Enable/disable a channel"},
	"channel:pair":   {Resource: "channel", Action: "pair", DefaultMinRole: RoleOperator, Description: "Start channel pairing (QR)"},

	// Skills
	"skill:publish": {Resource: "skill", Action: "publish", DefaultMinRole: RoleOperator, Description: "Publish a skill"},
	"skill:manage":  {Resource: "skill", Action: "manage", DefaultMinRole: RoleOperator, Description: "Manage skill lifecycle"},
	"skill:review":  {Resource: "skill", Action: "review", DefaultMinRole: RoleAdmin, Description: "Approve/reject skills"},
	"skill:install": {Resource: "skill", Action: "install", DefaultMinRole: RoleOperator, Description: "Install a skill package"},
	"skill:suspend": {Resource: "skill", Action: "suspend", DefaultMinRole: RoleAdmin, Description: "Suspend a published skill"},

	// Providers
	"provider:create": {Resource: "provider", Action: "create", DefaultMinRole: RoleAdmin, Description: "Add an LLM provider"},
	"provider:update": {Resource: "provider", Action: "update", DefaultMinRole: RoleAdmin, Description: "Edit provider credentials/config"},
	"provider:delete": {Resource: "provider", Action: "delete", DefaultMinRole: RoleAdmin, Description: "Remove an LLM provider"},

	// Config
	"config:read":  {Resource: "config", Action: "read", DefaultMinRole: RoleViewer, Description: "Read gateway config"},
	"config:write": {Resource: "config", Action: "write", DefaultMinRole: RoleAdmin, Description: "Modify gateway config"},

	// Users / tenant
	"user:invite": {Resource: "user", Action: "invite", DefaultMinRole: RoleAdmin, Description: "Invite users to the tenant"},
	"user:remove": {Resource: "user", Action: "remove", DefaultMinRole: RoleAdmin, Description: "Remove users from the tenant"},
	"user:role":   {Resource: "user", Action: "role", DefaultMinRole: RoleAdmin, Description: "Change a member's role"},

	// Quota / policies
	"policy:read":  {Resource: "policy", Action: "read", DefaultMinRole: RoleAdmin, Description: "Read tenant policies"},
	"policy:write": {Resource: "policy", Action: "write", DefaultMinRole: RoleAdmin, Description: "Modify tenant policies"},
	"role:manage":  {Resource: "role", Action: "manage", DefaultMinRole: RoleAdmin, Description: "Create/update/delete custom roles"},
	"role:assign":  {Resource: "role", Action: "assign", DefaultMinRole: RoleAdmin, Description: "Assign custom roles to members"},
}

// MinRoleFor returns the default minimum role for a resource:action pair.
// ok=false means the pair is not in the catalog — callers must deny (fail
// closed). This is the tier fallback used when a caller has no custom role.
func MinRoleFor(resource, action string) (Role, bool) {
	perm, ok := PermissionCatalog[resource+":"+action]
	if !ok {
		return RoleNone, false
	}
	return perm.DefaultMinRole, true
}

// CatalogKey returns the canonical key for a resource:action pair.
func CatalogKey(resource, action string) string {
	return resource + ":" + action
}

// CatalogResource returns the resource half of a "resource:action" key.
// Returns "" for malformed keys (no colon).
func CatalogResource(key string) string {
	before, _, ok := strings.Cut(key, ":")
	if !ok {
		return ""
	}
	return before
}

// CatalogAction returns the action half of a "resource:action" key.
// Returns "" for malformed keys (no colon).
func CatalogAction(key string) string {
	_, after, ok := strings.Cut(key, ":")
	if !ok {
		return ""
	}
	return after
}
