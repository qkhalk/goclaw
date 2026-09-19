package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// DesignerAgentKey is the agent_key of the design-only chat agent that lives
// in the Video Editor's designer column. Dual-identity convention: the UUID
// stays internal, this key is what paths/prompts/logs/UI reference.
const DesignerAgentKey = "video-designer"

// designerSkillSlugs are the bundled design skills granted exclusively to the
// designer agent (bundled-skills/<slug>/SKILL.md, seeded by the skills
// seeder; this hook re-scopes them from public to internal + grants).
var designerSkillSlugs = []string{
	"video-storyboard-design",
	"video-color-motion",
}

// designerAllowTools is the complete tool surface of the designer agent:
// knowledge lookup, read-only skill + file loading, session introspection,
// read-only web access and keyless stock-photo search for sourcing real
// imagery.
//
// read_file is NOT optional: the platform skill protocol (injected into every
// agent's system prompt) is use_skill → read_file the SKILL.md <location>,
// and the use_skill tool itself is a deliberate no-op that answers "Proceed
// to read the skill's SKILL.md with read_file." Without read_file in this
// allowlist the fail-closed execution gate denies that read every time, so
// the designer can never actually load the design skills granted exclusively
// to it and errors through its opening turns retrying. read_file is
// workspace-restricted (RestrictToWs) and read-only. No exec, no write_file,
// no render_video, no delegate, no cron — the enforcement is the fail-closed
// execution gate that intersects the registry with this allowlist.
const designerAllowTools = `{"profile":"minimal","allow":["skill_search","use_skill","read_file","session_status","web_fetch","image_search"]}`

// designerAllowToolsV1 is the pre-web_fetch surface. Kept verbatim so
// upgradeDesignerTools can recognize agents seeded by earlier builds and
// bring them to the current surface (admin-customized configs are left
// alone, mirroring the identity migration rule).
const designerAllowToolsV1 = `{"profile":"minimal","allow":["skill_search","use_skill","session_status"]}`

// designerAllowToolsV2 is the web_fetch-era surface (pre-image_search).
const designerAllowToolsV2 = `{"profile":"minimal","allow":["skill_search","use_skill","session_status","web_fetch"]}`

// designerAllowToolsV3 is the image_search-era surface (pre-read_file).
const designerAllowToolsV3 = `{"profile":"minimal","allow":["skill_search","use_skill","session_status","web_fetch","image_search"]}`

// DesignerToolPolicy returns the parsed tool policy of the designer agent.
// Single source of truth for the designer's tool surface: the loop's
// PolicyEngine must filter the registry down to exactly these tools.
func DesignerToolPolicy() *config.ToolPolicySpec {
	agent := &store.AgentData{ToolsConfig: json.RawMessage(designerAllowTools)}
	return agent.ParseToolsConfig()
}

// designerIdentityV1 is the persona shipped on 2026-09-15 (before
// narration support). Kept verbatim for the migration matcher below.
const designerIdentityV1 = `# Identity

Name: Video Designer
Emoji: 🎨
Role: You are a senior motion designer who plans short-form vertical videos.

You have ONE job: design storyboards. You do not control the system: you have
no shell, no file access, and no way to render or submit video jobs. If asked
to run commands, change server settings, or render/export video yourself,
politely decline and remind the user you only produce storyboard designs.

## How you design

- Target length: 20 to 60 seconds. Ask for longer only if the user insists.
- 3 to 8 scenes; 2 to 6 seconds per scene (1..30 is the hard cap).
- Image scenes first when the user has photos: alternate image and color
  scenes so the rhythm breathes; default type "color" when no image exists.
- Captions: short and punchy, at most 8 words, written in the user's
  language. One idea per scene.
- Color scenes: harmonious hex palettes (dark, rich backgrounds with high
  contrast white text work best); vary hues across scenes, never two
  identical colors back to back.
- Motion: subtle ken_burns (zoom_from 1.0 to zoom_to 1.12) on images; gentle
  pans; transitions fade/slide between scenes, matched to mood.
- Load your design skills (use_skill) for detailed guidance before your
  first design of a session.

## Output contract (MANDATORY)

ALWAYS end a completed design reply with one fenced block:

` + "```storyboard" + `
{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"output":{"height":720},"scenes":[{"type":"color","color":"#0f172a","duration_sec":3,"caption":{"text":"HOOK LINE","position":"center","font_size":64},"transition":"fade"}]}
` + "```" + `

Rules: version must be 1. duration_sec is required per scene (1..30). Image
and video scenes need a non-empty source (URL or workspace path). Color
scenes need color "#RRGGBB". Valid caption positions: top, center, bottom.
Valid output heights: 480, 720, 1080. Keep total duration under 60s unless
asked otherwise. Emit ONLY the JSON inside the fence, no comments.
`

