//go:build sqlite || sqliteonly

package sqlitestore

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestSQLiteSubagentListByParentArchivedFilter verifies the Phase 5 archive
// visibility contract: archived tasks disappear from the default list and
// reappear (with archived_at) when includeArchived=true.
func TestSQLiteSubagentListByParentArchivedFilter(t *testing.T) {
	db := newHookTestDB(t)
	tenantID, rootID := seedHookTenantAgent(t, db)
	ctx := sqliteTenantCtx(tenantID)
	taskStore := NewSQLiteSubagentTaskStore(db)

	visible := createSQLiteSubagentTask(t, taskStore, ctx, rootID, "root-a", "session-live", "queued")
	archived := createSQLiteSubagentTask(t, taskStore, ctx, rootID, "root-a", "session-archived", "queued")
	if err := taskStore.UpdateStatus(ctx, rootID, archived, "completed", nil, 0, 0, 0); err != nil {
		t.Fatalf("complete archived task: %v", err)
	}
	if err := taskStore.ArchiveByID(ctx, archived); err != nil {
		t.Fatalf("ArchiveByID: %v", err)
	}

	got, err := taskStore.ListByParent(ctx, rootID, "", false)
	if err != nil {
		t.Fatalf("ListByParent default: %v", err)
	}
	if len(got) != 1 || got[0].ID != visible {
		t.Fatalf("ListByParent default = %v, want only %s", idsOf(got), visible)
	}

	got, err = taskStore.ListByParent(ctx, rootID, "", true)
	if err != nil {
		t.Fatalf("ListByParent includeArchived: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByParent includeArchived = %v, want 2 rows", idsOf(got))
	}
	byID := map[uuid.UUID]store.SubagentTaskData{}
	for _, task := range got {
		byID[task.ID] = task
	}
	if byID[archived].ArchivedAt == nil {
		t.Fatalf("archived task missing archived_at: %+v", byID[archived])
	}
	if byID[visible].ArchivedAt != nil {
		t.Fatalf("live task unexpectedly archived: %+v", byID[visible])
	}

	// The status filter composes with the archive filter.
	got, err = taskStore.ListByParent(ctx, rootID, "completed", false)
	if err != nil {
		t.Fatalf("ListByParent status+archive filter: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListByParent(completed, no archived) = %v, want 0 rows", idsOf(got))
	}
}

// TestSQLiteSubagentArchiveByID covers per-ID archive semantics: terminal-only
// enforcement, precise sentinels, idempotency, and tenant scoping.
func TestSQLiteSubagentArchiveByID(t *testing.T) {
	db := newHookTestDB(t)
	tenantA, rootAID := seedHookTenantAgent(t, db)
	tenantB, rootBID := seedHookTenantAgent(t, db)
	ctxA := sqliteTenantCtx(tenantA)
	ctxB := sqliteTenantCtx(tenantB)
	taskStore := NewSQLiteSubagentTaskStore(db)

	completed := createSQLiteSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "s-completed", "queued")
	if err := taskStore.UpdateStatus(ctxA, rootAID, completed, "completed", nil, 0, 0, 0); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	running := createSQLiteSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "s-running", "running")
	foreign := createSQLiteSubagentTask(t, taskStore, ctxB, rootBID, "root-b", "s-foreign", "queued")
	if err := taskStore.UpdateStatus(ctxB, rootBID, foreign, "completed", nil, 0, 0, 0); err != nil {
		t.Fatalf("complete foreign task: %v", err)
	}

	// Non-terminal → precise sentinel, row untouched.
	if err := taskStore.ArchiveByID(ctxA, running); !errors.Is(err, store.ErrSubagentTaskNotTerminal) {
		t.Fatalf("ArchiveByID(running) error = %v, want ErrSubagentTaskNotTerminal", err)
	}
	var archivedAt sql.NullString
	if err := db.QueryRow(`SELECT archived_at FROM subagent_tasks WHERE id = ?`, running).Scan(&archivedAt); err != nil {
		t.Fatalf("read running archived_at: %v", err)
	}
	if archivedAt.Valid {
		t.Fatal("non-terminal task was archived")
	}

	// Cross-tenant → not found (tenant-scoped UPDATE).
	if err := taskStore.ArchiveByID(ctxA, foreign); !errors.Is(err, store.ErrSubagentTaskNotFound) {
		t.Fatalf("ArchiveByID(foreign) error = %v, want ErrSubagentTaskNotFound", err)
	}

	// Unknown ID → not found.
	if err := taskStore.ArchiveByID(ctxA, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrSubagentTaskNotFound) {
		t.Fatalf("ArchiveByID(unknown) error = %v, want ErrSubagentTaskNotFound", err)
	}

	// Terminal → archived once, then idempotent.
	if err := taskStore.ArchiveByID(ctxA, completed); err != nil {
		t.Fatalf("ArchiveByID(completed): %v", err)
	}
	if err := taskStore.ArchiveByID(ctxA, completed); err != nil {
		t.Fatalf("ArchiveByID(completed) second call should be idempotent: %v", err)
	}
	got, err := taskStore.Get(ctxA, rootAID, completed)
	if err != nil {
		t.Fatalf("Get archived task: %v", err)
	}
	if got == nil || got.ArchivedAt == nil {
		t.Fatalf("completed task not archived: %+v", got)
	}

	// Owner tenant can still archive its own row.
	if err := taskStore.ArchiveByID(ctxB, foreign); err != nil {
		t.Fatalf("ArchiveByID by owning tenant: %v", err)
	}
}

