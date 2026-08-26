package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/reliability"
)

// RetryConfig configures retry behavior for provider requests.
type RetryConfig struct {
	Attempts int           // max attempts (default 3, 1 = no retry)
	MinDelay time.Duration // initial delay (default 300ms)
	MaxDelay time.Duration // delay cap (default 30s)
	Jitter   float64       // jitter factor ±N (default 0.1 = ±10%)
}

// RetryHookFunc is called before each retry attempt.
// attempt is the failed attempt number (1-based), maxAttempts is the total.
type RetryHookFunc func(attempt, maxAttempts int, err error)

type retryHookKey struct{}

// WithRetryHook injects a retry notification callback into the context.
// RetryDo will call this hook before each retry attempt.
func WithRetryHook(ctx context.Context, fn RetryHookFunc) context.Context {
	return context.WithValue(ctx, retryHookKey{}, fn)
}

// retryHookFromContext returns the retry hook from context, or nil.
func retryHookFromContext(ctx context.Context) RetryHookFunc {
	fn, _ := ctx.Value(retryHookKey{}).(RetryHookFunc)
	return fn
}

// DefaultRetryConfig returns sensible defaults matching TS provider retry behavior.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		Attempts: 3,
		MinDelay: 300 * time.Millisecond,
		MaxDelay: 30 * time.Second,
		Jitter:   0.1,
	}
}

// HTTPError represents an HTTP error with status code and optional Retry-After.
type HTTPError struct {
	Status     int
	Body       string
	RetryAfter time.Duration // parsed from Retry-After header (0 if absent)
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
}

// streamEmittedError wraps a stream failure that occurred after user-visible
// output already escaped to the caller. Retrying would duplicate streamed text,
// tool calls, or images — it is never retryable regardless of the underlying
// error type.
type streamEmittedError struct {
	err error
}

func (e *streamEmittedError) Error() string { return e.err.Error() }
func (e *streamEmittedError) Unwrap() error { return e.err }

// StreamEmitted wraps err so IsRetryableError returns false. Use inside a
// RetryDoFor fn when chatStreamOnce reports that chunks already escaped.
func StreamEmitted(err error) error {
	return &streamEmittedError{err: err}
}

// IsRetryableError checks if an error is retryable.
// Retryable: 429 (rate limit), 500, 502, 503, 504, connection errors, timeouts.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Stream-emitted failures are never retryable (would duplicate output).
	var emErr *streamEmittedError
	if errors.As(err, &emErr) {
		return false
	}

	// Check for HTTPError
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case 429, 500, 502, 503, 504:
			return true
		}
		return false
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true // includes timeouts
	}

	// Check for connection reset / broken pipe / EOF in error string.
	// Some streaming providers (notably Codex/Responses) surface transient
	// backend failures as SSE `response.failed` events instead of HTTP 429/5xx.
	// Treat explicit retry guidance/rate-limit wording as retryable too.
	errStr := err.Error()
	lowerErr := strings.ToLower(errStr)
	if strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(lowerErr, "timeout") ||
		strings.Contains(lowerErr, "rate limit") ||
		strings.Contains(lowerErr, "429") ||
		(strings.Contains(lowerErr, "response failed") && strings.Contains(lowerErr, "retry")) ||
		strings.Contains(lowerErr, "while processing your request") {
		return true
	}

	return false
}

// RetryDo executes fn with retry logic using exponential backoff and jitter.
//
// Between attempts it re-checks the shared rate-limit coordinator (and the
// circuit breaker) for the target provider:model. When another run's 429 has
// armed a cooldown that the planned delay cannot outlast, further attempts
// would only burn quota against a known-closed window: RetryDoFor aborts
// early with the last error instead of sleeping through its backoff into an
// armed cooldown. A parsed Retry-After keeps being honoured verbatim by
// computeDelay, so when it is longer than the remaining cooldown the loop
// stays alive and lands its next attempt after the window closes.
func RetryDo[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	return RetryDoFor(ctx, cfg, "", "", fn)
}

