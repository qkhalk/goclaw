package skills

import (
	"context"
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
		if row.status == "deleted" {
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
