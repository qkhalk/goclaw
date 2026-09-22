package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// --- fakes ---

// fakeSubagentTaskStore is an in-memory store.SubagentTaskStore implementing
// exactly the archive/list semantics of the PG store (terminal-only archive,
// idempotent ArchiveByID, archived filtered from ListByParent by default).
type fakeSubagentTaskStore struct {
	tasks        map[uuid.UUID]*store.SubagentTaskData
	archived     []uuid.UUID // IDs archived via ArchiveByID / ArchiveCompletedForParent
	archiveAllOf []uuid.UUID // root agent IDs passed to ArchiveCompletedForParent
	listErr      error
}

func newFakeSubagentTaskStore() *fakeSubagentTaskStore {
	return &fakeSubagentTaskStore{tasks: map[uuid.UUID]*store.SubagentTaskData{}}
}

func (f *fakeSubagentTaskStore) addTask(rootAgentID uuid.UUID, status, subject string) *store.SubagentTaskData {
	t := &store.SubagentTaskData{
		BaseModel:   store.BaseModel{ID: store.GenNewID()},
		RootAgentID: rootAgentID,
		Subject:     subject,
		Status:      status,
	}
	f.tasks[t.ID] = t
	return t
}

func (f *fakeSubagentTaskStore) Create(_ context.Context, task *store.SubagentTaskData) error {
	f.tasks[task.ID] = task
	return nil
}

func (f *fakeSubagentTaskStore) Get(_ context.Context, rootAgentID, id uuid.UUID) (*store.SubagentTaskData, error) {
	t, ok := f.tasks[id]
	if !ok || t.RootAgentID != rootAgentID {
		return nil, store.ErrSubagentTaskNotFound
	}
	return t, nil
}

func (f *fakeSubagentTaskStore) UpdateStatus(_ context.Context, _, _ uuid.UUID, status string, result *string, iterations int, inputTokens, outputTokens int64) error {
	_ = result
	_ = iterations
	_ = inputTokens
	_ = outputTokens
	for _, t := range f.tasks {
		t.Status = status
		return nil
	}
	return nil
}

func (f *fakeSubagentTaskStore) ListByParent(_ context.Context, rootAgentID uuid.UUID, statusFilter string, includeArchived bool) ([]store.SubagentTaskData, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []store.SubagentTaskData
	for _, t := range f.tasks {
		if t.RootAgentID != rootAgentID {
			continue
		}
		if t.ArchivedAt != nil && !includeArchived {
			continue
		}
		if statusFilter != "" && t.Status != statusFilter {
			continue
		}
		out = append(out, *t)
	}
	return out, nil
}

func (f *fakeSubagentTaskStore) ListBySession(context.Context, uuid.UUID, string) ([]store.SubagentTaskData, error) {
	return nil, nil
}

// GetByID mirrors the PG contract: (nil, nil) when the task is not addressable.
func (f *fakeSubagentTaskStore) GetByID(_ context.Context, taskID uuid.UUID) (*store.SubagentTaskData, error) {
	return f.tasks[taskID], nil
}

func (f *fakeSubagentTaskStore) Archive(context.Context, uuid.UUID, time.Duration, int) (int64, error) {
	return 0, nil
}

func (f *fakeSubagentTaskStore) ArchiveByID(_ context.Context, taskID uuid.UUID) error {
	t, ok := f.tasks[taskID]
	if !ok {
		return store.ErrSubagentTaskNotFound
	}
	if !store.IsTerminalSubagentTaskStatus(t.Status) {
		return store.ErrSubagentTaskNotTerminal
	}
	if t.ArchivedAt == nil {
		now := time.Now()
		t.ArchivedAt = &now
		f.archived = append(f.archived, taskID)
	}
	return nil
}

func (f *fakeSubagentTaskStore) ArchiveCompletedForParent(_ context.Context, rootAgentID uuid.UUID) (int64, error) {
	f.archiveAllOf = append(f.archiveAllOf, rootAgentID)
	var n int64
	now := time.Now()
	for _, t := range f.tasks {
		if t.RootAgentID != rootAgentID || t.ArchivedAt != nil {
			continue
		}
		if !store.IsTerminalSubagentTaskStatus(t.Status) {
			continue
		}
		t.ArchivedAt = &now
		f.archived = append(f.archived, t.ID)
		n++
	}
	return n, nil
}

func (f *fakeSubagentTaskStore) UpdateMetadata(context.Context, uuid.UUID, uuid.UUID, map[string]any) error {
	return nil
}

// fakeArchivePermStore is a permissive-by-config store.ConfigPermissionStore.
type fakeArchivePermStore struct {
	allow bool
}

