package pg

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cron"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const defaultCronCacheTTL = 2 * time.Minute

// PGCronStore implements store.CronStore backed by Postgres.
// GetDueJobs() uses an in-memory cache with TTL to reduce DB polling (1s interval).
type PGCronStore struct {
	db        *sql.DB
	mu        sync.Mutex
	baseCtx   context.Context    // lifecycle context for background DB operations
	cancelCtx context.CancelFunc // cancelled on Stop()
	onJob     func(job *store.CronJob) (*store.CronJobResult, error)
	onEvent   func(event store.CronEvent)
	running   bool
	stop      chan struct{}

	// Job cache: reduces GetDueJobs polling from 86,400 queries/day to ~720/day
	jobCache    []store.CronJob
	cacheLoaded bool
	cacheTime   time.Time
	cacheTTL    time.Duration

	retryCfg  cron.RetryConfig
	defaultTZ string // fallback IANA timezone for cron jobs without explicit TZ

	// Stale-execution lease reclaim. cron_exec stamps updated_at when a job
	// enters 'running'; a job still 'running' with next_run_at=NULL after
	// staleReclaimWindow can never self-recover while the process is alive
	// (0 = disabled; cmd wires job_timeout × (retries+1) + grace).
	staleReclaimWindow time.Duration
	lastReclaim        time.Time
}

func NewPGCronStore(db *sql.DB) *PGCronStore {
	return &PGCronStore{db: db, cacheTTL: defaultCronCacheTTL, retryCfg: cron.DefaultRetryConfig()}
}

// SetRetryConfig overrides the default retry configuration.
func (s *PGCronStore) SetRetryConfig(cfg cron.RetryConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retryCfg = cfg
}

// SetDefaultTimezone sets the fallback IANA timezone for cron expressions
// when a job does not specify its own timezone.
func (s *PGCronStore) SetDefaultTimezone(tz string) {
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			slog.Warn("security.invalid_default_timezone", "tz", tz, "err", err)
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultTZ = tz
}

// SetStaleReclaimWindow configures how long a job may stay in 'running'
// before the scheduler reclaims it as interrupted and reschedules it.
// Zero (default) disables reclaim.
func (s *PGCronStore) SetStaleReclaimWindow(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staleReclaimWindow = d
	s.lastReclaim = time.Time{}
}

func (s *PGCronStore) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	s.baseCtx, s.cancelCtx = context.WithCancel(context.Background())
	s.stop = make(chan struct{})
	s.running = true
	s.recomputeStaleJobs()
	go s.runLoop()
	slog.Info("pg cron service started")
	return nil
}

func (s *PGCronStore) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	close(s.stop)
	if s.cancelCtx != nil {
		s.cancelCtx()
	}
	s.running = false
}

func (s *PGCronStore) SetOnJob(handler func(job *store.CronJob) (*store.CronJobResult, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onJob = handler
}

func (s *PGCronStore) SetOnEvent(handler func(event store.CronEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = handler
}

func (s *PGCronStore) emitEvent(event store.CronEvent) {
	s.mu.Lock()
	fn := s.onEvent
	s.mu.Unlock()
	if fn != nil {
		fn(event)
	}
}
