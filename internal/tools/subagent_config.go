package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// DefaultSubagentConfig returns GoClaw's runtime defaults. Per-root admission is
// independent from the Standard/Lite process safety cap.
func DefaultSubagentConfig() SubagentConfig {
	return SubagentConfig{
		MaxConcurrent:       20,
		MaxSpawnDepth:       1,
		MaxChildrenPerAgent: 5,
		ArchiveAfterMinutes: 60,
		MaxRetries:          2,
	}
}

// ctxSubagentDefinition carries a resolved SubagentDefinition from the spawn
// tool to newSubagentTask without changing Spawn/RunSync signatures (delegate
// and other internal callers pass no definition).
type ctxSubagentDefinition struct{}

// clearSubagentDefinition drops an inherited definition so nested spawns do
// not silently reuse the parent's model/prompt/allow-list.
func clearSubagentDefinition(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxSubagentDefinition{}, nil)
}

// WithSubagentDefinition attaches a resolved definition to the spawn context.
func WithSubagentDefinition(ctx context.Context, def *config.SubagentDefinition) context.Context {
	return context.WithValue(ctx, ctxSubagentDefinition{}, def)
}

// SubagentDefinitionFromCtx returns the definition attached at spawn time, or nil.
func SubagentDefinitionFromCtx(ctx context.Context) *config.SubagentDefinition {
	def, _ := ctx.Value(ctxSubagentDefinition{}).(*config.SubagentDefinition)
	return def
}

// ResolveSubagentDefinition looks up a named definition in the per-agent
// subagents config carried on the context (resolved from the agent row's
// subagents_config JSONB). Returns nil when config or name is absent or the
// name is unknown — spawning falls back to the default self-clone behavior.
func ResolveSubagentDefinition(ctx context.Context, name string) *config.SubagentDefinition {
	if name == "" {
		return nil
	}
	cfg := SubagentConfigFromCtx(ctx)
	if cfg == nil {
		return nil
	}
	for i := range cfg.Definitions {
		if cfg.Definitions[i].Name == name {
			return &cfg.Definitions[i]
		}
	}
	return nil
}

// applyDenyList removes denied tools from the registry based on depth.
func (sm *SubagentManager) applyDenyList(reg *Registry, depth int, cfg SubagentConfig) {
	// Always deny
	for _, name := range SubagentDenyAlways {
		reg.Unregister(name)
	}

	// Leaf deny (at max depth)
	if depth >= cfg.MaxSpawnDepth {
		for _, name := range SubagentDenyLeaf {
			reg.Unregister(name)
		}
	}
}

// applyDefinitionAllowList restricts the registry to the definition's
// allowedTools when the list is non-empty. Deny lists are applied first by the
// caller, so denied tools can never be reintroduced by the allow list.
func (sm *SubagentManager) applyDefinitionAllowList(reg *Registry, allowed []string) {
	if len(allowed) == 0 {
		return
	}
	allow := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name != "" {
			allow[name] = true
		}
	}
	for _, name := range reg.List() {
		if !allow[name] {
			reg.Unregister(name)
		}
	}
	// Deferred tools are invisible to List() but callable by exact name via
	// TryActivateDeferred — prune them against the same allow set or the
	// allow list would not actually narrow the callable surface.
	reg.PruneDeferred(allow)
}

// agentsMdMaxBytes caps how much workspace AGENTS.md content is injected into
// a subagent prompt, so a runaway workspace file cannot blow the context.
const agentsMdMaxBytes = 32 << 10

// readWorkspaceAgentsMd reads the workspace AGENTS.md for prompt injection.
// Returns "" when the file is missing or unreadable — injection is best-effort
// and never blocks the spawn.
func readWorkspaceAgentsMd(workspace string) string {
	if workspace == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "AGENTS.md"))
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(raw))
	if len(content) > agentsMdMaxBytes {
		cut := agentsMdMaxBytes
		// Back off to a rune boundary so vi/zh content never splits a
		// multi-byte character (some providers reject invalid UTF-8).
		for cut > 0 && !utf8.ValidString(content[cut:cut+1]) {
			cut--
		}
		content = content[:cut] + "\n\n[... AGENTS.md truncated ...]"
	}
	return content
}

