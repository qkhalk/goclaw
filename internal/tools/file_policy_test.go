package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeAgentLookup struct {
	agents map[string]*store.AgentData
	err    error
}

func (f *fakeAgentLookup) GetByKey(_ context.Context, agentKey string) (*store.AgentData, error) {
	if f.err != nil {
		return nil, f.err
	}
	if ag, ok := f.agents[agentKey]; ok {
		return ag, nil
	}
	return nil, nil
}

func agentWithPolicy(t *testing.T, read, write, create *bool) *store.AgentData {
	t.Helper()
	inner, err := json.Marshal(config.FilePolicy{Read: read, Write: write, Create: create})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]json.RawMessage{"file_policy": inner})
	if err != nil {
		t.Fatal(err)
	}
	return &store.AgentData{
		AgentKey:    "test-agent",
		OtherConfig: raw,
	}
}

func ctxWithAgent(key string) context.Context {
	return WithToolAgentKey(context.Background(), key)
}

func TestFilePolicyGuardDeniesRead(t *testing.T) {
	no := false
	lookup := &fakeAgentLookup{agents: map[string]*store.AgentData{
		"reader": agentWithPolicy(t, &no, nil, nil),
	}}
	guard := NewFilePolicyGuard(lookup)

	// read denied
	if err := guard.Check(ctxWithAgent("reader"), FileActionRead); err == nil {
		t.Fatal("expected read denial for read=false policy")
	}
	// write/create untouched (nil = allow)
	if err := guard.Check(ctxWithAgent("reader"), FileActionWrite); err != nil {
		t.Fatalf("write should be allowed, got: %v", err)
	}
	if err := guard.Check(ctxWithAgent("reader"), FileActionCreate); err != nil {
		t.Fatalf("create should be allowed, got: %v", err)
	}
}

func TestFilePolicyGuardCheckAny(t *testing.T) {
	no := false
	lookup := &fakeAgentLookup{agents: map[string]*store.AgentData{
		"creator": agentWithPolicy(t, nil, &no, nil), // create allowed, write denied
	}}
	guard := NewFilePolicyGuard(lookup)

	// write_file composite: one of (write, create) allowed → OK
	if err := guard.CheckAny(ctxWithAgent("creator"), FileActionWrite, FileActionCreate); err != nil {
		t.Fatalf("CheckAny should pass when create is allowed: %v", err)
	}
	if err := guard.Check(ctxWithAgent("creator"), FileActionWrite); err == nil {
		t.Fatal("write alone should be denied")
	}
}

func TestFilePolicyGuardFailOpen(t *testing.T) {
	lookup := &fakeAgentLookup{agents: map[string]*store.AgentData{}}
	guard := NewFilePolicyGuard(lookup)

	// Unknown agent / no agent key / nil guard → allow.
	if err := guard.Check(ctxWithAgent("ghost"), FileActionRead); err != nil {
		t.Fatalf("unknown agent must fail open: %v", err)
	}
	if err := guard.Check(context.Background(), FileActionWrite); err != nil {
		t.Fatalf("missing agent key must fail open: %v", err)
	}
	var nilGuard *FilePolicyGuard
	if err := nilGuard.Check(ctxWithAgent("x"), FileActionCreate); err != nil {
		t.Fatalf("nil guard must fail open: %v", err)
	}
}

func TestFilePolicyGuardCacheRespectsTTL(t *testing.T) {
	no := false
	lookup := &fakeAgentLookup{agents: map[string]*store.AgentData{
		"cached": agentWithPolicy(t, &no, nil, nil),
	}}
	guard := NewFilePolicyGuard(lookup)
	if err := guard.Check(ctxWithAgent("cached"), FileActionRead); err == nil {
		t.Fatal("expected denial")
	}
	// Flip the policy in the backing store; TTL cache still holds the old value.
	yes := true
	lookup.agents["cached"] = agentWithPolicy(t, &yes, nil, nil)
	if err := guard.Check(ctxWithAgent("cached"), FileActionRead); err == nil {
		t.Fatal("expected cached denial to persist within TTL")
	}
	guard.mu.Lock()
	for k, e := range guard.cache {
		e.expires = time.Now().Add(-time.Minute)
		guard.cache[k] = e
	}
	guard.mu.Unlock()
	if err := guard.Check(ctxWithAgent("cached"), FileActionRead); err != nil {
		t.Fatalf("after TTL expiry the new policy should apply: %v", err)
	}
}

