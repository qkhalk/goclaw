package methods

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/nodes"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// stubNodeStore implements store.NodeStore for handler tests.
type stubNodeStore struct {
	byID      map[string]*store.Node
	byKeyHash map[string]*store.Node
	created   []*store.Node
	trustLog  [][2]string // id → trust
	revoked   []string
}

func newStubNodeStore() *stubNodeStore {
	return &stubNodeStore{
		byID:      make(map[string]*store.Node),
		byKeyHash: make(map[string]*store.Node),
	}
}

func (s *stubNodeStore) Create(_ context.Context, n *store.Node) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	clone := *n
	s.byID[n.ID] = &clone
	s.byKeyHash[n.NodeKeyHash] = &clone
	s.created = append(s.created, &clone)
	return nil
}

func (s *stubNodeStore) GetByID(_ context.Context, id string) (*store.Node, error) {
	if n, ok := s.byID[id]; ok {
		clone := *n
		return &clone, nil
	}
	return nil, errStubNoRows{}
}

func (s *stubNodeStore) GetByKeyHash(_ context.Context, hash string) (*store.Node, error) {
	if n, ok := s.byKeyHash[hash]; ok {
		clone := *n
		return &clone, nil
	}
	return nil, errStubNoRows{}
}

func (s *stubNodeStore) List(_ context.Context) ([]*store.Node, error) {
	out := make([]*store.Node, 0, len(s.byID))
	for _, n := range s.byID {
		clone := *n
		out = append(out, &clone)
	}
	return out, nil
}

func (s *stubNodeStore) UpdateRegistration(_ context.Context, id, name, platform string, capabilities []string) error {
	n, ok := s.byID[id]
	if !ok || n.Trust == store.NodeTrustRevoked {
		return errStubNoRows{}
	}
	n.Name = name
	n.Platform = platform
	n.Capabilities = capabilities
	now := time.Now()
	n.LastSeenAt = &now
	return nil
}

func (s *stubNodeStore) TouchSeen(_ context.Context, id string) error { return nil }

func (s *stubNodeStore) SetTrust(_ context.Context, id, trust string) error {
	n, ok := s.byID[id]
	if !ok {
		return errStubNoRows{}
	}
	if err := store.ValidateNodeTrustTransition(n.Trust, trust); err != nil {
		return err
	}
	n.Trust = trust
	s.trustLog = append(s.trustLog, [2]string{id, trust})
	return nil
}

func (s *stubNodeStore) Revoke(_ context.Context, id string) error {
	n, ok := s.byID[id]
	if !ok {
		return errStubNoRows{}
	}
	n.Trust = store.NodeTrustRevoked
	s.revoked = append(s.revoked, id)
	return nil
}

type errStubNoRows struct{}

func (errStubNoRows) Error() string { return "no rows" }

