package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const SkillRecoveryBundledSlugCollision = "bundled_slug_collision"

// Skill status lifecycle (Phase 3 W1 — skill approval/curation).
//
// skills.status is a plain VARCHAR(20) in both PostgreSQL and SQLite, so the
// new states are TEXT constants validated here in the store layer — no
// ALTER TYPE / DDL lock on PG. The legacy "active" state is preserved as an
// alias for "published" after the 000105 migration rewrites existing rows:
//
//	draft          → just created/uploaded; owner-only (not discoverable)
//	pending_review → submitted for admin review (public skills only)
//	approved       → admin approved, awaiting publish
//	published      → discoverable by agents (replaces legacy "active")
//	rejected       → admin rejected; review_note carries the reason
//	suspended      → admin pulled from discovery (reversible)
//	archived       → legacy: dependency missing (unchanged semantics)
//	deleted        → soft-delete (unchanged semantics)
const (
	SkillStatusDraft         = "draft"
	SkillStatusPendingReview = "pending_review"
	SkillStatusApproved      = "approved"
	SkillStatusPublished     = "published"
	SkillStatusRejected      = "rejected"
	SkillStatusSuspended     = "suspended"

	// Legacy statuses kept for compatibility with pre-review rows.
	SkillStatusLegacyActive = "active"
	SkillStatusArchived     = "archived"
	SkillStatusDeleted      = "deleted"
)

// ValidSkillStatuses is the whitelist enforced by CreateSkillManaged.
// A status outside this set is rejected before touching the DB.
var ValidSkillStatuses = map[string]struct{}{
	SkillStatusDraft:         {},
	SkillStatusPendingReview: {},
	SkillStatusApproved:      {},
	SkillStatusPublished:     {},
	SkillStatusRejected:      {},
	SkillStatusSuspended:     {},
	SkillStatusLegacyActive:  {},
	SkillStatusArchived:      {},
	SkillStatusDeleted:       {},
}

// IsValidSkillStatus reports whether status is in the whitelist.
func IsValidSkillStatus(status string) bool {
	_, ok := ValidSkillStatuses[status]
	return ok
}

// ErrSystemSkillSlugConflict prevents a bundled skill from replacing or
// shadowing an existing custom skill with the same slug.
var ErrSystemSkillSlugConflict = errors.New("system skill slug conflicts with custom skill")

// ErrSkillNotReviewable is returned by ApproveSkill/RejectSkill when the skill
// is not in a review-eligible state (e.g. a system skill, or a skill that has
// already left the review lifecycle). Callers pre-fetch the skill to give a
// precise message; this sentinel is the store-level guard backing that check.
var ErrSkillNotReviewable = errors.New("skill is not in a reviewable state")

// ErrMisclassifiedCustomSkill marks a custom row repaired by the schema
// migration that still needs its metadata restored from managed files.
var ErrMisclassifiedCustomSkill = errors.New("custom skill requires bundled collision recovery")

// SkillInfo describes a discovered skill.
type SkillInfo struct {
	ID            string          `json:"id,omitempty" db:"id"` // DB UUID
	TenantID      string          `json:"-" db:"tenant_id"`
	Name          string          `json:"name" db:"name"`
	Slug          string          `json:"slug" db:"slug"`
	Path          string          `json:"path" db:"path"`
	BaseDir       string          `json:"baseDir" db:"-"`
	Source        string          `json:"source" db:"-"`
	Description   string          `json:"description" db:"description"`
	Visibility    string          `json:"visibility,omitempty" db:"visibility"`
	OwnerID       string          `json:"-" db:"owner_id"`
	Tags          []string        `json:"tags,omitempty" db:"tags"`
	Version       int             `json:"version,omitempty" db:"version"`
	IsSystem      bool            `json:"is_system,omitempty" db:"is_system"`
	Status        string          `json:"status,omitempty" db:"status"`
	Enabled       bool            `json:"enabled" db:"enabled"`
	Author        string          `json:"author,omitempty" db:"author"`
	CreatorAgent  *SkillAgentRef  `json:"creator_agent,omitempty" db:"-"`
	ManagerAgents []SkillAgentRef `json:"manager_agents,omitempty" db:"-"`
	MissingDeps   []string        `json:"missing_deps,omitempty" db:"missing_deps"`

	// Review metadata (Phase 3 W1). ReviewedBy is the actor who approved or
	// rejected the skill; ReviewNote carries the rejection reason.
	ReviewedBy string    `json:"reviewed_by,omitempty" db:"reviewed_by"`
	ReviewedAt time.Time `json:"reviewed_at" db:"reviewed_at"`
	ReviewNote string    `json:"review_note,omitempty" db:"review_note"`
}

