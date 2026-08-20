package methods

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// stubAgentPolicies is a minimal AgentPolicies double for exercising the
// enforcement helpers. The nil-safe fast path (nil store / nil tenant) is
// exercised by passing nil explicitly.
type stubAgentPolicies struct {
	policy       *store.TenantPolicy
	getErr       error
	activeErr    error
	createErr    error
	sessionErr   error
	teamErr      error
}

// GetTenantPolicy implements store.AgentPolicies.
func (s *stubAgentPolicies) GetTenantPolicy(ctx context.Context, tenantID uuid.UUID) (*store.TenantPolicy, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.policy == nil {
		return nil, store.ErrTenantPolicyNotFound
	}
	return s.policy, nil
}

func (s *stubAgentPolicies) CheckTenantActive(ctx context.Context, tenantID uuid.UUID) error { return s.activeErr }

func (s *stubAgentPolicies) CheckCanCreateAgent(ctx context.Context, tenantID uuid.UUID) error {
	return s.createErr
}

func (s *stubAgentPolicies) CheckCanCreateSession(ctx context.Context, tenantID uuid.UUID) error {
	return s.sessionErr
}

func (s *stubAgentPolicies) CheckCanCreateTeam(ctx context.Context, tenantID uuid.UUID) error {
	return s.teamErr
}

func uid() uuid.UUID { return uuid.New() }

func Test_checkTenantActive(t *testing.T) {
	ctx := context.Background()
	locale := "en"
	tid := uid()

	t.Run("nil store skips", func(t *testing.T) {
		if fail := checkTenantActive(ctx, nil, tid, locale); fail != nil {
			t.Fatalf("nil store should skip: got %+v", fail)
		}
	})
	t.Run("nil tenant skips", func(t *testing.T) {
		if fail := checkTenantActive(ctx, &stubAgentPolicies{activeErr: store.ErrTenantSuspended}, uuid.Nil, locale); fail != nil {
			t.Fatalf("nil tenant should skip: got %+v", fail)
		}
	})
	t.Run("active tenant passes", func(t *testing.T) {
		if fail := checkTenantActive(ctx, &stubAgentPolicies{}, tid, locale); fail != nil {
			t.Fatalf("active tenant should pass: got %+v", fail)
		}
	})
	t.Run("suspended tenant blocked", func(t *testing.T) {
		fail := checkTenantActive(ctx, &stubAgentPolicies{activeErr: store.ErrTenantSuspended}, tid, locale)
		if fail == nil {
			t.Fatal("suspended tenant should be blocked")
		}
		if fail.code != tenantPolicySuspended {
			t.Fatalf("code = %q, want %q", fail.code, tenantPolicySuspended)
		}
	})
	t.Run("db error fails closed", func(t *testing.T) {
		fail := checkTenantActive(ctx, &stubAgentPolicies{activeErr: errors.New("db down")}, tid, locale)
		if fail == nil {
			t.Fatal("db error should fail closed")
		}
		if fail.code != tenantPolicyInternal {
			t.Fatalf("code = %q, want %q", fail.code, tenantPolicyInternal)
		}
	})
}

