package methods

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/backup"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// BackupScheduleMethods handles backup.schedule.get/set/run — the periodic
// backup-to-cloud schedule (user item 18).
//
// Security: the schedule mutates the encrypted config_secrets store and can
// exfiltrate a full system archive to cloud storage, so every method is gated
// owner-only AND master-scope — the same guard pair as config.* (the S3
// credentials this feature drives are master-scope data).
//
// Lifecycle: NewBackupScheduleMethods constructs the ScheduleService and starts
// its ticker goroutine (idempotent). This makes the constructor the single
// wiring point: one line in registerAllMethods both registers the RPC surface
// and launches the scheduler.
type BackupScheduleMethods struct {
	service  *backup.ScheduleService
	secrets  store.ConfigSecretsStore
	eventBus bus.EventPublisher
}

// NewBackupScheduleMethods creates the schedule service and starts its loop.
// dsn/version mirror what the one-shot HTTP backup handler receives.
func NewBackupScheduleMethods(cfg *config.Config, dsn, version string, secrets store.ConfigSecretsStore, eventBus bus.EventPublisher) *BackupScheduleMethods {
	deps := backup.ScheduleDeps{
		DSN:           dsn,
		DataDir:       cfg.ResolvedDataDir(),
		WorkspacePath: cfg.WorkspacePath(),
		GoclawVersion: version,
	}
	svc := backup.NewScheduleService(secrets, deps)
	if err := svc.Start(); err != nil {
		slog.Error("backup schedule service failed to start", "error", err)
	}
	return &BackupScheduleMethods{service: svc, secrets: secrets, eventBus: eventBus}
}

// Service exposes the underlying scheduler (for graceful shutdown wiring).
func (m *BackupScheduleMethods) Service() *backup.ScheduleService {
	return m.service
}

func (m *BackupScheduleMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodBackupScheduleGet, m.requireMasterScope(m.requireOwner(m.handleGet)))
	router.Register(protocol.MethodBackupScheduleSet, m.requireMasterScope(m.requireOwner(m.handleSet)))
	router.Register(protocol.MethodBackupScheduleRun, m.requireMasterScope(m.requireOwner(m.handleRun)))
}

// requireOwner wraps a handler to only allow owner-role users.
func (m *BackupScheduleMethods) requireOwner(next gateway.MethodHandler) gateway.MethodHandler {
	return func(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
		if !client.IsOwner() {
			locale := store.LocaleFromContext(ctx)
			client.SendResponse(protocol.NewErrorResponse(
				req.ID, protocol.ErrUnauthorized,
				i18n.T(locale, i18n.MsgPermissionDenied, req.Method),
			))
			return
		}
		next(ctx, client, req)
	}
}

// requireMasterScope rejects calls whose context is scoped to a non-master
// tenant (system owners bypass). Same predicate as config.* — one rule.
func (m *BackupScheduleMethods) requireMasterScope(next gateway.MethodHandler) gateway.MethodHandler {
	return func(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
		if !store.IsMasterScope(ctx) {
			locale := store.LocaleFromContext(ctx)
			client.SendResponse(protocol.NewErrorResponse(
				req.ID, protocol.ErrUnauthorized,
				i18n.T(locale, i18n.MsgConfigMasterScopeOnly),
			))
			return
		}
		next(ctx, client, req)
	}
}

// handleGet returns the persisted schedule, or defaults when none exists yet.
func (m *BackupScheduleMethods) handleGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	schedule, err := backup.LoadSchedule(ctx, m.secrets)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	if schedule == nil {
		schedule = &backup.BackupSchedule{
			Enabled:     false,
			Interval:    backup.DefaultScheduleInterval,
			Destination: "s3",
			Retention:   0,
		}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"schedule": schedule,
	}))
}

// handleSet validates and persists schedule changes. Run-state fields
// (last_run/next_run/last_status/last_error) are preserved from the stored
// schedule — callers cannot forge them. next_run is recomputed when enabling
// or changing the interval, and cleared when disabling.
func (m *BackupScheduleMethods) handleSet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	var params struct {
		Enabled     bool   `json:"enabled"`
		Interval    string `json:"interval"`
		Destination string `json:"destination"`
		Retention   *int   `json:"retention"` // pointer: 0 is meaningful (unlimited)
	}
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	if params.Interval == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
			i18n.T(locale, i18n.MsgRequired, "interval")))
		return
	}

	existing, err := backup.LoadSchedule(ctx, m.secrets)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	if existing == nil {
		existing = &backup.BackupSchedule{}
	}

	intervalChanged := existing.Interval != params.Interval
	existing.Enabled = params.Enabled
	existing.Interval = params.Interval
	if params.Destination != "" {
		existing.Destination = params.Destination
	}
	if params.Retention != nil {
		existing.Retention = *params.Retention
	}

	if params.Enabled {
		now := time.Now()
		// Re-anchor the next run when enabling fresh or changing the interval;
		// keep the existing next_run otherwise so edits don't drift the cadence.
		if existing.NextRun == nil || intervalChanged {
			next, nerr := backup.NextRunTime(existing.Interval, now)
			if nerr != nil {
				client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
					i18n.T(locale, i18n.MsgInvalidRequest, nerr.Error())))
				return
			}
			existing.NextRun = &next
		}
	} else {
		existing.NextRun = nil
	}

	if err := backup.SaveSchedule(ctx, m.secrets, existing); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest,
			i18n.T(locale, i18n.MsgInvalidRequest, err.Error())))
		return
	}

	slog.Info("backup schedule updated", "enabled", existing.Enabled, "interval", existing.Interval, "retention", existing.Retention)
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"schedule": existing,
	}))
	emitAudit(m.eventBus, client, "backup_schedule.updated", "backup_schedule", backup.KeySchedule)
}

// handleRun triggers one backup run immediately (owner-initiated "run now").
// Responds before the run completes — status lands in last_run/last_status.
func (m *BackupScheduleMethods) handleRun(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	schedule, err := backup.LoadSchedule(ctx, m.secrets)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	if schedule == nil {
		schedule = &backup.BackupSchedule{
			Interval:    backup.DefaultScheduleInterval,
			Destination: "s3",
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"ok": true,
	}))
	emitAudit(m.eventBus, client, "backup_schedule.run", "backup_schedule", backup.KeySchedule)

	// Preserve master scope for the background run (secrets store reads).
	go func() {
		bgCtx := context.Background()
		m.service.RunNow(bgCtx, schedule)
	}()
}
