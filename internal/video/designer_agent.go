package video

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// DesignerAgentKey is the agent_key of the design-only video designer agent.
// Identity convention: agent_key for prompts/paths/logs, UUID stays in the DB.
const DesignerAgentKey = "video-designer"

// designerAllowTools is the complete tool allowlist for the designer agent.
// Enforced at three layers (prompt filter, loop wiring, fail-closed execution
// gate) — the LLM never even sees other tool schemas. Deliberately excludes
// exec, write_file/edit, render_video, delegate, cron: the agent designs
// storyboards only; rendering is a user action in the video tool UI.
const designerAllowTools = `{"allow":["skill_search","use_skill","session_status"]}`

// designerIdentity is the IDENTITY.md persona for the designer agent. Kept as
// a raw string without backticks inside (fences are described in words).
const designerIdentity = `# Video Designer

You are a video designer. Your single job: turn what the user describes into a
GoClaw video storyboard. You design; you never operate the system.

## Output contract (STRICT)

Every design answer MUST end with ONE fenced code block whose info string is
the word "storyboard" (three backticks + storyboard), containing the complete
storyboard JSON. Inside the block put ONLY the JSON, nothing else. Example
shape (abbreviated):

  {"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"scenes":[ ... ]}

Rules for the JSON (validated server-side, invalid designs are rejected):
- version is exactly 1. Canvas: 1080x1920 (9:16), 1920x1080 (16:9), or 1080x1080 (1:1); fps 1-60.
- 1-60 scenes; each scene duration_sec is 1-30; total <= 600.
- Scene types: "image" (source = workspace-relative path or https URL), "video" (same), "color" (color = #RRGGBB).
- When the user gives no imagery, use "color" scenes with strong solid brand colors, one color per scene.
- Optional per scene: fit ("cover"|"contain"), mute, ken_burns {zoom_from, zoom_to, pan}, caption {text, position: "top"|"center"|"bottom", font_size}, narration, transition ("fade"|"crossfade"|"slide_left"|"slide_up"), transform {scale,x,y,rotate,opacity}, filter {brightness,contrast,saturate,blur}.
- Captions are short: max 8 words. They are titles, not paragraphs.

## Design guidelines

- Open with a hook scene (bold color, big caption), hold interest with varied
  compositions, end with a clear closing caption.
- Motion restraint: Ken Burns zoom 1.0 to 1.12, rotate at most 6 degrees,
  transitions at scene starts only. One idea per scene.
- Prefer 3-6 scenes for promos, 6-10 for explainers. Pacing: 2-4s per scene.
- Use your design skills (video-storyboard-design, video-color-motion) when a
  question needs depth; do not guess color or motion values.

## Boundaries

- You have NO system tools. You cannot run commands, write files, render
  videos, manage servers, or schedule jobs. Decline such requests politely
  and redirect to what you can do: design the storyboard.
- When the user pastes an article or long text, extract the key message and
  shape it into scene captions and pacing; keep total duration reasonable.
- Answer in the user's language. Keep prose brief; the storyboard block is
  the deliverable. When iterating, always re-emit the FULL storyboard JSON.`

// designerSkill is one design skill seeded for the designer agent.
type designerSkill struct {
	Slug        string
	Name        string
	Description string
	Content     string // full SKILL.md body (with frontmatter)
}

const skillStoryboardDesign = `---
name: video-storyboard-design
description: Composition rules for GoClaw video storyboards - scene rhythm, caption writing, pacing by video goal (intro, promo, explainer, news recap).
version: 1
---

# Video Storyboard Design

## Scene rhythm

- Hook (scene 1): strongest visual + shortest caption. Earn the next 5 seconds.
- Body: one idea per scene. Vary composition: alternate image/color, change
  caption position, alternate motion direction.
- Close (last scene): a call-to-action or summary caption, held 3-4s.

## Pacing by goal

| Goal | Scenes | Per scene | Total |
|------|--------|-----------|-------|
| Intro | 3-4 | 2-3s | 8-12s |
| Promo | 4-6 | 2-4s | 12-20s |
| Explainer | 6-10 | 3-4s | 25-40s |
| News recap | 4-8 | 2-3s | 10-25s |

## Captions

- Max 8 words, title case, no trailing punctuation.
- One message per caption; split complex ideas across scenes.
- Numeric hooks ("3 tips", "in 9 seconds") outperform sentences.

## Article-to-video (news recap)

1. Read the article once; write a 1-sentence summary.
2. Pick 3-5 key points; each becomes one scene caption (rewrite as max 8
   words, active voice, present tense).
3. Scene 1 = topic + source vibe (bold color, outlet-style caption).
4. Last scene = takeaway or question to the viewer.
5. Prefer color scenes unless the article names concrete imagery; keep the
   palette to 2-3 brand-consistent colors.
`

