package video

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

// fakeAgentStore embeds the interface so only the methods EnsureDesignerAgent
// (and bootstrap.SeedToStore) touch are implemented; any other call panics on
// the nil embedded value, which is exactly what we want in a unit test.
type fakeAgentStore struct {
	store.AgentStore
	byKey      map[string]*store.AgentData
	created    int
	files      map[uuid.UUID]map[string]string
}

func newFakeAgentStore() *fakeAgentStore {
	return &fakeAgentStore{
		byKey: map[string]*store.AgentData{},
		files: map[uuid.UUID]map[string]string{},
	}
}

// GetByKey mirrors the PG contract: a missing row surfaces as (nil, error).
func (f *fakeAgentStore) GetByKey(_ context.Context, key string) (*store.AgentData, error) {
	if a, ok := f.byKey[key]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("agent not found: %s", key)
}

func (f *fakeAgentStore) Create(_ context.Context, agent *store.AgentData) error {
	agent.ID = store.GenNewID()
	f.byKey[agent.AgentKey] = agent
	f.created++
	return nil
}

func (f *fakeAgentStore) GetAgentContextFiles(_ context.Context, agentID uuid.UUID) ([]store.AgentContextFileData, error) {
	var out []store.AgentContextFileData
	for name, content := range f.files[agentID] {
		out = append(out, store.AgentContextFileData{AgentID: agentID, FileName: name, Content: content})
	}
	return out, nil
}

func (f *fakeAgentStore) SetAgentContextFile(_ context.Context, agentID uuid.UUID, name, content string) error {
	if f.files[agentID] == nil {
		f.files[agentID] = map[string]string{}
	}
	f.files[agentID][name] = content
	return nil
}

type fakeSkillStore struct {
	store.SkillManageStore
	skills   []store.SkillWithGrantStatus
	granted  int
	rescoped int
}

func (f *fakeSkillStore) ListWithGrantStatus(_ context.Context, _ uuid.UUID) ([]store.SkillWithGrantStatus, error) {
	return f.skills, nil
}

func (f *fakeSkillStore) UpdateSkill(_ context.Context, id uuid.UUID, updates map[string]any) error {
	f.rescoped++
	for i := range f.skills {
		if f.skills[i].ID == id {
			if v, ok := updates["visibility"].(string); ok {
				f.skills[i].Visibility = v
			}
			if v, ok := updates["is_system"].(bool); ok {
				f.skills[i].IsSystem = v
			}
		}
	}
	return nil
}

func (f *fakeSkillStore) GrantToAgent(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ int, _ string, _ ...bool) error {
	f.granted++
	for i := range f.skills {
		f.skills[i].Granted = true
	}
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func testConfig() *config.Config {
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Provider:          "test-provider",
				Model:             "test-model",
				MaxTokens:         4096,
				ContextWindow:     64000,
				MaxToolIterations: 10,
			},
		},
	}
}

