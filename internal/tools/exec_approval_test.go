package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/hooks"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// fakeApprovalStore captures ApprovalStore interactions for engine tests.
type fakeApprovalStore struct {
	mu       sync.Mutex
	created  []*store.ApprovalRequest
	pending  []store.ApprovalRequest
	resolved map[uuid.UUID]store.ApprovalGrant
	expired  []uuid.UUID
}

func (f *fakeApprovalStore) CreateRequest(_ context.Context, req *store.ApprovalRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.ID == uuid.Nil {
		req.ID = uuid.Must(uuid.NewV7())
	}
	cp := *req
	f.created = append(f.created, &cp)
	f.pending = append(f.pending, cp)
	return nil
}

func (f *fakeApprovalStore) ListPending(_ context.Context, tenantID uuid.UUID) ([]store.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.ApprovalRequest
	for _, r := range f.pending {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeApprovalStore) ListPendingAll(context.Context) ([]store.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.ApprovalRequest, len(f.pending))
	copy(out, f.pending)
	return out, nil
}

func (f *fakeApprovalStore) ResolveWithScope(_ context.Context, id uuid.UUID, decision string, decidedBy *uuid.UUID, grant store.ApprovalGrant) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.pending {
		if r.ID == id {
			f.pending = append(f.pending[:i], f.pending[i+1:]...)
			if f.resolved == nil {
				f.resolved = map[uuid.UUID]store.ApprovalGrant{}
			}
			f.resolved[id] = grant
			_ = decision
			_ = decidedBy
			return nil
		}
	}
	return store.ErrApprovalAlreadyResolved
}