const skillColorMotion = `---
name: video-color-motion
description: Safe color palettes and restrained motion values for GoClaw video scenes - gradients, Ken Burns, rotate, filters, transition matching by mood.
version: 1
---

# Color & Motion

## Palettes (hex, dark-friendly)

- Tech launch: #667eea / #764ba2 / #2C5364
- Sunset energy: #ee0979 / #ff6a00 / #2b1055
- Trust blue: #1D4ED8 / #0EA5E9 / #0F172A
- Forest calm: #134E5E / #71B280 / #0B2027
- News studio: #0F172A / #DC2626 / #F8FAFC

Rules: 2-3 colors per video; captions stay white with a soft dark shadow;
never place saturated text on saturated backgrounds.

## Motion restraint

- Ken Burns: zoom_from 1.0, zoom_to 1.08-1.15, one pan direction max.
- Rotate: 0 by default; -6..6 degrees for dynamism on image scenes only.
- Filter: brightness 0.9-1.1, contrast 0.95-1.15, saturate 0.9-1.2, blur 0
  (blur only as a deliberate backdrop effect).
- Transform opacity: keep 1 except for crossfading feel; scale 1.0-1.1.

## Transition matching

| Mood | transition |
|------|-----------|
| Calm, corporate | fade |
| Narrative flow | crossfade |
| Energetic, news | slide_left |
| Upbeat, social | slide_up |

First scene never carries a transition. Use the same transition family
throughout one video; mixing more than two families reads as noise.
`

// DesignerSkills returns the design skills seeded for the designer agent.
func DesignerSkills() []designerSkill {
	return []designerSkill{
		{Slug: "video-storyboard-design", Name: "Video Storyboard Design", Description: "Scene rhythm, caption writing, pacing by video goal, article-to-video conversion", Content: skillStoryboardDesign},
		{Slug: "video-color-motion", Name: "Video Color and Motion", Description: "Safe palettes and restrained motion values (Ken Burns, rotate, filters, transitions)", Content: skillColorMotion},
	}
}

// EnsureDesignerAgent idempotently creates the video-designer predefined
// agent with a fail-closed tool allowlist and grants it the design skills
// (internal visibility, granted to this agent only). Safe to call at every
// gateway start; existing agents are left untouched apart from skill grants.
func EnsureDesignerAgent(ctx context.Context, cfg *config.Config, agentStore store.AgentStore, skillManage store.SkillManageStore, dataDir string) {
	if agentStore == nil {
		slog.Warn("video: designer agent not ensured (agent store unavailable)")
		return
	}
	ctx = store.WithTenantID(ctx, store.MasterTenantID)

	agent, err := agentStore.GetByKey(ctx, DesignerAgentKey)
	if err != nil || agent == nil {
		provider := cfg.Agents.Defaults.Provider
		model := cfg.Agents.Defaults.Model
		ws := cfg.Agents.Defaults.Workspace
		agent = &store.AgentData{
			AgentKey:         DesignerAgentKey,
			DisplayName:      "Video Designer",
			Frontmatter:      "Design-only agent for the video tool: turns requests into storyboard JSON. No system tools.",
			TenantID:         store.MasterTenantID,
			Provider:         provider,
			Model:            model,
			AgentType:        store.AgentTypePredefined,
			Status:           store.AgentStatusActive,
			Workspace:        filepath.Join(ws, "agents", DesignerAgentKey),
			ToolsConfig:      json.RawMessage(designerAllowTools),
			AgentDescription: "Video designer: chat in the video tool, get storyboard JSON you can apply to the timeline with one click.",
		}
		if err := agentStore.Create(ctx, agent); err != nil {
			slog.Warn("video: designer agent create failed", "error", err)
			return
		}
		slog.Info("video: designer agent created", "agent_id", agent.ID, "provider", provider, "model", model)
	}

	// Context files: bootstrap templates + designer identity.
	if _, err := bootstrap.SeedToStore(ctx, agentStore, agent.ID, store.AgentTypePredefined); err != nil {
		slog.Warn("video: designer agent bootstrap seed failed", "error", err)
	}
	if err := agentStore.SetAgentContextFile(ctx, agent.ID, "IDENTITY.md", designerIdentity); err != nil {
		slog.Warn("video: designer agent identity write failed", "error", err)
	}

	if skillManage == nil {
		slog.Warn("video: designer skills not seeded (skill manage store unavailable)")
		return
	}
	ensureDesignerSkills(ctx, skillManage, agent.ID, dataDir)
}

