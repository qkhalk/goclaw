package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ResumeDriver evaluates the durable run-state machine: checkpoint round-trip,
// resume safety across status transitions, stale-run recovery and interrupted
// timeline reconciliation. Scenarios are coded here and selected by name in
// YAML — a state-machine sequence does not compress well into declarative
// fields, and inventing a mini-DSL for it would be its own bug surface.
type ResumeDriver struct {
	tenantID uuid.UUID
	cleanup  func()
}

func init() { RegisterDriver(&ResumeDriver{}) }

func (d *ResumeDriver) Name() string { return "resume" }

func (d *ResumeDriver) Setup(env *Env, runID string) (func(), error) {
	d.tenantID, d.cleanup = env.SeedTenant("run-" + runID)
	return d.cleanup, nil
}

func (d *ResumeDriver) Run(ctx context.Context, env *Env, runID string, c EvalCase) (string, error) {
	tctx := env.TenantCtx(d.tenantID)
	runIDFull := "eval-" + c.Name + "-" + runID

	switch c.Scenario {
	case "checkpoint_preserved":
		return d.scenarioCheckpointPreserved(tctx, env, runIDFull)
	case "create_idempotent":
		return d.scenarioCreateIdempotent(tctx, env, runIDFull)
	case "stale_recovery":
		return d.scenarioStaleRecovery(ctx, env, tctx, runIDFull)
	case "timeline_recovery":
		return d.scenarioTimelineRecovery(ctx, env, tctx, runIDFull)
	default:
		return "", fmt.Errorf("unknown scenario %q (see internal/eval/driver_resume.go)", c.Scenario)
	}
}

// scenarioCheckpointPreserved: a written checkpoint must survive non-terminal
// status transitions. Resume depends on this — UpdateRunStatus(paused→running)
// must never clear the state a resume will continue from.
func (d *ResumeDriver) scenarioCheckpointPreserved(ctx context.Context, env *Env, runID string) (string, error) {
	run := &store.AgentRun{
		RunID:      runID,
		SessionKey: "eval-sess-" + runID,
		Status:     store.AgentRunStatusRunning,
	}
	if err := env.Runs.CreateRun(ctx, run); err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}

	checkpoint := json.RawMessage(`{"iteration":3,"messages":[{"role":"user","content":"eval checkpoint"}]}`)
	if err := env.Runs.UpdateRunCheckpoint(ctx, runID, store.RunTimelineStatusPaused, checkpoint); err != nil {
		return "", fmt.Errorf("write checkpoint: %w", err)
	}

	got, err := env.Runs.GetRun(ctx, runID)
	if err != nil {
		return "", fmt.Errorf("get run after checkpoint: %w", err)
	}
	if got.Status != store.RunTimelineStatusPaused {
		return "", fmt.Errorf("status after checkpoint: want %q, got %q", store.RunTimelineStatusPaused, got.Status)
	}
	if len(got.Checkpoint) == 0 {
		return "", fmt.Errorf("checkpoint missing after write")
	}

	if err := env.Runs.UpdateRunStatus(ctx, runID, store.AgentRunStatusRunning); err != nil {
		return "", fmt.Errorf("resume status transition: %w", err)
	}
	got, err = env.Runs.GetRun(ctx, runID)
	if err != nil {
		return "", fmt.Errorf("get run after resume: %w", err)
	}
	if got.Status != store.AgentRunStatusRunning {
		return "", fmt.Errorf("status after resume: want running, got %q", got.Status)
	}
	if jsonEqual(got.Checkpoint, checkpoint) {
		return "checkpoint intact across paused→running", nil
	}
	return "", fmt.Errorf("checkpoint changed across resume transition: want %s, got %s", checkpoint, got.Checkpoint)
}