func (f *fakeArchivePermStore) CheckPermission(context.Context, uuid.UUID, string, string, string) (bool, error) {
	return f.allow, nil
}

func (f *fakeArchivePermStore) Grant(context.Context, *store.ConfigPermission) error { return nil }

func (f *fakeArchivePermStore) Revoke(context.Context, uuid.UUID, string, string, string) error {
	return nil
}

func (f *fakeArchivePermStore) List(context.Context, uuid.UUID, string, string) ([]store.ConfigPermission, error) {
	return nil, nil
}

func (f *fakeArchivePermStore) ListFileWriters(context.Context, uuid.UUID, string) ([]store.ConfigPermission, error) {
	return nil, nil
}

// --- channel wiring ---

// archiveFailingEditCaller records calls like recordingTelegramCaller but
// fails editMessageText — the "message too old to edit" fallback path.
type archiveFailingEditCaller struct {
	calls []recordedTelegramCall
}

func (c *archiveFailingEditCaller) Call(_ context.Context, url string, data *ta.RequestData) (*ta.Response, error) {
	method := url[strings.LastIndex(url, "/")+1:]
	body := map[string]any{}
	if len(data.BodyRaw) > 0 {
		_ = json.Unmarshal(data.BodyRaw, &body)
	}
	c.calls = append(c.calls, recordedTelegramCall{method: method, body: body})
	if method == "editMessageText" {
		return nil, fmt.Errorf("Bad Request: message can't be edited")
	}
	result := json.RawMessage(`{"message_id":101,"date":0,"chat":{"id":123,"type":"private"}}`)
	return &ta.Response{Ok: true, Result: result}, nil
}

func (c *archiveFailingEditCaller) countCalls(method string) int {
	n := 0
	for _, call := range c.calls {
		if call.method == method {
			n++
		}
	}
	return n
}

