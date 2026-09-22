package pg

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestPGSubagentArchiveByIDAndVisibility covers the Phase 5 per-ID archive +
// includeArchived list contract on PostgreSQL. Skips without TEST_DATABASE_URL
// (same harness as the other PG store tests).
func TestPGSubagentArchiveByIDAndVisibility(t *testing.T) {
	db := hooksTestDB(t)
	tenantA, rootAID := seedTenantAndAgent(t, db)
	tenantB, _ := seedTenantAndAgent(t, db)
	ctxA := tenantScopedCtx(tenantA)
	ctxB := tenantScopedCtx(tenantB)
	taskStore := NewPGSubagentTaskStore(db)

	t.Cleanup(func() {
		db.Exec("DELETE FROM subagent_tasks WHERE tenant_id IN ($1,$2)", tenantA, tenantB)
	})

	completed := createPGSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "session-a", "queued")
	if err := taskStore.UpdateStatus(ctxA, rootAID, completed, "completed", nil, 0, 0, 0); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	running := createPGSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "session-a", "running")

	// Non-terminal rows cannot be archived.
	if err := taskStore.ArchiveByID(ctxA, running); !errors.Is(err, store.ErrSubagentTaskNotTerminal) {
		t.Fatalf("ArchiveByID(running) error = %v, want ErrSubagentTaskNotTerminal", err)
	}
	// Cross-tenant and unknown IDs are not found.
	if err := taskStore.ArchiveByID(ctxB, completed); !errors.Is(err, store.ErrSubagentTaskNotFound) {
		t.Fatalf("ArchiveByID(cross tenant) error = %v, want ErrSubagentTaskNotFound", err)
	}
	if err := taskStore.ArchiveByID(ctxA, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrSubagentTaskNotFound) {
		t.Fatalf("ArchiveByID(unknown) error = %v, want ErrSubagentTaskNotFound", err)
	}

	// Terminal row archives once, then idempotently.
	if err := taskStore.ArchiveByID(ctxA, completed); err != nil {
		t.Fatalf("ArchiveByID(completed): %v", err)
	}
	if err := taskStore.ArchiveByID(ctxA, completed); err != nil {
		t.Fatalf("ArchiveByID(completed) second call should be idempotent: %v", err)
	}

	// GetByID is tenant-scoped and root-agnostic.
	got, err := taskStore.GetByID(ctxA, completed)
	if err != nil {
		t.Fatalf("GetByID owning tenant: %v", err)
	}
	if got == nil || got.ID != completed || got.ArchivedAt == nil {
		t.Fatalf("GetByID = %+v, want archived task %s", got, completed)
	}
	got, err = taskStore.GetByID(ctxB, completed)
	if err != nil {
		t.Fatalf("GetByID cross tenant: %v", err)
	}
	if got != nil {
		t.Fatalf("GetByID cross tenant = %#v, want nil", got)
	}

	// Default list hides archived; includeArchived=true reveals it.
	rows, err := taskStore.ListByParent(ctxA, rootAID, "", false)
	if err != nil {
		t.Fatalf("ListByParent default: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != running {
		t.Fatalf("ListByParent default = %v, want only running %s", rows, running)
	}
	rows, err = taskStore.ListByParent(ctxA, rootAID, "", true)
	if err != nil {
		t.Fatalf("ListByParent includeArchived: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListByParent includeArchived = %v, want 2 rows", rows)
	}
}

// TestPGSubagentArchiveCompletedForParent verifies the bulk endpoint archives
// every terminal non-archived row of the root and nothing else.
func TestPGSubagentArchiveCompletedForParent(t *testing.T) {
	db := hooksTestDB(t)
	tenantA, rootAID := seedTenantAndAgent(t, db)
	tenantB, rootBID := seedTenantAndAgent(t, db)
	ctxA := tenantScopedCtx(tenantA)
	ctxB := tenantScopedCtx(tenantB)
	taskStore := NewPGSubagentTaskStore(db)

	t.Cleanup(func() {
		db.Exec("DELETE FROM subagent_tasks WHERE tenant_id IN ($1,$2)", tenantA, tenantB)
	})

	var terminal []uuid.UUID
	for i := range 3 {
		id := createPGSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "session-a", "queued")
		if err := taskStore.UpdateStatus(ctxA, rootAID, id, "completed", nil, 0, 0, 0); err != nil {
			t.Fatalf("complete task %d: %v", i, err)
		}
		terminal = append(terminal, id)
	}
	queued := createPGSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "session-a", "queued")
	foreign := createPGSubagentTask(t, taskStore, ctxB, rootBID, "root-b", "session-b", "queued")
	if err := taskStore.UpdateStatus(ctxB, rootBID, foreign, "completed", nil, 0, 0, 0); err != nil {
		t.Fatalf("complete foreign task: %v", err)
	}

	archived, err := taskStore.ArchiveCompletedForParent(ctxA, rootAID)
	if err != nil {
		t.Fatalf("ArchiveCompletedForParent: %v", err)
	}
	if archived != int64(len(terminal)) {
		t.Fatalf("ArchiveCompletedForParent affected %d rows, want %d", archived, len(terminal))
	}

	rows, err := taskStore.ListByParent(ctxA, rootAID, "", false)
	if err != nil {
		t.Fatalf("ListByParent after batch: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != queued {
		t.Fatalf("ListByParent after batch = %v, want only queued %s", rows, queued)
	}
	foreignRows, err := taskStore.ListByParent(ctxB, rootBID, "", false)
	if err != nil {
		t.Fatalf("ListByParent foreign root: %v", err)
	}
	if len(foreignRows) != 1 || foreignRows[0].ID != foreign {
		t.Fatalf("ListByParent foreign root = %v, want untouched %s", foreignRows, foreign)
	}

	// Second pass is a no-op.
	archived, err = taskStore.ArchiveCompletedForParent(ctxA, rootAID)
	if err != nil {
		t.Fatalf("ArchiveCompletedForParent second pass: %v", err)
	}
	if archived != 0 {
		t.Fatalf("ArchiveCompletedForParent second pass affected %d, want 0", archived)
	}
}
