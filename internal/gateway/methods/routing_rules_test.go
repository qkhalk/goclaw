package methods

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ---- stub store ----

type stubRoutingRulesStore struct {
	rules      []*store.RoutingRule
	upserted   []*store.RoutingRule
	deletedIDs []string
	listErr    error
}

func (s *stubRoutingRulesStore) ListRules(_ context.Context, tenantID string) ([]*store.RoutingRule, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []*store.RoutingRule
	for _, r := range s.rules {
		if r.TenantID == tenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *stubRoutingRulesStore) UpsertRule(_ context.Context, rule *store.RoutingRule) error {
	if rule.ID != "" {
		for _, r := range s.rules {
			if r.ID == rule.ID && r.TenantID != rule.TenantID {
				return store.ErrRoutingRuleNotFound
			}
		}
	}
	cp := *rule
	s.upserted = append(s.upserted, &cp)
	return nil
}

func (s *stubRoutingRulesStore) DeleteRule(_ context.Context, tenantID, id string) error {
	for i, r := range s.rules {
		if r.ID == id {
			if r.TenantID != tenantID {
				return store.ErrRoutingRuleNotFound
			}
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			s.deletedIDs = append(s.deletedIDs, id)
			return nil
		}
	}
	return store.ErrRoutingRuleNotFound
}

// ---- harness ----

const (
	routingCallerTID  = "11111111-1111-1111-1111-111111111111"
	routingOtherTID   = "22222222-2222-2222-2222-222222222222"
	routingTargetUUID = "00000000-0000-0000-0000-0000000000aa"
)

func routingCallCtx(client *gateway.Client) context.Context {
	return store.WithTenantID(context.Background(), client.TenantID())
}

func routingRequest(t *testing.T, method string, payload map[string]any) *protocol.RequestFrame {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &protocol.RequestFrame{
		Type:   protocol.FrameTypeRequest,
		ID:     "routing-req-1",
		Method: method,
		Params: raw,
	}
}

func readRoutingResponse(t *testing.T, ch <-chan []byte) map[string]any {
	t.Helper()
	select {
	case raw := <-ch:
		var frame protocol.ResponseFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatalf("unmarshal response %s: %v", raw, err)
		}
		payload := map[string]any{}
		if frame.Payload != nil {
			if blob, err := json.Marshal(frame.Payload); err == nil {
				_ = json.Unmarshal(blob, &payload)
			}
		}
		if !frame.OK {
			payload["__error"] = frame.Error.Code
		}
		return payload
	default:
		t.Fatal("no response frame sent")
		return nil
	}
}

// ---- tests ----

func TestRoutingRulesList_TenantAdminAllowed(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	stub := &stubRoutingRulesStore{rules: []*store.RoutingRule{{
		ID:            uuid.NewString(),
		TenantID:      callerTID.String(),
		TargetAgentID: routingTargetUUID,
		Enabled:       true,
	}}}
	m := NewRoutingRulesMethods(stub, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, callerTID, "user-1", 4)

	m.handleList(routingCallCtx(client), client, routingRequest(t, protocol.MethodRoutingRulesList, nil))

	res := readRoutingResponse(t, ch)
	rules, ok := res["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("rules = %#v, want 1 entry", res["rules"])
	}
}

func TestRoutingRulesList_CrossTenantAdminDenied(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	otherTID := uuid.MustParse(routingOtherTID)
	// Rules live in another tenant; the stub filters by tenant anyway, but the
	// key assertion is that a caller whose ctx tenant ≠ own tenant is denied.
	stub := &stubRoutingRulesStore{rules: []*store.RoutingRule{{
		ID:            uuid.NewString(),
		TenantID:      otherTID.String(),
		TargetAgentID: routingTargetUUID,
	}}}
	m := NewRoutingRulesMethods(stub, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, otherTID, "user-1", 4)

	// Ctx tenant (other tenant) ≠ caller's own tenant → denied.
	ctx := store.WithTenantID(context.Background(), callerTID)
	m.handleList(ctx, client, routingRequest(t, protocol.MethodRoutingRulesList, nil))

	res := readRoutingResponse(t, ch)
	if _, hasRules := res["rules"]; hasRules {
		t.Fatalf("cross-tenant admin must be denied, got %#v", res)
	}
}

func TestRoutingRulesList_OperatorDenied(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	m := NewRoutingRulesMethods(&stubRoutingRulesStore{}, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleOperator, callerTID, "user-1", 4)

	m.handleList(routingCallCtx(client), client, routingRequest(t, protocol.MethodRoutingRulesList, nil))

	res := readRoutingResponse(t, ch)
	if _, hasRules := res["rules"]; hasRules {
		t.Fatalf("operator must be denied, got %#v", res)
	}
}

