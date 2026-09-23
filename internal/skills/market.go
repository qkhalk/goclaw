package skills

// market.go — bundled-skill market: catalog build over the bundled skills
// directory plus synchronous install/uninstall/update operations.
//
// The catalog's single source of truth is the bundled directory itself
// (same resolution as the startup seeder): every subdirectory with a
// SKILL.md becomes one row, parsed with the standard loader frontmatter
// parser. Rows are annotated with installed state from the skills store.
// kit.yaml is NOT consulted — the directory wins when they disagree.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// MarketCategoryGeneral is the fallback category for skills whose
// frontmatter does not declare a `category`.
const MarketCategoryGeneral = "general"

// MarketEntry is one row of the bundled-skill market catalog.
type MarketEntry struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Version     string   `json:"version,omitempty"` // frontmatter version (display only)
	Requires    []string `json:"requires,omitempty"`
	// Installed is true when an active (non-deleted) skills row with this
	// slug exists in the requesting scope. System skills are globally
	// visible, so the flag is tenant-independent for bundled skills.
	Installed bool `json:"installed"`
	// InstalledVersion is the DB integer version of the installed row.
	InstalledVersion int `json:"installedVersion,omitempty"`
	// UpdateAvailable is true when the bundled frontmatter version is
	// strictly newer than the installed row's version.
	UpdateAvailable bool `json:"updateAvailable,omitempty"`
}

// MarketKit is one bundled skill kit — a kit.yaml manifest inside a bundled
// skill directory (e.g. skills/goclaw-kit/kit.yaml). Skills lists the kit's
// slugs that are actually present in the current catalog; InstalledCount is
// how many of those have a live skills row.
type MarketKit struct {
	Slug           string   `json:"slug"` // directory name in the bundled tree
	Name           string   `json:"name"` // kit.yaml name
	Description    string   `json:"description,omitempty"`
	Version        string   `json:"version,omitempty"`
	Skills         []string `json:"skills"`
	InstalledCount int      `json:"installedCount"`
}

// MarketInstallResult reports what a market install did. Installing is a
// local directory copy, so it runs synchronously — no job layer.
type MarketInstallResult struct {
	Installed        []string `json:"installed"`
	AlreadyInstalled []string `json:"alreadyInstalled"`
	GrantErrors      []string `json:"grantErrors,omitempty"`
	Errors           []string `json:"errors,omitempty"`
}

// MarketManageStore is the store surface the market needs beyond the
// seeder: installed-state reads plus the writes for uninstall and grants.
// store.SkillManageStore satisfies it (both PG and SQLite implementations).
type MarketManageStore interface {
	store.SkillStore
	UpdateSkill(ctx context.Context, id uuid.UUID, updates map[string]any) error
	GetSkillHashBySlug(ctx context.Context, slug string) (hash string, version int, ok bool)
	GrantToAgent(ctx context.Context, skillID, agentID uuid.UUID, version int, grantedBy string, canManage ...bool) error
	ListAgentGrantsForSkill(ctx context.Context, skillID uuid.UUID) ([]store.SkillAgentGrantInfo, error)
	RevokeFromAgent(ctx context.Context, skillID, agentID uuid.UUID) error
	ListUserGrantsForSkill(ctx context.Context, skillID uuid.UUID) ([]store.SkillUserGrantInfo, error)
	RevokeFromUser(ctx context.Context, skillID uuid.UUID, userID string) error
}

// Market installs and removes bundled skills on demand. It reuses the
// Seeder for DB upserts (identical semantics to startup seeding) and the
// KitManager for dependency-closure-aware file copies into the managed
// (versioned) skills-store directory.
type Market struct {
	bundledDir string
	managedDir string
	seeder     *Seeder
	kit        *KitManager
	manage     MarketManageStore
}

// NewMarket creates a market over a bundled skills directory.
// bundledDir: source tree (one folder per skill + SKILL.md).
// managedDir: destination skills-store directory (tenant-scoped by the caller).
// seedStore: store used by the seeder (UpsertSystemSkill surface).
// manage: store used for installed-state, uninstall and grants.
func NewMarket(bundledDir, managedDir string, seedStore SystemSkillStore, manage MarketManageStore) *Market {
	m := &Market{
		bundledDir: bundledDir,
		managedDir: managedDir,
		manage:     manage,
	}
	if seedStore != nil {
		m.seeder = NewSeeder(bundledDir, managedDir, seedStore)
	}
	// manifestPath is only read by Load-backed helpers; InstallFrom takes
	// the manifest as an argument, so a synthetic path is fine here.
	m.kit = NewKitManager(filepath.Join(bundledDir, "goclaw-kit", "kit.yaml"), bundledDir)
	m.kit.SetManagedDir(managedDir)
	return m
}

