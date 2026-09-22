package methods

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// archiveStub wraps stubSessionStore with recording Archive/Restore so the
// handlers' type assertion to store.SessionArchiveStore succeeds, while Delete
// keeps using the embedded stub's recorder.
type archiveStub struct {
	*stubSessionStore
	archived []string
	restored []string
}

func newArchiveStub() *archiveStub {
	return &archiveStub{stubSessionStore: newStubSessionStore()}
}

func (a *archiveStub) ArchiveSession(_ context.Context, key string) error {
	a.archived = append(a.archived, key)
	return nil
}

func (a *archiveStub) RestoreSession(_ context.Context, key string) error {
	a.restored = append(a.restored, key)
	return nil
}

var (
	_ store.SessionStore        = (*archiveStub)(nil)
	_ store.SessionArchiveStore = (*archiveStub)(nil)
)

func buildArchiveSessionMethods(t *testing.T, sess *archiveStub) *SessionsMethods {
	t.Helper()
	return NewSessionsMethods(sess, &stubEventPub{}, &config.Config{})
}

// dispatchSessionMutation routes a parity-matrix method to its handler.
func dispatchSessionMutation(t *testing.T, m *SessionsMethods, method string, ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	t.Helper()
	switch method {
	case protocol.MethodSessionsArchive:
		m.handleArchive(ctx, client, req)
	case protocol.MethodSessionsRestore:
		m.handleRestore(ctx, client, req)
	case protocol.MethodSessionsDelete:
		m.handleDelete(ctx, client, req)
	default:
		t.Fatalf("unknown method %q", method)
	}
}

// decodeSessionResp unmarshals a captured client response frame.
func decodeSessionResp(t *testing.T, ch <-chan []byte) protocol.ResponseFrame {
	t.Helper()
	select {
	case raw := <-ch:
		var resp protocol.ResponseFrame
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		return resp
	default:
		t.Fatal("no response captured")
		return protocol.ResponseFrame{}
	}
}

// sessionMutationCallCount returns how many times the store-side operation for
// the given method was invoked.
func sessionMutationCallCount(method string, s *archiveStub) int {
	switch method {
	case protocol.MethodSessionsArchive:
		return len(s.archived)
	case protocol.MethodSessionsRestore:
		return len(s.restored)
	case protocol.MethodSessionsDelete:
		return len(s.deleted)
	default:
		return -1
	}
}