func TestRoutingRulesSet_ValidatesTargetAgent(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	stub := &stubRoutingRulesStore{}
	m := NewRoutingRulesMethods(stub, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, callerTID, "user-1", 4)

	// Missing targetAgentId → invalid request.
	m.handleSet(routingCallCtx(client), client, routingRequest(t, protocol.MethodRoutingRulesSet, map[string]any{
		"priority": 5,
	}))
	res := readRoutingResponse(t, ch)
	if res["rule"] != nil {
		t.Fatalf("set without targetAgentId must be rejected, got %#v", res)
	}

	// Malformed targetAgentId → invalid request.
	m.handleSet(routingCallCtx(client), client, routingRequest(t, protocol.MethodRoutingRulesSet, map[string]any{
		"targetAgentId": "not-a-uuid",
	}))
	res = readRoutingResponse(t, ch)
	if res["rule"] != nil {
		t.Fatalf("set with malformed targetAgentId must be rejected, got %#v", res)
	}
	if len(stub.upserted) != 0 {
		t.Fatalf("store must not be called for invalid payloads, got %d", len(stub.upserted))
	}
}

func TestRoutingRulesSet_UpsertsRule(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	stub := &stubRoutingRulesStore{}
	m := NewRoutingRulesMethods(stub, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, callerTID, "user-1", 4)

	enabled := false
	m.handleSet(routingCallCtx(client), client, routingRequest(t, protocol.MethodRoutingRulesSet, map[string]any{
		"priority":      7,
		"targetAgentId": routingTargetUUID,
		"enabled":       enabled,
		"match": map[string]any{
			"channel":  "telegram-bot",
			"peerKind": "direct",
			"peerId":   "chat-42",
		},
	}))

	res := readRoutingResponse(t, ch)
	if _, hasRule := res["rule"]; !hasRule {
		t.Fatalf("expected rule in response, got %#v", res)
	}
	if len(stub.upserted) != 1 {
		t.Fatalf("upsert calls = %d, want 1", len(stub.upserted))
	}
	rule := stub.upserted[0]
	if rule.Priority != 7 || rule.Enabled || rule.TargetAgentID != routingTargetUUID {
		t.Fatalf("stored rule = %+v", rule)
	}
	if rule.TenantID != callerTID.String() {
		t.Fatalf("rule must be tenant-scoped to caller, got %q", rule.TenantID)
	}
	if rule.Match.Channel == nil || *rule.Match.Channel != "telegram-bot" ||
		rule.Match.PeerID == nil || *rule.Match.PeerID != "chat-42" ||
		rule.Match.PeerKind == nil || *rule.Match.PeerKind != "direct" {
		t.Fatalf("match = %+v", rule.Match)
	}
}

func TestRoutingRulesSet_CrossTenantUpdateDenied(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	otherTID := uuid.MustParse(routingOtherTID)
	stub := &stubRoutingRulesStore{rules: []*store.RoutingRule{{
		ID:            "99999999-9999-9999-9999-999999999999",
		TenantID:      otherTID.String(),
		TargetAgentID: routingTargetUUID,
	}}}
	m := NewRoutingRulesMethods(stub, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, callerTID, "user-1", 4)

	// Caller scopes its own tenant but targets another tenant's rule ID.
	// The store layer enforces the boundary; the handler must surface it.
	ctx := store.WithTenantID(context.Background(), callerTID)
	m.handleSet(ctx, client, routingRequest(t, protocol.MethodRoutingRulesSet, map[string]any{
		"id":            "99999999-9999-9999-9999-999999999999",
		"targetAgentId": routingTargetUUID,
		"tenantId":      otherTID.String(),
	}))

	readRoutingResponse(t, ch) // handler must still respond (error or ok); no panic/hang
	if len(stub.upserted) != 0 {
		t.Fatalf("cross-tenant update must not reach upsert path, got %d", len(stub.upserted))
	}
}

func TestRoutingRulesDelete_NotFound(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	stub := &stubRoutingRulesStore{}
	m := NewRoutingRulesMethods(stub, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, callerTID, "user-1", 4)

	m.handleDelete(routingCallCtx(client), client, routingRequest(t, protocol.MethodRoutingRulesDelete, map[string]any{
		"id": uuid.NewString(),
	}))
	res := readRoutingResponse(t, ch)
	if res["ok"] == "true" {
		t.Fatal("deleting an unknown rule must not report ok")
	}
}

func TestRoutingRulesDelete_Success(t *testing.T) {
	callerTID := uuid.MustParse(routingCallerTID)
	ruleID := uuid.NewString()
	stub := &stubRoutingRulesStore{rules: []*store.RoutingRule{{
		ID:            ruleID,
		TenantID:      callerTID.String(),
		TargetAgentID: routingTargetUUID,
	}}}
	m := NewRoutingRulesMethods(stub, nil)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, callerTID, "user-1", 4)

	m.handleDelete(routingCallCtx(client), client, routingRequest(t, protocol.MethodRoutingRulesDelete, map[string]any{
		"id": ruleID,
	}))
	res := readRoutingResponse(t, ch)
	if res["ok"] != "true" {
		t.Fatalf("delete = %#v, want ok", res)
	}
	if len(stub.deletedIDs) != 1 || stub.deletedIDs[0] != ruleID {
		t.Fatalf("deletedIDs = %v", stub.deletedIDs)
	}
}