// BundledDir returns the source directory the market reads from.
func (m *Market) BundledDir() string { return m.bundledDir }

// ResolveBundledSkillsDir mirrors the gateway's bundled-skills resolution:
// GOCLAW_BUNDLED_SKILLS_DIR wins, then the first existing candidate of
// bundled-skills/, /app/bundled-skills/ (Docker), skills/ (dev checkout).
// Returns "" when nothing is found.
func ResolveBundledSkillsDir() string {
	if d := strings.TrimSpace(os.Getenv("GOCLAW_BUNDLED_SKILLS_DIR")); d != "" {
		return d
	}
	for _, candidate := range []string{"bundled-skills", "/app/bundled-skills", "skills"} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

// BuildMarketCatalog scans bundledDir and returns one MarketEntry per
// subdirectory holding a SKILL.md, annotated from the installed map
// (slug → skills row; nil map = nothing installed). `_`-prefixed shared
// directories are not skills and are skipped. Entries are sorted by slug.
func BuildMarketCatalog(bundledDir string, installed map[string]store.SkillInfo) ([]MarketEntry, error) {
	entries, err := os.ReadDir(bundledDir)
	if err != nil {
		return nil, fmt.Errorf("market: read bundled dir: %w", err)
	}
	rows := make([]MarketEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		slug := e.Name()
		skillFile := filepath.Join(bundledDir, slug, "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			slog.Debug("market: skip dir without SKILL.md", "slug", slug)
			continue
		}

		// Smoke test: the skill must parse. parseMetadata falls back to a
		// nameless struct when frontmatter is broken, so treat a missing
		// name as a parse failure the same way install validation does.
		meta := parseMetadata(skillFile)
		if meta == nil || meta.Name == "" {
			slog.Debug("market: skip skill with unparseable frontmatter", "slug", slug)
			continue
		}

		row := MarketEntry{
			Slug:        slug,
			Name:        meta.Name,
			Description: meta.Description,
			Version:     meta.Version,
			Category:    MarketCategoryGeneral,
		}
		if fm := extractFrontmatter(string(data)); fm != "" {
			if cat := strings.TrimSpace(parseSimpleYAML(fm)["category"]); cat != "" {
				row.Category = cat
			}
		}
		if meta.Requires != nil {
			row.Requires = meta.Requires.Bins
		}

		if info, ok := installed[slug]; ok {
			row.Installed = true
			row.InstalledVersion = info.Version
			if meta.Version != "" && compareVersions(meta.Version, strconv.Itoa(info.Version)) > 0 {
				row.UpdateAvailable = true
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Slug < rows[j].Slug })
	return rows, nil
}

// installedIndex builds the slug → row map for catalog annotation.
// System skills live in the master tenant, so the lookup always runs with
// the master tenant in context regardless of the caller's scope.
func (m *Market) installedIndex(ctx context.Context) map[string]store.SkillInfo {
	index := make(map[string]store.SkillInfo)
	if m.manage == nil {
		return index
	}
	for _, info := range m.manage.ListSkills(store.WithTenantID(ctx, store.MasterTenantID)) {
		// ListSkills deliberately includes soft-deleted system rows (the
		// skills UI dims them); the market must treat deleted as not
		// installed so uninstalled skills leave the catalog's installed set.
		if info.Status == "deleted" {
			continue
		}
		index[info.Slug] = info
	}
	return index
}

// Catalog returns the full market catalog annotated with installed state.
func (m *Market) Catalog(ctx context.Context) ([]MarketEntry, error) {
	return BuildMarketCatalog(m.bundledDir, m.installedIndex(ctx))
}

// catalogIndex rebuilds the catalog and indexes it by slug.
func (m *Market) catalogIndex(ctx context.Context) (map[string]MarketEntry, error) {
	rows, err := m.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]MarketEntry, len(rows))
	for _, row := range rows {
		index[row.Slug] = row
	}
	return index, nil
}

