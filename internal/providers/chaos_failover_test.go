package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ModelFallbackProvider through REAL HTTP: a fake LLM server scripts
// per-candidate chaos (429 primary, 5xx series) and asserts the fallback chain
// rotates to the backup. The streamed-chunk case drives the real HTTP SSE read
// path to prove a mid-stream failure that already emitted output settles the
// run and never triggers a fallback.
//
// Each test builds two OpenAIProvider candidates (key1 primary, key2 backup)
// pointed at the same scripted server and wires them into a
// ModelFallbackProvider with cooldown tracking disabled — the assertions here
// target the failover engine itself, not cooldown/health bookkeeping.
// ---------------------------------------------------------------------------

// newChaosFallbackProvider wires two OpenAI profiles (key1 primary, key2
// backup) of the same model into a ModelFallbackProvider over the fake server.
func newChaosFallbackProvider(t *testing.T, baseURL string, maxAttempts int) *ModelFallbackProvider {
	t.Helper()
	primary := NewOpenAIProvider("chaos-key1", "sk-key1", baseURL, "gpt-4o")
	primary.retryConfig.Attempts = 1
	backup := NewOpenAIProvider("chaos-key2", "sk-key2", baseURL, "gpt-4o")
	backup.retryConfig.Attempts = 1
	return NewModelFallbackProvider(FallbackCandidate{
		ProviderName: primary.Name(),
		Provider:     primary,
		Model:        "gpt-4o",
	}, []FallbackCandidate{
		{ProviderName: backup.Name(), Provider: backup, Model: "gpt-4o"},
	}, maxAttempts, false)
}

// TestFailover_HTTP_429Primary_BackupSucceeds scripts the primary (key1) to
// answer 429 for every request and the backup (key2) to return 200. The
// request flows through the real OpenAI adapter (RetryDo + SSE parsing), so
// the 429 must be classified as rate_limit and rotate to the backup.
func TestFailover_HTTP_429Primary_BackupSucceeds(t *testing.T) {
	server := newFakeLLMServerEmpty(t)
	server.script(
		server.httpErrorStep(http.StatusTooManyRequests, 0, "rate limited"), // key1
		server.openAICompleteStep("backup result"),                          // key2
	)

	provider := newChaosFallbackProvider(t, server.URL(), 2)
	resp, err := provider.Chat(context.Background(), ChatRequest{Model: "gpt-4o"})

	if err != nil {
		t.Fatalf("failover returned error on 429 primary -> 200 backup: %v", err)
	}
	if resp == nil || !strings.Contains(resp.Content, "backup result") {
		content := ""
		if resp != nil {
			content = resp.Content
		}
		t.Errorf("result = %q, want the backup candidate's output", content)
	}
	if got := server.requestCount(); got != 2 {
		t.Errorf("requestCount = %d, want 2 (primary 429, backup 200)", got)
	}

	diags := provider.LastAttempts()
	found := false
	for _, d := range diags {
		if d.Candidate.ProviderName == "chaos-key1" && !d.Skipped {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a tried diagnostic for the rate-limited primary, got %+v", diags)
	}
}

// TestFailover_HTTP_5xxSeries_Rotates scripts a persistent 503 on the primary
// and a 200 on the backup; the 503 is classified server_error (retryable on
// another candidate) and the run lands on the backup.
func TestFailover_HTTP_5xxSeries_Rotates(t *testing.T) {
	server := newFakeLLMServerEmpty(t)
	server.script(
		server.httpErrorStep(http.StatusServiceUnavailable, 0, "upstream down"), // key1
		server.openAICompleteStep("backup result"),                              // key2
	)

	provider := newChaosFallbackProvider(t, server.URL(), 2)
	resp, err := provider.Chat(context.Background(), ChatRequest{Model: "gpt-4o"})

	if err != nil {
		t.Fatalf("failover returned error on 503 primary -> 200 backup: %v", err)
	}
	if resp == nil || !strings.Contains(resp.Content, "backup result") {
		content := ""
		if resp != nil {
			content = resp.Content
		}
		t.Errorf("result = %q, want the backup candidate's output", content)
	}
	if got := server.requestCount(); got != 2 {
		t.Errorf("requestCount = %d, want 2 (primary 503, backup 200)", got)
	}
}

// TestFailover_HTTP_StreamedChunk_DoesNotFallback drives the real HTTP SSE
// read path: the primary emits one content delta, then the server closes the
// connection WITHOUT [DONE] (SSEDone=false + CloseAfterFrames). The scanner
// sees an abrupt EOF after a frame already escaped through onChunk, so
// OpenAIProvider.ChatStream returns a stream read error with partial content.
// runOrdered must wrap it in noFallbackAfterStreamError and settle the run
// without ever calling the backup. A 429-style retry would be wrong here:
// replaying through another candidate would duplicate emitted output.
func TestFailover_HTTP_StreamedChunk_DoesNotFallback(t *testing.T) {
	server := newFakeLLMServerEmpty(t)
	server.script(responseStep{
		Status:           http.StatusOK,
		SSEFrames:        []sseFrame{server.openAITextDelta("partial answer")},
		CloseAfterFrames: true,
	})

	provider := newChaosFallbackProvider(t, server.URL(), 2)
	var chunks int
	resp, err := provider.ChatStream(context.Background(), ChatRequest{Model: "gpt-4o"}, func(StreamChunk) {
		chunks++
	})

	if err == nil {
		t.Fatal("expected the streamed error to settle the run, got nil")
	}
	if resp == nil || !strings.Contains(resp.Content, "partial answer") {
		content := ""
		if resp != nil {
			content = resp.Content
		}
		t.Errorf("result = %q, want the partial output that escaped", content)
	}
	if chunks < 1 {
		t.Errorf("chunks delivered = %d, want >= 1 (the escaped partial answer)", chunks)
	}
}
