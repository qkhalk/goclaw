package cloud

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// fakeTokenSource returns queued tokens/errors then a stable final token.
type fakeTokenSource struct {
	mu    sync.Mutex
	calls int
	toks  []*oauth2.Token
	errs  []error
	delay time.Duration // set to widen the flight window in concurrency tests
}

func (f *fakeTokenSource) Token() (*oauth2.Token, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return nil, err
	}
	if len(f.toks) > 1 {
		tok := f.toks[0]
		f.toks = f.toks[1:]
		return tok, nil
	}
	return f.toks[0], nil
}

// fakeAccountStore records UpdateTokens calls.
type fakeAccountStore struct {
	updates []store.CloudAccountUpdate
}

func (f *fakeAccountStore) Upsert(ctx context.Context, a *store.CloudAccount) error { return nil }
func (f *fakeAccountStore) Get(ctx context.Context, id string) (*store.CloudAccount, error) {
	return nil, store.ErrCloudAccountNotFound
}
func (f *fakeAccountStore) GetByEmail(ctx context.Context, p, e string) (*store.CloudAccount, error) {
	return nil, store.ErrCloudAccountNotFound
}
func (f *fakeAccountStore) List(ctx context.Context) ([]store.CloudAccount, error) { return nil, nil }
func (f *fakeAccountStore) Delete(ctx context.Context, id string) error            { return nil }
func (f *fakeAccountStore) UpdateTokens(ctx context.Context, id string, upd store.CloudAccountUpdate) error {
	f.updates = append(f.updates, upd)
	return nil
}

func TestTokenSourcePersistsRefreshedToken(t *testing.T) {
	// The inner source (x/oauth2 reuseTokenSource in production) refreshes on
	// its own — the wrapper's job is to PERSIST the new token exactly once.
	fresh := &oauth2.Token{AccessToken: "fresh-token", RefreshToken: "rt-new", Expiry: time.Now().Add(time.Hour)}
	inner := &fakeTokenSource{toks: []*oauth2.Token{fresh, fresh}}
	storeFake := &fakeAccountStore{}
	ts := newAccountTokenSource(storeFake, &store.CloudAccount{ID: "acct-1", AccessToken: "stale-token"}, inner)

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "fresh-token" {
		t.Fatalf("token = %s", tok.AccessToken)
	}
	if len(storeFake.updates) != 1 || storeFake.updates[0].AccessToken != "fresh-token" {
		t.Fatalf("refreshed token not persisted: %+v", storeFake.updates)
	}
	if storeFake.updates[0].RefreshToken != "rt-new" {
		t.Fatalf("refresh token not carried: %+v", storeFake.updates[0])
	}

	// Second call: same token → no duplicate persist.
	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token #2: %v", err)
	}
	if len(storeFake.updates) != 1 {
		t.Fatalf("duplicate persist: %+v", storeFake.updates)
	}
}

func TestTokenSourceConcurrentSharesFlight(t *testing.T) {
	fresh := &oauth2.Token{AccessToken: "tok", Expiry: time.Now().Add(time.Hour)}
	inner := &fakeTokenSource{toks: []*oauth2.Token{fresh}, delay: 50 * time.Millisecond}
	storeFake := &fakeAccountStore{}
	ts := newAccountTokenSource(storeFake, &store.CloudAccount{ID: "acct-1"}, inner)

	const n = 8
	var wg sync.WaitGroup
	tokens := make([]*oauth2.Token, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tokens[i], _ = ts.Token()
		}(i)
	}
	wg.Wait()

	if inner.calls != 1 {
		t.Fatalf("inner called %d times, want 1 (single-flight)", inner.calls)
	}
	for i, tok := range tokens {
		if tok == nil || tok.AccessToken != "tok" {
			t.Fatalf("goroutine %d got %+v", i, tok)
		}
	}
}

func TestTokenSourceErrorPropagates(t *testing.T) {
	inner := &fakeTokenSource{errs: []error{errors.New("boom")}}
	ts := newAccountTokenSource(&fakeAccountStore{}, &store.CloudAccount{ID: "acct-1"}, inner)
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error")
	}
}

// fakeSecretsStore records saved credentials; Get returns sql-no-rows-style
// error for unknown keys (matching ConfigSecretsStore semantics).
type fakeSecretsStore struct {
	data map[string]string
}

func (f *fakeSecretsStore) Get(ctx context.Context, key string) (string, error) {
	v, ok := f.data[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (f *fakeSecretsStore) Set(ctx context.Context, key, value string) error {
	if f.data == nil {
		f.data = map[string]string{}
	}
	f.data[key] = value
	return nil
}
func (f *fakeSecretsStore) Delete(ctx context.Context, key string) error { return nil }
func (f *fakeSecretsStore) GetAll(ctx context.Context) (map[string]string, error) {
	return f.data, nil
}

func TestGoogleCredentialsDynamicResolution(t *testing.T) {
	secrets := &fakeSecretsStore{}
	m := NewManager(CloudProviderConfig{
		GoogleClientID:     "env-id",
		GoogleClientSecret: "env-secret",
	}, &fakeAccountStore{}, testKey)
	m.SetSecretsStore(secrets)

	// Nothing saved → env fallback.
	if !m.GoogleConfigured(context.Background()) {
		t.Fatal("env credentials should satisfy GoogleConfigured")
	}

	// Save from the "web UI" → dynamic wins over env.
	if err := m.SaveGoogleCredentials(context.Background(), "ui-id", "ui-secret"); err != nil {
		t.Fatalf("SaveGoogleCredentials: %v", err)
	}
	id, secretSet := m.GoogleCredentialsStatus(context.Background())
	if id != "ui-id" || !secretSet {
		t.Fatalf("status = %q, %v", id, secretSet)
	}
	gotID, gotSecret := m.googleCredentials(context.Background())
	if gotID != "ui-id" || gotSecret != "ui-secret" {
		t.Fatalf("dynamic creds not used: %q/%q", gotID, gotSecret)
	}

	// Client-ID-only update keeps the saved secret.
	if err := m.SaveGoogleCredentials(context.Background(), "ui-id-2", ""); err != nil {
		t.Fatalf("SaveGoogleCredentials #2: %v", err)
	}
	gotID, _ = m.googleCredentials(context.Background())
	if gotID != "ui-id-2" {
		t.Fatalf("client id not updated: %q", gotID)
	}
	_, secretSet = m.GoogleCredentialsStatus(context.Background())
	if !secretSet {
		t.Fatal("secret lost on id-only update")
	}

	// Empty store (no env either) → not configured.
	m2 := NewManager(CloudProviderConfig{}, &fakeAccountStore{}, testKey)
	m2.SetSecretsStore(&fakeSecretsStore{})
	if m2.GoogleConfigured(context.Background()) {
		t.Fatal("empty store+env must not be configured")
	}
}
