package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The two-tier RunWithFailover engine (former failover.go) was removed:
// ModelFallbackProvider.runOrdered is the single production failover
// implementation. These tests keep its public contracts covered — ordered
// candidate iteration, classification-driven fallback vs terminal stops,
// cancellation, and the FailoverSummaryError message format consumed by
// callers that surface an exhausted fallback chain.

func TestModelFallbackFirstCandidateSucceeds(t *testing.T) {
	primary := &testFallbackProvider{name: "mf-first-primary", model: "gpt-4o"}
	provider := NewModelFallbackProvider(FallbackCandidate{
		ProviderName: "mf-first-primary",
		Provider:     primary,
		Model:        "gpt-4o",
	}, nil, 1, false)

	resp, err := provider.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Content != "gpt-4o" {
		t.Errorf("expected primary model content, got %s", resp.Content)
	}
	if primary.calls != 1 {
		t.Errorf("expected 1 call, got %d", primary.calls)
	}
}

func TestModelFallbackRateLimitRotatesToBackup(t *testing.T) {
	primary := &testFallbackProvider{
		name:  "mf-rl-primary",
		model: "primary-model",
		err:   &HTTPError{Status: 429, Body: "Rate limit exceeded"},
	}
	backup := &testFallbackProvider{name: "mf-rl-backup", model: "backup-model"}
	provider := NewModelFallbackProvider(FallbackCandidate{
		ProviderName: "mf-rl-primary",
		Provider:     primary,
		Model:        "primary-model",
	}, []FallbackCandidate{
		{ProviderName: "mf-rl-backup", Provider: backup, Model: "backup-model"},
	}, 2, false)

	resp, err := provider.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Content != "backup-model" {
		t.Errorf("expected backup model content, got %s", resp.Content)
	}
	if primary.calls != 1 || backup.calls != 1 {
		t.Fatalf("calls primary=%d backup=%d, want 1/1", primary.calls, backup.calls)
	}

	// The failed 429 attempt must be recorded for diagnostics.
	diags := provider.LastAttempts()
	found := false
	for _, d := range diags {
		if d.Candidate.ProviderName == "mf-rl-primary" && !d.Skipped {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a tried diagnostic for the rate-limited primary, got %+v", diags)
	}
}

func TestModelFallbackAuthPermanentSkipsModel(t *testing.T) {
	primary := &testFallbackProvider{
		name:  "mf-auth-primary",
		model: "claude-opus-4-6",
		err:   &HTTPError{Status: 401, Body: "API key has been revoked"},
	}
	backup := &testFallbackProvider{name: "mf-auth-backup", model: "gpt-4o"}
	provider := NewModelFallbackProvider(FallbackCandidate{
		ProviderName: "mf-auth-primary",
		Provider:     primary,
		Model:        "claude-opus-4-6",
	}, []FallbackCandidate{
		{ProviderName: "mf-auth-backup", Provider: backup, Model: "gpt-4o"},
	}, 2, false)

	resp, err := provider.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Content != "gpt-4o" {
		t.Errorf("expected success on the next model after auth_permanent, got %s", resp.Content)
	}
	if primary.calls != 1 || backup.calls != 1 {
		t.Fatalf("calls primary=%d backup=%d, want 1/1", primary.calls, backup.calls)
	}
}

func TestModelFallbackAllExhausted(t *testing.T) {
	primary := &testFallbackProvider{
		name:  "mf-x-primary",
		model: "model-a",
		err:   &HTTPError{Status: 500, Body: "Internal server error"},
	}
	backup := &testFallbackProvider{
		name:  "mf-x-backup",
		model: "model-b",
		err:   &HTTPError{Status: 500, Body: "Internal server error"},
	}
	provider := NewModelFallbackProvider(FallbackCandidate{
		ProviderName: "mf-x-primary",
		Provider:     primary,
		Model:        "model-a",
	}, []FallbackCandidate{
		{ProviderName: "mf-x-backup", Provider: backup, Model: "model-b"},
	}, 2, false)

	resp, err := provider.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}

	var summaryErr *FailoverSummaryError
	if !errors.As(err, &summaryErr) {
		t.Fatalf("expected FailoverSummaryError, got %T", err)
	}
	if len(summaryErr.Attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", len(summaryErr.Attempts))
	}
	if primary.calls != 1 || backup.calls != 1 {
		t.Fatalf("calls primary=%d backup=%d, want 1/1", primary.calls, backup.calls)
	}
}

func TestModelFallbackContextOverflowReturnsImmediately(t *testing.T) {
	primary := &testFallbackProvider{
		name:  "mf-co-primary",
		model: "gpt-4o",
		err:   &HTTPError{Status: 400, Body: "Context length exceeded"},
	}
	backup := &testFallbackProvider{name: "mf-co-backup", model: "claude-opus-4-6"}
	provider := NewModelFallbackProvider(FallbackCandidate{
		ProviderName: "mf-co-primary",
		Provider:     primary,
		Model:        "gpt-4o",
	}, []FallbackCandidate{
		{ProviderName: "mf-co-backup", Provider: backup, Model: "claude-opus-4-6"},
	}, 2, false)

	_, err := provider.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Context overflow stops immediately — no profile/model rotation.
	var summaryErr *FailoverSummaryError
	if errors.As(err, &summaryErr) {
		t.Fatalf("context overflow must return the original error, got summary %v", err)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != 400 {
		t.Fatalf("expected the original HTTPError, got %v", err)
	}
	if primary.calls != 1 || backup.calls != 0 {
		t.Fatalf("calls primary=%d backup=%d, want 1/0 (no fallback on context overflow)", primary.calls, backup.calls)
	}
}

func TestModelFallbackContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	primary := &testFallbackProvider{name: "mf-cancel", model: "gpt-4o"}
	provider := NewModelFallbackProvider(FallbackCandidate{
		ProviderName: "mf-cancel",
		Provider:     primary,
		Model:        "gpt-4o",
	}, nil, 1, false)

	_, err := provider.Chat(ctx, ChatRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if primary.calls != 0 {
		t.Errorf("candidate should not be called when context is cancelled, got %d calls", primary.calls)
	}
}

func TestFailoverSummaryErrorFormat(t *testing.T) {
	summaryErr := &FailoverSummaryError{
		Attempts: []FailoverAttempt{
			{
				Candidate:      ModelCandidate{Provider: "openai", Model: "gpt-4o", ProfileID: "key1"},
				Classification: FailoverClassification{Kind: "reason", Reason: FailoverRateLimit},
				Err:            errors.New("rate limit"),
			},
			{
				Candidate:      ModelCandidate{Provider: "anthropic", Model: "claude-opus-4-6", ProfileID: "key2"},
				Classification: FailoverClassification{Kind: "reason", Reason: FailoverBilling},
				Err:            errors.New("billing error"),
			},
		},
	}

	errMsg := summaryErr.Error()
	if errMsg == "" {
		t.Fatal("expected non-empty error message")
	}
	// Should contain info about both attempts
	if !contains(errMsg, "openai") || !contains(errMsg, "anthropic") {
		t.Errorf("error message should contain provider names: %s", errMsg)
	}
	if !strings.Contains(errMsg, "rate_limit") || !strings.Contains(errMsg, "billing") {
		t.Errorf("error message should contain failure reasons: %s", errMsg)
	}
}

func contains(s, substr string) bool {
	for i := range len(s) - len(substr) + 1 {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
