package cmd

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/gateway/methods"
	"github.com/nextlevelbuilder/goclaw/internal/heartbeat"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/scheduler"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// watchdogSweeper is the slice of the run-level watchdog the heartbeat needs:
// classify live runs and escalate the recovery ladder. Optional so tests and
// lite builds can run the sweep without a router.
type watchdogSweeper interface {
	Sweep(ctx context.Context, now time.Time) int
}

// autoResumeConcurrency bounds parallel Loop.ResumeRun calls during D4
// startup auto-resume so a backlog of paused runs cannot flood the scheduler.
const autoResumeConcurrency = 2

// resumeFn resumes one interrupted run by ID (makeRunResumer shape).
type resumeFn func(ctx context.Context, runID string) (*agent.RunResult, error)

// runStaleRunsSweep periodically marks runs whose heartbeat has not advanced
// within staleAfter as failed (cross-tenant), preferring pause-if-checkpoint
// reconciliation: a stale run whose record carries a checkpoint is transitioned
// to "paused" (resumable) instead of terminal-failed. When a watchdog is
// supplied its per-cycle ladder escalation runs on the same cadence. Non-fatal
// per iteration. Negativized zero args fall back to the reliability.runs.*
// defaults (10s heartbeat → 60s stale → 30s sweep).
func runStaleRunsSweep(runs store.RunsStore, staleAfter, interval time.Duration, wd watchdogSweeper) {
	runStaleRunsSweepWithNotify(runs, staleAfter, interval, wd, nil)
}

// runStaleRunsSweepWithNotify is runStaleRunsSweep plus a per-failed-run
// callback. The gateway wires this to broadcast run.failed agent events so
// clients watching a swept run clear their streaming state instead of showing
// a stuck "agent is typing" indicator forever.
func runStaleRunsSweepWithNotify(runs store.RunsStore, staleAfter, interval time.Duration, wd watchdogSweeper, notify func(store.AgentRun)) {
	ctx := context.Background()
	if staleAfter <= 0 {
		staleAfter = (time.Duration(config.DefaultRunsStaleAfterMs) * time.Millisecond)
	}
	if interval <= 0 {
		interval = (time.Duration(config.DefaultRunsSweepIntervalMs) * time.Millisecond)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepStaleRunsOnce(ctx, runs, staleAfter, wd, notify)
		}
	}
}

// sweepStaleRunsOnce runs one sweep cycle. Terminal-failed runs are handed to
// notify (when set) after the store marks them failed. Pause-if-checkpoint
// reconciliation lives inside the store sweeps (RecoverStaleRuns[WithDetail]
// transition checkpointed stale rows to "paused"), so no pre-pass is needed.
func sweepStaleRunsOnce(ctx context.Context, runs store.RunsStore, staleAfter time.Duration, wd watchdogSweeper, notify func(store.AgentRun)) {
	var marked int
	if detailer, ok := runs.(store.StaleRunDetailer); ok && detailer != nil {
		failed, err := detailer.RecoverStaleRunsWithDetail(ctx, staleAfter)
		if err != nil {
			slog.Warn("runs.stale_sweep_failed", "error", err)
			return
		}
		marked = len(failed)
		if notify != nil {
			for _, r := range failed {
				notify(r)
			}
		}
	} else {
		n, err := runs.RecoverStaleRuns(ctx, staleAfter)
		if err != nil {
			slog.Warn("runs.stale_sweep_failed", "error", err)
			return
		}
		marked = int(n)
	}
	if marked > 0 {
		slog.Info("runs.stale_sweep_marked_failed", "count", marked)
	}

	// D2 — watchdog ladder on the heartbeat cadence.
	if wd != nil {
		if acted := wd.Sweep(ctx, time.Now()); acted > 0 {
			slog.Warn("run.watchdog_sweep_acted", "count", acted)
		}
	}
}

// autoResumePausedRuns resumes runs left in "paused" by a previous process,
// bounded to autoResumeConcurrency goroutines. Runs once per process start
// (called from the startup reconciliation block); each resume reuses the
// WS runs.resume path so no resume logic is duplicated. Failures are logged
// and skipped — a run that cannot resume stays paused and remains visible.
func autoResumePausedRuns(runs store.RunsStore, resumer resumeFn) {
	if resumer == nil {
		return
	}
	ctx := context.Background()
	rows, err := runs.ListRuns(ctx, store.RunListOpts{Status: store.RunTimelineStatusPaused, Limit: 100})
	if err != nil {
		slog.Warn("runs.auto_resume_list_failed", "error", err)
		return
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.RunID)
	}
	if len(ids) == 0 {
		return
	}
	slog.Info("runs.auto_resume_starting", "count", len(ids))
	sem := make(chan struct{}, autoResumeConcurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(runID string) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := resumer(ctx, runID); err != nil {
				slog.Warn("runs.auto_resume_failed", "run_id", runID, "error", err)
			} else {
				slog.Info("runs.auto_resumed", "run_id", runID)
			}
		}(id)
	}
	wg.Wait()
}