func (f *fakeApprovalStore) GetByID(_ context.Context, id uuid.UUID) (*store.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.pending {
		if r.ID == id {
			cp := r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeApprovalStore) MarkExpired(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.pending {
		if r.ID == id {
			f.pending = append(f.pending[:i], f.pending[i+1:]...)
			f.expired = append(f.expired, id)
			return nil
		}
	}
	return nil
}

func (f *fakeApprovalStore) ListHistory(context.Context, uuid.UUID, store.ApprovalListOpts) ([]store.ApprovalRequest, error) {
	return nil, nil
}

// ── CheckCommand (legacy behavior guard) ─────────────────────────────────────

func TestCheckCommand_LegacyMatrix(t *testing.T) {
	tests := []struct {
		name    string
		config  ExecApprovalConfig
		command string
		want    string
	}{
		{
			name:    "deny blocks everything",
			config:  ExecApprovalConfig{Security: ExecSecurityDeny},
			command: "ls",
			want:    "deny",
		},
		{
			name:    "full + off allows",
			config:  ExecApprovalConfig{Security: ExecSecurityFull, Ask: ExecAskOff},
			command: "rm -rf /",
			want:    "allow",
		},
		{
			name:    "full + always asks",
			config:  ExecApprovalConfig{Security: ExecSecurityFull, Ask: ExecAskAlways},
			command: "ls",
			want:    "ask",
		},
		{
			name:    "full + on-miss allows safe bin",
			config:  ExecApprovalConfig{Security: ExecSecurityFull, Ask: ExecAskOnMiss},
			command: "cat foo.txt",
			want:    "allow",
		},
		{
			name:    "full + on-miss asks unsafe bin",
			config:  ExecApprovalConfig{Security: ExecSecurityFull, Ask: ExecAskOnMiss},
			command: "docker ps",
			want:    "ask",
		},
		{
			name: "allowlist + on-miss asks non-matching",
			config: ExecApprovalConfig{
				Security:  ExecSecurityAllowlist,
				Ask:       ExecAskOnMiss,
				Allowlist: []string{"git"},
			},
			command: "curl example.com",
			want:    "ask",
		},
		{
			name: "allowlist + on-miss allows matching",
			config: ExecApprovalConfig{
				Security:  ExecSecurityAllowlist,
				Ask:       ExecAskOnMiss,
				Allowlist: []string{"git*"},
			},
			command: "git status",
			want:    "allow",
		},
		{
			name: "allowlist + off denies non-matching",
			config: ExecApprovalConfig{
				Security:  ExecSecurityAllowlist,
				Ask:       ExecAskOff,
				Allowlist: []string{"git"},
			},
			command: "curl example.com",
			want:    "deny",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewExecApprovalManager(tt.config)
			if got := m.CheckCommand(tt.command); got != tt.want {
				t.Errorf("CheckCommand(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// ── Request/Resolve lifecycle ────────────────────────────────────────────────

func TestRequestApproval_ResolveAllowsWaiter(t *testing.T) {
	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	ctx := context.Background()

	type result struct {
		decision ApprovalDecision
		err      error
	}
	resCh := make(chan result, 1)
	go func() {
		d, err := m.RequestApproval(ctx, "docker ps", "agent-1", 5*time.Second)
		resCh <- result{d, err}
	}()

	// Wait for the request to register.
	deadline := time.Now().Add(2 * time.Second)
	var pending *PendingApproval
	for time.Now().Before(deadline) {
		if l := m.ListPending(); len(l) == 1 {
			pending = l[0]
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if pending == nil {
		t.Fatal("pending approval not registered")
	}
	if pending.ToolClass != ToolClassExec {
		t.Errorf("class = %q, want exec", pending.ToolClass)
	}
	if !strings.HasPrefix(pending.ID, "exec-") {
		t.Errorf("id = %q, want exec- prefix", pending.ID)
	}

	if err := m.Resolve(ctx, pending.ID, ApprovalAllowOnce, nil); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	res := <-resCh
	if res.err != nil {
		t.Fatalf("RequestApproval: %v", res.err)
	}
	if res.decision != ApprovalAllowOnce {
		t.Errorf("decision = %q, want allow-once", res.decision)
	}
	if left := m.ListPending(); len(left) != 0 {
		t.Errorf("pending left = %d, want 0", len(left))
	}
}

func TestRequestApproval_TimeoutDenies(t *testing.T) {
	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	d, err := m.RequestApproval(context.Background(), "docker ps", "agent-1", 20*time.Millisecond)
	if !errors.Is(err, ErrApprovalTimedOut) {
		t.Fatalf("err = %v, want ErrApprovalTimedOut", err)
	}
	if d != ApprovalDeny {
		t.Errorf("decision = %q, want deny", d)
	}
}

func TestRequestApproval_TenantGuardOnResolve(t *testing.T) {
	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	tenantA := uuid.Must(uuid.NewV7())
	tenantB := uuid.Must(uuid.NewV7())
	ctx := store.WithTenantID(context.Background(), tenantA)

	resCh := make(chan error, 1)
	go func() {
		_, err := m.RequestApproval(ctx, "docker ps", "", 5*time.Second)
		resCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	var id string
	for time.Now().Before(deadline) {
		if l := m.ListPending(); len(l) == 1 {
			id = l[0].ID
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if id == "" {
		t.Fatal("pending approval not registered")
	}

	// A resolver from another tenant is refused.
	err := m.Resolve(store.WithTenantID(context.Background(), tenantB), id, ApprovalDeny, nil)
	if err == nil || !strings.Contains(err.Error(), "another tenant") {
		t.Fatalf("cross-tenant resolve err = %v, want tenant error", err)
	}
	if err := m.Resolve(ctx, id, ApprovalDeny, nil); err != nil {
		t.Fatalf("same-tenant Resolve: %v", err)
	}
	<-resCh
}

func TestRequestApproval_PersistsWithDigestAndSession(t *testing.T) {
	fs := &fakeApprovalStore{}
	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	m.SetApprovalStore(fs)
	// Wait for the background reinstatement (empty store → no-op) to finish.
	time.Sleep(20 * time.Millisecond)

	tenant := uuid.Must(uuid.NewV7())
	ctx := store.WithTenantID(context.Background(), tenant)
	ctx = store.WithUserID(ctx, "user-1")

	resCh := make(chan error, 1)
	go func() {
		_, err := m.RequestApproval(ctx, "kubectl delete pod", "", 5*time.Second)
		resCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	var id string
	for time.Now().Before(deadline) {
		if l := m.ListPending(); len(l) == 1 {
			id = l[0].ID
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	m.Resolve(ctx, id, ApprovalDeny, nil)
	<-resCh

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.created) != 1 {
		t.Fatalf("created rows = %d, want 1", len(fs.created))
	}
	row := fs.created[0]
	if row.TenantID != tenant {
		t.Errorf("tenant = %v, want %v", row.TenantID, tenant)
	}
	if row.ActionType != "exec" {
		t.Errorf("action_type = %q, want exec", row.ActionType)
	}
	if row.ArgsDigest == "" {
		t.Error("args_digest empty, want command digest")
	}
	if row.ExpiredAt == nil {
		t.Error("expired_at nil, want request deadline")
	}
}

// ── Reinstatement (waiters-without-waiter) ───────────────────────────────────

func TestSetApprovalStore_ReinstatesPendingRows(t *testing.T) {
	tenant := uuid.Must(uuid.NewV7())
	rowID := uuid.Must(uuid.NewV7())
	fs := &fakeApprovalStore{pending: []store.ApprovalRequest{{
		ID:         rowID,
		TenantID:   tenant,
		ActionType: "browser",
		Command:    `browser {"action":"navigate"}`,
		Status:     store.ApprovalStatusPending,
		Payload:    json.RawMessage(`{"risk":"hook wants approval"}`),
	}}}

	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	m.SetApprovalStore(fs)

	// reinstatePending runs in the background; poll for it.
	deadline := time.Now().Add(2 * time.Second)
	var pending *PendingApproval
	for time.Now().Before(deadline) {
		if l := m.ListPending(); len(l) == 1 {
			pending = l[0]
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if pending == nil {
		t.Fatal("pending row not reinstated")
	}
	if pending.ID != rowID.String() {
		t.Errorf("reinstated id = %q, want durable UUID", pending.ID)
	}
	if pending.ToolClass != ToolClassBrowser {
		t.Errorf("class = %q, want browser", pending.ToolClass)
	}
	if pending.Risk != "hook wants approval" {
		t.Errorf("risk = %q, want payload risk hint", pending.Risk)
	}

	// A decision after the "restart" resolves the durable row instead of
	// erroring with ErrApprovalNotFound.
	decidedBy := uuid.Must(uuid.NewV7())
	if err := m.Resolve(store.WithTenantID(context.Background(), tenant), rowID.String(), ApprovalAllowAlways, &decidedBy); err != nil {
		t.Fatalf("Resolve on reinstated row: %v", err)
	}

	// The durable transition happens in a best-effort background goroutine.
	deadline = time.Now().Add(2 * time.Second)
	var grant store.ApprovalGrant
	var ok bool
	var pendingLeft int
	for time.Now().Before(deadline) {
		fs.mu.Lock()
		grant, ok = fs.resolved[rowID]
		pendingLeft = len(fs.pending)
		fs.mu.Unlock()
		if ok {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !ok {
		t.Fatal("durable row not resolved")
	}
	if !grant.AllowAlways {
		t.Errorf("grant = %+v, want AllowAlways", grant)
	}
	if pendingLeft != 0 {
		t.Errorf("store pending left = %d, want 0", pendingLeft)
	}
}

func TestSetApprovalStore_SkipsStalePendingRows(t *testing.T) {
	tenant := uuid.Must(uuid.NewV7())
	staleID := uuid.Must(uuid.NewV7())
	past := time.Now().Add(-time.Minute)
	fs := &fakeApprovalStore{pending: []store.ApprovalRequest{{
		ID:         staleID,
		TenantID:   tenant,
		ActionType: "exec",
		Status:     store.ApprovalStatusPending,
		ExpiredAt:  &past,
	}}}

	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	m.SetApprovalStore(fs)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fs.mu.Lock()
		n := len(fs.pending)
		fs.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.pending) != 0 {
		t.Fatalf("stale row still pending in store: %d", len(fs.pending))
	}
	expired := false
	for _, id := range fs.expired {
		if id == staleID {
			expired = true
		}
	}
	if !expired {
		t.Error("stale row not marked expired")
	}
	if l := m.ListPending(); len(l) != 0 {
		t.Errorf("stale row reinstated into memory: %d", len(l))
	}
}

// ── Grant scopes ─────────────────────────────────────────────────────────────

func TestGrantStore_ScopesAndExpiry(t *testing.T) {
	g := newGrantStore()
	expired := time.Now().Add(-time.Second)
	future := time.Now().Add(time.Minute)

	g.addOnce(ToolClassBrowser, "browser", "digest-1", nil)
	if !g.checkOnce(ToolClassBrowser, "browser", "digest-1") {
		t.Fatal("once grant miss on first check")
	}
	if g.checkOnce(ToolClassBrowser, "browser", "digest-1") {
		t.Error("once grant matched twice; allow-once must be single-use")
	}

	g.addSession(ToolClassBrowser, "browser", "sess-1", nil)
	if !g.checkSession(ToolClassBrowser, "browser", "sess-1") {
		t.Error("session grant miss")
	}
	if g.checkSession(ToolClassBrowser, "browser", "sess-2") {
		t.Error("session grant leaked across sessions")
	}

	g.addAlways(ToolClassWorkstationExec, "workstation_exec", nil)
	if !g.checkAlways(ToolClassWorkstationExec, "workstation_exec") {
		t.Error("always grant miss")
	}

	// Expiry: a grant created already-expired never matches.
	g.addOnce(ToolClassBrowser, "browser", "digest-2", &expired)
	if g.checkOnce(ToolClassBrowser, "browser", "digest-2") {
		t.Error("expired once grant matched")
	}
	g.addSession(ToolClassBrowser, "browser", "sess-3", &expired)
	if g.checkSession(ToolClassBrowser, "browser", "sess-3") {
		t.Error("expired session grant matched")
	}
	g.addAlways(ToolClassBrowser, "browser_x", &expired)
	if g.checkAlways(ToolClassBrowser, "browser_x") {
		t.Error("expired always grant matched")
	}
	_ = future

	// Missing keys never match.
	if g.checkOnce(ToolClassBrowser, "browser", "") {
		t.Error("empty digest matched a once grant")
	}
	if g.checkSession(ToolClassBrowser, "browser", "") {
		t.Error("empty session matched a session grant")
	}
}

// ── Tool gate ────────────────────────────────────────────────────────────────

func TestClassifyTool(t *testing.T) {
	tests := []struct {
		tool string
		want ToolClass
	}{
		{"exec", ToolClassExec},
		{"bash", ToolClassExec},
		{"EXEC", ToolClassExec},
		{"browser", ToolClassBrowser},
		{"browser_navigate", ToolClassBrowser},
		{"workstation_exec", ToolClassWorkstationExec},
		{"write_file", ToolClassWriteFile},
		{"edit_file", ToolClassWriteFile},
		{"read_file", ""},
		{"web_search", ""},
		{"", ""},
		{"mcp_unknown", ""},
	}
	for _, tt := range tests {
		if got := ClassifyTool(tt.tool); got != tt.want {
			t.Errorf("ClassifyTool(%q) = %q, want %q", tt.tool, got, tt.want)
		}
	}
}

func TestGateToolCall_Modes(t *testing.T) {
	tests := []struct {
		name      string
		policies  map[ToolClass]ToolApprovalMode
		tool      string
		wantAllow bool
	}{
		{
			name:      "no policy → allow",
			policies:  nil,
			tool:      "browser",
			wantAllow: true,
		},
		{
			name:      "off → allow",
			policies:  map[ToolClass]ToolApprovalMode{ToolClassBrowser: ToolModeOff},
			tool:      "browser",
			wantAllow: true,
		},
		{
			name:      "deny → blocked",
			policies:  map[ToolClass]ToolApprovalMode{ToolClassBrowser: ToolModeDeny},
			tool:      "browser",
			wantAllow: false,
		},
		{
			name:      "exec class never double-gated",
			policies:  map[ToolClass]ToolApprovalMode{ToolClassExec: ToolModeDeny},
			tool:      "exec",
			wantAllow: true,
		},
		{
			name:      "unknown class → allow",
			policies:  map[ToolClass]ToolApprovalMode{ToolClassBrowser: ToolModeDeny},
			tool:      "web_search",
			wantAllow: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewExecApprovalManager(ExecApprovalConfig{ToolPolicies: tt.policies})
			m.askTimeout = 20 * time.Millisecond
			allow, _ := m.GateToolCall(context.Background(), hooks.ApprovalQuery{ToolName: tt.tool})
			if allow != tt.wantAllow {
				t.Errorf("GateToolCall(%q) allow = %v, want %v", tt.tool, allow, tt.wantAllow)
			}
		})
	}
}

func TestResolveAsk_ApproveThenGrantPassesRetry(t *testing.T) {
	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	m.askTimeout = 5 * time.Second

	q := hooks.ApprovalQuery{
		ToolName:   "browser",
		ToolInput:  map[string]any{"action": "navigate"},
		ArgsDigest: "digest-abc",
		SessionKey: "sess-9",
		Risk:       "policy review",
	}

	type result struct {
		allow bool
		err   bool
	}
	resCh := make(chan result, 1)
	go func() {
		allow, _ := m.ResolveAsk(context.Background(), q)
		resCh <- result{allow, false}
	}()

	deadline := time.Now().Add(2 * time.Second)
	var pending *PendingApproval
	for time.Now().Before(deadline) {
		if l := m.ListPending(); len(l) == 1 {
			pending = l[0]
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if pending == nil {
		t.Fatal("ask approval request not created")
	}
	if pending.ToolClass != ToolClassBrowser {
		t.Errorf("class = %q, want browser", pending.ToolClass)
	}
	if pending.Risk != "policy review" {
		t.Errorf("risk = %q, want hook risk hint", pending.Risk)
	}

	// Approve with a session scope: the current wait unblocks AND the grant
	// lets the retried/resumed identical call pass without prompting.
	if err := m.ResolveScope(context.Background(), pending.ID, ApprovalAllowForSession, nil, nil); err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	res := <-resCh
	if !res.allow {
		t.Fatal("approved ask returned deny")
	}

	// Retry: grant fast-path, no new request.
	allow, _ := m.ResolveAsk(context.Background(), q)
	if !allow {
		t.Fatal("retried ask denied; session grant missing")
	}
	if l := m.ListPending(); len(l) != 0 {
		t.Errorf("retry created extra requests: %d", len(l))
	}
}

func TestResolveAsk_TimeoutDenies(t *testing.T) {
	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	m.askTimeout = 20 * time.Millisecond

	allow, reason := m.ResolveAsk(context.Background(), hooks.ApprovalQuery{
		ToolName:   "workstation_exec",
		ToolInput:  map[string]any{"cmd": "reboot"},
		ArgsDigest: "digest-xyz",
	})
	if allow {
		t.Fatal("timed-out ask returned allow")
	}
	if reason == "" {
		t.Error("deny reason empty")
	}
}

func TestResolveAsk_AllowOnceGrantSingleUse(t *testing.T) {
	m := NewExecApprovalManager(DefaultExecApprovalConfig())
	m.askTimeout = 5 * time.Second

	q := hooks.ApprovalQuery{ToolName: "browser", ArgsDigest: "digest-1", SessionKey: "s"}

	goApprove := func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if l := m.ListPending(); len(l) == 1 {
				m.Resolve(context.Background(), l[0].ID, ApprovalAllowOnce, nil)
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}

	go func() { _, _ = m.ResolveAsk(context.Background(), q) }()
	goApprove()

	// First retry passes via the allow-once digest grant...
	if allow, _ := m.ResolveAsk(context.Background(), q); !allow {
		t.Fatal("retry denied; allow-once digest grant missing")
	}
	// ...the second must prompt again (grant consumed).
	m.askTimeout = 20 * time.Millisecond
	if allow, _ := m.ResolveAsk(context.Background(), q); allow {
		t.Fatal("second retry allowed; allow-once grant must be single-use")
	}
}
