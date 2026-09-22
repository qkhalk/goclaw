package methods

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ---- stubs ----

// stubSubagentAgentStore embeds store.AgentStore and overrides only what the
// subagents.* handlers touch (resolve + accessibility checks).
type stubSubagentAgentStore struct {
	store.AgentStore
	agents     []*store.AgentData
	accessible []store.AgentData // returned by ListAccessible for any user
	accessErr  error
}

func (s *stubSubagentAgentStore) GetByKey(_ context.Context, key string) (*store.AgentData, error) {
	for _, a := range s.agents {
		if a.AgentKey == key {
			return a, nil
		}
	}
	return nil, nil
}

func (s *stubSubagentAgentStore) GetByID(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	for _, a := range s.agents {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, nil
}

func (s *stubSubagentAgentStore) ListAccessible(_ context.Context, _ string) ([]store.AgentData, error) {
	if s.accessErr != nil {
		return nil, s.accessErr
	}
	return s.accessible, nil
}

// stubSubagentTaskStore implements store.SubagentTaskStore for handler tests.
type stubSubagentTaskStore struct {
	rows        map[uuid.UUID]*store.SubagentTaskData
	listed      []store.SubagentTaskData
	listOpts    []listCall
	archiveByID []uuid.UUID
	archiveErr  error
	batchRoots  []uuid.UUID
	batchN      int64
	statusCalls []statusCall
}

type listCall struct {
	root    uuid.UUID
	status  string
	withArc bool
}

type statusCall struct {
	root   uuid.UUID
	id     uuid.UUID
	status string
	result *string
}

func (s *stubSubagentTaskStore) Create(_ context.Context, _ *store.SubagentTaskData) error {
	return nil
}

func (s *stubSubagentTaskStore) Get(_ context.Context, root, id uuid.UUID) (*store.SubagentTaskData, error) {
	t, ok := s.rows[id]
	if !ok || t.RootAgentID != root {
		return nil, nil
	}
	return t, nil
}

func (s *stubSubagentTaskStore) GetByID(_ context.Context, id uuid.UUID) (*store.SubagentTaskData, error) {
	if t, ok := s.rows[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (s *stubSubagentTaskStore) UpdateStatus(_ context.Context, root, id uuid.UUID, status string, result *string, _ int, _, _ int64) error {
	s.statusCalls = append(s.statusCalls, statusCall{root: root, id: id, status: status, result: result})
	return nil
}

func (s *stubSubagentTaskStore) ListByParent(_ context.Context, root uuid.UUID, status string, includeArchived bool) ([]store.SubagentTaskData, error) {
	s.listOpts = append(s.listOpts, listCall{root: root, status: status, withArc: includeArchived})
	return s.listed, nil
}

func (s *stubSubagentTaskStore) ListBySession(_ context.Context, _ uuid.UUID, _ string) ([]store.SubagentTaskData, error) {
	return nil, nil
}

func (s *stubSubagentTaskStore) Archive(_ context.Context, _ uuid.UUID, _ time.Duration, _ int) (int64, error) {
	return 0, nil
}

func (s *stubSubagentTaskStore) ArchiveByID(_ context.Context, id uuid.UUID) error {
	s.archiveByID = append(s.archiveByID, id)
	return s.archiveErr
}

func (s *stubSubagentTaskStore) ArchiveCompletedForParent(_ context.Context, root uuid.UUID) (int64, error) {
	s.batchRoots = append(s.batchRoots, root)
	return s.batchN, nil
}

func (s *stubSubagentTaskStore) UpdateMetadata(_ context.Context, _, _ uuid.UUID, _ map[string]any) error {
	return nil
}

// ---- helpers ----

func subagentTestAgent(ownerID string) *store.AgentData {
	return &store.AgentData{
		BaseModel: store.BaseModel{ID: uuid.Must(uuid.NewV7())},
		AgentKey:  "researcher",
		AgentType: store.AgentTypePredefined,
		Status:    store.AgentStatusActive,
		OwnerID:   ownerID,
	}
}

func subagentTestTask(root uuid.UUID, status, result string) *store.SubagentTaskData {
	model := "test-model"
	return &store.SubagentTaskData{
		BaseModel:   store.BaseModel{ID: uuid.Must(uuid.NewV7())},
		TenantID:    uuid.Must(uuid.NewV7()),
		RootAgentID: root,
		Subject:     "do research",
		Description: "do research task",
		Status:      status,
		Model:       &model,
		Result:      &result,
	}
}

func buildSubagentMethods(agentStore store.AgentStore, tasks store.SubagentTaskStore) *SubagentMethods {
	return NewSubagentMethods(&config.Config{}, tasks, agentStore)
}

// decodeSubagentRows re-decodes the JSON-roundtripped payload value into the
// typed wire rows (payload arrives from the wire as []any of maps).
func decodeSubagentRows(t *testing.T, v any) []subagentTaskJSON {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("re-marshal rows: %v", err)
	}
	var rows []subagentTaskJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("decode rows: %v", err)
	}
	return rows
}

func decodeSubagentDetail(t *testing.T, v any) subagentTaskDetailJSON {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("re-marshal detail: %v", err)
	}
	var row subagentTaskDetailJSON
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	return row
}

