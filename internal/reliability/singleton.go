package reliability

import (
	"sync"
	"time"
)

// Runtime bundles the process-wide reliability components so consumers (agent
// loop, provider adapters, health commands) can reach one shared instance
// through Default() instead of each constructing their own. The bundle is
// rebuilt once at gateway startup from configuration via Configure(); tests
// may call Configure() again to swap in a fresh bundle.
type Runtime struct {
	Breaker   *CircuitBreaker
	Health    *HealthRegistry
	RateLimit *RateLimitCoordinator
	Metrics   *Metrics

	// Stream carries the stream watchdog timeouts consumed by the provider
	// adapters. Zero durations disable the corresponding watchdog. Set via
	// SetStream at gateway startup after Configure(); Default() returns it as
	// part of the current bundle snapshot.
	Stream StreamOptions

	// PrematureCompletion carries the opt-in premature-completion gate
	// configuration consumed by the pipeline's continuation gate. Disabled by
	// default (zero value) so the gate is off unless the operator enables it.
	PrematureCompletion PrematureCompletionOptions

	// Recovery carries the global recovery-engine budget consumed by the
	// pipeline's recovery policy engine. Zero value keeps the engine active
	// with its built-in defaults.
	Recovery RecoveryOptions
}

// PrematureCompletionOptions configures the premature-completion gate that
// runs after the think stage. A zero value keeps the gate disabled.
type PrematureCompletionOptions struct {
	Enabled bool
}

// RecoveryOptions carries the global recovery-engine budget consumed by the
// pipeline's recovery policy engine (internal/pipeline recover.go). Zero values
// keep the engine active with the production defaults (8 attempts / 5m).
type RecoveryOptions struct {
	// MaxRetryCount caps total recovery attempts (LLM re-asks, repairs,
	// compactions) per run. <=0 means the engine falls back to its default.
	MaxRetryCount int
	// MaxRetryTime caps wall-clock time spent recovering per run. <=0 means
	// the engine falls back to its default.
	MaxRetryTime time.Duration
}

// StreamOptions are the streaming watchdog timeouts enforced by provider
// adapters via reliability.Default().Stream. A zero duration disables that
// watchdog (0 = disabled); per-model overrides use ModelSpec.StreamTimeoutMs.
type StreamOptions struct {
	IdleTimeout      time.Duration // silence between two stream events (0 = disabled)
	FirstByteTimeout time.Duration // time to first stream event (0 = disabled)
}

var (
	// once lazily constructs the default bundle on the first Default() call.
	// It is never reset: Configure() only replaces curRuntime under mu, and
	// the once closure double-checks before constructing, so the two cannot
	// race or double-construct.
	once sync.Once
	// mu guards curRuntime for both reads and writes.
	mu         sync.RWMutex
	curRuntime *Runtime
)

// defaultRuntime builds the process default bundle. The circuit breaker uses
// the production defaults (5 consecutive failures → open, 2 → degraded,
// 30s cooldown, max 1 half-open probe with a 30s stale-probe timeout); the
// health registry shares the same breaker; the rate-limit coordinator is
// unlimited (maxPending 0); metrics start with a fresh recorder.
func defaultRuntime() *Runtime {
	breaker := NewCircuitBreaker(DefaultCircuitOptions())
	return &Runtime{
		Breaker:   breaker,
		Health:    NewHealthRegistry(breaker),
		RateLimit: NewRateLimitCoordinator(0),
		Metrics:   NewMetrics(),
	}
}

// Default returns the current process-wide reliability bundle, lazily
// constructed with the default options on first use. Callers must not retain
// the returned bundle across a Configure() call — treat it as the current
// view.
func Default() *Runtime {
	once.Do(func() {
		mu.Lock()
		defer mu.Unlock()
		if curRuntime == nil {
			curRuntime = defaultRuntime()
		}
	})
	mu.RLock()
	defer mu.RUnlock()
	return curRuntime
}

// Configure rebuilds the process-wide reliability bundle with the given
// circuit options and rate-limit pending cap (maxPending <= 0 disables the
// cap). It atomically replaces the previous bundle; references obtained
// earlier continue to work on their old state. The gateway calls this once at
// startup from its configuration; tests may call it multiple times to swap in
// fresh state between cases.
func Configure(opts CircuitOptions, maxPending int) *Runtime {
	r := &Runtime{
		Breaker:   NewCircuitBreaker(opts),
		RateLimit: NewRateLimitCoordinator(maxPending),
		Metrics:   NewMetrics(),
	}
	r.Health = NewHealthRegistry(r.Breaker)

	mu.Lock()
	curRuntime = r
	mu.Unlock()
	return r
}

// SetStream atomically swaps the stream watchdog timeouts on the current
// bundle (the one Default() returns). Callers that already hold a reference
// to an older bundle keep their previous snapshot — the swap only affects
// Default() consumers. 0 durations disable the corresponding watchdog.
func (r *Runtime) SetStream(opts StreamOptions) {
	// Rebuild the bundle so Default() readers get one consistent snapshot
	// rather than observing a half-updated runtime.
	next := &Runtime{
		Breaker:             r.Breaker,
		Health:              r.Health,
		RateLimit:           r.RateLimit,
		Metrics:             r.Metrics,
		Stream:              opts,
		PrematureCompletion: r.PrematureCompletion,
	}

	mu.Lock()
	curRuntime = next
	mu.Unlock()
}

// SetPrematureCompletion atomically swaps the premature-completion gate
// options on the current bundle. Consumers read the gate via
// reliability.Default().PrematureCompletion.Enabled; a zero value (or a
// bundle never touched by this method) keeps the gate disabled.
func (r *Runtime) SetPrematureCompletion(opts PrematureCompletionOptions) {
	next := &Runtime{
		Breaker:             r.Breaker,
		Health:              r.Health,
		RateLimit:           r.RateLimit,
		Metrics:             r.Metrics,
		Stream:              r.Stream,
		PrematureCompletion: opts,
	}

	mu.Lock()
	curRuntime = next
	mu.Unlock()
}

// SetRecovery atomically swaps the recovery-engine budget on the current
// bundle. Consumers read the budget via reliability.Default().Recovery; a zero
// value keeps the engine active with its built-in defaults. Mirrors the
// SetPrematureCompletion bundle-swap pattern so Default() readers observe one
// consistent snapshot.
func (r *Runtime) SetRecovery(opts RecoveryOptions) {
	next := &Runtime{
		Breaker:             r.Breaker,
		Health:              r.Health,
		RateLimit:           r.RateLimit,
		Metrics:             r.Metrics,
		Stream:              r.Stream,
		PrematureCompletion: r.PrematureCompletion,
		Recovery:            opts,
	}

	mu.Lock()
	curRuntime = next
	mu.Unlock()
}