// makeHeartbeatRunFn creates a function that routes a heartbeat run through the scheduler's cron lane.
func makeHeartbeatRunFn(sched *scheduler.Scheduler) func(ctx context.Context, req agent.RunRequest) <-chan scheduler.RunOutcome {
	return func(ctx context.Context, req agent.RunRequest) <-chan scheduler.RunOutcome {
		return sched.Schedule(ctx, scheduler.LaneCron, req)
	}
}

// startCronAndHeartbeat starts the cron service and heartbeat ticker, wires the heartbeat
// wake function to the tool + RPC methods, and sets the adaptive token estimate function.
// Returns the heartbeat ticker (needed by lifecycle for shutdown).
func startCronAndHeartbeat(
	pgStores *store.Stores,
	server *gateway.Server,
	sched *scheduler.Scheduler,
	msgBus *bus.MessageBus,
	providerRegistry *providers.Registry,
	channelMgr *channels.Manager,
	cfg *config.Config,
	heartbeatTool *tools.HeartbeatTool,
	heartbeatMethods *methods.HeartbeatMethods,
) *heartbeat.Ticker {
	// Start cron service with job handler (routes through scheduler's cron lane)
	pgStores.Cron.SetOnJob(makeCronJobHandler(sched, msgBus, cfg, channelMgr, pgStores.Sessions, pgStores.Agents, pgStores.Tenants, pgStores.Providers, providerRegistry, pgStores.Missions))
	pgStores.Cron.SetOnEvent(func(event store.CronEvent) {
		server.BroadcastEvent(*protocol.NewEvent(protocol.EventCron, event))
	})
	// Reclaim hung cron executions: a job stuck in 'running' past its lease
	// window is marked interrupted and rescheduled. The window covers
	// worst-case runtime — (retries+1) × job_timeout — plus retry-backoff
	// headroom, so a legitimately retrying job is never reclaimed.
	if reclaimer, ok := interface{}(pgStores.Cron).(interface{ SetStaleReclaimWindow(time.Duration) }); ok {
		retries := cfg.Cron.MaxRetries
		if retries < 0 {
			retries = 0
		}
		reclaimer.SetStaleReclaimWindow(cfg.Cron.JobTimeoutDuration()*time.Duration(retries+1) + 2*time.Minute)
	}
	if err := pgStores.Cron.Start(); err != nil {
		slog.Warn("cron service failed to start", "error", err)
	}

	// Start heartbeat ticker (routes through scheduler's cron lane)
	heartbeatTicker := heartbeat.NewTicker(heartbeat.TickerConfig{
		Store:         pgStores.Heartbeats,
		Agents:        pgStores.Agents,
		Sessions:      pgStores.Sessions,
		ProviderStore: pgStores.Providers,
		ProviderReg:   providerRegistry,
		MsgBus:        msgBus,
		Sched:         sched,
		RunAgent:      makeHeartbeatRunFn(sched),
		ResolveGroupContext: func(ctx context.Context, channel, chatID string) (string, string) {
			channelType := resolveChannelType(channelMgr, channel)
			return channelType, resolveGroupDisplayTitle(ctx, channelMgr, channel, chatID, string(sessions.PeerGroup), "")
		},
	})
	heartbeatTicker.SetOnEvent(func(event store.HeartbeatEvent) {
		server.BroadcastEvent(*protocol.NewEvent(protocol.EventHeartbeat, event))
	})
	heartbeatTicker.Start()
	heartbeatMethods.SetAgentStore(pgStores.Agents)
	heartbeatMethods.SetProviderStore(pgStores.Providers)
	cronHeartbeatWakeFn = func(agentID string) {
		if id, err := uuid.Parse(agentID); err == nil {
			heartbeatTicker.Wake(id)
		}
	}

	// Adaptive throttle: reduce per-session concurrency when nearing the summary threshold.
	sched.SetTokenEstimateFunc(func(sessionKey string) (int, int) {
		bctx := context.Background()
		history := pgStores.Sessions.GetHistory(bctx, sessionKey)
		lastPT, lastMC := pgStores.Sessions.GetLastPromptTokens(bctx, sessionKey)
		tokens := agent.EstimateTokensWithCalibration(history, lastPT, lastMC)
		cw := pgStores.Sessions.GetContextWindow(bctx, sessionKey)
		if cw <= 0 {
			cw = config.DefaultContextWindow
		}
		return tokens, cw
	})

	return heartbeatTicker
}
