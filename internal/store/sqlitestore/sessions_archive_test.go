//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func findArchivedListed(res store.SessionListResult, key string) (store.SessionInfo, bool) {
	for _, s := range res.Sessions {
		if s.Key == key {
			return s, true
		}
	}
	return store.SessionInfo{}, false
}

func findArchivedListedRich(res store.SessionListRichResult, key string) (store.SessionInfoRich, bool) {
	for _, s := range res.Sessions {
		if s.Key == key {
			return s, true
		}
	}
	return store.SessionInfoRich{}, false
}

// TestSessionArchiveRestoreCycle covers the full archive → filtered → restore
// cycle: archived sessions disappear from the default ListPaged/ListPagedRich
// (the chat sidebar + sessions.list contract), reappear with ArchivedAt set
// when IncludeArchived=true, keep every message while hidden, and return to
// the default list after restore.
func TestSessionArchiveRestoreCycle(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	s := NewSQLiteSessionStore(db)
	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	key := "agent:archive-agent:ws:direct:archive-cycle-test"

	s.GetOrCreate(ctx, key)
	s.SetHistory(ctx, key, []providers.Message{
		{Role: "user", Content: "keep me"},
		{Role: "assistant", Content: "and me"},
	})
	if err := s.Save(ctx, key); err != nil {
		t.Fatalf("Save: %v", err)
	}

	activeOpts := func() store.SessionListOpts {
		return store.SessionListOpts{Limit: 50, TenantID: store.MasterTenantID}
	}

	// 1. Active session is visible in the default list with ArchivedAt nil.
	res := s.ListPaged(ctx, activeOpts())
	info, ok := findArchivedListed(res, key)
	if !ok {
		t.Fatalf("active session missing from default ListPaged (total=%d)", res.Total)
	}
	if info.ArchivedAt != nil {
		t.Fatalf("active session ArchivedAt = %v, want nil", info.ArchivedAt)
	}

	// 2. Archive → hidden from default paged + rich lists.
	if err := s.ArchiveSession(ctx, key); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
	if _, ok := findArchivedListed(s.ListPaged(ctx, activeOpts()), key); ok {
		t.Fatal("archived session still present in default ListPaged")
	}
	if _, ok := findArchivedListedRich(s.ListPagedRich(ctx, activeOpts()), key); ok {
		t.Fatal("archived session still present in default ListPagedRich")
	}

	// 3. IncludeArchived returns it, with ArchivedAt stamped, on both lists.
	allOpts := activeOpts()
	allOpts.IncludeArchived = true
	info, ok = findArchivedListed(s.ListPaged(ctx, allOpts), key)
	if !ok {
		t.Fatal("archived session missing from IncludeArchived ListPaged")
	}
	if info.ArchivedAt == nil {
		t.Fatal("ListPaged ArchivedAt = nil, want stamp after archive")
	}
	rich, ok := findArchivedListedRich(s.ListPagedRich(ctx, allOpts), key)
	if !ok {
		t.Fatal("archived session missing from IncludeArchived ListPagedRich")
	}
	if rich.ArchivedAt == nil {
		t.Fatal("ListPagedRich ArchivedAt = nil, want stamp after archive")
	}

	// 4. Messages survive archiving (cold store = fresh DB read, no cache).
	cold := NewSQLiteSessionStore(db).GetHistory(ctx, key)
	if len(cold) != 2 || cold[0].Content != "keep me" || cold[1].Content != "and me" {
		t.Fatalf("history while archived = %+v, want both messages intact", cold)
	}

	// 5. Restore → back in the default list, ArchivedAt cleared, messages intact.
	if err := s.RestoreSession(ctx, key); err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	info, ok = findArchivedListed(s.ListPaged(ctx, activeOpts()), key)
	if !ok {
		t.Fatal("restored session missing from default ListPaged")
	}
	if info.ArchivedAt != nil {
		t.Fatalf("restored ArchivedAt = %v, want nil", info.ArchivedAt)
	}
	if _, ok := findArchivedListedRich(s.ListPagedRich(ctx, activeOpts()), key); !ok {
		t.Fatal("restored session missing from default ListPagedRich")
	}
	cold = NewSQLiteSessionStore(db).GetHistory(ctx, key)
	if len(cold) != 2 {
		t.Fatalf("history after restore = %d messages, want 2", len(cold))
	}

	// 6. Restore on an active session is a harmless no-op.
	if err := s.RestoreSession(ctx, key); err != nil {
		t.Fatalf("RestoreSession (already active): %v", err)
	}
}

// TestSessionArchive_TenantScope pins the store-level scope: ArchiveSession
// issued from another tenant's context must not touch the row (matches how
// Delete scopes on session_key + tenant_id). Per-user ownership is enforced
// one layer up in the WS handler.
func TestSessionArchive_TenantScope(t *testing.T) {
	db := openTestDB(t)
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	s := NewSQLiteSessionStore(db)
	ctx := store.WithTenantID(context.Background(), store.MasterTenantID)
	key := "agent:archive-agent:ws:direct:archive-tenant-test"

	s.GetOrCreate(ctx, key)
	if err := s.Save(ctx, key); err != nil {
		t.Fatalf("Save: %v", err)
	}

	foreign := store.WithTenantID(context.Background(), uuid.Must(uuid.NewV7()))
	if err := s.ArchiveSession(foreign, key); err != nil {
		t.Fatalf("ArchiveSession (foreign tenant): %v", err)
	}

	opts := store.SessionListOpts{Limit: 50, TenantID: store.MasterTenantID}
	if _, ok := findArchivedListed(s.ListPaged(ctx, opts), key); !ok {
		t.Fatal("foreign-tenant archive hid the session — scope leak")
	}

	// Owner tenant archives successfully, proving the row was reachable all along.
	if err := s.ArchiveSession(ctx, key); err != nil {
		t.Fatalf("ArchiveSession (owner tenant): %v", err)
	}
	if _, ok := findArchivedListed(s.ListPaged(ctx, opts), key); ok {
		t.Fatal("owner-tenant archive did not hide the session")
	}
}