func seededSkillStore() *fakeSkillStore {
	return &fakeSkillStore{
		skills: []store.SkillWithGrantStatus{
			{ID: store.GenNewID(), Slug: "video-storyboard-design", Visibility: "public", Version: 1},
			{ID: store.GenNewID(), Slug: "video-color-motion", Visibility: "public", Version: 1},
		},
	}
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestEnsureDesignerAgentCreatesWithLockedToolSurface(t *testing.T) {
	agents := newFakeAgentStore()
	skills := seededSkillStore()

	if err := EnsureDesignerAgent(context.Background(), testConfig(), agents, skills, t.TempDir()); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	agent, ok := agents.byKey[DesignerAgentKey]
	if !ok {
		t.Fatal("designer agent was not created")
	}
	if agent.AgentType != store.AgentTypePredefined {
		t.Errorf("agent_type = %q, want predefined", agent.AgentType)
	}
	spec := agent.ParseToolsConfig()
	if spec == nil {
		t.Fatal("tools_config did not parse")
	}
	if spec.Profile != "minimal" {
		t.Errorf("policy profile = %q, want minimal", spec.Profile)
	}
	want := []string{"skill_search", "use_skill", "session_status"}
	if !reflect.DeepEqual(spec.Allow, want) {
		t.Errorf("allow = %v, want exactly %v", spec.Allow, want)
	}
	if agent.Provider != "test-provider" || agent.Model != "test-model" {
		t.Errorf("provider/model = %s/%s, want defaults from cfg", agent.Provider, agent.Model)
	}
	// IDENTITY.md must carry the design-only persona and the storyboard fence.
	id := agents.files[agent.ID]["IDENTITY.md"]
	if id == "" {
		t.Fatal("IDENTITY.md was not written")
	}
	for _, marker := range []string{"storyboard", "version\":1", "design"} {
		if !strings.Contains(id, marker) {
			t.Errorf("IDENTITY.md missing %q", marker)
		}
	}
}

func TestEnsureDesignerAgentIsIdempotent(t *testing.T) {
	agents := newFakeAgentStore()
	skills := seededSkillStore()

	if err := EnsureDesignerAgent(context.Background(), testConfig(), agents, skills, t.TempDir()); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if err := EnsureDesignerAgent(context.Background(), testConfig(), agents, skills, t.TempDir()); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if agents.created != 1 {
		t.Errorf("agent created %d times, want 1 (second ensure must reuse)", agents.created)
	}
	if skills.granted != 2 {
		t.Errorf("grants issued = %d, want 2 total (no duplicate grants on re-ensure)", skills.granted)
	}
	if skills.rescoped != 2 {
		t.Errorf("rescopes = %d, want 2 total (no repeated visibility writes)", skills.rescoped)
	}
}

// Admin-edited agents must never be overwritten by the boot-time ensure.
func TestEnsureDesignerAgentDoesNotResurrectOrOverwrite(t *testing.T) {
	agents := newFakeAgentStore()
	edited := &store.AgentData{
		TenantID:    store.MasterTenantID,
		AgentKey:    DesignerAgentKey,
		DisplayName: "Custom Name",
		AgentType:   store.AgentTypePredefined,
		Status:      store.AgentStatusActive,
		Provider:    "custom",
		Model:       "custom-model",
	}
	edited.ID = store.GenNewID()
	agents.byKey[DesignerAgentKey] = edited

	if err := EnsureDesignerAgent(context.Background(), testConfig(), agents, &fakeSkillStore{}, t.TempDir()); err != nil {
		t.Fatalf("ensure over existing agent: %v", err)
	}
	if agents.created != 0 {
		t.Errorf("existing agent was recreated (%d creates), want 0", agents.created)
	}
	if agents.byKey[DesignerAgentKey].DisplayName != "Custom Name" {
		t.Error("existing agent fields were overwritten")
	}
}

// The persona migration upgrades an untouched system persona but leaves a
// custom-edited IDENTITY.md alone.
func TestUpgradeDesignerIdentity(t *testing.T) {
	agents := newFakeAgentStore()
	agent := &store.AgentData{AgentKey: DesignerAgentKey, AgentType: store.AgentTypePredefined}
	agent.ID = store.GenNewID()
	agents.byKey[DesignerAgentKey] = agent

	// System v1 persona → upgraded to current.
	agents.files[agent.ID] = map[string]string{"IDENTITY.md": designerIdentityV1}
	if err := upgradeDesignerIdentity(context.Background(), agents, agent.ID); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if got := agents.files[agent.ID]["IDENTITY.md"]; got != designerIdentity {
		t.Error("v1 persona was not upgraded to the current identity")
	}

	// Custom persona → untouched.
	agents.files[agent.ID] = map[string]string{"IDENTITY.md": "# custom persona"}
	if err := upgradeDesignerIdentity(context.Background(), agents, agent.ID); err != nil {
		t.Fatalf("upgrade over custom: %v", err)
	}
	if got := agents.files[agent.ID]["IDENTITY.md"]; got != "# custom persona" {
		t.Error("custom persona was overwritten")
	}
}

// The seeder may not have produced the skill rows yet (fresh install ordering);
// ensure must warn and continue, not fail the boot hook.
func TestEnsureDesignerAgentToleratesMissingSkills(t *testing.T) {
	agents := newFakeAgentStore()
	skills := &fakeSkillStore{}

	if err := EnsureDesignerAgent(context.Background(), testConfig(), agents, skills, t.TempDir()); err != nil {
		t.Fatalf("ensure with unseeded skills: %v", err)
	}
	if skills.granted != 0 {
		t.Errorf("granted = %d, want 0 when no skills exist", skills.granted)
	}
}

func TestDesignerSkillSlugsAreCoveredBySeeder(t *testing.T) {
	if len(designerSkillSlugs) != 2 {
		t.Fatalf("designerSkillSlugs = %v, want exactly 2 design skills", designerSkillSlugs)
	}
	seen := map[string]bool{}
	for _, s := range designerSkillSlugs {
		if seen[s] {
			t.Errorf("duplicate slug %q", s)
		}
		seen[s] = true
	}
}