func TestRegistryFilePolicyDeniesReadTool(t *testing.T) {
	no := false
	lookup := &fakeAgentLookup{agents: map[string]*store.AgentData{
		"reader": agentWithPolicy(t, &no, nil, nil),
	}}
	reg := NewRegistry()
	reg.Register(NewReadFileTool(t.TempDir(), true))
	reg.SetFilePolicyGuard(NewFilePolicyGuard(lookup))

	res := reg.Execute(ctxWithAgent("reader"), "read_file", map[string]any{"path": "x.txt"})
	if res == nil || !res.IsError {
		t.Fatal("read_file should be denied by file policy")
	}
	if !strings.Contains(res.ForLLM, "file policy") {
		t.Fatalf("unexpected denial message: %q", res.ForLLM)
	}

	// Alias resolution must classify by canonical name.
	reg.RegisterAlias("Read", "read_file")
	res = reg.Execute(ctxWithAgent("reader"), "Read", map[string]any{"path": "x.txt"})
	if res == nil || !res.IsError || !strings.Contains(res.ForLLM, "file policy") {
		t.Fatalf("alias Read should be denied too: %+v", res)
	}

	// No guard wired → no enforcement.
	reg2 := NewRegistry()
	reg2.Register(NewReadFileTool(t.TempDir(), true))
	res = reg2.Execute(ctxWithAgent("reader"), "read_file", map[string]any{"path": "missing.txt"})
	if res != nil && res.IsError && strings.Contains(res.ForLLM, "file policy") {
		t.Fatal("without a guard there must be no file policy denial")
	}
}

func TestWriteFileToolPolicySplit(t *testing.T) {
	no := false
	lookup := &fakeAgentLookup{agents: map[string]*store.AgentData{
		"creator": agentWithPolicy(t, nil, &no, nil), // create allowed, write denied
	}}
	dir := t.TempDir()
	tool := NewWriteFileTool(dir, false)
	tool.SetFilePolicyGuard(NewFilePolicyGuard(lookup))

	// New file → "create" → allowed.
	res := tool.Execute(ctxWithAgent("creator"), map[string]any{"path": "new.txt", "content": "hi"})
	if res.IsError {
		t.Fatalf("create should be allowed: %s", res.ForLLM)
	}
	// Same file now exists → "write" → denied.
	res = tool.Execute(ctxWithAgent("creator"), map[string]any{"path": "new.txt", "content": "again"})
	if !res.IsError || !strings.Contains(res.ForLLM, "file policy") {
		t.Fatalf("overwrite should be denied by write=false policy: %+v", res)
	}
}

func TestFilePolicyAllowedDefaults(t *testing.T) {
	p := config.ParseFilePolicy(nil)
	if !p.CanRead() || !p.CanWrite() || !p.CanCreate() {
		t.Fatal("empty policy must allow everything")
	}
	p = config.ParseFilePolicy([]byte(`{"write":false}`))
	if !p.CanRead() || p.CanWrite() || !p.CanCreate() {
		t.Fatal("partial policy must default unset caps to allow")
	}
}

func TestFilePolicyActionCoversCloudMutations(t *testing.T) {
	cases := map[string]struct {
		action FileAction
		ok     bool
	}{
		"cloud_ls":      {FileActionRead, true},
		"cloud_read":    {FileActionRead, true},
		"cloud_write":   {FileActionWrite, true},
		"cloud_delete":  {FileActionWrite, true},
		"cloud_move":    {FileActionWrite, true},
		"cloud_share":   {FileActionWrite, true},
		"cloud_mkdir":   {FileActionCreate, true},
		"cloud_copy":    {FileActionCreate, true},
		"cloud_account": {"", false}, // metadata tool — outside the policy
		"send_file":     {"", false}, // documented gap: exec-class stays outside
	}
	for name, want := range cases {
		action, ok := filePolicyAction(name)
		if action != want.action || ok != want.ok {
			t.Errorf("filePolicyAction(%q) = (%q, %v), want (%q, %v)", name, action, ok, want.action, want.ok)
		}
	}
}
