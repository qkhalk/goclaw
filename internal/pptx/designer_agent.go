package pptx

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
// in the PPTX Studio's designer column. Dual-identity convention: the UUID
// stays internal, this key is what paths/prompts/logs/UI reference.
const DesignerAgentKey = "pptx-designer"

// designerSkillSlugs are the bundled design skills granted exclusively to the
// designer agent (bundled-skills/<slug>/SKILL.md, seeded by the skills
// seeder; this hook re-scopes them from public to internal + grants).
var designerSkillSlugs = []string{
	"pptx-deck-design",
	"pptx-visual-style",
}

// designerAllowTools is the complete tool surface of the designer agent:
// knowledge lookup only. No exec, no write_file, no delegate, no cron — the
// enforcement is the fail-closed execution gate that intersects the registry
// with this allowlist.
const designerAllowTools = `{"profile":"minimal","allow":["skill_search","use_skill","session_status"]}`

// DesignerToolPolicy returns the parsed tool policy of the designer agent.
// Single source of truth for the designer's tool surface: the loop's
// PolicyEngine must filter the registry down to exactly these tools.
func DesignerToolPolicy() *config.ToolPolicySpec {
	agent := &store.AgentData{ToolsConfig: json.RawMessage(designerAllowTools)}
	return agent.ParseToolsConfig()
}

// designerIdentity is the IDENTITY.md persona (English, LLM consumption).
// The deck contract mirrors the web studio's parse-deck-blocks validation.
const designerIdentity = `# Identity

Name: PPTX Designer
Emoji: 🖼️
Role: You are a senior presentation designer who plans slide decks.

You have ONE job: design decks. You do not control the system: you have no
shell, no file access, and no way to create files or export presentations.
If asked to run commands, change server settings, or export a .pptx file
yourself, politely decline and remind the user you only produce deck designs.

## How you design

- Deck length: 6 to 15 slides for most topics. Ask before going longer.
- Narrative arc: title → agenda or context → 3 to 5 content sections →
  takeaway → end. One idea per slide, never a wall of text.
- Bullets: at most 6 per slide, at most 12 words each, written in the user's
  language. Prefer concrete specifics over abstractions.
- Numbers: only use figures the user provided or asked you to estimate, and
  label estimates as such. Never invent statistics.
- Stats layout is for 2 to 4 key figures; quote layout for testimonials and
  sayings; image layout only when the user gave an image URL or path.
- Theme: harmonious palette, background/foreground contrast at least 4.5:1,
  accent used sparingly. Headings and body pick two distinct fonts from
  Arial, Calibri, Georgia, Verdana, Tahoma, Trebuchet MS, Times New Roman,
  Courier New. Load your design skills (use_skill) for detailed guidance
  before your first design of a session.

## Output contract (MANDATORY)

ALWAYS end a completed design reply with one fenced block:

` + "```deck" + `
{"version":1,"theme":{"background":"#0f172a","foreground":"#f8fafc","accent":"#38bdf8","muted":"#94a3b8","font_heading":"Arial","font_body":"Calibri"},"slides":[{"layout":"title","title":"DECK TITLE","subtitle":"One-line promise"},{"layout":"bullets","title":"Section","bullets":["First point","Second point"],"notes":"Speaker notes are optional"}]}
` + "```" + `

Rules: version must be 1. 1 to 40 slides. Valid layouts: title, section,
bullets, two_column, quote, stats, image, end. theme colors are "#RRGGBB".
two_column needs left and right objects with heading and bullets arrays.
stats needs 2 to 4 stats objects with value and label. quote needs quote
and author. image needs a non-empty source (URL or workspace path) and a
title. notes, when used, is a plain string. Emit ONLY the JSON inside the
fence, no comments.
`

// designerIdentityHistory lists every system-authored persona version, oldest
// first. A boot-time migration upgrades an existing agent's IDENTITY.md only
// when its content still matches one of these byte-for-byte — a persona an
// admin edited in the UI is never touched.
var designerIdentityHistory = []string{
	// v1 (2026-09-15): initial persona.
	designerIdentity,
}

// EnsureDesignerAgent creates the pptx-designer predefined agent when it does
// not exist yet (idempotent by agent_key; an admin's edits are never
// overwritten). It also re-scopes the bundled design skills to internal
// visibility and grants them to the agent so no other agent sees them. Runs
// at master scope like the skill seeder. The PPTX studio is fully client-side
// (preview + export happen in the browser), so there is no feature gate.
func EnsureDesignerAgent(ctx context.Context, cfg *config.Config, agentStore store.AgentStore, skills store.SkillManageStore, workspace string) error {
	if agentStore == nil {
		return fmt.Errorf("pptx: designer agent ensure: agent store unavailable")
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
			slog.Warn("pptx: designer agent identity upgrade failed", "error", err)
		}
	} else {
		provider := cfg.Agents.Defaults.Provider
		model := cfg.Agents.Defaults.Model
		ws := filepath.Join(workspace, "agents", DesignerAgentKey)
		agent := &store.AgentData{
			TenantID:          store.MasterTenantID,
			AgentKey:          DesignerAgentKey,
			DisplayName:       "PPTX Designer",
			Frontmatter:       "Design-only agent for the PPTX Studio: deck narrative, slide content, theme palettes. No system tools.",
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
			Emoji:             "🖼️",
			AgentDescription:  "Designs presentation decks. Replies always end with a ```deck JSON block.",
		}
		if err := agentStore.Create(ctx, agent); err != nil {
			return fmt.Errorf("pptx: designer agent create: %w", err)
		}
		agentID = agent.ID
		if _, err := bootstrap.SeedToStore(ctx, agentStore, agent.ID, agent.AgentType); err != nil {
			slog.Warn("pptx: designer agent bootstrap seed failed", "error", err)
		}
		if err := agentStore.SetAgentContextFile(ctx, agent.ID, "IDENTITY.md", designerIdentity); err != nil {
			slog.Warn("pptx: designer agent IDENTITY.md write failed", "error", err)
		}
		os.MkdirAll(ws, 0o755)
		slog.Info("pptx: designer agent created", "agent_key", DesignerAgentKey, "id", agentID.String())
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
			slog.Info("pptx: designer agent identity upgraded", "agent_id", agentID)
			return nil
		}
	}
	return nil // custom content — leave it alone
}

// grantDesignerSkills scopes the bundled design skills to the designer agent
// only: internal visibility (system flag off, so the ListAccessible filter
// requires a grant) plus an agent grant. Idempotent per boot.
func grantDesignerSkills(ctx context.Context, skills store.SkillManageStore, agentID uuid.UUID) error {
	grants, err := skills.ListWithGrantStatus(ctx, agentID)
	if err != nil {
		return fmt.Errorf("pptx: designer skill grant lookup: %w", err)
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
			slog.Warn("pptx: design skill not seeded yet", "slug", slug)
			continue
		}
		if g.Visibility != "internal" || g.IsSystem {
			updates := map[string]any{"visibility": "internal", "is_system": false}
			if err := skills.UpdateSkill(ctx, g.ID, updates); err != nil {
				return fmt.Errorf("pptx: design skill %s scope: %w", slug, err)
			}
		}
		if !g.Granted {
			if err := skills.GrantToAgent(ctx, g.ID, agentID, g.Version, "system"); err != nil {
				return fmt.Errorf("pptx: design skill %s grant: %w", slug, err)
			}
			slog.Info("pptx: design skill granted to designer", "slug", slug)
		}
	}
	return nil
}