// Install validates the requested slugs against the catalog, copies the
// dependency closure into the managed dir via the KitManager, upserts the
// DB rows via the seeder (seedOne), and optionally grants the skills to
// agents. Bundled rows previously removed through the market (soft-deleted
// with unchanged content) are reactivated. Everything is a local copy, so
// the call is synchronous; very large slug lists simply take proportionally
// longer (each skill is a small directory copy + one upsert).
func (m *Market) Install(ctx context.Context, slugs []string, grantAgentIDs []uuid.UUID, grantedBy string) (*MarketInstallResult, error) {
	if len(slugs) == 0 {
		return nil, errors.New("market install: no slugs requested")
	}
	if m.seeder == nil || m.manage == nil {
		return nil, errors.New("market install: stores not configured")
	}

	requested := uniqueSlugs(slugs)
	index, err := m.catalogIndex(ctx)
	if err != nil {
		return nil, err
	}
	installed := m.installedIndex(ctx)

	var unknown []string
	var todo []string
	res := &MarketInstallResult{}
	for _, slug := range requested {
		if _, ok := index[slug]; !ok {
			unknown = append(unknown, slug)
			continue
		}
		if _, ok := installed[slug]; ok {
			res.AlreadyInstalled = append(res.AlreadyInstalled, slug)
			continue
		}
		todo = append(todo, slug)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("market install: unknown slugs: %s", strings.Join(unknown, ", "))
	}

	// seededIDs collects slug → skill id for the grant pass (valid for both
	// freshly seeded and reactivated rows).
	seededIDs := make(map[string]seededSkill, len(requested))

	if len(todo) > 0 {
		// Closure-aware copy into the managed dir (deps first, kit.lock
		// updated). Slugs already present on disk land in AlreadySatisfied.
		manifest := KitManifest{
			Name:    "market-selection",
			Version: time.Now().UTC().Format("20060102"),
			Skills:  todo,
		}
		plan, err := m.kit.InstallFrom(ctx, manifest, InstallOpts{})
		if err != nil {
			return nil, fmt.Errorf("market install: %w", err)
		}

		// Seed every slug the plan copied plus everything explicitly
		// requested (a requested skill may already be on disk while its DB
		// row is missing — the reconciler normally fixes that at startup).
		seedSet := uniqueSlugs(append(append([]string{}, plan.Install...), todo...))
		// Bundled rows live in the master tenant; seedOne's store calls
		// (GetNextVersion/UpsertSystemSkill) must run against that scope.
		seedCtx := store.WithTenantID(ctx, store.MasterTenantID)
		for _, slug := range seedSet {
			outcome, seedErr := m.seeder.seedOne(seedCtx, slug)
			if seedErr != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", slug, seedErr))
				continue
			}
			switch outcome.kind {
			case outcomeSeeded:
				res.Installed = append(res.Installed, slug)
				seededIDs[slug] = outcome.skill
			case outcomeSkippedUnchanged:
				// Row exists with identical content — after a market
				// uninstall that means a soft-deleted row. Reactivate so
				// the install is actually visible to agents.
				reactivate := map[string]any{"status": "active", "enabled": true}
				if err := m.manage.UpdateSkill(seedCtx, outcome.skill.id, reactivate); err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("%s: reactivate: %v", slug, err))
					continue
				}
				res.Installed = append(res.Installed, slug)
				seededIDs[slug] = outcome.skill
			case outcomeSkippedConflict:
				res.Errors = append(res.Errors, fmt.Sprintf("%s: slug is owned by a custom skill", slug))
			case outcomeNoSkillFile, outcomeSeedFailed:
				res.Errors = append(res.Errors, fmt.Sprintf("%s: seed failed (see logs)", slug))
			}
		}
	}

	// Grants (best effort — failures are reported, not fatal).
	if len(grantAgentIDs) > 0 {
		for _, slug := range requested {
			sk, ok := seededIDs[slug]
			if !ok {
				continue
			}
			// Prefer the persisted row version: on the reactivate path the
			// seeder's version is MAX(version)+1, which does not exist.
			version := sk.version
			if _, v, ok := m.manage.GetSkillHashBySlug(store.WithTenantID(ctx, store.MasterTenantID), slug); ok {
				version = v
			}
			for _, agentID := range grantAgentIDs {
				if err := m.manage.GrantToAgent(ctx, sk.id, agentID, version, grantedBy); err != nil {
					res.GrantErrors = append(res.GrantErrors, fmt.Sprintf("%s → %s: %v", slug, agentID, err))
				}
			}
		}
	}

	if len(res.Installed) > 0 {
		m.manage.BumpVersion()
	}
	sort.Strings(res.Installed)
	sort.Strings(res.AlreadyInstalled)
	return res, nil
}

