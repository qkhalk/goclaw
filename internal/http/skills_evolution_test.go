package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestApplySkillSuggestionPatchCreatesNewReferenceFile(t *testing.T) {
	handler, skillStore, ctx, root := newTestUploadHandler(t)
	evolution := &skillEvolutionStoreStub{}
	handler.SetEvolutionStore(evolution, nil)

	currentDir := filepath.Join(root, "skills-store", "reference-skill", "1")
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatalf("mkdir current skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "SKILL.md"), []byte(skillMarkdown("Reference Skill", "reference-skill")), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	skillID := skillStore.seedCustomSkill("reference-skill", currentDir, "active", nil)
	content := "Use the documented query syntax.\n"
	patch, err := json.Marshal(skillDraftPatch{Content: &content})
	if err != nil {
		t.Fatalf("marshal draft patch: %v", err)
	}
	suggestion := &store.SkillImprovementSuggestion{
		ID:             uuid.New(),
		SkillID:        skillID,
		TargetFile:     "references/troubleshooting.md",
		DraftPatch:     patch,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		SkillSlug:      "reference-skill",
		Status:         store.SkillSuggestionStatusApproved,
		SuggestionType: "skill_reference_add",
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/skills/"+skillID.String()+"/evolution/suggestions/"+suggestion.ID.String()+"/apply", nil).WithContext(ctx)

	applied, err := handler.applySkillSuggestionPatch(req, skillID, suggestion)
	if err != nil {
		t.Fatalf("apply suggestion: %v", err)
	}
	if applied.Status != store.SkillSuggestionStatusApplied {
		t.Fatalf("status = %q, want applied", applied.Status)
	}

	newReference := filepath.Join(root, "skills-store", "reference-skill", "2", "references", "troubleshooting.md")
	got, err := os.ReadFile(newReference)
	if err != nil {
		t.Fatalf("read created reference: %v", err)
	}
	if string(got) != content {
		t.Fatalf("created reference = %q, want %q", got, content)
	}
	if len(evolution.versions) != 1 || evolution.versions[0].Version != 2 {
		t.Fatalf("versions = %+v, want one version 2", evolution.versions)
	}
}

type skillEvolutionStoreStub struct {
	versions []store.SkillVersion
}

func (s *skillEvolutionStoreStub) GetSettings(context.Context, uuid.UUID) (*store.SkillEvolutionSettings, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) UpsertSettings(context.Context, store.SkillEvolutionSettings) (*store.SkillEvolutionSettings, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) RecordUsage(context.Context, store.SkillUsageMetric) error {
	return nil
}

func (s *skillEvolutionStoreStub) AggregateUsage(context.Context, uuid.UUID, *time.Time) (*store.SkillUsageStats, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) ListUsage(context.Context, uuid.UUID, int) ([]store.SkillUsageMetric, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) CreateSuggestion(context.Context, store.SkillImprovementSuggestion) (*store.SkillImprovementSuggestion, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) ListSuggestions(context.Context, uuid.UUID, string, int) ([]store.SkillImprovementSuggestion, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) GetSuggestion(context.Context, uuid.UUID) (*store.SkillImprovementSuggestion, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) UpdateSuggestionStatus(context.Context, uuid.UUID, string, string, string) (*store.SkillImprovementSuggestion, error) {
	return nil, nil
}

func (s *skillEvolutionStoreStub) MarkSuggestionApplied(_ context.Context, id uuid.UUID, version int, actorType, actorID string) (*store.SkillImprovementSuggestion, error) {
	return &store.SkillImprovementSuggestion{
		ID:                  id,
		Status:              store.SkillSuggestionStatusApplied,
		ReviewedByActorType: actorType,
		ReviewedByActorID:   actorID,
		AppliedVersion:      &version,
	}, nil
}

func (s *skillEvolutionStoreStub) CreateSkillVersion(_ context.Context, version store.SkillVersion) (*store.SkillVersion, error) {
	s.versions = append(s.versions, version)
	return &version, nil
}

func (s *skillEvolutionStoreStub) ListSkillVersions(context.Context, uuid.UUID, int) ([]store.SkillVersion, error) {
	return s.versions, nil
}

func (s *skillEvolutionStoreStub) GetSkillVersion(context.Context, uuid.UUID, int) (*store.SkillVersion, error) {
	return nil, nil
}
