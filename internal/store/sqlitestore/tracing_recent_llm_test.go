//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestSQLiteTracingStoreListRecentLLMRequests covers the newest-first ordering,
// the tenant scope and the llm_call-only filter of the recent-requests query.
func TestSQLiteTracingStoreListRecentLLMRequests(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	traces := NewSQLiteTracingStore(db)
	tenantA := uuid.Must(uuid.NewV7())
	tenantB := uuid.Must(uuid.NewV7())
	for _, tc := range []struct {
		id   uuid.UUID
		slug string
	}{{tenantA, "tenant-a"}, {tenantB, "tenant-b"}} {
		if _, err := db.Exec(
			`INSERT INTO tenants (id, name, slug, status) VALUES (?, ?, ?, 'active')`,
			tc.id.String(), tc.slug, tc.slug,
		); err != nil {
			t.Fatalf("insert tenant: %v", err)
		}
	}
	ctxA := store.WithTenantID(context.Background(), tenantA)
	ctxB := store.WithTenantID(context.Background(), tenantB)
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)

	createLLMSpan := func(t *testing.T, ctx context.Context, tenant uuid.UUID, at time.Time, model string, in, out int) {
		t.Helper()
		traceID := uuid.Must(uuid.NewV7())
		if err := traces.CreateTrace(ctx, &store.TraceData{
			ID:        traceID,
			Name:      "run",
			StartTime: at,
			Status:    "completed",
		}); err != nil {
			t.Fatalf("CreateTrace: %v", err)
		}
		if err := traces.CreateSpan(ctx, &store.SpanData{
			TraceID:      traceID,
			SpanType:     "llm_call",
			StartTime:    at,
			EndTime:      &at,
			DurationMS:   1200,
			Status:       "completed",
			Model:        model,
			Provider:     "openai-compat",
			InputTokens:  in,
			OutputTokens: out,
			TenantID:     tenant,
		}); err != nil {
			t.Fatalf("CreateSpan: %v", err)
		}
	}

	createLLMSpan(t, ctxA, tenantA, now.Add(-2*time.Minute), "model-old", 10, 5)
	createLLMSpan(t, ctxA, tenantA, now, "model-new", 100, 50)
	createLLMSpan(t, ctxB, tenantB, now, "model-other-tenant", 7, 3)
	// tool_call spans must never appear in recent LLM requests.
	toolTraceID := uuid.Must(uuid.NewV7())
	if err := traces.CreateTrace(ctxA, &store.TraceData{
		ID: toolTraceID, Name: "run", StartTime: now, Status: "completed",
	}); err != nil {
		t.Fatalf("CreateTrace: %v", err)
	}
	if err := traces.CreateSpan(ctxA, &store.SpanData{
		TraceID: toolTraceID, SpanType: "tool_call", StartTime: now, Status: "completed", TenantID: tenantA,
	}); err != nil {
		t.Fatalf("CreateSpan: %v", err)
	}

	rows, err := traces.ListRecentLLMRequests(ctxA, 10)
	if err != nil {
		t.Fatalf("ListRecentLLMRequests: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (tenant-scoped, llm_call only): %#v", len(rows), rows)
	}
	if rows[0].Model != "model-new" || rows[1].Model != "model-old" {
		t.Fatalf("order = [%s, %s], want newest first", rows[0].Model, rows[1].Model)
	}
	if rows[0].InputTokens != 100 || rows[0].OutputTokens != 50 {
		t.Fatalf("tokens = %d/%d, want 100/50", rows[0].InputTokens, rows[0].OutputTokens)
	}

	// Limit clamping: only the single newest row survives limit=1.
	limited, err := traces.ListRecentLLMRequests(ctxA, 1)
	if err != nil {
		t.Fatalf("ListRecentLLMRequests(limit=1): %v", err)
	}
	if len(limited) != 1 || limited[0].Model != "model-new" {
		t.Fatalf("limit=1 rows = %#v, want [model-new]", limited)
	}

	// Queries without an explicit tenant (and without master/cross-tenant
	// scope) are rejected — same contract as ListChildTraces.
	if _, err := traces.ListRecentLLMRequests(context.Background(), 10); err == nil {
		t.Fatal("ListRecentLLMRequests(no tenant) = nil error, want tenant_id required")
	}
}

// TestSQLiteTracingStoreListRoutingEdges covers the provider→model
// aggregation, the time window and the tenant scope of the routing graph.
func TestSQLiteTracingStoreListRoutingEdges(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	traces := NewSQLiteTracingStore(db)
	tenant := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(
		`INSERT INTO tenants (id, name, slug, status) VALUES (?, 't', 't', 'active')`,
		tenant.String(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	ctx := store.WithTenantID(context.Background(), tenant)
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)

	createLLMSpan := func(at time.Time, provider, model, status string) {
		traceID := uuid.Must(uuid.NewV7())
		if err := traces.CreateTrace(ctx, &store.TraceData{
			ID: traceID, Name: "run", StartTime: at, Status: "completed",
		}); err != nil {
			t.Fatalf("CreateTrace: %v", err)
		}
		if err := traces.CreateSpan(ctx, &store.SpanData{
			TraceID: traceID, SpanType: "llm_call", StartTime: at, Status: status,
			Model: model, Provider: provider, InputTokens: 10, OutputTokens: 5, TenantID: tenant,
		}); err != nil {
			t.Fatalf("CreateSpan: %v", err)
		}
	}

	// 3 calls provider-a/model-x (one error), 1 call provider-a/model-y,
	// 1 call outside the window, 1 call with an empty model (excluded).
	createLLMSpan(now.Add(-3*time.Hour), "provider-a", "model-x", "completed")
	createLLMSpan(now.Add(-2*time.Hour), "provider-a", "model-x", "completed")
	createLLMSpan(now.Add(-1*time.Hour), "provider-a", "model-x", "error")
	createLLMSpan(now.Add(-1*time.Hour), "provider-a", "model-y", "completed")
	createLLMSpan(now.Add(-30*time.Hour), "provider-a", "model-x", "completed")
	createLLMSpan(now.Add(-1*time.Hour), "provider-b", "", "completed")

	edges, err := traces.ListRoutingEdges(ctx, now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("ListRoutingEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("edges = %d, want 2: %#v", len(edges), edges)
	}
	if edges[0].Provider != "provider-a" || edges[0].Model != "model-x" {
		t.Fatalf("top edge = %s/%s, want provider-a/model-x", edges[0].Provider, edges[0].Model)
	}
	if edges[0].Calls != 3 || edges[0].Errors != 1 || edges[0].InputTokens != 30 || edges[0].OutputTokens != 15 {
		t.Fatalf("top edge aggregates = %+v, want calls=3 errors=1 in=30 out=15", edges[0])
	}
	if edges[1].Model != "model-y" || edges[1].Calls != 1 {
		t.Fatalf("second edge = %+v, want model-y calls=1", edges[1])
	}
}