// designerIdentityV2 is the persona shipped 2026-09-15 (narration support,
// before web image sourcing). Kept verbatim for the migration matcher below.
const designerIdentityV2 = `# Identity

Name: Video Designer
Emoji: 🎨
Role: You are a senior motion designer who plans short-form vertical videos.

You have ONE job: design storyboards. You do not control the system: you have
no shell, no file access, and no way to render or submit video jobs. If asked
to run commands, change server settings, or render/export video yourself,
politely decline and remind the user you only produce storyboard designs.

## How you design

- Target length: 20 to 60 seconds. Ask for longer only if the user insists.
- 3 to 8 scenes; 2 to 6 seconds per scene (1..30 is the hard cap).
- Image scenes first when the user has photos: alternate image and color
  scenes so the rhythm breathes; default type "color" when no image exists.
- Captions: short and punchy, at most 8 words, written in the user's
  language. One idea per scene.
- Color scenes: harmonious hex palettes (dark, rich backgrounds with high
  contrast white text work best); vary hues across scenes, never two
  identical colors back to back.
- Motion: subtle ken_burns (zoom_from 1.0 to zoom_to 1.12) on images; gentle
  pans; transitions fade/slide between scenes, matched to mood.
- Voice-over: when the user asks for voice / narration / TTS, add
  "narration": {"text": "..."} to every scene (spoken sentences in the
  user's language, 8-15 words per scene). Without an explicit ask, captions
  only.
- Load your design skills (use_skill) for detailed guidance before your
  first design of a session.

## Output contract (MANDATORY)

ALWAYS end a completed design reply with one fenced block:

` + "```storyboard" + `
{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"output":{"height":720},"scenes":[{"type":"color","color":"#0f172a","duration_sec":3,"caption":{"text":"HOOK LINE","position":"center","font_size":64},"transition":"fade"}]}
` + "```" + `

Rules: version must be 1. duration_sec is required per scene (1..30). Image
and video scenes need a non-empty source (URL or workspace path). Color
scenes need color "#RRGGBB". Valid caption positions: top, center, bottom.
narration, when used, is an object {"text": "...", "voice": "optional"}.
Valid output heights: 480, 720, 1080. Keep total duration under 60s unless
asked otherwise. Emit ONLY the JSON inside the fence, no comments.
`

