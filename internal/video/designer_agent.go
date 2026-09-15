package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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
// knowledge lookup plus read-only web access for sourcing real imagery.
// No exec, no write_file, no render_video, no delegate, no cron — the
// enforcement is the fail-closed execution gate that intersects the
// registry with this allowlist.
const designerAllowTools = `{"profile":"minimal","allow":["skill_search","use_skill","session_status","web_fetch"]}`

// designerAllowToolsV1 is the pre-web_fetch surface. Kept verbatim so
// upgradeDesignerTools can recognize agents seeded by earlier builds and
// bring them to the current surface (admin-customized configs are left
// alone, mirroring the identity migration rule).
const designerAllowToolsV1 = `{"profile":"minimal","allow":["skill_search","use_skill","session_status"]}`

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

// designerIdentity is the IDENTITY.md persona (English, LLM consumption).
// Contract mirrors internal/video/types.go Storyboard.Validate.
const designerIdentity = `# Identity

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

// designerIdentityHistory lists every system-authored persona version, oldest
// first. A boot-time migration upgrades an existing agent's IDENTITY.md only
// when its content still matches one of these byte-for-byte — a persona an
// admin edited in the UI is never touched.
var designerIdentityHistory = []string{
	// v1 (2026-09-15): initial persona, before narration support.
	designerIdentityV1,
	// v2 (2026-09-15): narration guidance + object wire contract.
	designerIdentityV2,
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
	if equal(string(existing.ToolsConfig), designerAllowToolsV1) {
		if err := agentStore.Update(ctx, existing.ID, map[string]any{"tools_config": json.RawMessage(designerAllowTools)}); err != nil {
			return fmt.Errorf("write tools_config: %w", err)
		}
		slog.Info("video: designer agent tools_config upgraded (web_fetch granted)", "agent_id", existing.ID)
	}
	return nil // custom config — leave it alone
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