// Uninstall removes a bundled-origin skill: revokes grants, soft-deletes
// the DB row (status=deleted + disabled — hard DELETE is reserved for the
// store layer's cascade logic and refuses system skills), and removes the
// managed skill directory. User-custom skills (owner != "system") are
// refused — they are managed through the regular skill CRUD endpoints.
func (m *Market) Uninstall(ctx context.Context, slug string) error {
	if m.manage == nil {
		return errors.New("market uninstall: stores not configured")
	}
	index, err := m.catalogIndex(ctx)
	if err != nil {
		return err
	}
	if _, ok := index[slug]; !ok {
		return fmt.Errorf("market uninstall: %q is not a bundled skill", slug)
	}

	// System rows live in the master tenant.
	masterCtx := store.WithTenantID(ctx, store.MasterTenantID)

	// Prefer the system row when both a system and a custom row exist for
	// the slug; a lone custom row is refused explicitly below.
	var row *store.SkillInfo
	all := m.manage.ListSkills(masterCtx)
	for i := range all {
		if all[i].Slug != slug {
			continue
		}
		if row == nil || all[i].IsSystem {
			row = &all[i]
		}
	}
	if row == nil {
		return fmt.Errorf("market uninstall: %q is not installed", slug)
	}
	if !row.IsSystem || row.OwnerID != "system" {
		return fmt.Errorf("market uninstall: %q is a user-custom skill — delete it from the skills list instead", slug)
	}

	id, err := uuid.Parse(row.ID)
	if err != nil {
		return fmt.Errorf("market uninstall: invalid skill id %q: %w", row.ID, err)
	}

	// Revoke grants so no stale grant rows survive the uninstall.
	if grants, err := m.manage.ListAgentGrantsForSkill(masterCtx, id); err == nil {
		for _, g := range grants {
			if err := m.manage.RevokeFromAgent(masterCtx, id, g.AgentID); err != nil {
				slog.Warn("market: revoke agent grant failed", "slug", slug, "agent", g.AgentID, "error", err)
			}
		}
	}
	if grants, err := m.manage.ListUserGrantsForSkill(masterCtx, id); err == nil {
		for _, g := range grants {
			if err := m.manage.RevokeFromUser(masterCtx, id, g.UserID); err != nil {
				slog.Warn("market: revoke user grant failed", "slug", slug, "user", g.UserID, "error", err)
			}
		}
	}

	// Disable + soft-delete. DeleteSkill refuses system rows, so drive the
	// same transition through UpdateSkill (identical end state to the
	// regular delete flow: status='deleted').
	if err := m.manage.UpdateSkill(masterCtx, id, map[string]any{"status": "deleted", "enabled": false}); err != nil {
		return fmt.Errorf("market uninstall: delete row: %w", err)
	}

	// Remove the managed directory (all versions). row.Path is the managed
	// file_path recorded at seed time; its parent is <managedDir>/<slug>.
	// Containment (not basename) equality: only ever delete the directory
	// this market manages for the slug, never anything a stale row points at.
	if row.Path != "" {
		skillRoot := filepath.Dir(filepath.Clean(row.Path))
		if skillRoot == filepath.Clean(filepath.Join(m.managedDir, slug)) {
			if err := os.RemoveAll(skillRoot); err != nil {
				slog.Warn("market: remove managed dir failed", "slug", slug, "dir", skillRoot, "error", err)
			}
		}
	}

	m.manage.BumpVersion()
	slog.Info("market: skill uninstalled", "slug", slug)
	return nil
}