// ---- list ----

func TestSubagentsListHappyPath(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	done := "finished research"
	running := subagentTestTask(agent.ID, "running", "")
	completed := subagentTestTask(agent.ID, "completed", done)
	tasks := &stubSubagentTaskStore{listed: []store.SubagentTaskData{*running, *completed}}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleViewer, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleList(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsList, map[string]any{
		"agentId": "researcher",
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	data, ok := resp.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T", resp.Payload)
	}
	if len(tasks.listOpts) != 1 || tasks.listOpts[0].root != agent.ID || tasks.listOpts[0].withArc {
		t.Fatalf("ListByParent opts = %+v, want root=%s includeArchived=false", tasks.listOpts, agent.ID)
	}
	rows := decodeSubagentRows(t, data["tasks"])
	if len(rows) != 2 {
		t.Fatalf("tasks = %+v, want 2 rows", data["tasks"])
	}
	if rows[1].Status != "completed" || rows[1].Summary == nil || *rows[1].Summary != done {
		t.Fatalf("completed row = %+v, want summary %q", rows[1], done)
	}
	if rows[0].Status != "running" || rows[0].Summary != nil {
		t.Fatalf("running row = %+v, want no summary", rows[0])
	}
	if rows[1].TaskID != completed.ID.String() {
		t.Fatalf("taskId = %s, want %s", rows[1].TaskID, completed.ID)
	}
}

func TestSubagentsListSessionKeyResolvesAgent(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	tasks := &stubSubagentTaskStore{}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleAdmin, tenantID, "admin", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleList(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsList, map[string]any{
		"sessionKey": "agent:researcher:main",
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if len(tasks.listOpts) != 1 || tasks.listOpts[0].root != agent.ID {
		t.Fatalf("ListByParent opts = %+v, want agent resolved from session key", tasks.listOpts)
	}
}

func TestSubagentsListOwnershipDeniedForOtherUser(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}, accessible: nil}
	m := buildSubagentMethods(agentStore, &stubSubagentTaskStore{})

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleViewer, tenantID, "user-b", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleList(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsList, map[string]any{
		"agentId": "researcher",
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("error = %+v, want UNAUTHORIZED", resp.Error)
	}
}

func TestSubagentsListSharedAgentAllowed(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{
		agents:     []*store.AgentData{agent},
		accessible: []store.AgentData{*agent}, // granted via agent_shares
	}
	tasks := &stubSubagentTaskStore{}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleViewer, tenantID, "user-b", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleList(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsList, map[string]any{
		"agentId": agent.ID.String(), // UUID form also resolves
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if len(tasks.listOpts) != 1 {
		t.Fatal("ListByParent not called for shared agent")
	}
}

// ---- get ----

func TestSubagentsGetHappyPathAndNotFound(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	task := subagentTestTask(agent.ID, "completed", "done")
	tasks := &stubSubagentTaskStore{rows: map[uuid.UUID]*store.SubagentTaskData{task.ID: task}}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleAdmin, tenantID, "admin", 2)
	ctx := store.WithTenantID(context.Background(), tenantID)

	m.handleGet(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsGet, map[string]any{
		"taskId": task.ID.String(),
	}))
	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	data, _ := resp.Payload.(map[string]any)
	row := decodeSubagentDetail(t, data["task"])
	if row.TaskID != task.ID.String() || row.Description != "do research task" {
		t.Fatalf("task = %+v", data["task"])
	}

	m.handleGet(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsGet, map[string]any{
		"taskId": uuid.Must(uuid.NewV7()).String(),
	}))
	resp = readTimelineResponse(t, responses)
	if resp.Error == nil || resp.Error.Code != protocol.ErrNotFound {
		t.Fatalf("error = %+v, want NOT_FOUND", resp.Error)
	}
}

// ---- archive ----

func TestSubagentsArchiveRejectsNonTerminal(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	task := subagentTestTask(agent.ID, "running", "")
	tasks := &stubSubagentTaskStore{rows: map[uuid.UUID]*store.SubagentTaskData{task.ID: task}}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleArchive(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsArchive, map[string]any{
		"taskId": task.ID.String(),
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error == nil || resp.Error.Code != protocol.ErrInvalidRequest {
		t.Fatalf("error = %+v, want INVALID_REQUEST", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "running") || !strings.Contains(resp.Error.Message, "archived") {
		t.Fatalf("message = %q, want non-terminal explanation", resp.Error.Message)
	}
	if len(tasks.archiveByID) != 0 {
		t.Fatal("ArchiveByID must not be called for non-terminal task")
	}
}

func TestSubagentsArchiveHappyPath(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	task := subagentTestTask(agent.ID, "completed", "done")
	tasks := &stubSubagentTaskStore{rows: map[uuid.UUID]*store.SubagentTaskData{task.ID: task}}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleArchive(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsArchive, map[string]any{
		"taskId": task.ID.String(),
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if len(tasks.archiveByID) != 1 || tasks.archiveByID[0] != task.ID {
		t.Fatalf("ArchiveByID calls = %v", tasks.archiveByID)
	}
}

func TestSubagentsArchiveOwnershipDenied(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}, accessible: nil}
	task := subagentTestTask(agent.ID, "completed", "done")
	tasks := &stubSubagentTaskStore{rows: map[uuid.UUID]*store.SubagentTaskData{task.ID: task}}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "user-b", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleArchive(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsArchive, map[string]any{
		"taskId": task.ID.String(),
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("error = %+v, want UNAUTHORIZED", resp.Error)
	}
	if len(tasks.archiveByID) != 0 {
		t.Fatal("ArchiveByID must not be called for foreign user")
	}
}

func TestSubagentsArchiveViewerRoleDenied(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	tasks := &stubSubagentTaskStore{
		rows: map[uuid.UUID]*store.SubagentTaskData{}, // store would be reachable
	}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleViewer, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	// Exercise the operator gate exactly as Register applies it.
	m.requireOperator(m.handleArchive)(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsArchive, map[string]any{
		"taskId": uuid.Must(uuid.NewV7()).String(),
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error == nil || resp.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("error = %+v, want UNAUTHORIZED for viewer", resp.Error)
	}
}

func TestSubagentsArchiveCompletedBatch(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	tasks := &stubSubagentTaskStore{batchN: 7}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleArchiveCompleted(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsArchiveCompleted, map[string]any{
		"agentId": "researcher",
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if len(tasks.batchRoots) != 1 || tasks.batchRoots[0] != agent.ID {
		t.Fatalf("ArchiveCompletedForParent roots = %v", tasks.batchRoots)
	}
	data, _ := resp.Payload.(map[string]any)
	if data["archived"] != float64(7) {
		t.Fatalf("archived = %v, want 7", data["archived"])
	}
}

// ---- cancel ----

func TestSubagentsCancelRejectsTerminal(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	task := subagentTestTask(agent.ID, "completed", "done")
	tasks := &stubSubagentTaskStore{rows: map[uuid.UUID]*store.SubagentTaskData{task.ID: task}}
	m := buildSubagentMethods(agentStore, tasks)

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleCancel(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsCancel, map[string]any{
		"taskId": task.ID.String(),
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error == nil || resp.Error.Code != protocol.ErrInvalidRequest {
		t.Fatalf("error = %+v, want INVALID_REQUEST for terminal task", resp.Error)
	}
	if len(tasks.statusCalls) != 0 {
		t.Fatal("terminal cancel must not write status")
	}
}

func TestSubagentsCancelLiveRun(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	task := subagentTestTask(agent.ID, "running", "")
	tasks := &stubSubagentTaskStore{rows: map[uuid.UUID]*store.SubagentTaskData{task.ID: task}}
	m := buildSubagentMethods(agentStore, tasks)
	cancelled := false
	m.SetCancelFn(func(_ context.Context, got *store.SubagentTaskData) bool {
		cancelled = got.ID == task.ID
		return true
	})

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleCancel(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsCancel, map[string]any{
		"taskId": task.ID.String(),
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if !cancelled {
		t.Fatal("cancel fn not invoked")
	}
	if len(tasks.statusCalls) != 0 {
		t.Fatal("live cancel must not fall back to orphan UpdateStatus")
	}
	data, _ := resp.Payload.(map[string]any)
	if data["orphan"] != nil {
		t.Fatalf("orphan flag = %v, want absent for live cancel", data["orphan"])
	}
}

func TestSubagentsCancelOrphanMarksTaskCancelled(t *testing.T) {
	agent := subagentTestAgent("user-a")
	agentStore := &stubSubagentAgentStore{agents: []*store.AgentData{agent}}
	task := subagentTestTask(agent.ID, "running", "")
	tasks := &stubSubagentTaskStore{rows: map[uuid.UUID]*store.SubagentTaskData{task.ID: task}}
	m := buildSubagentMethods(agentStore, tasks)
	m.SetCancelFn(func(_ context.Context, _ *store.SubagentTaskData) bool { return false })

	tenantID := uuid.Must(uuid.NewV7())
	client, responses := gateway.NewCapturingTestClient(permissions.RoleOperator, tenantID, "user-a", 1)
	ctx := store.WithTenantID(context.Background(), tenantID)
	m.handleCancel(ctx, client, sessionReqFrame(t, protocol.MethodSubagentsCancel, map[string]any{
		"taskId": task.ID.String(),
	}))

	resp := readTimelineResponse(t, responses)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if len(tasks.statusCalls) != 1 {
		t.Fatalf("status calls = %d, want 1 orphan mark", len(tasks.statusCalls))
	}
	call := tasks.statusCalls[0]
	if call.id != task.ID || call.root != task.RootAgentID || call.status != "cancelled" ||
		call.result == nil || !strings.Contains(*call.result, "no live run handle") {
		t.Fatalf("orphan cancel call = %+v", call)
	}
	data, _ := resp.Payload.(map[string]any)
	if data["orphan"] != true {
		t.Fatalf("orphan = %v, want true", data["orphan"])
	}
}