// decodeResponse unmarshals the captured client response.
func decodeNodesResponse(t *testing.T, ch <-chan []byte) protocol.ResponseFrame {
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

func nodesReqFrame(t *testing.T, method string, params map[string]any) *protocol.RequestFrame {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return &protocol.RequestFrame{ID: "req-1", Method: method, Params: raw}
}

func seedNodeWithKey(t *testing.T, st *stubNodeStore, trust string) (*store.Node, string) {
	t.Helper()
	key, err := nodes.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	n := &store.Node{
		ID:          uuid.NewString(),
		Name:        "build-1",
		NodeKeyHash: nodes.HashKey(key),
		Trust:       trust,
	}
	if err := st.Create(context.Background(), n); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return n, key
}

func TestNodesRegisterBadKeyFormat(t *testing.T) {
	m := NewNodesMethods(newStubNodeStore(), nodes.NewRegistry(), nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "u", 2)
	m.handleRegister(context.Background(), client, nodesReqFrame(t, protocol.MethodNodesRegister, map[string]any{
		"nodeKey": "garbage",
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error == nil || resp.Error.Code != protocol.ErrInvalidRequest {
		t.Fatalf("err = %+v, want INVALID_REQUEST", resp.Error)
	}
}

func TestNodesRegisterUnknownKey(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "u", 2)
	other, _ := nodes.GenerateKey()
	m.handleRegister(context.Background(), client, nodesReqFrame(t, protocol.MethodNodesRegister, map[string]any{
		"nodeKey": other,
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("err = %+v, want UNAUTHORIZED", resp.Error)
	}
}

func TestNodesRegisterRevokedKey(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	_, key := seedNodeWithKey(t, st, store.NodeTrustRevoked)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "u", 2)
	m.handleRegister(context.Background(), client, nodesReqFrame(t, protocol.MethodNodesRegister, map[string]any{
		"nodeKey": key,
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error == nil || resp.Error.Code != protocol.ErrFailedPrecondition {
		t.Fatalf("err = %+v, want FAILED_PRECONDITION", resp.Error)
	}
}

func TestNodesRegisterSuccessMarksOnline(t *testing.T) {
	st := newStubNodeStore()
	reg := nodes.NewRegistry()
	m := NewNodesMethods(st, reg, nil)
	node, key := seedNodeWithKey(t, st, store.NodeTrustPending)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "u", 2)
	m.handleRegister(context.Background(), client, nodesReqFrame(t, protocol.MethodNodesRegister, map[string]any{
		"nodeKey":      key,
		"platform":     "linux/amd64",
		"capabilities": []string{"exec", "bogus-cap"},
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	data, _ := resp.Payload.(map[string]any)
	if data["nodeId"] != node.ID {
		t.Fatalf("nodeId = %v, want %v", data["nodeId"], node.ID)
	}
	if data["trust"] != store.NodeTrustPending {
		t.Fatalf("trust = %v, want pending", data["trust"])
	}
	// Unknown capabilities are filtered.
	caps, _ := data["capabilities"].([]any)
	if len(caps) != 1 || caps[0] != store.NodeCapabilityExec {
		t.Fatalf("capabilities = %v, want [exec]", caps)
	}
	if !reg.Online(node.ID) {
		t.Fatal("node should be online after registration")
	}
}

func TestNodesListCreateKeyAdminGate(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	tenantID := uuid.Must(uuid.NewV7())
	client, ch := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "u", 2)
	m.handleList(store.WithTenantID(context.Background(), tenantID), client, nodesReqFrame(t, protocol.MethodNodesList, map[string]any{
		"create": true,
		"name":   "build-9",
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("err = %+v, want UNAUTHORIZED for operator", resp.Error)
	}
}

func TestNodesListCreateKeySuccess(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	tenantID := uuid.Must(uuid.NewV7())
	ctx := store.WithTenantID(context.Background(), tenantID)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, tenantID, "u", 2)

	m.handleList(ctx, client, nodesReqFrame(t, protocol.MethodNodesList, map[string]any{
		"create": true,
		"name":   "build-9",
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	data, _ := resp.Payload.(map[string]any)
	key, _ := data["key"].(string)
	if !nodes.ValidateKeyFormat(key) {
		t.Fatalf("key %q fails format validation", key)
	}
	nodeData, _ := data["node"].(map[string]any)
	if nodeData["trust"] != store.NodeTrustPending {
		t.Fatalf("new node trust = %v, want pending", nodeData["trust"])
	}
	if len(st.created) != 1 {
		t.Fatalf("created = %d nodes, want 1", len(st.created))
	}
	// The stored hash must match the revealed key, and the key must not be stored.
	if st.created[0].NodeKeyHash != nodes.HashKey(key) {
		t.Fatal("stored hash does not match revealed key")
	}
	if strings.Contains(strings.ToLower(key), st.created[0].NodeKeyHash[:8]) {
		t.Fatal("key must not embed the hash")
	}
}

func TestNodesListCreateKeyRequiresName(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleOwner, store.MasterTenantID, "u", 2)
	m.handleList(context.Background(), client, nodesReqFrame(t, protocol.MethodNodesList, map[string]any{
		"create": true,
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error == nil || resp.Error.Code != protocol.ErrInvalidRequest {
		t.Fatalf("err = %+v, want INVALID_REQUEST", resp.Error)
	}
}

func TestNodesSetTrustLifecycle(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	node, _ := seedNodeWithKey(t, st, store.NodeTrustPending)
	tenantID := store.MasterTenantID
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, tenantID, "u", 4)
	ctx := store.WithTenantID(context.Background(), tenantID)

	// pending → trusted
	m.handleList(ctx, client, nodesReqFrame(t, protocol.MethodNodesList, map[string]any{
		"nodeId": node.ID, "trust": store.NodeTrustTrusted,
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error != nil {
		t.Fatalf("trust pending→trusted: %+v", resp.Error)
	}

	// trusted → revoked is invalid through SetTrust (use nodes.revoke)
	m.handleList(ctx, client, nodesReqFrame(t, protocol.MethodNodesList, map[string]any{
		"nodeId": node.ID, "trust": store.NodeTrustRevoked,
	}))
	resp = decodeNodesResponse(t, ch)
	if resp.Error == nil || resp.Error.Code != protocol.ErrInvalidRequest {
		t.Fatalf("err = %+v, want INVALID_REQUEST for trust=revoked", resp.Error)
	}
}

func TestNodesRevokeForceDisconnects(t *testing.T) {
	st := newStubNodeStore()
	reg := nodes.NewRegistry()
	m := NewNodesMethods(st, reg, nil)
	node, key := seedNodeWithKey(t, st, store.NodeTrustTrusted)

	// The daemon registers first.
	daemon, daemonCh := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "goclaw-node", 4)
	m.handleRegister(context.Background(), daemon, nodesReqFrame(t, protocol.MethodNodesRegister, map[string]any{
		"nodeKey": key,
	}))
	<-daemonCh // consume register response
	if !reg.Online(node.ID) {
		t.Fatal("daemon should be online pre-revoke")
	}

	admin, ch2 := gateway.NewCapturingTestClient(permissions.RoleOwner, store.MasterTenantID, "owner", 2)
	m.handleRevoke(context.Background(), admin, nodesReqFrame(t, protocol.MethodNodesRevoke, map[string]any{
		"nodeId": node.ID,
	}))
	resp := decodeNodesResponse(t, ch2)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if reg.Online(node.ID) {
		t.Fatal("registry entry should be gone after revoke")
	}
	if len(st.revoked) != 1 || st.revoked[0] != node.ID {
		t.Fatalf("revoked log = %v", st.revoked)
	}
	// The force-disconnected connection is dead (its send channel was closed
	// by Client.Close, mirroring a torn-down WS). A real daemon reconnects
	// with a fresh connection: re-register with the same key must be rejected.
	reconnecting, reconnectCh := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "goclaw-node", 2)
	m.handleRegister(context.Background(), reconnecting, nodesReqFrame(t, protocol.MethodNodesRegister, map[string]any{
		"nodeKey": key,
	}))
	resp = decodeNodesResponse(t, reconnectCh)
	if resp.Error == nil || resp.Error.Code != protocol.ErrFailedPrecondition {
		t.Fatalf("re-register err = %+v, want FAILED_PRECONDITION", resp.Error)
	}
	if reg.Online(node.ID) {
		t.Fatal("revoked node must not come back online")
	}
}

func TestNodesRevokeRequiresAdmin(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	node, _ := seedNodeWithKey(t, st, store.NodeTrustTrusted)
	operator, ch := gateway.NewCapturingTestClient(permissions.RoleOperator, store.MasterTenantID, "u", 2)
	m.handleRevoke(context.Background(), operator, nodesReqFrame(t, protocol.MethodNodesRevoke, map[string]any{
		"nodeId": node.ID,
	}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("err = %+v, want UNAUTHORIZED", resp.Error)
	}
	if len(st.revoked) != 0 {
		t.Fatal("operator must not be able to revoke")
	}
}

func TestNodesResultRequiresRegisteredSender(t *testing.T) {
	st := newStubNodeStore()
	reg := nodes.NewRegistry()
	m := NewNodesMethods(st, reg, nil)
	node, key := seedNodeWithKey(t, st, store.NodeTrustTrusted)

	daemon, dch := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "goclaw-node", 2)
	m.handleRegister(context.Background(), daemon, nodesReqFrame(t, protocol.MethodNodesRegister, map[string]any{
		"nodeKey": key,
	}))
	<-dch

	// A different client impersonating the node is rejected.
	impostor, ich := gateway.NewCapturingTestClient(permissions.RoleAdmin, store.MasterTenantID, "u", 2)
	m.handleResult(context.Background(), impostor, nodesReqFrame(t, protocol.MethodNodesResult, map[string]any{
		"nodeId": node.ID, "invokeId": "inv-1", "exitCode": 0,
	}))
	resp := decodeNodesResponse(t, ich)
	if resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("err = %+v, want UNAUTHORIZED", resp.Error)
	}

	// The registered daemon is accepted (no UNAUTHORIZED). delivered=false is
	// correct here: no agent is waiting on this correlation id, so the result
	// has nowhere to go — the handler still answers OK.
	m.handleResult(context.Background(), daemon, nodesReqFrame(t, protocol.MethodNodesResult, map[string]any{
		"nodeId": node.ID, "invokeId": "inv-1", "exitCode": 0, "stdout": "hi",
	}))
	resp = decodeNodesResponse(t, dch)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestNodesListDefaultTenantScoped(t *testing.T) {
	st := newStubNodeStore()
	m := NewNodesMethods(st, nodes.NewRegistry(), nil)
	seedNodeWithKey(t, st, store.NodeTrustPending)
	seedNodeWithKey(t, st, store.NodeTrustTrusted)

	client, ch := gateway.NewCapturingTestClient(permissions.RoleViewer, store.MasterTenantID, "u", 2)
	m.handleList(store.WithTenantID(context.Background(), store.MasterTenantID), client,
		nodesReqFrame(t, protocol.MethodNodesList, map[string]any{}))
	resp := decodeNodesResponse(t, ch)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	data, _ := resp.Payload.(map[string]any)
	items, _ := data["nodes"].([]any)
	if len(items) != 2 {
		t.Fatalf("nodes = %d, want 2", len(items))
	}
	first, _ := items[0].(map[string]any)
	if _, hasHash := first["nodeKeyHash"]; hasHash {
		t.Fatal("list must not expose the key hash")
	}
}