// Update refreshes a single installed skill from the bundled source.
// It tries the KitManager update path (frontmatter-version-aware, records
// the new checksum in kit.lock) and falls back to the seeder's seedOne for
// skills that were installed by the startup seeder and are not tracked in
// kit.lock — seedOne creates a new version when the content hash changed.
func (m *Market) Update(ctx context.Context, slug string) error {
	if m.seeder == nil || m.manage == nil {
		return errors.New("market update: stores not configured")
	}
	index, err := m.catalogIndex(ctx)
	if err != nil {
		return err
	}
	if _, ok := index[slug]; !ok {
		return fmt.Errorf("market update: %q is not a bundled skill", slug)
	}

	if err := m.kit.Update(ctx, slug); err == nil {
		m.manage.BumpVersion()
		return nil
	} else if !strings.Contains(err.Error(), "not tracked") && !errors.Is(err, os.ErrNotExist) {
		// A real failure (source unreadable, copy failed) — surface it.
		if !strings.Contains(err.Error(), "load lock") {
			return fmt.Errorf("market update: %w", err)
		}
		// Missing/failed lock load → not kit-tracked → fall through to the
		// seeder path below.
		slog.Debug("market: skill not kit-tracked, falling back to seeder", "slug", slug, "error", err)
	}

	seedCtx := store.WithTenantID(ctx, store.MasterTenantID)
	outcome, seedErr := m.seeder.seedOne(seedCtx, slug)
	if seedErr != nil {
		return fmt.Errorf("market update: %w", seedErr)
	}
	switch outcome.kind {
	case outcomeSeeded:
		m.manage.BumpVersion()
		return nil
	case outcomeSkippedUnchanged:
		return nil // already up to date
	case outcomeSkippedConflict:
		return fmt.Errorf("market update: %q is owned by a custom skill", slug)
	default:
		return fmt.Errorf("market update: %q: seed failed (see logs)", slug)
	}
}

// BuildKitList discovers kit manifests (each <bundledDir>/<dir>/kit.yaml) and
// projects them against the catalog + installed index. Kit skills missing from
// the catalog (no SKILL.md / unparseable frontmatter) are dropped; kits left
// with no catalog skills are skipped. Results are sorted by kit name.
func BuildKitList(bundledDir string, catalog map[string]MarketEntry, installed map[string]store.SkillInfo) ([]MarketKit, error) {
	entries, err := os.ReadDir(bundledDir)
	if err != nil {
		return nil, fmt.Errorf("market: read bundled dir: %w", err)
	}
	var kits []MarketKit
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(bundledDir, e.Name(), "kit.yaml"))
		if err != nil {
			continue // not a kit directory
		}
		var manifest KitManifest
		if err := yaml.Unmarshal(raw, &manifest); err != nil || strings.TrimSpace(manifest.Name) == "" {
			slog.Debug("market: skip invalid kit manifest", "dir", e.Name())
			continue
		}
		kit := MarketKit{
			Slug:        e.Name(),
			Name:        manifest.Name,
			Description: manifest.Description,
			Version:     manifest.Version,
			Skills:      []string{},
		}
		for _, slug := range manifest.Skills {
			if _, ok := catalog[slug]; !ok {
				continue
			}
			kit.Skills = append(kit.Skills, slug)
			if _, ok := installed[slug]; ok {
				kit.InstalledCount++
			}
		}
		if len(kit.Skills) == 0 {
			continue
		}
		kits = append(kits, kit)
	}
	sort.Slice(kits, func(i, j int) bool { return kits[i].Name < kits[j].Name })
	return kits, nil
}

// Kits returns the bundled kit manifests with catalog-filtered skill lists and
// installed counts. One catalog build serves both the rows and the kits.
func (m *Market) Kits(ctx context.Context) ([]MarketKit, []MarketEntry, error) {
	installed := m.installedIndex(ctx)
	rows, err := BuildMarketCatalog(m.bundledDir, installed)
	if err != nil {
		return nil, nil, err
	}
	catalog := make(map[string]MarketEntry, len(rows))
	for _, row := range rows {
		catalog[row.Slug] = row
	}
	kits, err := BuildKitList(m.bundledDir, catalog, installed)
	if err != nil {
		return nil, nil, err
	}
	return kits, rows, nil
}

// uniqueSlugs deduplicates and preserves first-seen order.
func uniqueSlugs(slugs []string) []string {
	seen := make(map[string]bool, len(slugs))
	out := make([]string, 0, len(slugs))
	for _, s := range slugs {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
