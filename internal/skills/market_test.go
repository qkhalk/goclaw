package skills

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- fake market store -------------------------------------------------------
//
// Implements both SystemSkillStore (seeder surface) and MarketManageStore so
// the install → visible → uninstall cycle runs end-to-end without a DB.

type fakeSkillRow struct {
	id       uuid.UUID
	name     string
	slug     string
	desc     string
	owner    string
	status   string
	enabled  bool
	version  int
	path     string
	hash     string
	isSystem bool
}

type fakeMarketStore struct {
	rows   map[string]*fakeSkillRow // slug → row (master-tenant semantics)
	grants map[uuid.UUID]map[uuid.UUID]bool
	bumps  int
	// listIncludesDeleted mimics PGSkillStore.ListSkills, which returns
	// system rows regardless of soft-delete status (the skills UI dims them).
	listIncludesDeleted bool
}

func newFakeMarketStore() *fakeMarketStore {
	return &fakeMarketStore{rows: map[string]*fakeSkillRow{}, grants: map[uuid.UUID]map[uuid.UUID]bool{}}
}

func (s *fakeMarketStore) UpsertSystemSkill(_ context.Context, p store.SkillCreateParams) (uuid.UUID, bool, string, error) {
	hash := ""
	if p.FileHash != nil {
		hash = *p.FileHash
	}
	if row, ok := s.rows[p.Slug]; ok {
		if row.hash == hash {
			return row.id, false, row.path, nil
		}
		row.name, row.desc, row.version = p.Name, deref(p.Description), p.Version
		row.path, row.hash, row.status = p.FilePath, hash, p.Status
		return row.id, true, p.FilePath, nil
	}
	id := uuid.New()
	desc := deref(p.Description)
	s.rows[p.Slug] = &fakeSkillRow{
		id: id, name: p.Name, slug: p.Slug, desc: desc, owner: p.OwnerID,
		status: p.Status, enabled: true, version: p.Version, path: p.FilePath,
		hash: hash, isSystem: p.OwnerID == "system",
	}
	return id, true, p.FilePath, nil
}

func (s *fakeMarketStore) GetNextVersion(_ context.Context, slug string) int {
	if row, ok := s.rows[slug]; ok {
		return row.version + 1
	}
	return 1
}

func (s *fakeMarketStore) BumpVersion() { s.bumps++ }

func (s *fakeMarketStore) Version() int64 { return int64(s.bumps) }

func (s *fakeMarketStore) Dirs() []string { return nil }

func (s *fakeMarketStore) UpdateSkill(_ context.Context, id uuid.UUID, updates map[string]any) error {
	for _, row := range s.rows {
		if row.id != id {
			continue
		}
		if v, ok := updates["status"].(string); ok {
			row.status = v
		}
		if v, ok := updates["enabled"].(bool); ok {
			row.enabled = v
		}
		return nil
	}
	return fmt.Errorf("skill %s not found", id)
}

func (s *fakeMarketStore) StoreMissingDeps(context.Context, uuid.UUID, []string) error { return nil }

func (s *fakeMarketStore) ListSkills(context.Context) []store.SkillInfo {
	out := make([]store.SkillInfo, 0, len(s.rows))
	for _, row := range s.rows {
		if row.status == "deleted" && !s.listIncludesDeleted {
			continue
		}
		out = append(out, store.SkillInfo{
			ID: row.id.String(), Name: row.name, Slug: row.slug, Description: row.desc,
			OwnerID: row.owner, Status: row.status, Enabled: row.enabled,
			Version: row.version, Path: row.path, IsSystem: row.isSystem,
		})
	}
	return out
}

func (s *fakeMarketStore) LoadSkill(context.Context, string) (string, bool) { return "", false }
func (s *fakeMarketStore) LoadForContext(context.Context, []string) string { return "" }
func (s *fakeMarketStore) BuildSummary(context.Context, []string) string   { return "" }
func (s *fakeMarketStore) GetSkill(context.Context, string) (*store.SkillInfo, bool) {
	return nil, false
}
func (s *fakeMarketStore) FilterSkills(ctx context.Context, _ []string) []store.SkillInfo {
	return s.ListSkills(ctx)
}

func (s *fakeMarketStore) GetSkillHashBySlug(_ context.Context, slug string) (string, int, bool) {
	row, ok := s.rows[slug]
	if !ok || row.status == "deleted" {
		return "", 0, false
	}
	return row.hash, row.version, true
}

