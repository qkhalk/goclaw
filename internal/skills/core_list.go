package skills

// core_list.go — seed-mode constants and the "core" slug allowlist.
//
// skills.seed_mode (config skills.seed_mode / env GOCLAW_SKILLS_SEED_MODE)
// controls how many bundled skills the startup seeder installs on a fresh
// data directory:
//
//	all  (default, back-compat) — seed every bundled skill
//	core — seed only CoreSlugs below; the rest are installable on demand
//	      from the skill market (market.go / /v1/skills/market)
//	none — seed nothing; the reconciler still registers managed skills
//	      that are already on disk
//
// The mode never uninstalls anything: switching an existing deployment to
// "core" or "none" only affects fresh installs, and the reconciler keeps
// previously installed skills alive across upgrades.

// Seed mode values accepted by NormalizeSeedMode.
const (
	SeedModeAll  = "all"
	SeedModeCore = "core"
	SeedModeNone = "none"
)

// CoreSlugs is the slug allowlist seeded when skills.seed_mode = "core".
// Slugs were verified against the bundled skills/ directory — every entry
// has a SKILL.md on disk. Unknown slugs are ignored by the seeder filter,
// so a stale entry never breaks startup.
//
// The original plan named four "designer" skills (pptx-deck-design,
// pptx-visual-style, video-storyboard, video-color-motion) that do not
// exist in the bundled tree; the closest real document/media-design skills
// are listed instead (see plans/…/phase-10).
var CoreSlugs = []string{
	// System core — agent workflow staples (review, plan→issue-to-plan,
	// cook, fix, test, scout, journal).
	"review",
	"issue-to-plan",
	"cook",
	"fix",
	"test",
	"scout",
	"journal",
	// Document & media design — output skills for studio agents.
	"pptx",
	"docx",
	"pdf",
	"xlsx",
	"html-video",
	"remotion",
}

// CoreSlugSet returns CoreSlugs as a set for the seeder filter.
func CoreSlugSet() map[string]bool {
	set := make(map[string]bool, len(CoreSlugs))
	for _, slug := range CoreSlugs {
		set[slug] = true
	}
	return set
}

// NormalizeSeedMode validates a skills.seed_mode value. Empty and unknown
// values fall back to SeedModeAll so a typo never silently disables
// seeding (back-compat with pre-seed_mode deployments).
func NormalizeSeedMode(mode string) string {
	switch mode {
	case SeedModeCore:
		return SeedModeCore
	case SeedModeNone:
		return SeedModeNone
	default:
		return SeedModeAll
	}
}