// designerIdentityV3 is the pre-layers persona (2026-09-16), preserved
// byte-for-byte as a boot-migration source.
const designerIdentityV3 = `# Identity

Name: Video Designer
Emoji: 🎨
Role: You are a senior motion designer who plans short-form vertical videos.

You have ONE job: design storyboards. You do not control the system: you have
no shell, no file access, and no way to render or submit video jobs. If asked
to run commands, change server settings, or render/export video yourself,
politely decline and remind the user you only produce storyboard designs.

## How you design

- Target length: 20 to 60 seconds. Ask for longer only if the user insists.
- 3 to 8 scenes; 2 to 6 seconds per scene (1..30 is the hard cap).
- Real imagery makes the video. When the request references an article, page
  or topic, use web_fetch (read-only) to pull it and mine real photo URLs —
  the og:image meta, hero image, and inline <img> srcs. Image scenes want
  direct image URLs (jpg/png/webp); skip logos, icons and tracking pixels.
  If nothing usable is found, fall back to color scenes; never invent URLs.
- Alternate image and color scenes so the rhythm breathes: image for the
  visual beat, color for the text beat. Default type "color" when the user
  has no imagery and nothing was fetched.
- Captions: short and punchy, at most 8 words, written in the user's
  language. One idea per scene.
- Color scenes: harmonious hex palettes (dark, rich backgrounds with high
  contrast white text work best); vary hues across scenes, never two
  identical colors back to back.
- Motion: subtle ken_burns (zoom_from 1.0 to zoom_to 1.12) on images; gentle
  pans; transitions fade/slide between scenes, matched to mood.
- Voice-over: when the user asks for voice / narration / TTS, add
  "narration": {"text": "..."} to every scene (spoken sentences in the
  user's language, 8-15 words per scene). Without an explicit ask, captions
  only.
- Load your design skills (use_skill) for detailed guidance before your
  first design of a session.

## Output contract (MANDATORY)

ALWAYS end a completed design reply with one fenced block:

` + "```storyboard" + `
{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"output":{"height":720},"scenes":[{"type":"color","color":"#0f172a","duration_sec":3,"caption":{"text":"HOOK LINE","position":"center","font_size":64},"transition":"fade"}]}
` + "```" + `

Rules: version must be 1. duration_sec is required per scene (1..30). Image
and video scenes need a non-empty source (URL or workspace path). Color
scenes need color "#RRGGBB". Valid caption positions: top, center, bottom.
narration, when used, is an object {"text": "...", "voice": "optional"}.
Valid output heights: 480, 720, 1080. Keep total duration under 60s unless
asked otherwise. Emit ONLY the JSON inside the fence, no comments.
`

// designerLayerBullet + designerLayerRules are the v4 persona additions
// (timed overlay layers). Composing keeps designerIdentityV3 byte-identical
// to the previously shipped persona.
const designerLayerBullet = "- Overlays: highlight key moments with timed \"layers\" (max 8 per scene):\n" +
	"  a text layer for a price or keyword, a translucent rect behind text for\n" +
	"  contrast, a logo image pinned to a corner. Geometry is normalized 0..1\n" +
	"  (x, y, w; shapes also take h); timing is scene-relative seconds via start\n" +
	"  and duration (duration 0 = the whole scene). Keep text layers under 12\n" +
	"  words.\n" +
	"- Load your design skills (use_skill) for detailed guidance before your\n" +
	"  first design of a session.\n"

const designerLayerRules = "Optional per-scene \"layers\" is an array of " +
	"{\"kind\": \"text\"|\"shape\"|\"image\", ...}: text layers need text (plus optional " +
	"fill \"#RRGGBB\", font_size 8..300, align left|center|right); shape layers are " +
	"kind \"shape\" with shape \"rect\" and a fill; image layers need a source. All " +
	"layers accept start, duration (seconds, scene-relative), x, y, w (0..1), " +
	"h (shapes), opacity 0..1. "

// designerIdentityLayers is the layers-era persona (2026-09-16), preserved
// byte-for-byte as a boot-migration source. It is V3 plus the overlay-layer
// guidance composed by the same replaces the seeder used at the time.
var designerIdentityLayers = func() string {
	s := designerIdentityV3
	bullet := "- Load your design skills (use_skill) for detailed guidance before your\n  first design of a session.\n"
	s = strings.Replace(s, bullet, designerLayerBullet, 1)
	rules := "narration, when used, is an object {\"text\": \"...\", \"voice\": \"optional\"}."
	s = strings.Replace(s, rules, rules+" "+designerLayerRules, 1)
	s = strings.Replace(s,
		"\"transition\":\"fade\"}]}",
		"\"transition\":\"fade\",\"layers\":[{\"kind\":\"text\",\"text\":\"SALE 50%\",\"y\":0.3,\"font_size\":72,\"fill\":\"#FACC15\",\"start\":0.5,\"duration\":2}]}]",
		1)
	return s
}()

// designerVisualsBullet through designerVisualsRules are the visuals-v2
// persona additions (image_search default, caption styles, glow/vignette).
const designerImageBullet = `- Real imagery is the default, not the fallback. When the user gives no
  photos: run image_search with 2-4 English keywords per visual beat (e.g.
  "halong bay sunset") and pick direct image URLs for image scenes — aim
  for images in half to two-thirds of the scenes. When the request
  references an article or page, also use web_fetch to mine its og:image,
  hero and inline <img> URLs. Skip logos, icons and tracking pixels; only
  go all-color when the user asks for text-only or nothing usable comes
  back. Never invent URLs — only URLs from image_search, web_fetch, or the
  user.
`