func (s *fakeMarketStore) GrantToAgent(_ context.Context, skillID, agentID uuid.UUID, _ int, _ string, _ ...bool) error {
	if s.grants[skillID] == nil {
		s.grants[skillID] = map[uuid.UUID]bool{}
	}
	s.grants[skillID][agentID] = true
	return nil
}

func (s *fakeMarketStore) ListAgentGrantsForSkill(_ context.Context, skillID uuid.UUID) ([]store.SkillAgentGrantInfo, error) {
	var out []store.SkillAgentGrantInfo
	for agentID := range s.grants[skillID] {
		out = append(out, store.SkillAgentGrantInfo{AgentID: agentID})
	}
	return out, nil
}

func (s *fakeMarketStore) RevokeFromAgent(_ context.Context, skillID, agentID uuid.UUID) error {
	delete(s.grants[skillID], agentID)
	return nil
}

func (s *fakeMarketStore) ListUserGrantsForSkill(context.Context, uuid.UUID) ([]store.SkillUserGrantInfo, error) {
	return nil, nil
}

func (s *fakeMarketStore) RevokeFromUser(_ context.Context, skillID uuid.UUID, _ string) error {
	s.grants[skillID] = map[uuid.UUID]bool{}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- fixtures ----------------------------------------------------------------

// writeMarketFixture creates a bundled dir with two skills, one _shared dir
// and one dir without a SKILL.md. Returns the bundled dir path.
func writeMarketFixture(t *testing.T) string {
	t.Helper()
	bundled := filepath.Join(t.TempDir(), "bundled")
	writeSeederSkillFile(t, filepath.Join(bundled, "alpha", "SKILL.md"),
		"---\nname: Alpha Skill\ndescription: Does alpha things\ncategory: productivity\nversion: 1.0.0\n---\n\n# Alpha\n")
	writeSeederSkillFile(t, filepath.Join(bundled, "beta", "SKILL.md"),
		"---\nname: Beta Skill\ndescription: Does beta things\nversion: 2.0.0\n---\n\n# Beta\n")
	writeSeederSkillFile(t, filepath.Join(bundled, "_shared", "office", "helper.txt"), "shared code\n")
	if err := os.MkdirAll(filepath.Join(bundled, "noskill"), 0755); err != nil {
		t.Fatalf("create noskill dir: %v", err)
	}
	return bundled
}

// --- catalog tests -----------------------------------------------------------

func TestBuildMarketCatalog_FixtureShape(t *testing.T) {
	bundled := writeMarketFixture(t)

	rows, err := BuildMarketCatalog(bundled, nil)
	if err != nil {
		t.Fatalf("BuildMarketCatalog error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("catalog rows = %d, want 2 (skills only, no _shared, no SKILL.md-less dir): %+v", len(rows), rows)
	}

	bySlug := map[string]MarketEntry{}
	for _, row := range rows {
		bySlug[row.Slug] = row
	}
	alpha := bySlug["alpha"]
	if alpha.Name != "Alpha Skill" || alpha.Category != "productivity" || alpha.Version != "1.0.0" {
		t.Fatalf("alpha row = %+v, want name/category/version parsed from frontmatter", alpha)
	}
	if alpha.Installed {
		t.Fatalf("alpha reported installed with nil installed map")
	}
	beta := bySlug["beta"]
	if beta.Category != MarketCategoryGeneral {
		t.Fatalf("beta category = %q, want fallback %q", beta.Category, MarketCategoryGeneral)
	}
	// Sorted by slug.
	if rows[0].Slug != "alpha" || rows[1].Slug != "beta" {
		t.Fatalf("rows not sorted by slug: %+v", rows)
	}
}

func TestBuildMarketCatalog_InstalledAnnotations(t *testing.T) {
	bundled := writeMarketFixture(t)
	installed := map[string]store.SkillInfo{
		"alpha": {Slug: "alpha", Version: 1, Status: "active"},
	}

	rows, err := BuildMarketCatalog(bundled, installed)
	if err != nil {
		t.Fatalf("BuildMarketCatalog error: %v", err)
	}
	bySlug := map[string]MarketEntry{}
	for _, row := range rows {
		bySlug[row.Slug] = row
	}
	if !bySlug["alpha"].Installed || bySlug["alpha"].InstalledVersion != 1 {
		t.Fatalf("alpha installed annotation = %+v", bySlug["alpha"])
	}
	// alpha frontmatter 1.0.0 vs installed v1 → not newer.
	if bySlug["alpha"].UpdateAvailable {
		t.Fatalf("alpha updateAvailable = true, want false for 1.0.0 vs v1")
	}
	if bySlug["beta"].Installed {
		t.Fatalf("beta reported installed without a row")
	}
}

// --- seedOne idempotency ------------------------------------------------------

func TestSeedOne_Idempotent(t *testing.T) {
	bundled := writeMarketFixture(t)
	managed := filepath.Join(t.TempDir(), "managed")
	st := newFakeMarketStore()
	seeder := NewSeeder(bundled, managed, st)

	outcome, err := seeder.seedOne(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("seedOne error: %v", err)
	}
	if outcome.kind != outcomeSeeded || !outcome.changed {
		t.Fatalf("first seedOne outcome = %+v, want seeded/changed", outcome)
	}
	if outcome.skill.version != 1 {
		t.Fatalf("first version = %d, want 1", outcome.skill.version)
	}
	if _, err := os.Stat(filepath.Join(managed, "alpha", "1", "SKILL.md")); err != nil {
		t.Fatalf("managed copy missing after first seed: %v", err)
	}

	// Second run with identical content → unchanged, no new version dir.
	outcome2, err := seeder.seedOne(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("second seedOne error: %v", err)
	}
	if outcome2.kind != outcomeSkippedUnchanged || outcome2.changed {
		t.Fatalf("second seedOne outcome = %+v, want skipped-unchanged", outcome2)
	}
	if _, err := os.Stat(filepath.Join(managed, "alpha", "2")); !os.IsNotExist(err) {
		t.Fatalf("second run created a new version dir")
	}
	if len(st.rows) != 1 {
		t.Fatalf("store rows = %d, want 1", len(st.rows))
	}

	// Dir without SKILL.md is ignored.
	outcome3, err := seeder.seedOne(context.Background(), "noskill")
	if err != nil || outcome3.kind != outcomeNoSkillFile {
		t.Fatalf("noskill outcome = %+v err = %v, want no-skill-file", outcome3, err)
	}
}

func TestSeed_FilterCoreMode(t *testing.T) {
	bundled := writeMarketFixture(t)
	managed := filepath.Join(t.TempDir(), "managed")
	st := newFakeMarketStore()
	seeder := NewSeeder(bundled, managed, st)
	seeder.SetFilter([]string{"alpha"})

	seeded, skipped, seededSkills, err := seeder.Seed(context.Background())
	if err != nil {
		t.Fatalf("Seed error: %v", err)
	}
	if seeded != 1 || skipped != 0 || len(seededSkills) != 1 {
		t.Fatalf("Seed = (%d seeded, %d skipped, %d listed), want (1, 0, 1)", seeded, skipped, len(seededSkills))
	}
	if _, ok := st.rows["beta"]; ok {
		t.Fatalf("beta seeded despite filter")
	}
	// _shared is still copied in core mode.
	if _, err := os.Stat(filepath.Join(managed, "_shared", "office", "helper.txt")); err != nil {
		t.Fatalf("_shared not copied in core mode: %v", err)
	}
}

// --- install → visible → uninstall cycle --------------------------------------

func newTestMarket(t *testing.T) (*Market, *fakeMarketStore, string, string) {
	t.Helper()
	bundled := writeMarketFixture(t)
	managed := filepath.Join(t.TempDir(), "managed")
	st := newFakeMarketStore()
	return NewMarket(bundled, managed, st, st), st, bundled, managed
}

func TestMarket_InstallVisibleUninstallCycle(t *testing.T) {
	ctx := context.Background()
	market, st, _, managed := newTestMarket(t)

	// Install.
	res, err := market.Install(ctx, []string{"alpha"}, nil, "")
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(res.Installed) != 1 || res.Installed[0] != "alpha" || len(res.Errors) > 0 {
		t.Fatalf("Install result = %+v, want alpha installed with no errors", res)
	}

	// Visible: catalog marks it installed, managed dir + DB row exist.
	rows, err := market.Catalog(ctx)
	if err != nil {
		t.Fatalf("Catalog error: %v", err)
	}
	for _, row := range rows {
		if row.Slug == "alpha" && !row.Installed {
			t.Fatalf("alpha not marked installed after install")
		}
	}
	if _, err := os.Stat(filepath.Join(managed, "alpha", "1", "SKILL.md")); err != nil {
		t.Fatalf("managed copy missing: %v", err)
	}
	row := st.rows["alpha"]
	if row == nil || row.status != "active" {
		t.Fatalf("DB row missing or inactive: %+v", row)
	}

	// Re-install → already installed, no duplicate work.
	res2, err := market.Install(ctx, []string{"alpha"}, nil, "")
	if err != nil {
		t.Fatalf("re-Install error: %v", err)
	}
	if len(res2.AlreadyInstalled) != 1 || len(res2.Installed) != 0 {
		t.Fatalf("re-Install result = %+v, want alpha already-installed", res2)
	}

	// Uninstall → row soft-deleted, dir removed, catalog shows not installed.
	if err := market.Uninstall(ctx, "alpha"); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}
	if st.rows["alpha"].status != "deleted" || st.rows["alpha"].enabled {
		t.Fatalf("row not soft-deleted: %+v", st.rows["alpha"])
	}
	if _, err := os.Stat(filepath.Join(managed, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("managed dir still present after uninstall")
	}
	rows, err = market.Catalog(ctx)
	if err != nil {
		t.Fatalf("Catalog error: %v", err)
	}
	for _, row := range rows {
		if row.Slug == "alpha" && row.Installed {
			t.Fatalf("alpha still marked installed after uninstall")
		}
	}

	// Reinstall after uninstall → reactivates the soft-deleted row.
	res3, err := market.Install(ctx, []string{"alpha"}, nil, "")
	if err != nil {
		t.Fatalf("reinstall error: %v", err)
	}
	if len(res3.Installed) != 1 || res3.Installed[0] != "alpha" {
		t.Fatalf("reinstall result = %+v, want alpha installed (reactivated)", res3)
	}
	if st.rows["alpha"].status != "active" || !st.rows["alpha"].enabled {
		t.Fatalf("row not reactivated: %+v", st.rows["alpha"])
	}
}

// PG ListSkills returns soft-deleted SYSTEM rows on purpose (the skills UI
// dims them so admins can reactivate) — the market catalog must still report
// such rows as not installed, and installing must reactivate them.
func TestMarket_SoftDeletedSystemRowNotInstalled(t *testing.T) {
	ctx := context.Background()
	bundled := writeMarketFixture(t)
	managed := filepath.Join(t.TempDir(), "managed")
	st := newFakeMarketStore()
	st.listIncludesDeleted = true
	// Same SKILL.md hash the seeder computes, so the reinstall takes the
	// unchanged-content reactivate path (like a plain uninstall→install).
	skillContent, err := os.ReadFile(filepath.Join(bundled, "alpha", "SKILL.md"))
	if err != nil {
		t.Fatalf("read fixture SKILL.md: %v", err)
	}
	skillHash := fmt.Sprintf("%x", sha256.Sum256(skillContent))
	st.rows["alpha"] = &fakeSkillRow{
		id: uuid.New(), name: "alpha", slug: "alpha", owner: "system",
		status: "deleted", enabled: false, version: 2, hash: skillHash,
		path: filepath.Join(managed, "alpha", "2", "SKILL.md"), isSystem: true,
	}
	market := NewMarket(bundled, managed, st, st)

	rows, err := market.Catalog(ctx)
	if err != nil {
		t.Fatalf("Catalog error: %v", err)
	}
	for _, row := range rows {
		if row.Slug == "alpha" && row.Installed {
			t.Fatalf("soft-deleted system row must not be marked installed")
		}
	}

	res, err := market.Install(ctx, []string{"alpha"}, nil, "")
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(res.Installed) != 1 || res.Installed[0] != "alpha" {
		t.Fatalf("Install result = %+v, want alpha installed (reactivated)", res)
	}
	if st.rows["alpha"].status != "active" || !st.rows["alpha"].enabled {
		t.Fatalf("row not reactivated: %+v", st.rows["alpha"])
	}
}

func TestMarket_InstallUnknownSlugRejected(t *testing.T) {
	ctx := context.Background()
	market, _, _, _ := newTestMarket(t)
	if _, err := market.Install(ctx, []string{"does-not-exist"}, nil, ""); err == nil || !strings.Contains(err.Error(), "unknown slugs") {
		t.Fatalf("Install unknown slug err = %v, want unknown-slugs error", err)
	}
}

func TestMarket_InstallWithGrants(t *testing.T) {
	ctx := context.Background()
	market, st, _, _ := newTestMarket(t)
	agentID := uuid.New()

	res, err := market.Install(ctx, []string{"alpha"}, []uuid.UUID{agentID}, "user-1")
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(res.GrantErrors) > 0 {
		t.Fatalf("grant errors: %v", res.GrantErrors)
	}
	skillID := st.rows["alpha"].id
	if !st.grants[skillID][agentID] {
		t.Fatalf("agent grant missing after install with grantAgentIds")
	}
}

func TestMarket_UninstallRefusesCustomSkill(t *testing.T) {
	ctx := context.Background()
	market, st, _, _ := newTestMarket(t)

	// A user-custom row that shadows a bundled slug.
	st.rows["beta"] = &fakeSkillRow{
		id: uuid.New(), name: "Beta", slug: "beta", owner: "user-1",
		status: "active", enabled: true, version: 1, path: filepath.Join(t.TempDir(), "beta", "1"),
	}

	err := market.Uninstall(ctx, "beta")
	if err == nil || !strings.Contains(err.Error(), "custom skill") {
		t.Fatalf("Uninstall custom err = %v, want refusal", err)
	}
	if st.rows["beta"].status != "active" {
		t.Fatalf("custom row mutated by refused uninstall: %+v", st.rows["beta"])
	}
}

func TestMarket_UpdateUnchangedIsNoop(t *testing.T) {
	ctx := context.Background()
	market, _, bundled, managed := newTestMarket(t)

	if _, err := market.Install(ctx, []string{"alpha"}, nil, ""); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if err := market.Update(ctx, "alpha"); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	// No new version directory for unchanged bundled content.
	if _, err := os.Stat(filepath.Join(managed, "alpha", "2")); !os.IsNotExist(err) {
		t.Fatalf("unchanged update created a new version dir")
	}
	_ = bundled
}

// --- kit discovery ------------------------------------------------------------

func TestBuildKitList_FiltersToCatalogAndCountsInstalled(t *testing.T) {
	bundled := writeMarketFixture(t)
	// Kit spanning both catalog skills plus one missing slug (dropped).
	writeSeederSkillFile(t, filepath.Join(bundled, "full-kit", "kit.yaml"),
		"name: Full Kit\nversion: 1.0.0\ndescription: Everything\nskills:\n  - alpha\n  - beta\n  - ghost\n")
	// Single-skill kit.
	writeSeederSkillFile(t, filepath.Join(bundled, "engineer", "kit.yaml"),
		"name: Engineer\nversion: 0.1.0\nskills:\n  - alpha\n")
	// Invalid manifest (no name) is skipped.
	writeSeederSkillFile(t, filepath.Join(bundled, "broken", "kit.yaml"), "no_name_field: true\n")

	catalog := map[string]MarketEntry{
		"alpha": {Slug: "alpha"},
		"beta":  {Slug: "beta"},
	}
	installed := map[string]store.SkillInfo{"alpha": {Slug: "alpha", Status: "active"}}

	kits, err := BuildKitList(bundled, catalog, installed)
	if err != nil {
		t.Fatalf("BuildKitList error: %v", err)
	}
	if len(kits) != 2 {
		t.Fatalf("kits = %d, want 2 (broken manifest skipped, ghost slug dropped): %+v", len(kits), kits)
	}
	// Sorted by kit name: "Engineer" < "Full Kit".
	if kits[0].Name != "Engineer" || kits[1].Name != "Full Kit" {
		t.Fatalf("kits not sorted by name: %+v", kits)
	}
	full := kits[1]
	if full.Slug != "full-kit" || len(full.Skills) != 2 {
		t.Fatalf("full kit skills = %+v, want alpha+beta (ghost dropped)", full.Skills)
	}
	if full.InstalledCount != 1 {
		t.Fatalf("full kit installedCount = %d, want 1 (only alpha installed)", full.InstalledCount)
	}
	if full.Description != "Everything" || full.Version != "1.0.0" {
		t.Fatalf("full kit metadata = %+v", full)
	}
}

func TestMarket_Kits_AnnotatedFromLiveStore(t *testing.T) {
	ctx := context.Background()
	market, st, _, _ := newTestMarket(t)

	// No kit manifests in the fixture yet → no kits.
	kits, rows, err := market.Kits(ctx)
	if err != nil {
		t.Fatalf("Kits error: %v", err)
	}
	if len(kits) != 0 || len(rows) != 2 {
		t.Fatalf("base fixture kits/rows = %d/%d, want 0/2", len(kits), len(rows))
	}

	// Seed rows by installing alpha, then add a kit manifest over it.
	if _, err := market.Install(ctx, []string{"alpha"}, nil, ""); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	writeSeederSkillFile(t, filepath.Join(market.bundledDir, "engineer", "kit.yaml"),
		"name: Engineer\nversion: 0.1.0\nskills:\n  - alpha\n  - beta\n")

	kits, _, err = market.Kits(ctx)
	if err != nil {
		t.Fatalf("Kits error: %v", err)
	}
	if len(kits) != 1 || kits[0].Name != "Engineer" {
		t.Fatalf("kits = %+v, want one Engineer kit", kits)
	}
	if kits[0].InstalledCount != 1 {
		t.Fatalf("installedCount = %d, want 1 (alpha installed, beta not)", kits[0].InstalledCount)
	}
	_ = st
}

func TestBuildKitList_NestsSubKits(t *testing.T) {
	bundled := writeMarketFixture(t)
	// Parent kit referencing two sub-kits (one missing on disk) + a stray dup skill.
	writeSeederSkillFile(t, filepath.Join(bundled, "parent", "kit.yaml"),
		"name: parent-kit\nversion: 2.0.0\nkits:\n  - kit-a\n  - kit-b\n  - ghost-kit\nskills:\n  - beta\n")
	writeSeederSkillFile(t, filepath.Join(bundled, "kit-a", "kit.yaml"),
		"name: kit-a\nversion: 1.0.0\nskills:\n  - alpha\n  - beta\n")
	writeSeederSkillFile(t, filepath.Join(bundled, "kit-b", "kit.yaml"),
		"name: kit-b\nversion: 1.0.0\nskills:\n  - beta\n")

	catalog := map[string]MarketEntry{
		"alpha": {Slug: "alpha"},
		"beta":  {Slug: "beta"},
	}
	installed := map[string]store.SkillInfo{"beta": {Slug: "beta", Status: "active"}}

	kits, err := BuildKitList(bundled, catalog, installed)
	if err != nil {
		t.Fatalf("BuildKitList error: %v", err)
	}
	// Only the parent is top-level; kit-a/kit-b are claimed as sub-kits.
	if len(kits) != 1 || kits[0].Name != "parent-kit" {
		t.Fatalf("top-level kits = %+v, want only parent-kit", kits)
	}
	parent := kits[0]
	if len(parent.SubKits) != 2 {
		t.Fatalf("sub-kits = %+v, want 2 (ghost-kit missing on disk skipped)", parent.SubKits)
	}
	if parent.SubKits[0].Name != "kit-a" || parent.SubKits[1].Name != "kit-b" {
		t.Fatalf("sub-kits not sorted by name: %+v", parent.SubKits)
	}
	// Union of parent's own beta + kit-a's alpha,beta + kit-b's beta → alpha,beta.
	if len(parent.Skills) != 2 || parent.Skills[0] != "alpha" || parent.Skills[1] != "beta" {
		t.Fatalf("parent skills = %+v, want deduped union [alpha beta]", parent.Skills)
	}
	if parent.InstalledCount != 1 {
		t.Fatalf("parent installedCount = %d, want 1 (beta installed once)", parent.InstalledCount)
	}
	if parent.SubKits[0].InstalledCount != 1 || parent.SubKits[1].InstalledCount != 1 {
		t.Fatalf("sub-kit installed counts = %d/%d, want 1/1 (beta installed in both)",
			parent.SubKits[0].InstalledCount, parent.SubKits[1].InstalledCount)
	}
}

func TestMarket_ReconcileMissing(t *testing.T) {
	ctx := context.Background()
	market, st, _, _ := newTestMarket(t)

	// Nothing on disk is seeded yet → reconcile adds both catalog skills.
	added, err := market.ReconcileMissing(ctx)
	if err != nil {
		t.Fatalf("ReconcileMissing error: %v", err)
	}
	if len(added) != 2 || added[0] != "alpha" || added[1] != "beta" {
		t.Fatalf("added = %+v, want [alpha beta]", added)
	}
	if len(st.rows) != 2 {
		t.Fatalf("store rows = %d, want 2", len(st.rows))
	}

	// Second run: every slug already has a row → no additions.
	added, err = market.ReconcileMissing(ctx)
	if err != nil {
		t.Fatalf("second ReconcileMissing error: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("second run added = %+v, want none", added)
	}

	// A soft-deleted row (market uninstall) blocks re-adding.
	if err := market.Uninstall(ctx, "alpha"); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}
	added, err = market.ReconcileMissing(ctx)
	if err != nil {
		t.Fatalf("ReconcileMissing after uninstall error: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("reconcile re-added uninstalled skill: %+v", added)
	}
}
