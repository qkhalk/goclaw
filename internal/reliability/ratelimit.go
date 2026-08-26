package reliability

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrMaxPendingWaiters is returned by Wait when the total in-flight waiter
// count has reached the configured cap. The caller should abort the request
// rather than queue behind a full waiting room.
var ErrMaxPendingWaiters = errors.New("rate limit: max pending waiters exceeded")

// RateLimitCoordinator provides a shared, single-flight view of provider
// cooldowns. Multiple concurrent runs using the same provider:model must not
// collide: when one run gets a 429, every other run for that key waits for
// the same cooldown instead of retrying simultaneously (retry storm).
//
// It complements, rather than replaces, the existing per-provider
// CooldownTracker: that tracker handles a run's local decision about *its own*
// requests; this coordinator deduplicates the wait across concurrent runs.
type RateLimitCoordinator struct {
	mu           sync.Mutex
	cooldowns    map[string]time.Time
	waiters      map[string]int
	maxPending   int // cap on total in-flight waiters; <=0 disables enforcement
	pendingTotal int // sum of waiters across all keys
	nowFn        func() time.Time
}

// NewRateLimitCoordinator builds a coordinator with the given cap on total
// in-flight waiters. maxPending <= 0 disables the pending cap.
func NewRateLimitCoordinator(maxPending int) *RateLimitCoordinator {
	return &RateLimitCoordinator{
		cooldowns:  make(map[string]time.Time),
		waiters:    make(map[string]int),
		maxPending: maxPending,
		nowFn:      time.Now,
	}
}

// key returns the coordinator key for a provider:model.
func (r *RateLimitCoordinator) key(provider, model string) string {
	return provider + ":" + model
}

// Record429 registers a rate-limit event for a provider:model. retryAfter is
// the provider's Retry-After hint (0 if absent → normalized default 30s).
func (r *RateLimitCoordinator) Record429(provider, model string, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = 30 * time.Second
	}
	k := r.key(provider, model)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cooldowns[k] = r.nowFn().Add(retryAfter)
}

// CooldownFor returns the remaining cooldown for a key, and whether one is
// active.
func (r *RateLimitCoordinator) CooldownFor(provider, model string) (time.Duration, bool) {
	k := r.key(provider, model)
	r.mu.Lock()
	until, ok := r.cooldowns[k]
	r.mu.Unlock()
	if !ok {
		return 0, false
	}
	remaining := until.Sub(r.nowFn())
	if remaining <= 0 {
		return 0, false
	}
	return remaining, true
}

// PendingWaiters returns the number of runs currently blocked in Wait
// across all keys. Returns 0 when maxPending is disabled.
func (r *RateLimitCoordinator) PendingWaiters() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pendingTotal
}

// ClearCooldown removes any active cooldown for the key (e.g. after a
// successful request).
func (r *RateLimitCoordinator) ClearCooldown(provider, model string) {
	k := r.key(provider, model)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cooldowns, k)
}

// Wait blocks until the cooldown for a key expires or the context is done.
// It is a convenience that combines cooldown check with a cancellable sleep and
// writes a pessimistic wait registration so the cancellation path can't leak.
// When the total in-flight waiter count reaches maxPending the call is
// rejected with ErrMaxPendingWaiters to prevent starvation from a leaked
// counter.
func (r *RateLimitCoordinator) Wait(ctx context.Context, provider, model string) error {
	r.mu.Lock()
	k := r.key(provider, model)
	until, ok := r.cooldowns[k]
	registered := false
	if ok {
		if r.maxPending > 0 && r.pendingTotal >= r.maxPending {
			// Pending cap reached: reject the caller rather than
			// queue behind a full waiting room.
			r.mu.Unlock()
			return ErrMaxPendingWaiters
		}
		r.waiters[k]++
		r.pendingTotal++
		registered = true
	}
	r.mu.Unlock()
	if !registered {
		return nil
	}
	defer func() {
		r.mu.Lock()
		r.pendingTotal--
		r.waiters[k]--
		if r.waiters[k] <= 0 {
			delete(r.waiters, k)
		}
		r.mu.Unlock()
	}()

	d := until.Sub(r.nowFn())
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		// Leave the cooldown in place for other runs; only this waiter gives up.
		return ctx.Err()
	case <-t.C:
		// Only clear the cooldown we actually waited on. If another run recorded
		// a newer (likely longer) 429 during our wait, that deadline must not be
		// removed by this stale waiter — otherwise every other run would burst
		// the provider while it is still rate-limited.
		r.maybeClearCooldown(k, until)
		return nil
	}
}

// maybeClearCooldown removes a cooldown only if the given deadline is still the
// active one for the key. It is the guard that prevents a waiter with an older,
// shorter deadline from deleting a newer cooldown recorded mid-wait.
func (r *RateLimitCoordinator) maybeClearCooldown(k string, until time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if stored, ok := r.cooldowns[k]; ok && stored.Equal(until) {
		delete(r.cooldowns, k)
	}
}