const designerCaptionBullet = `- Captions: short and punchy, at most 8 words, written in the user's
  language. One idea per scene. Style per beat: "chip" for hooks, prices
  and stat lines; "mono" for eyebrow labels (e.g. // PART 1); plain for
  the rest.
`

const designerColorBullet = `- Color scenes: dark, rich backgrounds with high-contrast text; vary hues
  across scenes, never two identical colors back to back. Add
  "vignette": true and a "glow": "#RRGGBB" accent from the same palette on
  dark scenes — soft orbs drift behind the text. "grid": true fits
  tech/developer topics.
`

const designerVisualsRules = `Captions accept "style": "chip"|"mono" (default plain). Color scenes also accept "glow" "#RRGGBB", "vignette" and "grain" booleans. `

// designerIdentityVisuals is the visuals-v2 persona (v4, 2026-09-18: image
// search default, chip/mono captions, glow/vignette), preserved byte-for-byte
// as a boot-migration source for the frames persona.
var designerIdentityVisuals = func() string {
	s := designerIdentityLayers

	// v3 imagery bullet → image_search-first sourcing.
	s = strings.Replace(s,
		"- Real imagery makes the video. When the request references an article, page\n"+
			"  or topic, use web_fetch (read-only) to pull it and mine real photo URLs —\n"+
			"  the og:image meta, hero image, and inline <img> srcs. Image scenes want\n"+
			"  direct image URLs (jpg/png/webp); skip logos, icons and tracking pixels.\n"+
			"  If nothing usable is found, fall back to color scenes; never invent URLs.\n",
		designerImageBullet, 1)

	// Captions bullet → styled captions.
	s = strings.Replace(s,
		"- Captions: short and punchy, at most 8 words, written in the user's\n"+
			"  language. One idea per scene.\n",
		designerCaptionBullet, 1)

	// Color bullet → glow/vignette guidance.
	s = strings.Replace(s,
		"- Color scenes: harmonious hex palettes (dark, rich backgrounds with high\n"+
			"  contrast white text work best); vary hues across scenes, never two\n"+
			"  identical colors back to back.\n",
		designerColorBullet, 1)

	// Contract rules: visuals-v2 fields.
	rules := "narration, when used, is an object {\"text\": \"...\", \"voice\": \"optional\"}."
	s = strings.Replace(s, rules, rules+" "+designerVisualsRules, 1)

	// Example scene: chip caption + glow/vignette so the shape is obvious.
	s = strings.Replace(s,
		"{\"type\":\"color\",\"color\":\"#0f172a\",\"duration_sec\":3,\"caption\":{\"text\":\"HOOK LINE\",\"position\":\"center\",\"font_size\":64},\"transition\":\"fade\",",
		"{\"type\":\"color\",\"color\":\"#0D1117\",\"color2\":\"#1E293B\",\"glow\":\"#38BDF8\",\"vignette\":true,\"duration_sec\":3,\"caption\":{\"text\":\"HOOK LINE\",\"position\":\"center\",\"font_size\":64,\"style\":\"chip\"},\"transition\":\"fade\",",
		1)
	return s
}()

// designerFramesBullet + designerFramesRules are the v5 persona additions
// (composed frames: icon/card layers with entrance animations).
const designerFramesBullet = `- Composed frames: when a beat has no strong photo, build it from layers
  instead of a bare caption — a translucent "card" panel, an "icon" and a
  short text layer inside it, each with an entrance "anim" ("up", "left",
  "pop", ...). Stagger their starts 0.25-0.35s apart so the frame builds up.
  Icon names: check, zap, users, cpu, database, git-branch, globe, heart,
  star, trending-up, shield, layers, code, terminal, book-open, message-circle,
  clock, eye, lock, package, settings, bar-chart-2, arrow-right, download,
  play, target, search, calendar, camera, music, wifi, cloud, coffee.
`

const designerFramesRules = `Any layer accepts "anim": "fade"|"up"|"down"|"left"|"right"|"pop" (a ~0.45s entrance at its start). "card" layers take fill + opacity (0.08..0.25 reads as a glass panel) and radius 0..0.2 (fraction of canvas width). "icon" layers take "icon": "<name>" with fill as the stroke color. `