// SkillReviewer is an optional interface for stores that enforce the skill
// review lifecycle (Phase 3 W1). Both PGSkillStore and SQLiteSkillStore
// implement it; handlers type-assert like skillUpdater/skillOwnerGetter so
// file-backed or stub stores that do not participate in review still compile.
type SkillReviewer interface {
	// ApproveSkill transitions a pending skill to published and stamps the
	// reviewer identity. Approved transitions: draft|pending_review|approved → published.
	ApproveSkill(ctx context.Context, id uuid.UUID, reviewedBy string) error
	// RejectSkill transitions a pending skill to rejected with a review note.
	// Reject transitions: draft|pending_review|approved → rejected.
	RejectSkill(ctx context.Context, id uuid.UUID, reviewedBy, note string) error
}

// SkillAgentRef is a small UI/API-safe agent reference for skill metadata.
type SkillAgentRef struct {
	ID          string `json:"id,omitempty" db:"id"`
	AgentKey    string `json:"agent_key,omitempty" db:"agent_key"`
	DisplayName string `json:"display_name,omitempty" db:"display_name"`
}

// SkillSearchResult is a scored skill returned from embedding search.
type SkillSearchResult struct {
	Name        string  `json:"name" db:"name"`
	Slug        string  `json:"slug" db:"slug"`
	Description string  `json:"description" db:"description"`
	Path        string  `json:"path" db:"path"`
	Score       float64 `json:"score" db:"score"`
}

// SkillStore manages skill discovery and loading.
// Backed by Postgres (PGSkillStore) or filesystem (FileSkillStore).
type SkillStore interface {
	ListSkills(ctx context.Context) []SkillInfo
	LoadSkill(ctx context.Context, name string) (string, bool)
	LoadForContext(ctx context.Context, allowList []string) string
	BuildSummary(ctx context.Context, allowList []string) string
	GetSkill(ctx context.Context, name string) (*SkillInfo, bool)
	FilterSkills(ctx context.Context, allowList []string) []SkillInfo
	Version() int64
	BumpVersion()
	Dirs() []string
}

// SkillAccessStore is an optional interface for stores that support
// per-agent skill access filtering.
type SkillAccessStore interface {
	ListAccessible(ctx context.Context, agentID uuid.UUID, userID string) ([]SkillInfo, error)
}

// EmbeddingSkillSearcher is an optional interface for stores that support
// vector-based skill search. PGSkillStore implements this; FileSkillStore does not.
type EmbeddingSkillSearcher interface {
	SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]SkillSearchResult, error)
	SetEmbeddingProvider(provider EmbeddingProvider)
	BackfillSkillEmbeddings(ctx context.Context) (int, error)
}

// SkillCreateParams holds parameters for creating a managed skill.
// Shared by PGSkillStore and SQLiteSkillStore.
type SkillCreateParams struct {
	Name        string
	Slug        string
	Description *string
	OwnerID     string
	Visibility  string
	Status      string // "active", "archived" (missing deps), or "deleted" (user-deleted)
	MissingDeps []string
	Version     int
	FilePath    string
	FileSize    int64
	FileHash    *string
	Frontmatter map[string]string
}

