package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/mail"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrNoAccounts is returned when the user has no connected account matching
// the requested provider/email.
var ErrNoAccounts = errors.New("no connected cloud account matches the request (see the Cloud page)")

// MailService resolves authorized Gmail clients for connected accounts, with
// a per-account rate limiter so agent loops cannot hammer the Gmail API.
type MailService struct {
	manager *Manager
	limiter *rateLimiter
}

// NewMailService creates a MailService (rate = requests per minute per account).
func NewMailService(manager *Manager, ratePerMinute int) *MailService {
	return &MailService{manager: manager, limiter: newRateLimiter(ratePerMinute)}
}

// Accounts lists the caller's connected accounts (ctx-scoped).
func (s *MailService) Accounts(ctx context.Context) ([]store.CloudAccount, error) {
	return s.manager.store.List(ctx)
}

// MailClient returns an authorized Gmail client for the named account
// (empty name = the first active account). Rate-limited per account.
func (s *MailService) MailClient(ctx context.Context, account string) (*mail.Client, error) {
	acct, err := s.resolveAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	if wait := s.limiter.reserve(acct.ID); wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	ts, err := s.manager.TokenSource(ctx, acct.ID)
	if err != nil {
		return nil, err
	}
	return mail.NewClient(ts), nil
}

// resolveAccount picks by email/ID when given, else per-scope bindings
// (group → user → tenant default) and finally the caller's own accounts.
// Mail is a Google (Gmail) capability: shared OneDrive accounts never match,
// and zero-config embedded-client accounts (Drive-only scopes) are skipped —
// Gmail requires a BYO OAuth client.
func (s *MailService) resolveAccount(ctx context.Context, name string) (*store.CloudAccount, error) {
	return s.manager.ResolveAccount(ctx, name, []string{GoogleProvider}, func(a *store.CloudAccount) bool {
		return a.Provider == GoogleProvider && accountHasGmailScope(a)
	})
}

// accountHasGmailScope reports whether the account's granted scope set
// includes a gmail scope (Scopes is a JSON array string).
func accountHasGmailScope(a *store.CloudAccount) bool {
	if a.Scopes == "" {
		return false
	}
	var scopes []string
	if err := json.Unmarshal([]byte(a.Scopes), &scopes); err != nil {
		return false
	}
	for _, sc := range scopes {
		if strings.Contains(sc, "/auth/gmail") {
			return true
		}
	}
	return false
}

// rateLimiter is a minimal fixed-window limiter (per account): at most rate
// acquisitions per minute; excess returns the wait until the window resets.
type rateLimiter struct {
	mu    sync.Mutex
	rate  int
	state map[string]*rateWindow
}

type rateWindow struct {
	count int
	reset time.Time
}

func newRateLimiter(rate int) *rateLimiter {
	if rate <= 0 {
		rate = 10
	}
	return &rateLimiter{rate: rate, state: map[string]*rateWindow{}}
}

// reserve returns 0 when immediately allowed, else the wait duration.
func (l *rateLimiter) reserve(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.state[key]
	now := time.Now()
	if !ok || now.After(w.reset) {
		l.state[key] = &rateWindow{count: 1, reset: now.Add(time.Minute)}
		return 0
	}
	if w.count < l.rate {
		w.count++
		return 0
	}
	return time.Until(w.reset)
}