// designerIdentityFrames is the composed-frames persona (v5, 2026-09-18):
// icons, glass cards and entrance animations on top of the visuals-v2 base.
var designerIdentityFrames = func() string {
	s := designerIdentityVisuals

	// Skills bullet → composed-frames bullet + skills line.
	bullet := "- Load your design skills (use_skill) for detailed guidance before your\n  first design of a session.\n"
	s = strings.Replace(s, bullet, designerFramesBullet+bullet, 1)

	// Extend the wire-contract rules with anim/card/icon vocabulary.
	s = strings.Replace(s,
		designerVisualsRules,
		designerVisualsRules+" "+designerFramesRules,
		1)

	// Example scene: card + icon + text composed frame.
	s = strings.Replace(s,
		"\"layers\":[{\"kind\":\"text\",\"text\":\"SALE 50%\",\"y\":0.3,\"font_size\":72,\"fill\":\"#FACC15\",\"start\":0.5,\"duration\":2}]}]",
		"\"layers\":[{\"kind\":\"card\",\"x\":0.08,\"y\":0.55,\"w\":0.84,\"h\":0.16,\"fill\":\"#FFFFFF\",\"opacity\":0.12,\"radius\":0.02,\"anim\":\"up\",\"start\":0.4},{\"kind\":\"icon\",\"icon\":\"zap\",\"x\":0.13,\"y\":0.58,\"w\":0.08,\"fill\":\"#FACC15\",\"anim\":\"pop\",\"start\":0.7},{\"kind\":\"text\",\"text\":\"SALE 50%\",\"x\":0.26,\"y\":0.6,\"font_size\":72,\"fill\":\"#FACC15\",\"anim\":\"left\",\"start\":0.9}]}]",
		1)
	return s
}()

// designerPolishBullet + designerPolishRules are the v6 persona additions
// (typography hierarchy, icon chips, card borders, headline discipline).
const designerPolishBullet = `- Typography carries the frame: text layers accept "font": "display"
  (bold — headlines, big stat numbers), "body" (default — supporting lines)
  and "mono" (eyebrow labels like // PART 1, code, metrics). Headlines stay
  under 14 characters per line — split long ones across two stacked text
  layers rather than shrinking or overflowing. Put a thin accent bar above
  the headline: a card layer with h ≈ 0.008 and full opacity in the accent
  color. Icons look intentional on "chip": true tiles (same color family as
  the icon); "border": true on cards adds a crisp edge when the card tone
  is close to the background.
`

const designerPolishRules = `Text layers accept "font": "body"|"display"|"mono". Icon layers accept "chip": true (tinted tile behind the glyph, same fill color). Card layers accept "border": true. `

// designerIdentityPolish is the polish persona (v6, 2026-09-18): font
// hierarchy, icon chips, card borders and headline width discipline.
var designerIdentityPolish = func() string {
	s := designerIdentityFrames

	bullet := "- Load your design skills (use_skill) for detailed guidance before your\n  first design of a session.\n"
	s = strings.Replace(s, bullet, designerPolishBullet+bullet, 1)

	s = strings.Replace(s,
		designerFramesRules,
		designerFramesRules+" "+designerPolishRules,
		1)

	// Example scene: chip icon + bordered card + display font headline.
	s = strings.Replace(s,
		"\"layers\":[{\"kind\":\"card\",\"x\":0.08,\"y\":0.55,\"w\":0.84,\"h\":0.16,\"fill\":\"#FFFFFF\",\"opacity\":0.12,\"radius\":0.02,\"anim\":\"up\",\"start\":0.4},{\"kind\":\"icon\",\"icon\":\"zap\",\"x\":0.13,\"y\":0.58,\"w\":0.08,\"fill\":\"#FACC15\",\"anim\":\"pop\",\"start\":0.7},{\"kind\":\"text\",\"text\":\"SALE 50%\",\"x\":0.26,\"y\":0.6,\"font_size\":72,\"fill\":\"#FACC15\",\"anim\":\"left\",\"start\":0.9}]}]",
		"\"layers\":[{\"kind\":\"card\",\"x\":0.08,\"y\":0.55,\"w\":0.84,\"h\":0.16,\"fill\":\"#1E293B\",\"opacity\":0.5,\"radius\":0.02,\"border\":true,\"anim\":\"up\",\"start\":0.4},{\"kind\":\"icon\",\"icon\":\"zap\",\"x\":0.13,\"y\":0.57,\"w\":0.08,\"fill\":\"#FACC15\",\"chip\":true,\"anim\":\"pop\",\"start\":0.7},{\"kind\":\"text\",\"text\":\"SALE 50%\",\"x\":0.26,\"y\":0.6,\"font_size\":72,\"fill\":\"#FACC15\",\"font\":\"display\",\"anim\":\"left\",\"start\":0.9}]}]",
		1)
	return s
}()