// newArchiveTestChannel wires a Channel with the fake subagent task store and
// (optionally) a permission store for group writer gating.
func newArchiveTestChannel(t *testing.T, taskStore store.SubagentTaskStore, perm store.ConfigPermissionStore) (*Channel, *recordingTelegramCaller) {
	t.Helper()
	caller := &recordingTelegramCaller{}
	bot, err := telego.NewBot(
		"123456:abcdefghijklmnopqrstuvwxyzABCDE1234",
		telego.WithAPICaller(caller),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	// Avoid a typed-nil interface: nil must stay nil so the handler's
	// "not available" guard fires.
	var permStore store.ConfigPermissionStore
	if perm != nil {
		permStore = perm
	}
	ch := &Channel{
		BaseChannel:       channels.NewBaseChannel("telegram", nil, nil),
		bot:               bot,
		subagentTaskStore: taskStore,
		configPermStore:   permStore,
	}
	ch.SetName("telegram")
	ch.SetRunning(true)
	return ch, caller
}

// --- helpers ---

func countTelegramCalls(c *recordingTelegramCaller, method string) int {
	n := 0
	for _, call := range c.calls {
		if call.method == method {
			n++
		}
	}
	return n
}

func lastTelegramCall(t *testing.T, c *recordingTelegramCaller, method string) recordedTelegramCall {
	t.Helper()
	for i := len(c.calls) - 1; i >= 0; i-- {
		if c.calls[i].method == method {
			return c.calls[i]
		}
	}
	t.Fatalf("call %s not found, methods: %v", method, c.methodNames())
	return recordedTelegramCall{}
}

// keyboardRows decodes reply_markup.inline_keyboard from a recorded body.
func keyboardRows(t *testing.T, body map[string]any) []map[string]string {
	t.Helper()
	markup, _ := body["reply_markup"].(map[string]any)
	if markup == nil {
		t.Fatalf("reply_markup missing from body: %v", body)
	}
	rawRows, _ := markup["inline_keyboard"].([]any)
	rows := make([]map[string]string, 0, len(rawRows))
	for _, rawRow := range rawRows {
		rawBtns, _ := rawRow.([]any)
		row := map[string]string{}
		for i, rawBtn := range rawBtns {
			btn, _ := rawBtn.(map[string]any)
			text, _ := btn["text"].(string)
			data, _ := btn["callback_data"].(string)
			row[fmt.Sprintf("%d", i)] = text + "\x00" + data
		}
		rows = append(rows, row)
	}
	return rows
}

// rowButtons flattens a keyboard row into "text\x00data" entries ordered by index.
func rowButtons(row map[string]string) []string {
	out := make([]string, 0, len(row))
	for i := 0; ; i++ {
		v, ok := row[fmt.Sprintf("%d", i)]
		if !ok {
			break
		}
		out = append(out, v)
	}
	return out
}

func splitButton(entry string) (text, data string) {
	text, data, _ = strings.Cut(entry, "\x00")
	return text, data
}

// findRowByCallback returns the first keyboard row whose button idx matches prefix.
func findRowByCallback(rows []map[string]string, prefix string) (map[string]string, bool) {
	for _, row := range rows {
		for _, entry := range rowButtons(row) {
			if _, data := splitButton(entry); strings.HasPrefix(data, prefix) {
				return row, true
			}
		}
	}
	return nil, false
}

// --- /subagents list keyboard ---

func TestSubagentsList_ArchiveButtonsAndFooter(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	running := tasks.addTask(root, "running", "Explore repo")
	done := tasks.addTask(root, "completed", "Write docs")
	failed := tasks.addTask(root, "failed", "Broken job")

	ch, caller := newArchiveTestChannel(t, tasks, nil)
	ch.SetAgentID(root.String())

	ch.handleSubagentsList(context.Background(), -100, false, func(*telego.SendMessageParams) {})

	send := lastTelegramCall(t, caller, "sendMessage")
	rows := keyboardRows(t, send.body)
	if len(rows) != 4 {
		t.Fatalf("keyboard rows = %d, want 4 (3 tasks + archive-all footer)", len(rows))
	}

	// Running row: detail button only, status icon vocabulary unchanged.
	runningRow, ok := findRowByCallback(rows, "sa:"+running.ID.String())
	if !ok {
		t.Fatalf("running task row missing, rows: %v", rows)
	}
	if got := len(rowButtons(runningRow)); got != 1 {
		t.Errorf("running row buttons = %d, want 1 (no archive button)", got)
	}
	if text, _ := splitButton(rowButtons(runningRow)[0]); !strings.Contains(text, "🔄") {
		t.Errorf("running row label = %q, want 🔄 icon", text)
	}

	// Terminal rows: detail + 🗄 archive button alongside.
	for _, tc := range []struct {
		task *store.SubagentTaskData
		icon string
	}{{done, "✅"}, {failed, "❌"}} {
		row, ok := findRowByCallback(rows, "sa:"+tc.task.ID.String())
		if !ok {
			t.Fatalf("task %s row missing", tc.task.Subject)
		}
		buttons := rowButtons(row)
		if len(buttons) != 2 {
			t.Fatalf("terminal row buttons = %d, want 2 (detail + archive)", len(buttons))
		}
		text, data := splitButton(buttons[1])
		if data != "ar:"+tc.task.ID.String() {
			t.Errorf("archive callback = %q, want ar:%s", data, tc.task.ID)
		}
		if !strings.Contains(text, "🗄") {
			t.Errorf("archive button text = %q, want 🗄", text)
		}
		if dt, _ := splitButton(buttons[0]); !strings.Contains(dt, tc.icon) {
			t.Errorf("detail label = %q, want %s icon", dt, tc.icon)
		}
	}

	// Footer: archive-all with the terminal count (2), only terminal tasks count.
	footer, ok := findRowByCallback(rows, "ar:all:")
	if !ok {
		t.Fatalf("archive-all footer missing, rows: %v", rows)
	}
	footerBtns := rowButtons(footer)
	if len(footerBtns) != 1 {
		t.Fatalf("footer buttons = %d, want 1", len(footerBtns))
	}
	text, data := splitButton(footerBtns[0])
	if data != "ar:all:"+root.String() {
		t.Errorf("footer callback = %q, want ar:all:%s", data, root)
	}
	if !strings.Contains(text, "(2)") {
		t.Errorf("footer text = %q, want terminal count (2)", text)
	}

	// Callback budget: Telegram allows 64 bytes.
	for _, row := range rows {
		for _, entry := range rowButtons(row) {
			if _, d := splitButton(entry); len(d) > 64 {
				t.Errorf("callback data too long (%d bytes): %q", len(d), d)
			}
		}
	}
}

func TestSubagentsList_NoTerminalTasks_NoArchiveButtons(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	tasks.addTask(root, "running", "Explore repo")
	tasks.addTask(root, "queued", "Wait turn")

	ch, caller := newArchiveTestChannel(t, tasks, nil)
	ch.SetAgentID(root.String())

	ch.handleSubagentsList(context.Background(), -100, false, func(*telego.SendMessageParams) {})

	send := lastTelegramCall(t, caller, "sendMessage")
	rows := keyboardRows(t, send.body)
	if len(rows) != 2 {
		t.Fatalf("keyboard rows = %d, want 2 (detail rows only)", len(rows))
	}
	for _, row := range rows {
		for _, entry := range rowButtons(row) {
			if _, data := splitButton(entry); strings.HasPrefix(data, "ar:") {
				t.Errorf("unexpected archive callback %q without terminal tasks", data)
			}
		}
	}
}

// --- archive callback ---

// Route + happy path: pressed through handleCallbackQuery to prove the ar:
// prefix routes before the dispatcher's generic answer.
func TestHandleCallbackQuery_ArPrefixArchivesAndRerenders(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	done := tasks.addTask(root, "completed", "Write docs")
	tasks.addTask(root, "running", "Explore repo")

	ch, caller := newArchiveTestChannel(t, tasks, nil)
	ch.SetAgentID(root.String())
	ch.SetTenantID(uuid.New())

	ch.handleCallbackQuery(context.Background(),
		testCallbackQuery("ar:"+done.ID.String(), -100, 101, "en"))

	if len(tasks.archived) != 1 || tasks.archived[0] != done.ID {
		t.Fatalf("archived = %v, want [%s]", tasks.archived, done.ID)
	}
	// Exactly one answer — the handler owns it for ar: callbacks.
	if n := countTelegramCalls(caller, "answerCallbackQuery"); n != 1 {
		t.Errorf("answerCallbackQuery calls = %d, want 1", n)
	}
	edit := lastTelegramCall(t, caller, "editMessageText")
	text, _ := edit.body["text"].(string)
	if !strings.Contains(text, "Task archived") {
		t.Errorf("edited text missing confirmation:\n%s", text)
	}
	if strings.Contains(text, "Write docs") {
		t.Errorf("edited list still contains archived task:\n%s", text)
	}
	if !strings.Contains(text, "Explore repo") {
		t.Errorf("edited list lost running task:\n%s", text)
	}
}

func TestHandleArchiveCallback_RunningTaskRejectedWithToast(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	running := tasks.addTask(root, "running", "Explore repo")

	ch, caller := newArchiveTestChannel(t, tasks, nil)
	ch.SetAgentID(root.String())

	// Vietnamese client language → localized rejection toast.
	ch.handleArchiveCallback(context.Background(),
		testCallbackQuery("ar:"+running.ID.String(), -100, 101, "vi"))

	if len(tasks.archived) != 0 {
		t.Fatalf("running task must not be archived, got %v", tasks.archived)
	}
	if n := countTelegramCalls(caller, "editMessageText"); n != 0 {
		t.Errorf("editMessageText calls = %d, want 0 (list untouched)", n)
	}
	answer := lastTelegramCall(t, caller, "answerCallbackQuery")
	text, _ := answer.body["text"].(string)
	if !strings.Contains(text, "vẫn đang chạy") {
		t.Errorf("toast = %q, want Vietnamese not-terminal message", text)
	}
	if alert, _ := answer.body["show_alert"].(bool); !alert {
		t.Errorf("show_alert = %v, want true for rejection", alert)
	}
}

func TestHandleArchiveCallback_ArchiveAllCompleted(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	tasks.addTask(root, "completed", "Write docs")
	tasks.addTask(root, "failed", "Broken job")
	running := tasks.addTask(root, "running", "Explore repo")

	ch, caller := newArchiveTestChannel(t, tasks, nil)
	ch.SetAgentID(root.String())

	ch.handleArchiveCallback(context.Background(),
		testCallbackQuery("ar:all:"+root.String(), -100, 101, "en"))

	if len(tasks.archiveAllOf) != 1 || tasks.archiveAllOf[0] != root {
		t.Fatalf("ArchiveCompletedForParent calls = %v, want [%s]", tasks.archiveAllOf, root)
	}
	if len(tasks.archived) != 2 {
		t.Fatalf("archived = %v, want the 2 terminal tasks only", tasks.archived)
	}
	if running.ArchivedAt != nil {
		t.Errorf("running task must survive archive-all")
	}

	edit := lastTelegramCall(t, caller, "editMessageText")
	text, _ := edit.body["text"].(string)
	if !strings.Contains(text, "Archived 2 completed task(s)") {
		t.Errorf("edited text missing archive-all confirmation:\n%s", text)
	}
	rows := keyboardRows(t, edit.body)
	if len(rows) != 1 {
		t.Fatalf("edited keyboard rows = %d, want 1 (running task only)", len(rows))
	}
	row, ok := findRowByCallback(rows, "sa:"+running.ID.String())
	if !ok {
		t.Fatalf("running row missing from edited keyboard: %v", rows)
	}
	if got := len(rowButtons(row)); got != 1 {
		t.Errorf("running row buttons after archive-all = %d, want 1", got)
	}
	if _, ok := findRowByCallback(rows, "ar:all:"); ok {
		t.Errorf("archive-all footer must disappear when no terminal tasks remain")
	}
}

func TestHandleArchiveCallback_GroupNonWriterBlocked(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	done := tasks.addTask(root, "completed", "Write docs")

	ch, caller := newArchiveTestChannel(t, tasks, &fakeArchivePermStore{allow: false})
	ch.SetAgentID(root.String())

	q := testCallbackQuery("ar:"+done.ID.String(), -100123, 101, "en")
	q.Message.(*telego.Message).Chat = telego.Chat{ID: -100123, Type: "supergroup"}
	ch.handleArchiveCallback(context.Background(), q)

	if len(tasks.archived) != 0 {
		t.Fatalf("non-writer must not archive, got %v", tasks.archived)
	}
	if n := countTelegramCalls(caller, "editMessageText"); n != 0 {
		t.Errorf("editMessageText calls = %d, want 0", n)
	}
	denied := false
	for _, call := range caller.calls {
		if call.method != "sendMessage" {
			continue
		}
		if text, _ := call.body["text"].(string); strings.Contains(text, "Only file writers") {
			denied = true
		}
	}
	if !denied {
		t.Errorf("expected writer-gate denial message, methods: %v", caller.methodNames())
	}
}

func TestHandleArchiveCallback_GroupWriterAllowed(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	done := tasks.addTask(root, "completed", "Write docs")

	ch, _ := newArchiveTestChannel(t, tasks, &fakeArchivePermStore{allow: true})
	ch.SetAgentID(root.String())

	q := testCallbackQuery("ar:"+done.ID.String(), -100123, 101, "en")
	q.Message.(*telego.Message).Chat = telego.Chat{ID: -100123, Type: "supergroup"}
	ch.handleArchiveCallback(context.Background(), q)

	if len(tasks.archived) != 1 || tasks.archived[0] != done.ID {
		t.Fatalf("writer archive failed, archived = %v", tasks.archived)
	}
}

// EditMessageText failing (message too old / deleted) must fall back to a
// fresh confirmation message instead of swallowing the tap.
func TestHandleArchiveCallback_EditFailureSendsFreshConfirmation(t *testing.T) {
	root := uuid.New()
	tasks := newFakeSubagentTaskStore()
	done := tasks.addTask(root, "completed", "Write docs")

	caller := &archiveFailingEditCaller{}
	bot, err := telego.NewBot(
		"123456:abcdefghijklmnopqrstuvwxyzABCDE1234",
		telego.WithAPICaller(caller),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	ch := &Channel{
		BaseChannel:       channels.NewBaseChannel("telegram", nil, nil),
		bot:               bot,
		subagentTaskStore: tasks,
	}
	ch.SetName("telegram")
	ch.SetAgentID(root.String())
	ch.SetRunning(true)

	ch.handleArchiveCallback(context.Background(),
		testCallbackQuery("ar:"+done.ID.String(), -100, 101, "en"))

	if len(tasks.archived) != 1 {
		t.Fatalf("archive must succeed despite edit failure, archived = %v", tasks.archived)
	}
	if caller.countCalls("editMessageText") == 0 {
		t.Fatalf("editMessageText was not attempted")
	}
	fallback := false
	for _, call := range caller.calls {
		if call.method != "sendMessage" {
			continue
		}
		if text, _ := call.body["text"].(string); strings.Contains(text, "Task archived") {
			fallback = true
		}
	}
	if !fallback {
		t.Errorf("expected fresh confirmation message after edit failure, methods: %v", caller.calls)
	}
}

// Store unavailable → notice, no crash, spinner dismissed exactly once.
func TestHandleArchiveCallback_StoreUnavailable(t *testing.T) {
	ch, caller := newArchiveTestChannel(t, nil, nil)
	ch.SetAgentID(uuid.New().String())

	ch.handleArchiveCallback(context.Background(),
		testCallbackQuery("ar:"+uuid.New().String(), -100, 101, "en"))

	send := lastTelegramCall(t, caller, "sendMessage")
	text, _ := send.body["text"].(string)
	if !strings.Contains(text, "not available") {
		t.Errorf("reply = %q, want availability notice", text)
	}
	if n := countTelegramCalls(caller, "answerCallbackQuery"); n != 1 {
		t.Errorf("answerCallbackQuery calls = %d, want 1", n)
	}
}