// RetryDoFor is RetryDo with an explicit provider:model target key. When both
// are non-empty the reliability admission checks run between attempts; plain
// RetryDo (empty target) keeps the historical per-call-only behavior.
func RetryDoFor[T any](ctx context.Context, cfg RetryConfig, provider, model string, fn func() (T, error)) (T, error) {
	if cfg.Attempts <= 0 {
		cfg.Attempts = 1
	}
	tracked := provider != "" && model != ""

	var lastErr error
	var zero T

	for attempt := 1; attempt <= cfg.Attempts; attempt++ {
		// Mid-loop admission check: after a failure, if the shared coordinator
		// has armed a cooldown (or the breaker is Open) for this target and the
		// planned delay cannot outlast that block, further attempts would only
		// burn quota against a known-closed window — abort with the last real
		// error instead of sleeping into an armed cooldown. A Retry-After on
		// the current error keeps being honoured verbatim by computeDelay, so
		// when it is longer than the remaining block the loop stays alive.
		if tracked && attempt > 1 && !outlastsBlock(cfg, attempt, lastErr, provider, model) {
			return zero, lastErr
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry if not retryable or last attempt
		if !IsRetryableError(err) || attempt == cfg.Attempts {
			return zero, err
		}

		// Reliability metrics (nil-safe): one retry counter per retry, plus a
		// rate-limit counter when the provider signaled a 429 so the ops layer
		// can spot retry storms.
		safeRecord(func() {
			if reg := reliability.Default(); reg != nil && reg.Metrics != nil {
				reg.Metrics.RecordLLMRetry()
				if isRateLimitedErr(err) {
					reg.Metrics.RecordLLMRateLimited()
				}
			}
		})

		// Compute delay
		delay := computeDelay(cfg, attempt, err)

		slog.Debug("provider retry",
			"attempt", attempt,
			"maxAttempts", cfg.Attempts,
			"delay", delay,
			"error", err.Error(),
		)

		// Notify retry hook (for placeholder updates, etc.)
		if hook := retryHookFromContext(ctx); hook != nil {
			hook(attempt, cfg.Attempts, err)
		}

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}

	return zero, lastErr
}

// outlastsBlock reports whether the loop's planned wait for this retry is long
// enough to land its next attempt after the reliability layer would admit
// traffic for provider:model again. It consults, nil-safely:
//   - the shared rate-limit coordinator: a cooldown armed by any run's 429
//     for the same provider:model;
//   - the circuit breaker: an Open circuit with a future NextRetryAt.
//
// The candidate wait is computeDelay's result, which already honours a parsed
// Retry-After verbatim — so a Retry-After LONGER than the remaining block
// keeps the loop alive and the next attempt lands after the window closes.
// A block shorter than the wait is harmless; anything longer means retrying
// blind into a closed window.
func outlastsBlock(cfg RetryConfig, attempt int, err error, provider, model string) bool {
	blocked := time.Duration(0)
	reg := reliability.Default()
	if reg == nil {
		return true
	}
	if reg.RateLimit != nil {
		if remaining, ok := reg.RateLimit.CooldownFor(provider, model); ok && remaining > blocked {
			blocked = remaining
		}
	}
	if reg.Breaker != nil {
		key := provider + ":" + model
		// Read-only view: Allow() mutates breaker state (it consumes half-open
		// probe slots), so derive blockage from State + NextRetryAt instead.
		// An expired-but-still-Open circuit admits traffic on its next Allow,
		// so only a future deadline counts as blocking here.
		if reg.Breaker.State(key) == reliability.CircuitOpen {
			if remaining := time.Until(reg.Breaker.NextRetryAt(key)); remaining > blocked {
				blocked = remaining
			}
		}
	}
	if blocked <= 0 {
		return true
	}
	wait := computeDelay(cfg, attempt, err)
	return wait >= blocked
}

// computeDelay calculates the retry delay with exponential backoff, jitter, and Retry-After support.
func computeDelay(cfg RetryConfig, attempt int, err error) time.Duration {
	// Check for Retry-After header
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr.RetryAfter > 0 {
		return httpErr.RetryAfter
	}

	// Exponential backoff: minDelay * 2^(attempt-1)
	delay := float64(cfg.MinDelay) * math.Pow(2, float64(attempt-1))

	// Cap at maxDelay
	if time.Duration(delay) > cfg.MaxDelay {
		delay = float64(cfg.MaxDelay)
	}

	// Apply jitter: ±jitter%
	if cfg.Jitter > 0 {
		jitterRange := delay * cfg.Jitter
		delay += (rand.Float64()*2 - 1) * jitterRange
	}

	if delay < 0 {
		delay = float64(cfg.MinDelay)
	}

	return time.Duration(delay)
}

// ParseRetryAfter parses a Retry-After header value (seconds or HTTP-date).
func ParseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}

	// Try integer seconds first
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try HTTP-date format
	if t, err := time.Parse(time.RFC1123, value); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}

	return 0
}