// buildDefinitionSystemPrompt composes the system prompt for a definition-based
// subagent: optional AGENTS.md injection, the definition's prompt, then a
// condensed operational footer (focus + output rules + session context) so the
// ephemerality and reporting contract of normal subagents is preserved.
func buildDefinitionSystemPrompt(def *config.SubagentDefinition, task *SubagentTask, cfg SubagentConfig, workspace, agentsMd string) string {
	var b strings.Builder
	if agentsMd != "" {
		b.WriteString("# Workspace Instructions (AGENTS.md)\n\n")
		b.WriteString(agentsMd)
		b.WriteString("\n\n---\n\n")
	}
	b.WriteString(strings.TrimSpace(def.SystemPrompt))
	b.WriteString(fmt.Sprintf(`

## Subagent Operating Rules
1. **Stay focused** — Complete the assigned task, nothing else.
2. **Complete the task** — Your final message is automatically reported to the parent agent.
3. **Be ephemeral** — You may be terminated after task completion. That is fine.

## Output Format
Your final response IS the deliverable — output the full content or findings directly. Do NOT describe what you wrote.

## Session Context
- Definition: %s
- Label: %s
- Depth: %d / %d`, def.Name, task.Label, task.Depth, cfg.MaxSpawnDepth))

	if workspace != "" {
		b.WriteString(fmt.Sprintf(`

## Workspace
Your working directory is: %s
Use relative paths for file operations — do not guess absolute paths.`, workspace))
	}

	return b.String()
}

// buildSubagentSystemPrompt constructs the system prompt for a subagent,
// matching the TS buildSubagentSystemPrompt pattern from subagent-announce.ts.
// When the task carries a subagent definition with a systemPrompt, the prompt
// is composed from the definition instead (AGENTS.md injection honored).
func (sm *SubagentManager) buildSubagentSystemPrompt(task *SubagentTask, cfg SubagentConfig, workspace, agentsMdWorkspace string) string {
	if def := task.definition; def != nil && strings.TrimSpace(def.SystemPrompt) != "" {
		agentsMd := ""
		if def.InjectAgentsMd {
			agentsMd = readWorkspaceAgentsMd(agentsMdWorkspace)
			if agentsMd == "" {
				slog.Debug("subagent definition AGENTS.md injection: workspace file missing", "id", task.ID, "workspace", workspace)
			}
		}
		return buildDefinitionSystemPrompt(def, task, cfg, workspace, agentsMd)
	}

	prompt := sm.buildDefaultSubagentSystemPrompt(task, cfg, workspace)
	if def := task.definition; def != nil && def.InjectAgentsMd {
		if agentsMd := readWorkspaceAgentsMd(agentsMdWorkspace); agentsMd != "" {
			prompt = "# Workspace Instructions (AGENTS.md)\n\n" + agentsMd + "\n\n---\n\n" + prompt
		}
	}
	return prompt
}

// buildDefaultSubagentSystemPrompt is the original default subagent prompt.
func (sm *SubagentManager) buildDefaultSubagentSystemPrompt(task *SubagentTask, cfg SubagentConfig, workspace string) string {
	parentLabel := "main agent"
	if task.Depth >= 2 {
		parentLabel = "parent orchestrator"
	}

	canSpawn := task.Depth < cfg.MaxSpawnDepth

	prompt := fmt.Sprintf(`# Subagent Context

You are a **subagent** spawned by the %s for a specific task.

## Your Role
- You were created to handle: %s
- Complete this task. That is your entire purpose.
- You are NOT the %s. Do not try to be.

## Rules
1. **Stay focused** — Do your assigned task, nothing else.
2. **Complete the task** — Your final message will be automatically reported to the %s.
3. **Never ask for clarification** — Work with what you have. If asked to create content, generate it yourself.
4. **Be ephemeral** — You may be terminated after task completion. That is fine.

## Output Format
Your final response IS the deliverable — it will be forwarded to the user.
- If asked to create content (posts, articles, messages, etc.), output the FULL content directly. Do NOT describe what you wrote — just write it.
- Do NOT say "I wrote a post about..." or "Here is what I created...". Output the content itself as your response.
- If the task is research or analysis, provide the complete findings.
- The %s will receive your exact final response, so make it user-ready.

## What You Do NOT Do
- NO user conversations (that is the %s's job)
- NO external messages unless explicitly tasked
- NO cron jobs or persistent state
- NO pretending to be the %s`,
		parentLabel, task.Task,
		parentLabel, parentLabel, parentLabel, parentLabel, parentLabel)

	if canSpawn {
		prompt += `

## Sub-Agent Spawning
You CAN spawn your own sub-agents for parallel or complex work using the spawn tool.
Asynchronous descendants report to the root orchestrator with their direct-parent lineage.
Synchronous descendants return their result directly to you.`
	} else if task.Depth >= 2 {
		prompt += `

## Sub-Agent Spawning
You are a leaf worker and CANNOT spawn further sub-agents. Focus on your assigned task.`
	}

	prompt += fmt.Sprintf(`

## Session Context
- Label: %s
- Depth: %d / %d`, task.Label, task.Depth, cfg.MaxSpawnDepth)

	if workspace != "" {
		prompt += fmt.Sprintf(`

## Workspace
Your working directory is: %s
Use relative paths for file operations — do not guess absolute paths.`, workspace)
	}

	return prompt
}