// ensureDesignerSkills seeds the design skills as internal managed skills and
// grants them to the designer agent. Idempotent by slug + grant status.
func ensureDesignerSkills(ctx context.Context, skillManage store.SkillManageStore, agentID uuid.UUID, dataDir string) {
	granted := make(map[uuid.UUID]bool)
	if list, err := skillManage.ListWithGrantStatus(ctx, agentID); err == nil {
		for _, s := range list {
			granted[s.ID] = s.Granted
		}
	}

	for _, sk := range DesignerSkills() {
		id, version, ok := findSkillBySlug(skillManage, ctx, sk.Slug)
		if !ok {
			var err error
			id, version, err = createManagedSkill(skillManage, ctx, sk, dataDir)
			if err != nil {
				slog.Warn("video: designer skill create failed", "slug", sk.Slug, "error", err)
				continue
			}
			slog.Info("video: designer skill created", "slug", sk.Slug, "version", version)
		}
		if !granted[id] {
			if err := skillManage.GrantToAgent(ctx, id, agentID, version, "system"); err != nil {
				slog.Warn("video: designer skill grant failed", "slug", sk.Slug, "error", err)
				continue
			}
			slog.Info("video: designer skill granted", "slug", sk.Slug, "agent", agentID)
		}
	}
}

// findSkillBySlug resolves a skill UUID by slug via ListAllSkills (managed
// store has no GetIDBySlug query). SkillInfo.ID is a string UUID.
func findSkillBySlug(skillManage store.SkillManageStore, ctx context.Context, slug string) (uuid.UUID, int, bool) {
	for _, s := range skillManage.ListAllSkills(ctx) {
		if s.Slug == slug && s.Status != "deleted" && s.Status != "archived" {
			if id, err := uuid.Parse(s.ID); err == nil {
				return id, s.Version, true
			}
		}
	}
	return uuid.Nil, 0, false
}

// createManagedSkill writes the SKILL.md into the managed skills-store dir
// and registers it in the DB with internal visibility (pattern:
// internal/http/skills_upload.go). Returns the new skill's UUID.
func createManagedSkill(skillManage store.SkillManageStore, ctx context.Context, sk designerSkill, dataDir string) (uuid.UUID, int, error) {
	version := skillManage.GetNextVersion(ctx, sk.Slug)
	destDir := filepath.Join(dataDir, "skills-store", sk.Slug, strconv.Itoa(version))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return uuid.Nil, 0, fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	skillPath := filepath.Join(destDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(sk.Content), 0644); err != nil {
		return uuid.Nil, 0, fmt.Errorf("write SKILL.md: %w", err)
	}
	sum := sha256.Sum256([]byte(sk.Content))
	hash := hex.EncodeToString(sum[:])
	desc := sk.Description
	fm := map[string]string{"name": sk.Slug, "description": sk.Description, "version": strconv.Itoa(version)}
	id, err := skillManage.CreateSkillManaged(ctx, store.SkillCreateParams{
		Name:        sk.Name,
		Slug:        sk.Slug,
		Description: &desc,
		OwnerID:     "system",
		Visibility:  "internal",
		Status:      "active",
		Version:     version,
		FilePath:    destDir,
		FileSize:    int64(len(sk.Content)),
		FileHash:    &hash,
		Frontmatter: fm,
	})
	if err != nil {
		return uuid.Nil, 0, err
	}
	return id, version, nil
}
