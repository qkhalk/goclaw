package store

import (
	"context"
	"testing"
)

func TestIsSkillVisibleTo(t *testing.T) {
	alice := "alice"
	bob := "bob"
	ctx := WithUserID(context.Background(), alice)

	tests := []struct {
		name       string
		owner      string
		visibility string
		isSystem   bool
		want       bool
	}{
		{"system skill visible to anyone", "system", "private", true, true},
		{"public visible to non-owner", bob, "public", false, true},
		{"empty visibility treated as public", bob, "", false, true},
		{"private visible to owner", alice, "private", false, true},
		{"private hidden from non-owner", bob, "private", false, false},
		{"private with no owner treated as public", "", "private", false, true},
		{"internal requires explicit grant outside this helper", bob, "internal", false, false},
		{"unknown enum fails closed", bob, "team", false, false},
		{"uppercase private matched for owner", alice, "PRIVATE", false, true},
		{"whitespace public treated as public", bob, "  public  ", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSkillVisibleTo(ctx, tt.owner, tt.visibility, tt.isSystem)
			if got != tt.want {
				t.Fatalf("IsSkillVisibleTo(owner=%q, vis=%q, sys=%v) = %v, want %v",
					tt.owner, tt.visibility, tt.isSystem, got, tt.want)
			}
		})
	}
}

func TestFilterVisibleSkills(t *testing.T) {
	ctx := WithUserID(context.Background(), "alice")
	skills := []SkillInfo{
		{Slug: "sys", IsSystem: true, Visibility: "public"},
		{Slug: "mine-private", OwnerID: "alice", Visibility: "private"},
		{Slug: "theirs-private", OwnerID: "bob", Visibility: "private"},
		{Slug: "theirs-public", OwnerID: "bob", Visibility: "public"},
		{Slug: "theirs-pending", OwnerID: "bob", Visibility: "public", Status: SkillStatusPendingReview},
		{Slug: "mine-pending", OwnerID: "alice", Visibility: "public", Status: SkillStatusPendingReview},
		{Slug: "mine-draft", OwnerID: "alice", Visibility: "private", Status: SkillStatusDraft},
		{Slug: "legacy-active", OwnerID: "bob", Visibility: "public", Status: SkillStatusLegacyActive},
	}
	got := FilterVisibleSkills(ctx, skills)
	gotSlugs := map[string]bool{}
	for _, s := range got {
		gotSlugs[s.Slug] = true
	}
	for _, want := range []string{"sys", "mine-private", "theirs-public", "legacy-active"} {
		if !gotSlugs[want] {
			t.Errorf("expected %q in filtered output, got %v", want, gotSlugs)
		}
	}
	for _, leak := range []string{"theirs-private", "theirs-pending", "mine-pending", "mine-draft"} {
		if gotSlugs[leak] {
			t.Errorf("leaked non-discoverable skill %q to non-owner list: %v", leak, gotSlugs)
		}
	}
}

func TestIsStatusDiscoverable(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"", true},                    // legacy rows without status
		{SkillStatusPublished, true},  // review-approved
		{SkillStatusLegacyActive, true}, // pre-review rows
		{SkillStatusDraft, false},     // just created
		{SkillStatusPendingReview, false}, // submitted, awaiting admin
		{SkillStatusApproved, false},
		{SkillStatusRejected, false},
		{SkillStatusSuspended, false},
	}
	for _, tt := range tests {
		t.Run("status="+tt.status, func(t *testing.T) {
			if got := IsStatusDiscoverable(tt.status); got != tt.want {
				t.Fatalf("IsStatusDiscoverable(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
