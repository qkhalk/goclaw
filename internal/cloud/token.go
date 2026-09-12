package cloud

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// scopeContext rebuilds the tenant+user context carried inside a signed state
// payload (the callback request itself is unauthenticated).
func scopeContext(ctx context.Context, tenantID, userID string) (context.Context, error) {
	tenant, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("cloud: invalid tenant in state: %w", err)
	}
	if userID == "" {
		return nil, errors.New("cloud: invalid user in state")
	}
	return store.WithUserID(store.WithTenantID(ctx, tenant), userID), nil
}

// accountTokenSource wraps the x/oauth2 TokenSource with persistence and
// single-flight: the inner source performs the actual refresh; whenever it
// yields a NEW access token the value is written back to cloud_accounts (so
// it survives a restart), and concurrent callers share one in-flight call —
// racing Google's token endpoint invalidates the older access token of
// parallel refreshes.
type accountTokenSource struct {
	store  store.CloudAccountStore
	acctID string
	inner  oauth2.TokenSource

	mu             sync.Mutex
	flight         *tokenFlight // non-nil while a Token() call is in progress
	persistedToken string       // last access token written back to the store
}

// tokenFlight carries one in-flight Token() result. Results are written
// (under the source mutex) BEFORE wg.Done(), so any waiter that returns from
// wg.Wait() sees final values.
type tokenFlight struct {
	wg    sync.WaitGroup
	done  bool
	token *oauth2.Token
	err   error
}

func newAccountTokenSource(acctStore store.CloudAccountStore, acct *store.CloudAccount, inner oauth2.TokenSource) *accountTokenSource {
	return &accountTokenSource{store: acctStore, acctID: acct.ID, inner: inner, persistedToken: acct.AccessToken}
}

// Token implements oauth2.TokenSource.
func (s *accountTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	f := s.flight
	if f == nil {
		// Leader: run the call outside the lock; publish results before
		// releasing waiters.
		f = &tokenFlight{}
		f.wg.Add(1)
		s.flight = f
		s.mu.Unlock()

		tok, err := s.tokenOnce()

		s.mu.Lock()
		f.token, f.err, f.done = tok, err, true
		s.mu.Unlock()
		f.wg.Done()
		return tok, err
	}
	if !f.done {
		// Follower: the in-flight call publishes results before Done, so
		// returning from Wait guarantees final values — never (nil, nil).
		s.mu.Unlock()
		f.wg.Wait()
		return f.token, f.err
	}
	// Completed flight: reuse its final result once, then clear the slot so
	// the next caller re-evaluates token validity with a fresh call.
	s.flight = nil
	s.mu.Unlock()
	return f.token, f.err
}

// tokenOnce runs the inner source and persists a newly refreshed token.
func (s *accountTokenSource) tokenOnce() (*oauth2.Token, error) {
	tok, err := s.inner.Token()
	if err != nil {
		return nil, err
	}
	if tok == nil || tok.AccessToken == "" || tok.AccessToken == s.persistedSnapshot() {
		return tok, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	upd := store.CloudAccountUpdate{
		AccessToken:    tok.AccessToken,
		TokenExpiresAt: &tok.Expiry,
		Status:         "active",
	}
	if tok.RefreshToken != "" {
		upd.RefreshToken = tok.RefreshToken
	}
	if uerr := s.store.UpdateTokens(ctx, s.acctID, upd); uerr != nil {
		if !errors.Is(uerr, store.ErrCloudAccountNotFound) {
			return tok, fmt.Errorf("cloud: persist refreshed token: %w", uerr)
		}
		// Account deleted mid-flight: the token still works for this call.
	}
	s.setPersisted(tok.AccessToken)
	return tok, nil
}

func (s *accountTokenSource) persistedSnapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistedToken
}

func (s *accountTokenSource) setPersisted(v string) {
	s.mu.Lock()
	s.persistedToken = v
	s.mu.Unlock()
}

// Ensure interface compliance.
var _ oauth2.TokenSource = (*accountTokenSource)(nil)