// SkillWithGrantStatus is a skill with its grant status for a specific agent.
type SkillWithGrantStatus struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Slug        string    `json:"slug" db:"slug"`
	Description string    `json:"description" db:"description"`
	Visibility  string    `json:"visibility" db:"visibility"`
	Version     int       `json:"version" db:"version"`
	Granted     bool      `json:"granted" db:"granted"`
	CanManage   bool      `json:"can_manage" db:"can_manage"`
	PinnedVer   *int      `json:"pinned_version,omitempty" db:"pinned_version"`
	IsSystem    bool      `json:"is_system" db:"is_system"`
}

// SkillAgentGrantInfo is a grant row for one skill across agents.
type SkillAgentGrantInfo struct {
	AgentID       uuid.UUID `json:"agent_id" db:"agent_id"`
	AgentKey      string    `json:"agent_key,omitempty" db:"agent_key"`
	DisplayName   string    `json:"display_name,omitempty" db:"display_name"`
	PinnedVersion int       `json:"pinned_version" db:"pinned_version"`
	GrantedBy     string    `json:"granted_by" db:"granted_by"`
	CanManage     bool      `json:"can_manage" db:"can_manage"`
}

// SkillUserGrantInfo is a user grant row for one skill.
type SkillUserGrantInfo struct {
	UserID    string `json:"user_id" db:"user_id"`
	GrantedBy string `json:"granted_by" db:"granted_by"`
}

// SkillManageStore extends SkillStore with CRUD, ownership, and grant operations
// needed by HTTP upload handlers and agent tools (skill_manage, publish_skill).
// Implemented by both PGSkillStore and SQLiteSkillStore.
type SkillManageStore interface {
	SkillStore
	// CRUD
	CreateSkillManaged(ctx context.Context, p SkillCreateParams) (uuid.UUID, error)
	UpdateSkill(ctx context.Context, id uuid.UUID, updates map[string]any) error
	DeleteSkill(ctx context.Context, id uuid.UUID) error
	ToggleSkill(ctx context.Context, id uuid.UUID, enabled bool) error
	// Queries
	GetSkillByID(ctx context.Context, id uuid.UUID) (SkillInfo, bool)
	GetSkillOwnerID(ctx context.Context, id uuid.UUID) (string, bool)
	GetSkillOwnerIDBySlug(ctx context.Context, slug string) (string, bool)
	GetNextVersion(ctx context.Context, slug string) int
	GetNextVersionLocked(ctx context.Context, slug string) (int, func() error, error)
	// GetSkillHashBySlug returns the content hash and version of the latest non-deleted skill
	// version for the given slug (tenant-scoped). Returns ok=false if no skill exists.
	GetSkillHashBySlug(ctx context.Context, slug string) (hash string, version int, ok bool)
	IsSystemSkill(slug string) bool
	// System skill management
	ListAllSkills(ctx context.Context) []SkillInfo
	ListAllSystemSkills(ctx context.Context) []SkillInfo
	ListSystemSkillDirs(ctx context.Context) map[string]string
	StoreMissingDeps(ctx context.Context, id uuid.UUID, missing []string) error
	// Grants
	GrantToAgent(ctx context.Context, skillID, agentID uuid.UUID, version int, grantedBy string, canManage ...bool) error
	RevokeFromAgent(ctx context.Context, skillID, agentID uuid.UUID) error
	GrantToUser(ctx context.Context, skillID uuid.UUID, userID, grantedBy string) error
	RevokeFromUser(ctx context.Context, skillID uuid.UUID, userID string) error
	ListWithGrantStatus(ctx context.Context, agentID uuid.UUID) ([]SkillWithGrantStatus, error)
	ListAgentGrantsForSkill(ctx context.Context, skillID uuid.UUID) ([]SkillAgentGrantInfo, error)
	ListUserGrantsForSkill(ctx context.Context, skillID uuid.UUID) ([]SkillUserGrantInfo, error)
	AgentCanManageSkill(ctx context.Context, skillID, agentID uuid.UUID) (bool, error)
	// Files
	GetSkillFilePath(ctx context.Context, id uuid.UUID) (filePath string, slug string, version int, isSystem bool, ok bool)
}