// TestSessionsArchiveRestoreDelete_AuthParity drives sessions.archive,
// sessions.restore, and sessions.delete through the identical auth matrix and
// asserts the three handlers behave the same: invalid JSON → invalid-request,
// missing session → not-found, other user's session → unauthorized with no
// store call, authorized (admin or owning user) → ok:true with exactly one
// store call.
func TestSessionsArchiveRestoreDelete_AuthParity(t *testing.T) {
	methodsUnderTest := []string{
		protocol.MethodSessionsArchive,
		protocol.MethodSessionsRestore,
		protocol.MethodSessionsDelete,
	}

	for _, method := range methodsUnderTest {
		method := method

		t.Run(method+"/invalid_json", func(t *testing.T) {
			sess := newArchiveStub()
			m := buildArchiveSessionMethods(t, sess)
			client, ch := gateway.NewCapturingTestClient(permissions.RoleViewer, store.MasterTenantID, "intruder", 2)

			req := &protocol.RequestFrame{
				Type:   protocol.FrameTypeRequest,
				ID:     "req-1",
				Method: method,
				Params: json.RawMessage(`{bad`),
			}
			dispatchSessionMutation(t, m, method, context.Background(), client, req)

			resp := decodeSessionResp(t, ch)
			if resp.OK || resp.Error == nil || resp.Error.Code != protocol.ErrInvalidRequest {
				t.Fatalf("response = %+v, want ErrInvalidRequest", resp)
			}
			if got := sessionMutationCallCount(method, sess); got != 0 {
				t.Fatalf("store called %d times on invalid JSON, want 0", got)
			}
		})

		t.Run(method+"/not_found", func(t *testing.T) {
			sess := newArchiveStub()
			m := buildArchiveSessionMethods(t, sess)
			client, ch := gateway.NewCapturingTestClient(permissions.RoleViewer, store.MasterTenantID, "intruder", 2)

			req := sessionReqFrame(t, method, map[string]any{"key": "missing-session"})
			dispatchSessionMutation(t, m, method, context.Background(), client, req)

			resp := decodeSessionResp(t, ch)
			if resp.OK || resp.Error == nil || resp.Error.Code != protocol.ErrNotFound {
				t.Fatalf("response = %+v, want ErrNotFound", resp)
			}
			if got := sessionMutationCallCount(method, sess); got != 0 {
				t.Fatalf("store called %d times for missing session, want 0", got)
			}
		})

		t.Run(method+"/wrong_user", func(t *testing.T) {
			sess := newArchiveStub()
			sess.addSession("sess-owned", "owner-user")
			m := buildArchiveSessionMethods(t, sess) // OwnerIDs empty → no see-all
			client, ch := gateway.NewCapturingTestClient(permissions.RoleViewer, store.MasterTenantID, "intruder", 2)

			req := sessionReqFrame(t, method, map[string]any{"key": "sess-owned"})
			dispatchSessionMutation(t, m, method, context.Background(), client, req)

			resp := decodeSessionResp(t, ch)
			if resp.OK || resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
				t.Fatalf("response = %+v, want ErrUnauthorized", resp)
			}
			if got := sessionMutationCallCount(method, sess); got != 0 {
				t.Fatalf("store called %d times for foreign session, want 0", got)
			}
		})

		t.Run(method+"/admin_success", func(t *testing.T) {
			sess := newArchiveStub()
			sess.addSession("sess-owned", "owner-user")
			m := buildArchiveSessionMethods(t, sess)
			// Admin role → canSeeAll → ownership check skipped, same as delete.
			client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "admin-1", 2)

			req := sessionReqFrame(t, method, map[string]any{"key": "sess-owned"})
			dispatchSessionMutation(t, m, method, context.Background(), client, req)

			resp := decodeSessionResp(t, ch)
			if !resp.OK || resp.Error != nil {
				t.Fatalf("response = %+v, want ok", resp)
			}
			payload, ok := resp.Payload.(map[string]any)
			if !ok || payload["ok"] != true {
				t.Fatalf("payload = %+v, want {ok:true}", resp.Payload)
			}
			if got := sessionMutationCallCount(method, sess); got != 1 {
				t.Fatalf("store called %d times, want 1", got)
			}
		})

		t.Run(method+"/owner_user_success", func(t *testing.T) {
			sess := newArchiveStub()
			sess.addSession("sess-owned", "owner-user")
			m := buildArchiveSessionMethods(t, sess)
			// Regular viewer who owns the session — non-admin path must pass.
			client, ch := gateway.NewCapturingTestClient(permissions.RoleViewer, store.MasterTenantID, "owner-user", 2)

			req := sessionReqFrame(t, method, map[string]any{"key": "sess-owned"})
			dispatchSessionMutation(t, m, method, context.Background(), client, req)

			resp := decodeSessionResp(t, ch)
			if !resp.OK {
				t.Fatalf("response = %+v, want ok", resp)
			}
			if got := sessionMutationCallCount(method, sess); got != 1 {
				t.Fatalf("store called %d times, want 1", got)
			}
		})
	}
}

// listOptsRecorder captures the SessionListOpts handleList builds.
type listOptsRecorder struct {
	*stubSessionStore
	lastOpts store.SessionListOpts
	calls    int
}

func (r *listOptsRecorder) ListPagedRich(_ context.Context, opts store.SessionListOpts) store.SessionListRichResult {
	r.lastOpts = opts
	r.calls++
	return store.SessionListRichResult{Sessions: []store.SessionInfoRich{}, Total: 0}
}

// TestSessionsList_IncludeArchivedParam verifies sessions.list forwards the
// includeArchived request param into the store opts (default false) so the
// archived sidebar section can lift the archived_at IS NULL filter.
func TestSessionsList_IncludeArchivedParam(t *testing.T) {
	rec := &listOptsRecorder{stubSessionStore: newStubSessionStore()}
	m := NewSessionsMethods(rec, &stubEventPub{}, &config.Config{})
	client := nullClient()

	m.handleList(context.Background(), client, sessionReqFrame(t, protocol.MethodSessionsList, map[string]any{}))
	if rec.calls != 1 {
		t.Fatalf("ListPagedRich calls = %d, want 1", rec.calls)
	}
	if rec.lastOpts.IncludeArchived {
		t.Fatal("IncludeArchived = true by default, want false (archived hidden)")
	}

	m.handleList(context.Background(), client, sessionReqFrame(t, protocol.MethodSessionsList, map[string]any{"includeArchived": true}))
	if rec.calls != 2 {
		t.Fatalf("ListPagedRich calls = %d, want 2", rec.calls)
	}
	if !rec.lastOpts.IncludeArchived {
		t.Fatal("IncludeArchived = false, want true when param set")
	}
}
