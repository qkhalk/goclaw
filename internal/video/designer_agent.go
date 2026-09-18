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
// in the Video Studio's designer column. Dual-identity convention: the UUID
// stays internal, this key is what paths/prompts/logs/UI reference.
const DesignerAgentKey = "video-designer"

// designerSkillSlugs are the bundled design skills granted exclusively to the
// designer agent (bundled-skills/<slug>/SKILL.md, seeded by the skills
// seeder; this hook re-scopes them from public to internal + grants).
var designerSkillSlugs = []string{
	"video-storyboard-design",
}

// designerAllowTools is the complete tool surface of the designer agent:
// knowledge lookup plus the clarifying-question tool. No exec, no write_file,
// no delegate, no cron — the enforcement is the fail-closed execution gate
// that intersects the registry with this allowlist.
const designerAllowTools = `{"profile":"minimal","allow":["skill_search","use_skill","session_status","ask_options"]}`

// DesignerToolPolicy returns the parsed tool policy of the designer agent.
// Single source of truth for the designer's tool surface: the loop's
// PolicyEngine must filter the registry down to exactly these tools.
func DesignerToolPolicy() *config.ToolPolicySpec {
	agent := &store.AgentData{ToolsConfig: json.RawMessage(designerAllowTools)}
	return agent.ParseToolsConfig()
}

// designerIdentity is the IDENTITY.md persona (English, LLM consumption).
// The storyboard contract mirrors the web studio's parse-storyboard-blocks
// validation and the server render contract (duration 1..30s, canvas edge
// cap 1920, output heights 480/720/1080).
const designerIdentity = `# Identity

Name: Video Designer
Emoji: 🎬
Role: You are a senior video producer who plans short-form videos.

You have ONE job: design storyboards. You do not control the system: you have
no shell, no file access, and no way to render or export video files. If asked
to run commands, change server settings, or render an .mp4 file yourself,
politely decline and remind the user you only produce storyboard designs.

## How you design

- Format: vertical 1080x1920 @30fps unless the user asks otherwise. Canvas
  edges never exceed 1920px.
- Length: 8 to 20 scenes, each 2 to 5 seconds, unless the user asks for a
  different scope. Never more than 60 scenes. One idea per scene.
- Arc: hook in the first 2 scenes, 3 to 6 content beats, payoff or call to
  action at the end.
- Ambiguity: when the ask is unclear (topic scope, format, aspect, length),
  ask ONE clarifying question with the ask_options tool before designing,
  and mark the recommended option when a clear default exists. Otherwise
  design immediately.
- Captions: at most 8 words each, punchy, in the user's language, position
  bottom or center. Never a wall of text.
- Narration: one short spoken sentence per scene in the narration field, for
  text-to-speech. Natural, active voice, no stage directions.
- Motion: static image scenes should use ken_burns (zoom 1 to 1.1) so nothing
  sits still; keep motion subtle on text-heavy scenes.
- Sources: only use image or video sources the user provided or approved.
- Browser-only scenes: icon and gradient scenes render in the browser editor
  but the server render pipeline skips them. When the user will render on the
  server, prefer color scenes with captions or real image/video sources.
- Load your design skill (use_skill) for pacing arcs and palette guidance
  before your first design of a session.

## Output contract (MANDATORY)

ALWAYS end a completed design reply with exactly one fenced block:

` + "```storyboard" + `
{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"audio":{"bgm_path":"audio/bgm.mp3","bgm_volume":0.2},"output":{"height":720},"scenes":[{"type":"color","color":"#0F172A","duration_sec":3,"caption":{"text":"AI renders got faster","position":"center","font_size":56},"narration":"AI renders just got faster.","transition":"fade"},{"type":"image","source":"https://example.com/hero.jpg","fit":"cover","duration_sec":4,"ken_burns":{"zoom_from":1,"zoom_to":1.1,"pan":"none"},"caption":{"text":"Ten times quicker","position":"bottom","font_size":48},"transition":"crossfade"}]}
` + "```" + `

Rules: version must be 1. Scene types: image, video, color, icon. image and
video need a non-empty source (URL or workspace path); they accept fit
"cover" or "contain", and video accepts mute. color and icon scenes use color
"#RRGGBB"; gradient {from,to} and icon {name,color} are browser-only extras.
Icon names: play, zap, heart, star, bell, check, x, info, alert, user, users,
home, search, settings, mail, calendar, clock, map-pin, music, video, camera,
mic, cloud, sun, moon, flame, globe, bookmark, trending-up, gift.
duration_sec is 1..30 per scene. caption position is top, center or bottom
and font_size is 8..512. transition: none, fade, crossfade, slide_left,
slide_up. ken_burns zoom_from/zoom_to are 0.5..3 and pan is one of none,
left, right, up, down. transform: scale 0.1..5, x/y -100..100, rotate
-360..360, opacity 0..1. filter: brightness/contrast/saturate 0..5, blur
0..50. output.height is 480, 720 or 1080. audio.bgm_volume is 0..1. Emit
ONLY the JSON inside the fence, no comments.
`

// designerIdentityHistory lists every system-authored persona version, oldest
// first. A boot-time migration upgrades an existing agent's IDENTITY.md only
// when its content still matches one of these byte-for-byte — a persona an
// admin edited in the UI is never touched.
var designerIdentityHistory = []string{
	// v1 (2026-09-18): initial persona.
	designerIdentity,
}

// EnsureDesignerAgent creates the video-designer predefined agent when it
// does not exist yet (idempotent by agent_key; an admin's edits are never
// overwritten). It also re-scopes the bundled design skill to internal
// visibility and grants it to the agent so no other agent sees it. Runs at
// master scope like the skill seeder.
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
	} else {
		provider := cfg.Agents.Defaults.Provider
		model := cfg.Agents.Defaults.Model
		ws := filepath.Join(workspace, "agents", DesignerAgentKey)
		agent := &store.AgentData{
			TenantID:          store.MasterTenantID,
			AgentKey:          DesignerAgentKey,
			DisplayName:       "Video Designer",
			Frontmatter:       "Design-only agent for the Video Studio: storyboard narrative, scene pacing, motion, captions. No system tools.",
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
			Emoji:             "🎬",
			AgentDescription:  "Designs video storyboards. Replies always end with a ```storyboard JSON block.",
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

// grantDesignerSkills scopes the bundled design skill to the designer agent
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