func Test_checkTenantLimit(t *testing.T) {
	ctx := context.Background()
	locale := "en"
	tid := uid()

	t.Run("nil store skips", func(t *testing.T) {
		if fail := checkTenantLimit(ctx, nil, tid, func(c context.Context) error { return store.ErrTenantLimitReached }, locale); fail != nil {
			t.Fatalf("nil store should skip: got %+v", fail)
		}
	})
	t.Run("no cap passes", func(t *testing.T) {
		if fail := checkTenantLimit(ctx, &stubAgentPolicies{}, tid, func(c context.Context) error { return nil }, locale); fail != nil {
			t.Fatalf("no cap should pass: got %+v", fail)
		}
	})
	t.Run("cap reached blocked", func(t *testing.T) {
		limitErr := &store.TenantLimitError{Cap: store.CapAgents, Limit: 5, Existing: 5}
		fail := checkTenantLimit(ctx, &stubAgentPolicies{}, tid, func(c context.Context) error { return limitErr }, locale)
		if fail == nil {
			t.Fatal("cap reached should be blocked")
		}
		if fail.code != tenantPolicyLimit {
			t.Fatalf("code = %q, want %q", fail.code, tenantPolicyLimit)
		}
	})
	t.Run("no policy row passes", func(t *testing.T) {
		fail := checkTenantLimit(ctx, &stubAgentPolicies{}, tid, func(c context.Context) error { return store.ErrTenantPolicyNotFound }, locale)
		if fail != nil {
			t.Fatalf("no policy row should pass: got %+v", fail)
		}
	})
	t.Run("db error fails closed", func(t *testing.T) {
		fail := checkTenantLimit(ctx, &stubAgentPolicies{}, tid, func(c context.Context) error { return errors.New("db down") }, locale)
		if fail == nil {
			t.Fatal("db error should fail closed")
		}
		if fail.code != tenantPolicyInternal {
			t.Fatalf("code = %q, want %q", fail.code, tenantPolicyInternal)
		}
	})
}

func Test_checkProviderModelAccess(t *testing.T) {
	ctx := context.Background()
	locale := "en"
	tid := uid()

	t.Run("nil store skips", func(t *testing.T) {
		if fail := checkProviderModelAccess(ctx, nil, tid, "anthropic", "claude-3-5", locale); fail != nil {
			t.Fatalf("nil store should skip: got %+v", fail)
		}
	})
	t.Run("no policy passes", func(t *testing.T) {
		if fail := checkProviderModelAccess(ctx, &stubAgentPolicies{}, tid, "anthropic", "claude-3-5", locale); fail != nil {
			t.Fatalf("no policy should pass: got %+v", fail)
		}
	})
	t.Run("no allowlist passes", func(t *testing.T) {
		stub := &stubAgentPolicies{policy: &store.TenantPolicy{}}
		if fail := checkProviderModelAccess(ctx, stub, tid, "anthropic", "claude-3-5", locale); fail != nil {
			t.Fatalf("empty allowlist should pass: got %+v", fail)
		}
	})
	t.Run("provider denied", func(t *testing.T) {
		stub := &stubAgentPolicies{policy: &store.TenantPolicy{AllowedProviders: []string{"openai"}}}
		fail := checkProviderModelAccess(ctx, stub, tid, "anthropic", "claude-3-5", locale)
		if fail == nil {
			t.Fatal("denied provider should be blocked")
		}
		if fail.code != tenantPolicyProviderDen {
			t.Fatalf("code = %q, want %q", fail.code, tenantPolicyProviderDen)
		}
	})
	t.Run("model denied", func(t *testing.T) {
		stub := &stubAgentPolicies{policy: &store.TenantPolicy{
			AllowedProviders: []string{"anthropic"},
			AllowedModels:    []string{"claude-opus-5"},
		}}
		fail := checkProviderModelAccess(ctx, stub, tid, "anthropic", "claude-3-5", locale)
		if fail == nil {
			t.Fatal("denied model should be blocked")
		}
		if fail.code != tenantPolicyModelDen {
			t.Fatalf("code = %q, want %q", fail.code, tenantPolicyModelDen)
		}
	})
	t.Run("allowed pair passes", func(t *testing.T) {
		stub := &stubAgentPolicies{policy: &store.TenantPolicy{
			AllowedProviders: []string{"anthropic"},
			AllowedModels:    []string{"claude-opus-5"},
		}}
		if fail := checkProviderModelAccess(ctx, stub, tid, "anthropic", "claude-opus-5", locale); fail != nil {
			t.Fatalf("allowed pair should pass: got %+v", fail)
		}
	})
	t.Run("empty provider/model skipped", func(t *testing.T) {
		stub := &stubAgentPolicies{policy: &store.TenantPolicy{
			AllowedProviders: []string{"openai"},
			AllowedModels:    []string{"gpt-4o"},
		}}
		if fail := checkProviderModelAccess(ctx, stub, tid, "", "", locale); fail != nil {
			t.Fatalf("empty values should skip: got %+v", fail)
		}
	})
}