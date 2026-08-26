package agent

import (
	"context"
	"sync"
	"time"

	"log/slog"

	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// Run-level watchdog (Wave 1 WS-D, plan targets D1+D2).
//
// The per-provider stream watchdog (internal/providers reliability_wiring.go)
// covers transport silence inside one LLM call. Nothing reconciles the run as
// a whole: a loop that spins on the same tool, repeats the same output, or
// burns budget without producing artifacts stays "running" until the generic
// stale sweep terminal-fails it. This file adds that missing layer.
//
// D1 — classify: the watchdog subscribes to the same AgentEvent stream the
// resolver already forwards and maintains per-run event statistics:
//
//	no-event-N-s          → stalled        (silence across the whole run)
//	same-tool-repeat      → looping        (identical tool call repeatedly)
//	same-output-repeat    → looping        (byte-identical assistant text)
//	budget-growth-no-artifact → recovering-stuck (tokens climb, no deliverable)
//	everything else       → slow           (long-running but progressing)
//
// D2 — recovery ladder, applied in strict order, one rung per observation
// cycle so a mis-classification never skips straight to failure:
//
//	1. checkpoint  — the run record already holds a durable checkpoint; nothing
//	   to do but note it (resume capability is the safety net).
//	2. nudge       — inject a corrective user message via the router InjectCh
//	   pattern (read-only usage: Router.InjectMessage).
//	3. strategy switch / model fallback are OBSERVED here (metrics + log) but
//	   executed by WS-C's recovery engine inside ThinkStage / ModelFallbackProvider;
//	   this watchdog only escalates the signal.
//	4. safe-fail   — AbortRun + terminal(failed) with reason. Only after every
//	   earlier rung has been tried at least once for this run.
//
// The watchdog never touches agent_runs rows directly: persistence stays with
// runRecordUpdater (the single writer), and cross-restart reconciliation lives
// in cmd/gateway_heartbeat.go (D3).

// WatchdogVerdict classifies one observed run state (D1).
type WatchdogVerdict string

const (
	// VerdictHealthy means the run shows forward progress signals.
	VerdictHealthy WatchdogVerdict = "healthy"
	// VerdictSlow means the run has been active longer than the slow threshold
	// without tripping any stuck signal. Informational.
	VerdictSlow WatchdogVerdict = "slow"
	// VerdictStalled means no events arrived within the stall window.
	VerdictStalled WatchdogVerdict = "stalled"
	// VerdictLooping means repeated identical tool calls or outputs.
	VerdictLooping WatchdogVerdict = "looping"
	// VerdictRecoveringStuck means budget keeps growing while no artifact
	// (deliverable/final content) lands — the model is busy going nowhere.
	VerdictRecoveringStuck WatchdogVerdict = "recovering_stuck"
)

// WatchdogConfig tunes the run-level watchdog. Zero values fall back to the
// defaults below; negative values disable the corresponding detector.
type WatchdogConfig struct {
	// StallAfter with no events of any kind classifies a run stalled.
	// Default 90s (9 missed 10s heartbeats).
	StallAfter time.Duration
	// SlowAfter total runtime before a still-active run is flagged slow.
	// Default 10m.
	SlowAfter time.Duration
	// SameToolRepeatThreshold consecutive identical tool calls (name + args
	// hash) that classify looping. Default 4 — deliberately above the in-loop
	// detector's warning level (3) so this is a second, independent net.
	SameToolRepeatThreshold int
	// SameOutputRepeatThreshold byte-identical final-content repetitions that
	// classify looping. Default 3.
	SameOutputRepeatThreshold int
	// MaxRunDuration is the hard upper bound on run lifetime. Entries older
	// than this are silently evicted from watchdog tracking (no abort). This
	// catches zombie entries whose terminal event never arrived, preventing
	// the re-abort loop that the D2 ladder would otherwise trigger forever.
	// Default 30m. Set to a negative value to disable.
	MaxRunDuration time.Duration
}

// Watchdog defaults. SameToolRepeatThreshold sits above the tool-loop
// warning/critical thresholds (loop_tools.go) so the watchdog only fires when
// the in-loop net demonstrably did not stop the behavior.
const (
	DefaultWatchdogStallAfter          = 90 * time.Second
	DefaultWatchdogSlowAfter           = 10 * time.Minute
	DefaultWatchdogSameToolRepeat      = 4
	DefaultWatchdogSameOutputRepeat    = 3
	DefaultWatchdogMaxRunDuration      = 30 * time.Minute
	defaultWatchdogSweepInterval       = 30 * time.Second
	defaultWatchdogNudgeCooldown       = 2 * time.Minute
	defaultWatchdogLadderEscalateAfter = 2 // observations per rung before escalating
)

// Effective resolves zero/negative config values to the defaults.
func (c WatchdogConfig) Effective() WatchdogConfig {
	if c.StallAfter == 0 {
		c.StallAfter = DefaultWatchdogStallAfter
	}
	if c.SlowAfter == 0 {
		c.SlowAfter = DefaultWatchdogSlowAfter
	}
	if c.SameToolRepeatThreshold <= 0 {
		c.SameToolRepeatThreshold = DefaultWatchdogSameToolRepeat
	}
	if c.SameOutputRepeatThreshold <= 0 {
		c.SameOutputRepeatThreshold = DefaultWatchdogSameOutputRepeat
	}
	if c.MaxRunDuration == 0 {
		c.MaxRunDuration = DefaultWatchdogMaxRunDuration
	}
	return c
}

// runWatchdogState accumulates per-run signals from the agent event stream.
// All fields are guarded by the Watchdog mutex; entries live exactly as long
// as their run (created by Observe, removed by forgetLocked on terminal
// events or max-age eviction in Sweep).
type runWatchdogState struct {
	sessionKey string

	lastEventAt time.Time
	startedAt   time.Time

	lastToolKey   string // canonical name + args hash of last tool.call
	sameToolCount int

	lastOutput   string
	sameOutCount int

	lastTokens int // last reported cumulative usage
	tokensSeen bool

	hasArtifact bool // deliverable or non-empty final content seen
}

// verdict computes the D1 classification from accumulated signals. Order
// matters: hard-stuck signals beat duration-only ones.
func (s *runWatchdogState) verdict(cfg WatchdogConfig, now time.Time) WatchdogVerdict {
	if now.Sub(s.lastEventAt) >= cfg.StallAfter {
		return VerdictStalled
	}
	if s.sameToolCount >= cfg.SameToolRepeatThreshold ||
		s.sameOutCount >= cfg.SameOutputRepeatThreshold {
		return VerdictLooping
	}
	if s.tokensSeen && s.lastTokens > 0 && !s.hasArtifact &&
		now.Sub(s.startedAt) >= cfg.StallAfter {
		return VerdictRecoveringStuck
	}
	if now.Sub(s.startedAt) >= cfg.SlowAfter {
		return VerdictSlow
	}
	return VerdictHealthy
}

// Watchdog tracks live runs and drives the D2 recovery ladder.
// Construct with NewWatchdog; wire Observe into the gateway's agent-event fan-out
// and Run on its own goroutine (both nil-safe at the use sites).
type Watchdog struct {
	router *Router
	cfg    WatchdogConfig

	mu     sync.Mutex
	runs   map[string]*runWatchdogState
	ladder map[string]int // per-run escalation depth: 0=checkpoint … 4=safe-fail
	nudged map[string]time.Time
}

// NewWatchdog builds a watchdog over the given router. router may be nil in
// tests — nudges/aborts become logged no-ops.
func NewWatchdog(router *Router, cfg WatchdogConfig) *Watchdog {
	return &Watchdog{
		router: router,
		cfg:    cfg.Effective(),
		runs:   make(map[string]*runWatchdogState),
		ladder: make(map[string]int),
		nudged: make(map[string]time.Time),
	}
}

// Observe folds one agent event into per-run watchdog state. Called from the
// gateway's existing OnEvent fan-out; cheap enough for the hot path
// (map lookup + small string compares). Unknown/uninteresting events update
// only the liveness timestamp.
func (w *Watchdog) Observe(event AgentEvent, now time.Time) {
	if w == nil || event.RunID == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	st, ok := w.runs[event.RunID]
	if !ok {
		st = &runWatchdogState{startedAt: now, lastEventAt: now}
		w.runs[event.RunID] = st
	}
	st.lastEventAt = now
	switch event.Type {
	case protocol.AgentEventRunCompleted,
		protocol.AgentEventRunFailed,
		protocol.AgentEventRunCancelled:
		w.forgetLocked(event.RunID)
	case protocol.AgentEventActivity:
		if m, _ := event.Payload.(map[string]any); m != nil {
			if _, ok := m["phase"]; ok {
				st.sessionKey, _ = m["session_key"].(string)
			}
		}
	}
}

// ObserveToolCall records a tool-call observation (name + args identity).
// argsKey must be a stable hash of the arguments; the caller (gateway wiring)
// reuses the same hashing the loop detector uses.
func (w *Watchdog) ObserveToolCall(runID, sessionKey, toolName, argsKey string, now time.Time) {
	if w == nil || runID == "" {
		return
	}
	key := toolName + "|" + argsKey
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.ensureLocked(runID, sessionKey, now)
	st.lastEventAt = now
	if key == st.lastToolKey {
		st.sameToolCount++
	} else {
		st.lastToolKey = key
		st.sameToolCount = 1
	}
}

// ObserveAssistantOutput records an assistant final-content observation.
func (w *Watchdog) ObserveAssistantOutput(runID, sessionKey, content string, now time.Time) {
	if w == nil || runID == "" || content == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.ensureLocked(runID, sessionKey, now)
	st.lastEventAt = now
	if content != "" {
		st.hasArtifact = true
	}
	if content == st.lastOutput {
		st.sameOutCount++
	} else {
		st.lastOutput = content
		st.sameOutCount = 1
	}
}

// ObserveUsage records cumulative token usage for the budget-growth detector.
func (w *Watchdog) ObserveUsage(runID, sessionKey string, totalTokens int, now time.Time) {
	if w == nil || runID == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.ensureLocked(runID, sessionKey, now)
	st.lastEventAt = now
	st.lastTokens = totalTokens
	st.tokensSeen = true
}

// ensureLocked returns the state entry for runID, creating it when absent.
func (w *Watchdog) ensureLocked(runID, sessionKey string, now time.Time) *runWatchdogState {
	st, ok := w.runs[runID]
	if !ok {
		st = &runWatchdogState{startedAt: now, lastEventAt: now}
		w.runs[runID] = st
	}
	if sessionKey != "" {
		st.sessionKey = sessionKey
	}
	return st
}

// forget removes a finished run from tracking.
func (w *Watchdog) forget(runID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.forgetLocked(runID)
}

func (w *Watchdog) forgetLocked(runID string) {
	delete(w.runs, runID)
	delete(w.ladder, runID)
	delete(w.nudged, runID)
}

// Classify returns the current verdict for a run without mutating ladder
// state. Exposed for tests and status surfacing; production flow uses Sweep.
func (w *Watchdog) Classify(runID string, now time.Time) WatchdogVerdict {
	if w == nil {
		return VerdictHealthy
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	st, ok := w.runs[runID]
	if !ok {
		return VerdictHealthy
	}
	return st.verdict(w.cfg, now)
}

// Sweep evaluates every tracked run and applies the D2 recovery ladder to
// non-healthy ones. Returns the number of runs acted on (nudged or aborted).
// Called periodically from the heartbeat tick (cmd/gateway_heartbeat.go).
func (w *Watchdog) Sweep(ctx context.Context, now time.Time) int {
	if w == nil {
		return 0
	}
	type action struct {
		runID, sessionKey string
		verdict           WatchdogVerdict
		depth             int
	}
	var actions []action

	type eviction struct {
		runID, sessionKey string
		age               time.Duration
	}

	w.mu.Lock()

	// Age-based eviction: silently drop entries that exceeded MaxRunDuration.
	// This catches zombie entries whose terminal event never arrived (e.g.
	// lost under high load, crash before terminal emit). Without this the
	// D2 ladder would re-abort the same dead entry every sweep forever.
	var evictions []eviction
	for id, st := range w.runs {
		if w.cfg.MaxRunDuration > 0 && now.Sub(st.startedAt) >= w.cfg.MaxRunDuration {
			evictions = append(evictions, eviction{id, st.sessionKey, now.Sub(st.startedAt)})
			w.forgetLocked(id)
			continue
		}
		v := st.verdict(w.cfg, now)
		if v == VerdictHealthy {
			continue
		}
		depth := w.ladder[id]
		actions = append(actions, action{runID: id, sessionKey: st.sessionKey, verdict: v, depth: depth})
		w.ladder[id] = depth + 1
	}
	w.mu.Unlock()

	for _, e := range evictions {
		slog.Warn("run.watchdog_max_age_evicted",
			"run_id", e.runID,
			"session", e.sessionKey,
			"age", e.age,
			"max", w.cfg.MaxRunDuration,
		)
	}

	actuated := len(evictions)
	for _, a := range actions {
		if w.applyLadder(ctx, a.runID, a.sessionKey, a.verdict, a.depth, now) {
			actuated++
		}
	}
	return actuated
}

// applyLadder executes rung `depth` of the D2 ladder for one run.
//
// Rung order (plan D2): checkpoint → nudge → strategy-switch → model-fallback
// → safe-fail. Rungs 0/2/3 observe-and-log (checkpoint presence and the
// strategy/model levers belong to WS-C's engine); rung 1 injects a corrective
// nudge through Router.InjectMessage; rung 4 aborts the run via AbortRun and
// reports safe-fail so the caller can finalize the record terminal-failed.
func (w *Watchdog) applyLadder(ctx context.Context, runID, sessionKey string, v WatchdogVerdict, depth int, now time.Time) bool {
	switch depth {
	case 0:
		// Rung 1 — checkpoint: resume capability already exists via the
		// durable checkpoint written by CheckpointStage. Log-only.
		logWatchdog(v, runID, depth, "checkpoint available, observing")
		return false
	case 1:
		// Rung 2 — nudge via the router InjectCh pattern (read-only usage).
		if last, ok := w.nudged[runID]; ok && now.Sub(last) < defaultWatchdogNudgeCooldown {
			return false
		}
		msg := InjectedMessage{
			Content: "[System] Watchdog: your progress appears stalled (" + string(v) +
				"). Re-evaluate your approach: try a different tool or strategy instead of repeating previous steps.",
			UserID: "watchdog",
		}
		delivered := false
		if w.router != nil && sessionKey != "" {
			delivered = w.router.InjectMessage(sessionKey, msg)
		}
		w.mu.Lock()
		w.nudged[runID] = now
		w.mu.Unlock()
		logWatchdogAct(v, runID, depth, "nudge injected", delivered)
		return delivered
	case defaultWatchdogLadderEscalateAfter:
		// Rung 3 — strategy switch: owned by the WS-C recovery engine
		// (ThinkStage policy table). Escalate visibility only.
		logWatchdog(v, runID, depth, "strategy-switch lever owned by recovery engine")
		return false
	case 3:
		// Rung 4 — model fallback: owned by ModelFallbackProvider ordering.
		// Escalate visibility only.
		logWatchdog(v, runID, depth, "model-fallback lever owned by provider chain")
		return false
	default:
		// Rung 5 — safe fail: abort the run. The consumer outcome path
		// (loop error handling) finalizes the record; when the run has a
		// checkpoint it stays resumable per loop_run.go semantics.
		if w.router != nil {
			res := w.router.AbortRun(runID, "")
			logWatchdogAct(v, runID, depth, "safe-fail abort", res.Stopped || res.Forced)
		} else {
			logWatchdog(v, runID, depth, "safe-fail abort (router unavailable)")
		}
		return true
	}
}

func logWatchdog(v WatchdogVerdict, runID string, depth int, msg string) {
	watchdogLog("run.watchdog_ladder", v, runID, depth, msg, nil)
}

func logWatchdogAct(v WatchdogVerdict, runID string, depth int, msg string, ok bool) {
	watchdogLog("run.watchdog_action", v, runID, depth, msg, map[string]any{"delivered": ok})
}

// watchdogLog emits the shared watchdog log line. Split out so the ladder
// stays readable; attrs may be nil.
func watchdogLog(event string, v WatchdogVerdict, runID string, depth int, msg string, attrs map[string]any) {
	args := make([]any, 0, 8+len(attrs)*2)
	args = append(args, "verdict", string(v), "run_id", runID, "ladder_depth", depth, "detail", msg)
	for k, val := range attrs {
		args = append(args, k, val)
	}
	slog.Warn(event, args...)
}
