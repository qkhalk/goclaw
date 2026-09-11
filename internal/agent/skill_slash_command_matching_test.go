package agent

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

func infos(slugs ...string) []skills.Info {
	out := make([]skills.Info, 0, len(slugs))
	for _, s := range slugs {
		out = append(out, skills.Info{Name: s, Slug: s})
	}
	return out
}

func TestMatchSkillCommandTarget_UnderscoreMatchesHyphenSlug(t *testing.T) {
	all := infos("security-audit", "ssl-audit", "loadtest")

	got, ok, _ := matchSkillCommandTarget(all, "security_audit", false)
	if !ok || got.Slug != "security-audit" {
		t.Errorf("security_audit → %+v ok=%v, want security-audit", got, ok)
	}

	got, ok, _ = matchSkillCommandTarget(all, "ssl_audit", false)
	if !ok || got.Slug != "ssl-audit" {
		t.Errorf("ssl_audit → %+v ok=%v, want ssl-audit", got, ok)
	}

	// Prefix with a message: remainder must survive normalization.
	got, ok, rest := matchSkillCommandTarget(all, "security_audit the checkout page", false)
	if !ok || got.Slug != "security-audit" || rest != "the checkout page" {
		t.Errorf("prefix match: got %+v ok=%v rest=%q", got, ok, rest)
	}

	// Existing hyphenless behavior unchanged.
	got, ok, _ = matchSkillCommandTarget(all, "loadtest", false)
	if !ok || got.Slug != "loadtest" {
		t.Errorf("loadtest → %+v ok=%v", got, ok)
	}

	// Unknown target still misses.
	if _, ok, _ = matchSkillCommandTarget(all, "nonexistent", false); ok {
		t.Errorf("unknown target must not match")
	}
}

func TestMatchSkillCommandTarget_TieOnEquivalentNames(t *testing.T) {
	// Two skills whose slugs differ only by underscore vs hyphen normalize to
	// the same key — the tie rule must yield no match rather than a guess.
	all := infos("sec-audit", "sec_audit")
	if _, ok, _ := matchSkillCommandTarget(all, "sec-audit", false); ok {
		t.Errorf("equivalent names must tie and not match")
	}
}