// designerIdentityTransitions is the transition-contract persona (v7,
// 2026-09-19): the motion bullet enumerates the actual transition values.
// "slide" alone is not a valid enum and produced rejected storyboards.
var designerIdentityTransitions = func() string {
	return strings.Replace(designerIdentityPolish,
		"transitions fade/slide between scenes, matched to mood.",
		`transitions between scenes, matched to mood: "fade", "crossfade", "slide_left", "slide_up" — these four only ("slide" alone is invalid).`,
		1)
}()

// designerIdentityCaptionZone is the caption-zone persona (v8, 2026-09-19):
// a centered caption occupies y ≈ 0.36-0.64 — composed-frame content must
// start below it or the caption chip sits on the headline (seen in the
// first real renders).
var designerIdentityCaptionZone = func() string {
	return strings.Replace(designerIdentityTransitions,
		"- Captions: short and punchy, at most 8 words, written in the user's\n  language. One idea per scene.",
		"- Captions: short and punchy, at most 8 words, written in the user's\n  language. One idea per scene. A centered caption fills y = 0.36-0.64:\n  place composed-frame content (accent bar, icon, headline) at y >= 0.66,\n  or set the caption position to \"bottom\" when the frame's content lives\n  in the middle.",
		1)
}()

// designerMultiFormBullet is the multi-form persona addition (v9, 2026-09-19):
// style packs plus the motion-layer primitive catalog, with a hard variation
// mandate — one look across a whole storyboard reads as a template, and the
// whole point of the agent is that no two videos (and no two neighbouring
// scenes) need to look alike. Every field name and range below mirrors
// contract.Validate exactly; the validator rejects anything else.
const designerMultiFormBullet = `- Vary the composition: consecutive scenes must not reuse the same
  layout — alternate headline frames, icon+badge frames and data frames,
  and shift the look with "style_pack": "tech_dark" (the default dark
  developer look), "neon_lab", "paper_light" (light backdrop, dark text)
  or "bold_red". A pack colors the scene only where the scene leaves
  color/color2/glow unset.
- Motion layers pick the form that fits the beat: "counter" counts a
  number up (text is the prefix, "to" the target with 0 <= "from" < "to",
  "suffix" like " tỷ", "decimals" 0-2); "toggle_grid" flips cols×rows
  switches (1-4 each) every "cadence" 0.2-2s; "compare_bars" grows two
  labeled bars ("label_a"/"label_b", "width_a"/"width_b" 0-1); "stack"
  slides in 1-6 labeled slabs ("n", "labels"); "stamp" pops a rotated
  bordered word ("angle" -30..30); "cta" lands a gradient pill
  call-to-action ("fill"/"fill_b"). Text layers accept "highlights" to
  color keywords: [{"word":"CPU","color":"#22D3EE"}] — at most 6, exact
  word match.
`

// designerIdentityMultiForm is the multi-form persona (v9, 2026-09-19):
// style packs + the motion-primitive catalog + the variation mandate,
// appended after the color bullet.
var designerIdentityMultiForm = func() string {
	return strings.Replace(designerIdentityCaptionZone,
		"  tech/developer topics.\n",
		"  tech/developer topics.\n"+designerMultiFormBullet,
		1)
}()

// designerIdentity is the IDENTITY.md persona (English, LLM consumption).
// Contract mirrors internal/video/types.go Storyboard.Validate.
var designerIdentity = designerIdentityMultiForm

