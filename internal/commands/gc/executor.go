package gc

import (
	"context"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// CommandDispatcher resolves a /gc: command from a raw user message.
// Workstream B consumes this interface to intercept /gc:* messages inside the
// agent loop. Its signature is a fixed contract — do not change without
// coordinating with the agent-loop wiring.
type CommandDispatcher interface {
	// Resolve parses msg; returns a Dispatch when it is a recognized /gc: command,
	// or (nil, false) for passthrough.
	Resolve(ctx context.Context, msg string) (*Dispatch, bool)
}

// Dispatch is the resolved /gc: command: the parsed kind, the skill slug it
// maps to, the full SKILL.md content loaded via skills.Loader.LoadSkill, the
// input remaining after the command word and flags, and the extracted flags.
type Dispatch struct {
	Kind      CommandKind
	Skill     string   // skill slug
	Content   string   // full SKILL.md content loaded via skills.Loader.LoadSkill
	Remaining string   // input after the command word + flags
	Flags     []string // extracted --flags from the input
	// SkillSpec is the structured spec when skillExecutor is wired.
	// Nil in legacy mode (no skillExecutor set).
	SkillSpec *skills.SkillSpec
}

// Executor resolves /gc: commands to their kit skill and builds the
// system-prompt section that drives the agent loop.
type Executor struct {
	loader *skills.Loader
	reg    *Registry
	// statusSnapshot optionally renders the /gc:status canned reply from live
	// state (scheduler lanes, run counts). Nil ⇒ static fallback text.
	statusSnapshot func() string
	// skillExecutor provides structured skill execution (permissions, gates,
	// artifacts). Nil ⇒ legacy prompt-only execution (backward compatible).
	skillExecutor *skills.SkillExecutor
}

// NewExecutor creates an executor backed by the given skills loader and
// command-to-skill registry. A nil loader or registry is tolerated: Resolve
// falls back to passthrough when the skill cannot be loaded.
func NewExecutor(loader *skills.Loader, reg *Registry) *Executor {
	return &Executor{loader: loader, reg: reg}
}

// SetStatusSnapshot wires the live renderer used by /gc:status. Optional.
func (e *Executor) SetStatusSnapshot(fn func() string) { e.statusSnapshot = fn }

// SetSkillExecutor wires the structured skill executor. When set, Resolve
// also returns a SkillSpec for the resolved skill, enabling permission
// enforcement and quality gate tracking. Optional — nil = legacy mode.
func (e *Executor) SetSkillExecutor(se *skills.SkillExecutor) { e.skillExecutor = se }

// SkillExecutor returns the wired skill executor, or nil if not set.
func (e *Executor) SkillExecutor() *skills.SkillExecutor { return e.skillExecutor }

// Resolve parses msg as a /gc: command, looks up the mapped skill slug, and
// loads the skill content. Returns a Dispatch when the command is recognized
// and its skill content can be loaded; (nil, false) otherwise.
func (e *Executor) Resolve(ctx context.Context, msg string) (*Dispatch, bool) {
	cmd, ok := Parse(msg)
	if !ok {
		return nil, false
	}
	if e.reg == nil {
		return nil, false
	}
	slug, ok := e.reg.Lookup(cmd.Kind)
	if !ok || slug == "" {
		return nil, false
	}
	if e.loader == nil {
		return nil, false
	}
	content, ok := e.loader.LoadSkill(ctx, slug)
	if !ok {
		return nil, false
	}
	d := &Dispatch{
		Kind:      cmd.Kind,
		Skill:     slug,
		Content:   content,
		Remaining: cmd.Input,
		Flags:     cmd.Flags,
	}
	// When skillExecutor is wired, also resolve the structured SkillSpec
	if e.skillExecutor != nil {
		if resolver := e.skillExecutor.Resolver(); resolver != nil {
			if spec, err := resolver.Resolve(ctx, slug); err == nil {
				d.SkillSpec = spec
			}
		}
	}
	return d, true
}

// BuildSystemPrompt builds the system-prompt section that instructs the agent
// to execute the resolved command using the skill workflow. It returns the
// skill content plus a short directive naming the command kind and requiring
// verification before completion is claimed.
func (e *Executor) BuildSystemPrompt(d *Dispatch) string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## /gc: Command Execution\n\n")
	b.WriteString(fmt.Sprintf("You are executing /gc:%s. Follow the skill workflow below and its quality gates. Do not claim completion until verification passes.\n\n", d.Kind.String()))
	b.WriteString("### Skill: " + d.Skill + "\n\n")
	b.WriteString(d.Content)
	if len(d.Flags) > 0 {
		b.WriteString("\n\nExecution flags: " + strings.Join(d.Flags, " "))
	}
	return b.String()
}

// ControlReply is a canned control-plane answer: the text injected into the
// turn and whether this dispatcher handled the kind.
type ControlReply struct {
	Text    string
	Handled bool
}

// CommandDispatcher2 is the optional extension contract for control-plane
// /gc: kinds (status, runs, doctor, approve). Implementors answer without a
// SKILL.md; the agent loop injects Text as a system note and skips the skill
// pipeline entirely. Wave-1 replies are canned guidance only — no direct
// state mutation from chat.
type CommandDispatcher2 interface {
	CommandDispatcher
	ResolveControl(ctx context.Context, msg string) (ControlReply, bool)
}

var _ CommandDispatcher2 = (*Executor)(nil)

// cannedReplies holds the static control-plane answers per kind. Runs/approve
// point at the WS methods because direct mutation from chat is out of scope
// for Wave 1; status/doctor are capability checklists over injected state.
func controlReply(kind CommandKind, snapshot func() string) string {
	switch kind {
	case KindStatus:
		if snapshot != nil {
			return "## /gc:status\n\n" + snapshot()
		}
		return "## /gc:status\n\nScheduler snapshot unavailable in this build. Use the dashboard Status page."
	case KindRuns:
		return "## /gc:runs\n\nRecent run history lives on the dashboard Runs page (WS `runs.list`). " +
			"Resume a paused run with WS `runs.resume`; this chat surface does not mutate runs directly."
	case KindDoctor:
		return "## /gc:doctor\n\nCapability checklist:\n" +
			"- Durable runs + checkpoints: enabled when reliability.runs is configured\n" +
			"- Recovery engine: active by default (`reliability.recovery.*` caps spend)\n" +
			"- Watchdog: escalates stalled/looping runs each heartbeat sweep\n" +
			"- Verifier gate: mode = advisory | recover | hard (`reliability.completion_verifier.mode`)\n" +
			"- Kit lockfile: `.goclaw/kit.lock` via kit install/verify\n" +
			"For a live diagnosis use the dashboard Health page."
	case KindApprove:
		return "## /gc:approve\n\nPending tool approvals are resolved on the dashboard Approvals page or via WS `exec_approval.*` methods. " +
			"This chat surface cannot approve shell execution directly."
	default:
		return ""
	}
}

// ResolveControl answers control-plane kinds with canned replies. Returns
// (reply, true) when msg is a recognized /gc:<control-kind>; (zero, false)
// otherwise so callers fall through to the skill pipeline untouched.
func (e *Executor) ResolveControl(ctx context.Context, msg string) (ControlReply, bool) {
	cmd, ok := Parse(msg)
	if !ok || !cmd.Kind.IsControlPlane() {
		return ControlReply{}, false
	}
	return ControlReply{Text: controlReply(cmd.Kind, e.statusSnapshot), Handled: true}, true
}
