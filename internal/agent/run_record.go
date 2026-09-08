package agent

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// defaultRunHeartbeatInterval is the cadence at which a live run's heartbeat_at
// is advanced in agent_runs when no reliability.runs.heartbeat_interval_ms is
// configured. Coalesced writes (D6) — not one write per event.
const defaultRunHeartbeatInterval = 10 * time.Second

// runRecordUpdater drives the durable agent_runs state machine for one run:
// create on start, advance heartbeat while running, checkpoint on demand,
// terminal on exit. All writes are non-fatal (D9) — a DB failure logs and
// never blocks the run.
type runRecordUpdater struct {
	l         *Loop
	req       RunRequest
	runs      store.RunsStore
	runID     string
	heartbeat *time.Ticker
	done      chan struct{}
	once      sync.Once
}

// startRunRecord creates the agent_runs row for a run and starts a heartbeat
// goroutine that keeps heartbeat_at fresh while the run executes. Returns nil
// when run-record tracking is disabled (store not wired) or the write fails.
func startRunRecord(ctx context.Context, l *Loop, req RunRequest) *runRecordUpdater {
	if l.runsStore == nil {
		return nil
	}
	if req.RunID == "" {
		return nil
	}
	now := time.Now().UTC()
	run := &store.AgentRun{
		RunID:       req.RunID,
		SessionKey:  req.SessionKey,
		UserID:      req.UserID,
		Channel:     req.Channel,
		ChatID:      req.ChatID,
		Status:      store.AgentRunStatusRunning,
		Attempt:     1,
		HeartbeatAt: now,
		StartedAt:   now,
		UpdatedAt:   now,
		CreatedAt:   now,
	}
	if l.agentUUID != uuid.Nil {
		run.AgentID = &l.agentUUID
	}
	// CreateRun is idempotent on (tenant_id, run_id): heartbeat runs reuse the
	// same "heartbeat:<agentKey>" run_id every tick, so this upserts cleanly.
	if err := l.runsStore.CreateRun(ctx, run); err != nil {
		slog.Warn("runs.create_failed", "run_id", req.RunID, "error", err)
		return nil
	}
	u := newRunRecordUpdater(ctx, l, req.RunID)
	if u != nil {
		// Run request context (session key, user, channel, tenant) backs the
		// checkpoint.created event enrichment. The resume path constructs the
		// updater without a request — checkpoint events stay best-effort there
		// (the checkpoint DB write itself is unaffected).
		u.req = req
	}
	return u
}

// newRunRecordUpdater builds the run-record heartbeat updater for an EXISTING
// run row and starts the heartbeat goroutine. It does not create or upsert the
// run record: ResumeRun must preserve the stored checkpoint, and CreateRun's
// ON CONFLICT upsert would clobber checkpoint with NULL. Callers that already
// created the row (Loop.Run) use startRunRecord, which calls this after
// CreateRun. Returns nil when run-record tracking is disabled. ctx must carry
// the tenant ID (TouchHeartbeat scopes by tenant_id).
func newRunRecordUpdater(ctx context.Context, l *Loop, runID string) *runRecordUpdater {
	if l.runsStore == nil {
		return nil
	}
	interval := l.runHeartbeatInterval
	if interval <= 0 {
		interval = defaultRunHeartbeatInterval
	}
	u := &runRecordUpdater{
		l:         l,
		runs:      l.runsStore,
		runID:     runID,
		heartbeat: time.NewTicker(interval),
		done:      make(chan struct{}),
	}
	go u.heartbeatLoop(ctx)
	return u
}

// heartbeatLoop advances heartbeat_at every interval until the run finishes.
// Uses a detached context so a cancelled run still records its heartbeat.
func (u *runRecordUpdater) heartbeatLoop(ctx context.Context) {
	safeCtx := context.WithoutCancel(ctx)
	for {
		select {
		case <-u.done:
			return
		case <-u.heartbeat.C:
			hbCtx, cancel := context.WithTimeout(safeCtx, 5*time.Second)
			err := u.runs.TouchHeartbeat(hbCtx, u.runID)
			cancel()
			if err != nil {
				slog.Warn("runs.heartbeat_failed", "run_id", u.runID, "error", err)
			}
		}
	}
}

// checkpoint writes a durable pipeline checkpoint for this run, transitioning
// the run record to the given status. Called on the pipeline's checkpoint
// cadence (status running) and just before pausing a failed run as resumable
// (status compacting). Non-fatal: a failed write only forfeits resume
// capability, it never blocks the pipeline.
func (u *runRecordUpdater) checkpoint(ctx context.Context, status string, state *pipeline.RunState) error {
	if u == nil || state == nil {
		return nil
	}
	checkpointRaw, err := state.MarshalCheckpoint()
	if err != nil {
		slog.Warn("runs.checkpoint_marshal_failed", "run_id", u.runID, "error", err)
		return err
	}
	safeCtx := context.WithoutCancel(ctx)
	cpCtx, cancel := context.WithTimeout(safeCtx, 5*time.Second)
	defer cancel()
	if err := u.runs.UpdateRunCheckpoint(cpCtx, u.runID, status, checkpointRaw); err != nil {
		slog.Warn("runs.checkpoint_failed", "run_id", u.runID, "status", status, "error", err)
		return err
	}
	u.emitCheckpointCreated(ctx, status, state.Iteration)
	return nil
}

// emitCheckpointCreated broadcasts the checkpoint.created agent event after a
// successful durable checkpoint write so the timeline (and any other
// subscriber of l.emit) can render resume points. Best-effort: the updater
// holds the run request only on the normal-run path (startRunRecord); the
// resume path leaves req zero and the missing session key makes the recorder
// skip the event rather than mislabel it. Non-fatal by construction.
func (u *runRecordUpdater) emitCheckpointCreated(ctx context.Context, status string, iteration int) {
	if u.l == nil || u.req.RunID == "" || u.req.SessionKey == "" {
		return
	}
	u.l.emit(AgentEvent{
		Type:       protocol.AgentEventCheckpointCreated,
		AgentID:    u.l.id,
		RunID:      u.req.RunID,
		UserID:     u.req.UserID,
		Channel:    u.req.Channel,
		ChatID:     u.req.ChatID,
		SessionKey: u.req.SessionKey,
		TenantID:   store.TenantIDFromContext(ctx),
		Payload: map[string]string{
			"iteration": strconv.Itoa(iteration),
			"status":    status,
		},
	})
}

// terminal marks the run record as terminal (completed/failed/cancelled),
// stopping the heartbeat goroutine. Idempotent (sync.Once) so the normal exit
// path and the panic safety-net cannot double-fire or double-close. Non-fatal
// on error.
func (u *runRecordUpdater) terminal(ctx context.Context, status, errMsg string) {
	if u == nil {
		return
	}
	u.once.Do(func() {
		u.heartbeat.Stop()
		close(u.done)
		safeCtx := context.WithoutCancel(ctx)
		termCtx, cancel := context.WithTimeout(safeCtx, 5*time.Second)
		defer cancel()
		if err := u.runs.UpdateRunTerminal(termCtx, u.runID, status, errMsg, time.Now().UTC()); err != nil {
			slog.Warn("runs.terminal_failed", "run_id", u.runID, "status", status, "error", err)
		}
	})
}