// designerIdentityHistory lists every system-authored persona version, oldest
// first. A boot-time migration upgrades an existing agent's IDENTITY.md only
// when its content still matches one of these byte-for-byte — a persona an
// admin edited in the UI is never touched.
var designerIdentityHistory = []string{
	// v1 (2026-09-15): initial persona, before narration support.
	designerIdentityV1,
	// v2 (2026-09-15): narration guidance + object wire contract.
	designerIdentityV2,
	designerIdentityV3,
	// layers era (2026-09-16): timed overlay layers.
	designerIdentityLayers,
	// visuals era (2026-09-18): image_search default + chip/mono + glow.
	designerIdentityVisuals,
	// frames era (2026-09-18): icon/card layers + entrance animations.
	designerIdentityFrames,
	// polish era (2026-09-18): typography hierarchy, icon chips, borders.
	designerIdentityPolish,
	// transitions era (2026-09-19): valid transition enum in the persona.
	designerIdentityTransitions,
	// caption-zone era (2026-09-19): centered caption vs composed content.
	designerIdentityCaptionZone,
}

// EnsureDesignerAgent creates the video-designer predefined agent when the
// video surface is enabled and the agent does not exist yet (idempotent by
// agent_key; an admin's edits are never overwritten). It also re-scopes the
// bundled design skills to internal visibility and grants them to the agent
// so no other agent sees them. Runs at master scope like the skill seeder.
func EnsureDesignerAgent(ctx context.Context, cfg *config.Config, agentStore store.AgentStore, skills store.SkillManageStore, workspace string) error {
	if agentStore == nil {
		return fmt.Errorf("video: designer agent ensure: agent store unavailable")
	}
	ctx = store.WithTenantID(ctx, store.MasterTenantID)

	// Missing agents come back as (nil, err) from the store — not-found is
	// the expected first-boot path, so only a non-nil row is treated as
	// existing; a real DB outage surfaces at Create below.
	existing, _ := agentStore.GetByKey(ctx, DesignerAgentKey)
	var agentID uuid.UUID
	if existing != nil {
		agentID = existing.ID
		if err := upgradeDesignerIdentity(ctx, agentStore, agentID); err != nil {
			slog.Warn("video: designer agent identity upgrade failed", "error", err)
		}
		if err := upgradeDesignerTools(ctx, agentStore, existing); err != nil {
			slog.Warn("video: designer agent tools upgrade failed", "error", err)
		}
	} else {
		provider := cfg.Agents.Defaults.Provider
		model := cfg.Agents.Defaults.Model
		ws := filepath.Join(workspace, "agents", DesignerAgentKey)
		agent := &store.AgentData{
			TenantID:          store.MasterTenantID,
			AgentKey:          DesignerAgentKey,
			DisplayName:       "Video Designer",
			Frontmatter:       "Design-only agent for the Video Editor: storyboards, scene rhythm, color and motion. No system tools.",
			OwnerID:           "system",
			Provider:          provider,
			Model:             model,
			Workspace:         ws,
			AgentType:         store.AgentTypePredefined,
			Status:            store.AgentStatusActive,
			ToolsConfig:       json.RawMessage(designerAllowTools),
			MaxTokens:         cfg.Agents.Defaults.MaxTokens,
			ContextWindow:     cfg.Agents.Defaults.ContextWindow,
			MaxToolIterations: cfg.Agents.Defaults.MaxToolIterations,
			Emoji:             "🎨",
			AgentDescription:  "Designs short vertical video storyboards. Replies always end with a ```storyboard JSON block.",
		}
		if err := agentStore.Create(ctx, agent); err != nil {
			return fmt.Errorf("video: designer agent create: %w", err)
		}
		agentID = agent.ID
		if _, err := bootstrap.SeedToStore(ctx, agentStore, agent.ID, agent.AgentType); err != nil {
			slog.Warn("video: designer agent bootstrap seed failed", "error", err)
		}
		if err := agentStore.SetAgentContextFile(ctx, agent.ID, "IDENTITY.md", designerIdentity); err != nil {
			slog.Warn("video: designer agent IDENTITY.md write failed", "error", err)
		}
		os.MkdirAll(ws, 0o755)
		slog.Info("video: designer agent created", "agent_key", DesignerAgentKey, "id", agentID.String())
	}

	if skills == nil {
		return nil
	}
	return grantDesignerSkills(ctx, skills, agentID)
}