// scenarioCreateIdempotent: re-creating the same (tenant, run_id) must not
// error or duplicate rows. Note the upsert intentionally overwrites fields
// (including checkpoint) — heartbeat-loop reuse relies on the upsert, the
// resume path avoids it via newRunRecordUpdater; this eval pins only the
// no-duplicate/no-error contract.
func (d *ResumeDriver) scenarioCreateIdempotent(ctx context.Context, env *Env, runID string) (string, error) {
	mk := func() *store.AgentRun {
		return &store.AgentRun{
			RunID:      runID,
			SessionKey: "eval-sess-" + runID,
			Status:     store.AgentRunStatusRunning,
		}
	}
	if err := env.Runs.CreateRun(ctx, mk()); err != nil {
		return "", fmt.Errorf("first create: %w", err)
	}
	if err := env.Runs.CreateRun(ctx, mk()); err != nil {
		return "", fmt.Errorf("second create: %w", err)
	}
	runs, err := env.Runs.ListRuns(ctx, store.RunListOpts{RunID: runID})
	if err != nil {
		return "", fmt.Errorf("list runs: %w", err)
	}
	if len(runs) != 1 {
		return "", fmt.Errorf("want exactly 1 row for run_id, got %d", len(runs))
	}
	return "single row after duplicate create", nil
}

// scenarioStaleRecovery: a run whose heartbeat is older than the threshold
// must be marked failed by RecoverStaleRuns (gateway-restart reconciliation).
func (d *ResumeDriver) scenarioStaleRecovery(ctx context.Context, env *Env, tctx context.Context, runID string) (string, error) {
	run := &store.AgentRun{
		RunID:      runID,
		SessionKey: "eval-sess-" + runID,
		Status:     store.AgentRunStatusRunning,
	}
	if err := env.Runs.CreateRun(tctx, run); err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}
	// Backdate the heartbeat past the recovery threshold. RecoverStaleRuns is
	// cross-tenant (startup job), so it runs on the background context.
	if _, err := env.DB.ExecContext(tctx,
		`UPDATE agent_runs SET heartbeat_at = now() - interval '3 hours' WHERE run_id = $1`,
		runID); err != nil {
		return "", fmt.Errorf("backdate heartbeat: %w", err)
	}
	if _, err := env.Runs.RecoverStaleRuns(ctx, time.Hour); err != nil {
		return "", fmt.Errorf("recover stale runs: %w", err)
	}
	got, err := env.Runs.GetRun(tctx, runID)
	if err != nil {
		return "", fmt.Errorf("get run after recovery: %w", err)
	}
	if got.Status != store.AgentRunStatusFailed {
		return "", fmt.Errorf("stale run status: want failed, got %q", got.Status)
	}
	return "stale run marked failed", nil
}

// scenarioTimelineRecovery: a run with a started run.status timeline item but
// no terminal sibling must get a terminal failed item appended by
// RecoverInterruptedRuns (startup reconciliation for interrupted runs).
func (d *ResumeDriver) scenarioTimelineRecovery(ctx context.Context, env *Env, tctx context.Context, runID string) (string, error) {
	item := &store.RunTimelineItem{
		TenantID:   d.tenantID,
		RunID:      runID,
		SessionKey: "eval-sess-" + runID,
		Seq:        1,
		ItemType:   store.RunTimelineItemTypeRunStatus,
		Status:     store.RunTimelineStatusStarted,
	}
	if err := env.RunTimeline.AppendRunTimelineItem(tctx, item); err != nil {
		return "", fmt.Errorf("append started item: %w", err)
	}
	if _, err := env.RunTimeline.RecoverInterruptedRuns(ctx); err != nil {
		return "", fmt.Errorf("recover interrupted runs: %w", err)
	}
	items, err := env.RunTimeline.ListRunTimelineItems(tctx, store.RunTimelineListOpts{RunID: runID})
	if err != nil {
		return "", fmt.Errorf("list timeline: %w", err)
	}
	terminal := false
	for _, it := range items {
		if it.ItemType == store.RunTimelineItemTypeRunStatus &&
			(it.Status == store.RunTimelineStatusFailed ||
				it.Status == store.RunTimelineStatusCompleted ||
				it.Status == store.RunTimelineStatusCancelled) {
			terminal = true
		}
	}
	if !terminal {
		return "", fmt.Errorf("interrupted run got no terminal timeline item (%d items)", len(items))
	}
	return "terminal item appended for interrupted run", nil
}

// jsonEqual compares two JSON documents by value (key order independent).
func jsonEqual(a, b json.RawMessage) bool {
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	ja, _ := json.Marshal(va)
	jb, _ := json.Marshal(vb)
	return string(ja) == string(jb)
}
