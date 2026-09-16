package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// planMetaKey is the session-metadata key the plan tool persists its checklist
// under. SetSessionMetadata merges per key, so only this entry is touched —
// channel prefs (chat_mode etc.) sharing the same map are unaffected.
const planMetaKey = "plan"

const (
	planMaxSteps      = 20
	planMaxStepLength = 100 // runes
)

// planStep is one checklist entry.
type planStep struct {
	Text   string `json:"text"`
	Status string `json:"status"` // pending | in_progress | completed
}

// planState is the JSON shape persisted under planMetaKey and returned to the
// LLM as the canonical checklist state after every action. The same JSON is
// what the web chat's plan card renders from, so it must stay compact: a
// maxed-out plan marshals to well under the tool.result event preview cap.
type planState struct {
	Steps   []planStep `json:"steps"`
	Updated string     `json:"updated,omitempty"`
}

// PlanTool gives the agent a first-class, user-visible plan checklist for the
// current session. State persists in session metadata (survives across turns
// and renders as a card in the web chat). It is pure bookkeeping: no
// filesystem, no shell, no media — and deliberately NOT a deliverable, so the
// completion verifier never mistakes planning for finished work.
type PlanTool struct {
	sessions store.SessionStore
	now      func() time.Time
}

func NewPlanTool() *PlanTool { return &PlanTool{now: time.Now} }

func (t *PlanTool) SetSessionStore(s store.SessionStore) { t.sessions = s }

func (t *PlanTool) Name() string { return "plan" }

func (t *PlanTool) Description() string {
	return `Maintain a visible task plan (checklist) for the current session. The plan is shown to the user as a live card and persists across turns.

Actions:
- "set": replace the whole plan with the given steps, in order. Use when starting non-trivial work or when the approach changes.
- "update": change one step's status via its 1-based "step" number and "status". Mark "in_progress" when you start a step and "completed" only after it is verified done.
- "get": read the current plan.

Keep steps short and imperative (max 20 steps, 100 chars each). Update the plan as you work — the user watches progress through it.`
}

func (t *PlanTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"set", "update", "get"},
				"description": "set replaces the whole plan; update changes one step's status; get reads the current plan.",
			},
			"items": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Action=set only: the plan steps in execution order (1-20 items).",
			},
			"step": map[string]any{
				"type":        "integer",
				"description": "Action=update only: 1-based step number to change.",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "completed"},
				"description": "Action=update only: the new status for the step.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *PlanTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.sessions == nil {
		return ErrorResult("session store not available")
	}
	sessionKey := ToolSandboxKeyFromCtx(ctx)
	if sessionKey == "" {
		return ErrorResult("session context required")
	}

	action, _ := args["action"].(string)
	state := t.loadPlan(ctx, sessionKey)

	switch action {
	case "set":
		items, err := planItemsFromArgs(args)
		if err != nil {
			return ErrorResult(err.Error())
		}
		steps := make([]planStep, len(items))
		for i, text := range items {
			steps[i] = planStep{Text: text, Status: "pending"}
		}
		state = planState{Steps: steps}

	case "update":
		if len(state.Steps) == 0 {
			return ErrorResult("no plan to update: call action=set first")
		}
		stepIdx, ok := planStepIndex(args["step"], len(state.Steps))
		if !ok {
			return ErrorResult(fmt.Sprintf("step must be an integer between 1 and %d", len(state.Steps)))
		}
		status, _ := args["status"].(string)
		switch status {
		case "pending", "in_progress", "completed":
		default:
			return ErrorResult("status must be one of: pending, in_progress, completed")
		}
		state.Steps[stepIdx].Status = status

	case "get":
		// Read-only: no persistence, no Updated refresh.

	default:
		return ErrorResult("action must be one of: set, update, get")
	}

	if action != "get" {
		state.Updated = t.now().UTC().Format(time.RFC3339)
		raw, err := json.Marshal(state)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to encode plan: %v", err))
		}
		t.sessions.SetSessionMetadata(ctx, sessionKey, map[string]string{planMetaKey: string(raw)})
		if err := t.sessions.Save(ctx, sessionKey); err != nil {
			return ErrorResult(fmt.Sprintf("failed to persist plan: %v", err))
		}
	}

	// Canonical state back to the LLM: compact JSON only — the web card parses
	// exactly this shape from the tool.result event, so no extra prose.
	out, err := json.Marshal(state)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to encode plan: %v", err))
	}
	return NewResult(string(out))
}

// loadPlan reads the persisted checklist; a missing/corrupt entry yields an
// empty state (set rebuilds from scratch either way).
func (t *PlanTool) loadPlan(ctx context.Context, sessionKey string) planState {
	meta := t.sessions.GetSessionMetadata(ctx, sessionKey)
	raw := meta[planMetaKey]
	if raw == "" {
		return planState{}
	}
	var state planState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return planState{}
	}
	if state.Steps == nil {
		state.Steps = []planStep{}
	}
	return state
}

// planItemsFromArgs validates action=set's items: 1..planMaxSteps non-empty
// strings, each trimmed to planMaxStepLength.
func planItemsFromArgs(args map[string]any) ([]string, error) {
	raw, ok := args["items"].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("items is required for action=set (1-%d steps)", planMaxSteps)
	}
	if len(raw) > planMaxSteps {
		return nil, fmt.Errorf("too many items: %d (max %d)", len(raw), planMaxSteps)
	}
	items := make([]string, 0, len(raw))
	for _, v := range raw {
		text, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("items must be an array of strings")
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, fmt.Errorf("items must not contain empty steps")
		}
		// Rune-safe cut: slicing bytes can split a multi-byte character and
		// json.Marshal would silently replace it with U+FFFD.
		if runes := []rune(text); len(runes) > planMaxStepLength {
			text = string(runes[:planMaxStepLength])
		}
		items = append(items, text)
	}
	return items, nil
}

// planStepIndex converts the 1-based step argument (JSON number, json.Number,
// or numeric string) to a 0-based index. ok=false when non-integral or out of
// range — fractional values are rejected rather than silently truncated.
func planStepIndex(v any, count int) (int, bool) {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case json.Number:
		parsed, err := x.Float64()
		if err != nil {
			return 0, false
		}
		n = parsed
	case string:
		parsed, err := json.Number(x).Float64()
		if err != nil {
			return 0, false
		}
		n = parsed
	default:
		return 0, false
	}
	idx := int(n) - 1
	if n != float64(idx+1) {
		return 0, false
	}
	if idx < 0 || idx >= count {
		return 0, false
	}
	return idx, true
}