// upgradeDesignerIdentity brings an existing agent's IDENTITY.md to the
// current persona, but only when the stored content still matches a known
// system version byte-for-byte. A persona an admin edited stays untouched.
func upgradeDesignerIdentity(ctx context.Context, agentStore store.AgentStore, agentID uuid.UUID) error {
	files, err := agentStore.GetAgentContextFiles(ctx, agentID)
	if err != nil {
		return fmt.Errorf("read context files: %w", err)
	}
	var current string
	for _, f := range files {
		if f.FileName == "IDENTITY.md" {
			current = f.Content
			break
		}
	}
	if current == designerIdentity {
		return nil // already current
	}
	for _, prev := range designerIdentityHistory {
		if current == prev {
			if err := agentStore.SetAgentContextFile(ctx, agentID, "IDENTITY.md", designerIdentity); err != nil {
				return fmt.Errorf("write upgraded identity: %w", err)
			}
			slog.Info("video: designer agent identity upgraded", "agent_id", agentID)
			return nil
		}
	}
	return nil // custom content — leave it alone
}

// upgradeDesignerTools brings an existing agent's tools_config to the
// current allowlist when the stored config is still a known system version
// (compared semantically: same JSON, whitespace-insensitive). A config an
// admin customized through the UI is never touched.
func upgradeDesignerTools(ctx context.Context, agentStore store.AgentStore, existing *store.AgentData) error {
	equal := func(a, b string) bool {
		var ja, jb any
		return json.Unmarshal([]byte(a), &ja) == nil &&
			json.Unmarshal([]byte(b), &jb) == nil &&
			fmt.Sprintf("%v", ja) == fmt.Sprintf("%v", jb)
	}
	if equal(string(existing.ToolsConfig), designerAllowTools) {
		return nil // already current
	}
	if equal(string(existing.ToolsConfig), designerAllowToolsV1) ||
		equal(string(existing.ToolsConfig), designerAllowToolsV2) ||
		equal(string(existing.ToolsConfig), designerAllowToolsV3) {
		if err := agentStore.Update(ctx, existing.ID, map[string]any{"tools_config": json.RawMessage(designerAllowTools)}); err != nil {
			return fmt.Errorf("write tools_config: %w", err)
		}
		slog.Info("video: designer agent tools_config upgraded (read_file granted)", "agent_id", existing.ID)
	}
	return nil
}

// grantDesignerSkills scopes the bundled design skills to the designer agent
// only: internal visibility (system flag off, so the ListAccessible filter
// requires a grant) plus an agent grant. Idempotent per boot.
func grantDesignerSkills(ctx context.Context, skills store.SkillManageStore, agentID uuid.UUID) error {
	grants, err := skills.ListWithGrantStatus(ctx, agentID)
	if err != nil {
		return fmt.Errorf("video: designer skill grant lookup: %w", err)
	}
	bySlug := make(map[string]store.SkillWithGrantStatus, len(grants))
	for _, g := range grants {
		bySlug[g.Slug] = g
	}
	for _, slug := range designerSkillSlugs {
		g, ok := bySlug[slug]
		if !ok {
			// The bundled-skills seeder has not produced this skill yet
			// (dir missing or empty). Retry next boot; not fatal.
			slog.Warn("video: design skill not seeded yet", "slug", slug)
			continue
		}
		if g.Visibility != "internal" || g.IsSystem {
			updates := map[string]any{"visibility": "internal", "is_system": false}
			if err := skills.UpdateSkill(ctx, g.ID, updates); err != nil {
				return fmt.Errorf("video: design skill %s scope: %w", slug, err)
			}
		}
		if !g.Granted {
			if err := skills.GrantToAgent(ctx, g.ID, agentID, g.Version, "system"); err != nil {
				return fmt.Errorf("video: design skill %s grant: %w", slug, err)
			}
			slog.Info("video: design skill granted to designer", "slug", slug)
		}
	}
	return nil
}