// TestSQLiteSubagentGetByID covers the tenant-scoped by-ID fetch used by the
// WS archive/cancel handlers: same tenant resolves, cross tenant is absent,
// and legacy NULL-root rows are not addressable.
func TestSQLiteSubagentGetByID(t *testing.T) {
	db := newHookTestDB(t)
	tenantA, rootAID := seedHookTenantAgent(t, db)
	tenantB, _ := seedHookTenantAgent(t, db)
	ctxA := sqliteTenantCtx(tenantA)
	ctxB := sqliteTenantCtx(tenantB)
	taskStore := NewSQLiteSubagentTaskStore(db)

	task := createSQLiteSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "s-get", "queued")

	got, err := taskStore.GetByID(ctxA, task)
	if err != nil {
		t.Fatalf("GetByID owning tenant: %v", err)
	}
	if got == nil || got.ID != task {
		t.Fatalf("GetByID = %#v, want task %s", got, task)
	}

	got, err = taskStore.GetByID(ctxB, task)
	if err != nil {
		t.Fatalf("GetByID cross tenant: %v", err)
	}
	if got != nil {
		t.Fatalf("GetByID cross tenant = %#v, want nil", got)
	}

	// Legacy row without root agent is unaddressable.
	legacy := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(
		`INSERT INTO subagent_tasks (id, tenant_id, parent_agent_key, subject, description, status, metadata)
		 VALUES (?,?,?,?,?,'queued','{}')`,
		legacy, tenantA, "root-a", "legacy", "no root agent",
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	got, err = taskStore.GetByID(ctxA, legacy)
	if err != nil {
		t.Fatalf("GetByID legacy row: %v", err)
	}
	if got != nil {
		t.Fatalf("GetByID legacy row = %#v, want nil (unaddressable)", got)
	}
}

// TestSQLiteSubagentArchiveCompletedForParent covers the bulk endpoint:
// terminal non-archived rows of the root are archived, queued rows and other
// roots stay untouched.
func TestSQLiteSubagentArchiveCompletedForParent(t *testing.T) {
	db := newHookTestDB(t)
	tenantA, rootAID := seedHookTenantAgent(t, db)
	tenantB, tenantBRootID := seedHookTenantAgent(t, db)
	ctxA := sqliteTenantCtx(tenantA)
	ctxB := sqliteTenantCtx(tenantB)
	taskStore := NewSQLiteSubagentTaskStore(db)

	var terminal []uuid.UUID
	for i := 0; i < 3; i++ {
		id := createSQLiteSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "s-terminal", "queued")
		if err := taskStore.UpdateStatus(ctxA, rootAID, id, "completed", nil, 0, 0, 0); err != nil {
			t.Fatalf("complete task %d: %v", i, err)
		}
		terminal = append(terminal, id)
	}
	queued := createSQLiteSubagentTask(t, taskStore, ctxA, rootAID, "root-a", "s-queued", "queued")
	otherRoot := uuid.Must(uuid.NewV7())
	if _, err := db.Exec(
		`INSERT INTO agents (id, tenant_id, agent_key, agent_type, status, provider, model, owner_id)
		 VALUES (?,?,?,'predefined','active','test','test-model','owner')`,
		otherRoot, tenantA, "root-other",
	); err != nil {
		t.Fatalf("seed other root agent: %v", err)
	}
	otherTask := createSQLiteSubagentTask(t, taskStore, ctxA, otherRoot, "root-other", "s-other", "queued")
	if err := taskStore.UpdateStatus(ctxA, otherRoot, otherTask, "completed", nil, 0, 0, 0); err != nil {
		t.Fatalf("complete other root task: %v", err)
	}
	foreignTask := createSQLiteSubagentTask(t, taskStore, ctxB, tenantBRootID, "root-a", "s-foreign", "queued")
	if err := taskStore.UpdateStatus(ctxB, tenantBRootID, foreignTask, "completed", nil, 0, 0, 0); err != nil {
		t.Fatalf("complete foreign task: %v", err)
	}

	archived, err := taskStore.ArchiveCompletedForParent(ctxA, rootAID)
	if err != nil {
		t.Fatalf("ArchiveCompletedForParent: %v", err)
	}
	if archived != int64(len(terminal)) {
		t.Fatalf("ArchiveCompletedForParent affected %d rows, want %d", archived, len(terminal))
	}
	assertSQLiteArchivedCount(t, db, tenantA, rootAID, len(terminal))
	assertSQLiteArchivedCount(t, db, tenantA, otherRoot, 0)
	assertSQLiteArchivedCount(t, db, tenantB, tenantBRootID, 0)

	got, err := taskStore.ListByParent(ctxA, rootAID, "", false)
	if err != nil {
		t.Fatalf("ListByParent after batch archive: %v", err)
	}
	if len(got) != 1 || got[0].ID != queued {
		t.Fatalf("ListByParent after batch = %v, want only queued %s", idsOf(got), queued)
	}

	// Second pass is a no-op.
	archived, err = taskStore.ArchiveCompletedForParent(ctxA, rootAID)
	if err != nil {
		t.Fatalf("ArchiveCompletedForParent second pass: %v", err)
	}
	if archived != 0 {
		t.Fatalf("ArchiveCompletedForParent second pass affected %d, want 0", archived)
	}

	if _, err := taskStore.ArchiveCompletedForParent(ctxA, uuid.Nil); !errors.Is(err, store.ErrSubagentRootAgentIDRequired) {
		t.Fatalf("ArchiveCompletedForParent(nil root) error = %v, want ErrSubagentRootAgentIDRequired", err)
	}
}

func idsOf(tasks []store.SubagentTaskData) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
