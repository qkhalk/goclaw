package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/internal/tracing"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

const (
	runTimelinePreviewLimit = 2000
	// runTimelineFlushDelay is the window after the first buffered delta before
	// the drain goroutine flushes a coalesced row. Keeps per-run writes
	// sequential (no ordering drift) while collapsing rapid deltas.
	runTimelineFlushDelay = 250 * time.Millisecond
	// runTimelineOrphanReapInterval is how often the background reap goroutine
	// sweeps for stale per-run write states left behind by missed terminal events.
	runTimelineOrphanReapInterval = 10 * time.Minute
	// runTimelineOrphanMaxAge is the maximum idle duration before an orphaned
	// per-run write state is discarded (lost deltas on orphaned streams are
	// acceptable for a background persistence path).
	runTimelineOrphanMaxAge = 1 * time.Hour
)

// isCoalesceableTimelineType reports whether the given item type is a
// stream-delta that should be merged with adjacent same-type items before
// being persisted. Currently chunk and thinking deltas are merged; all other
// types are persisted per-event.
func isCoalesceableTimelineType(itemType string) bool {
	return itemType == store.RunTimelineItemTypeChunk || itemType == store.RunTimelineItemTypeThinking
}

// runWriteState is the per-run write queue and coalescing buffer. Each run
// that has at least one item enqueued gets a state; the drain goroutine
// processes items sequentially preserving arrival order. Adjacent same-type
// stream deltas (chunk, thinking) are coalesced into a single DB row with
// concatenated content to bound row amplification from long LLM streams.
type runWriteState struct {
	mu        sync.Mutex
	queue     []store.RunTimelineItem
	flushing  bool
	nextSeq   int
	lastWrite time.Time // last time drain wrote to store (for orphan reap)
}

func (rs *runWriteState) enqueue(item store.RunTimelineItem) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	item.Seq = rs.nextSeq
	rs.nextSeq++
	rs.queue = append(rs.queue, item)
}

// writeItem persists one item to the store. Timeout context prevents
// indefinite blocking on a slow store. Errors are logged, not propagated
// (the next item proceeds regardless).
func (rs *runWriteState) writeItem(ctx context.Context, s store.RunTimelineStore, item store.RunTimelineItem) {
	wCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	wCtx = store.WithTenantID(wCtx, item.TenantID)
	if err := s.AppendRunTimelineItem(wCtx, &item); err != nil {
		slog.Warn("run_timeline.persist_failed",
			"run_id", item.RunID,
			"item_type", item.ItemType,
			"error", err,
		)
	}
}

// flush coalesces adjacent same-type stream deltas and persists the resulting
// items to the store, then marks the run as no longer flushing. If a terminal
// event is present it is always the last item flushed. Must not be called
// while rs.mu is held.
func (rs *runWriteState) flush(ctx context.Context, s store.RunTimelineStore) {
	for {
		rs.mu.Lock()
		items := rs.queue
		rs.queue = nil
		rs.flushing = false
		rs.mu.Unlock()

		if len(items) == 0 {
			return
		}

		merged := coalesceTimelineItems(items)
		for _, item := range merged {
			rs.writeItem(ctx, s, item)
		}
		rs.mu.Lock()
		rs.lastWrite = time.Now()
		rs.mu.Unlock()

		// Re-check: if more items arrived during flush, drain again.
		rs.mu.Lock()
		empty := len(rs.queue) == 0
		if !empty && !rs.flushing {
			rs.flushing = true
			rs.mu.Unlock()
			continue
		}
		rs.mu.Unlock()
		return
	}
}

// coalesceTimelineItems merges adjacent same-type stream deltas (chunk, chunk
// → single merged chunk; thinking, thinking → single merged thinking) while
// leaving all other items unmerged. Adjacent but differently-typed deltas
// (chunk → thinking) are flushed as separate rows. Returns a new slice; the
// input is not modified.
func coalesceTimelineItems(items []store.RunTimelineItem) []store.RunTimelineItem {
	if len(items) == 0 {
		return nil
	}

	var out []store.RunTimelineItem
	i := 0
	for i < len(items) {
		item := items[i]
		if !isCoalesceableTimelineType(item.ItemType) {
			out = append(out, item)
			i++
			continue
		}
		// Coalesce consecutive same-type stream items.
		merged := item
		var buf strings.Builder
		buf.WriteString(item.Content)
		i++
		for i < len(items) && items[i].ItemType == item.ItemType {
			prev := buf.String()
			if len(prev) > 0 && prev[len(prev)-1] != ' ' && prev[len(prev)-1] != '\n' &&
				len(items[i].Content) > 0 && items[i].Content[0] != ' ' && items[i].Content[0] != '\n' {
				buf.WriteByte(' ')
			}
			buf.WriteString(items[i].Content)
			i++
		}
		merged.Content = buf.String()
		// Preview: tail of merged content (truncated).
		merged.Preview = sanitizeTimelinePreview(merged.Content)
		out = append(out, merged)
	}
	return out
}

// RunTimelineRecorder persists display-safe run events without blocking
// delivery. Adjacent chunk/thinking deltas are coalesced into single rows
// by a per-run drain goroutine to bound row amplification from long LLM
// streams. All public methods are safe for concurrent use.
type RunTimelineRecorder struct {
	store   store.RunTimelineStore
	timeout time.Duration

	mu      sync.Mutex
	nextSeq map[string]int
	runs    map[string]*runWriteState
}

func NewRunTimelineRecorder(timelineStore store.RunTimelineStore) *RunTimelineRecorder {
	r := &RunTimelineRecorder{
		store:   timelineStore,
		timeout: 2 * time.Second,
		nextSeq: make(map[string]int),
		runs:    make(map[string]*runWriteState),
	}
	go r.orphanReap()
	return r
}

func (r *RunTimelineRecorder) Record(event AgentEvent) {
	if r == nil || r.store == nil {
		return
	}
	if event.RunID == "" || event.SessionKey == "" || event.TenantID == uuid.Nil {
		return
	}
	if _, _, ok := timelineKindForEvent(event); !ok {
		return
	}

	seq := r.reserveSeq(event.RunID)
	item, ok := runTimelineItemFromEvent(event, seq)
	if !ok {
		return
	}

	if isTerminalRunTimelineEvent(event.Type) {
		// Opportunistic cleanup: best-effort removal of seq state when
		// the terminal event fires. The authoritative cleanup is in the
		// drain goroutine, which runs after enqueue; this covers the
		// case where no drain was started (e.g. only terminal events
		// were recorded for this run).
		defer r.forgetRun(event.RunID)
	}

	rs := r.getOrCreateRunState(event.RunID)
	rs.enqueue(item)

	rs.mu.Lock()
	alreadyFlushing := rs.flushing
	if !alreadyFlushing {
		rs.flushing = true
	}
	rs.mu.Unlock()

	if !alreadyFlushing {
		go rs.flush(context.Background(), r.store)
	}
}

func (r *RunTimelineRecorder) reserveSeq(runID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSeq[runID]++
	return r.nextSeq[runID]
}

func (r *RunTimelineRecorder) forgetRun(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nextSeq, runID)
}

func (r *RunTimelineRecorder) getOrCreateRunState(runID string) *runWriteState {
	r.mu.Lock()
	defer r.mu.Unlock()
	rs, ok := r.runs[runID]
	if !ok {
		rs = &runWriteState{lastWrite: time.Now()}
		r.runs[runID] = rs
	}
	return rs
}

// orphanReap periodically removes per-run write states that have not been
// written to for longer than runTimelineOrphanMaxAge. This catches runs
// whose terminal event was lost (crash, bug) and prevents unbounded map
// growth. Runs once per 10 minutes; exits when the process does.
func (r *RunTimelineRecorder) orphanReap() {
	ticker := time.NewTicker(runTimelineOrphanReapInterval)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		now := time.Now()
		for id, rs := range r.runs {
			rs.mu.Lock()
			idle := now.Sub(rs.lastWrite) > runTimelineOrphanMaxAge
			empty := len(rs.queue) == 0
			rs.mu.Unlock()
			if idle && empty {
				delete(r.runs, id)
			}
		}
		r.mu.Unlock()
	}
}

func isTerminalRunTimelineEvent(eventType string) bool {
	switch eventType {
	case protocol.AgentEventRunCompleted, protocol.AgentEventRunFailed, protocol.AgentEventRunCancelled:
		return true
	default:
		return false
	}
}

func runTimelineItemFromEvent(event AgentEvent, seq int) (store.RunTimelineItem, bool) {
	if event.RunID == "" || event.SessionKey == "" || event.TenantID == uuid.Nil {
		return store.RunTimelineItem{}, false
	}
	itemType, status, ok := timelineKindForEvent(event)
	if !ok {
		return store.RunTimelineItem{}, false
	}
	metadata := timelineMetadata(event)
	agentUUID, agentIsUUID := parseOptionalUUID(event.AgentID)
	if event.AgentID != "" && !agentIsUUID {
		metadata["agent_key"] = event.AgentID
	}
	traceID, spanID := payloadTraceIDs(event.Payload)
	item := store.RunTimelineItem{
		TenantID:   event.TenantID,
		RunID:      event.RunID,
		SessionKey: event.SessionKey,
		AgentID:    agentUUID,
		UserID:     event.UserID,
		Channel:    event.Channel,
		ChatID:     event.ChatID,
		Seq:        seq,
		ItemType:   itemType,
		Status:     status,
		Title:      timelineTitle(event),
		Preview:    timelinePreview(event),
		ToolName:   payloadString(event.Payload, "name"),
		ToolCallID: payloadString(event.Payload, "id"),
		TraceID:    traceID,
		SpanID:     spanID,
		Metadata:   mustJSON(metadata),
	}
	item.Content = timelineContent(event, itemType)
	return item, true
}

func timelineKindForEvent(event AgentEvent) (string, string, bool) {
	switch event.Type {
	case protocol.AgentEventRunStarted:
		return store.RunTimelineItemTypeRunStatus, store.RunTimelineStatusStarted, true
	case protocol.AgentEventRunCompleted:
		return store.RunTimelineItemTypeRunStatus, store.RunTimelineStatusCompleted, true
	case protocol.AgentEventRunFailed:
		return store.RunTimelineItemTypeRunStatus, store.RunTimelineStatusFailed, true
	case protocol.AgentEventRunCancelled:
		return store.RunTimelineItemTypeRunStatus, store.RunTimelineStatusCancelled, true
	case protocol.AgentEventActivity:
		if payloadString(event.Payload, "phase") == "verifying" {
			return store.RunTimelineItemTypeActivity, store.RunTimelineStatusVerifying, true
		}
		return store.RunTimelineItemTypeActivity, store.RunTimelineStatusRunning, true
	case protocol.AgentEventBlockReply:
		return store.RunTimelineItemTypeAssistantMessage, store.RunTimelineStatusCompleted, true
	case protocol.AgentEventToolCall:
		return store.RunTimelineItemTypeToolCall, store.RunTimelineStatusRunning, true
	case protocol.AgentEventToolResult:
		if payloadBool(event.Payload, "is_error") {
			return store.RunTimelineItemTypeToolResult, store.RunTimelineStatusFailed, true
		}
		return store.RunTimelineItemTypeToolResult, store.RunTimelineStatusCompleted, true
	case protocol.AgentEventToolStarted:
		return store.RunTimelineItemTypeToolStarted, store.RunTimelineStatusWaitingTool, true
	case protocol.ChatEventThinking:
		return store.RunTimelineItemTypeThinking, store.RunTimelineStatusThinking, true
	case protocol.ChatEventChunk:
		return store.RunTimelineItemTypeChunk, store.RunTimelineStatusRunning, true
	default:
		return "", "", false
	}
}

func timelineTitle(event AgentEvent) string {
	if name := payloadString(event.Payload, "name"); name != "" {
		return name
	}
	switch event.Type {
	case protocol.AgentEventRunStarted:
		return "Run started"
	case protocol.AgentEventRunCompleted:
		return "Run completed"
	case protocol.AgentEventRunFailed:
		return "Run failed"
	case protocol.AgentEventRunCancelled:
		return "Run cancelled"
	case protocol.AgentEventBlockReply:
		return "Assistant message"
	case protocol.AgentEventActivity:
		return "Activity"
	case protocol.AgentEventToolStarted:
		if name := payloadString(event.Payload, "name"); name != "" {
			return name
		}
		return "Tool started"
	case protocol.ChatEventThinking:
		return "Thinking"
	case protocol.ChatEventChunk:
		return "Stream"
	default:
		return event.Type
	}
}

func timelinePreview(event AgentEvent) string {
	switch event.Type {
	case protocol.AgentEventRunStarted:
		return sanitizeTimelinePreview(payloadString(event.Payload, "message"))
	case protocol.AgentEventRunCompleted, protocol.AgentEventBlockReply:
		return sanitizeTimelinePreview(payloadString(event.Payload, "content"))
	case protocol.AgentEventRunFailed:
		return sanitizeTimelinePreview(payloadString(event.Payload, "error"))
	case protocol.AgentEventActivity:
		return sanitizeTimelinePreview(payloadAnyString(event.Payload))
	case protocol.AgentEventToolCall:
		return sanitizeTimelinePreview(payloadJSON(event.Payload, "arguments"))
	case protocol.AgentEventToolResult:
		if result := payloadString(event.Payload, "result"); result != "" {
			return sanitizeTimelinePreview(result)
		}
		return sanitizeTimelinePreview(payloadString(event.Payload, "content"))
	case protocol.AgentEventToolStarted:
		return "" // tool identity lives in ToolName/ToolCallID; no preview needed
	case protocol.ChatEventThinking:
		return sanitizeTimelinePreview(payloadString(event.Payload, "content"))
	case protocol.ChatEventChunk:
		return sanitizeTimelinePreview(payloadString(event.Payload, "content"))
	default:
		return ""
	}
}

// timelineContent returns the full content persisted for content-carrying
// timeline types. Chunk/thinking persist the streamed text deltas; tool.started
// persists a compact JSON description of the tool + call id so replay clients
// can render what began executing. Non-carrying types return "" (preview-only).
func timelineContent(event AgentEvent, itemType string) string {
	if !store.RunTimelineItemContentPersisted(itemType) {
		return ""
	}
	switch event.Type {
	case protocol.ChatEventThinking, protocol.ChatEventChunk:
		return payloadString(event.Payload, "content")
	case protocol.AgentEventToolStarted:
		entry := map[string]any{}
		if name := payloadString(event.Payload, "name"); name != "" {
			entry["name"] = name
		}
		if rawName := payloadString(event.Payload, "rawName"); rawName != "" {
			entry["raw_name"] = rawName
		}
		if id := payloadString(event.Payload, "id"); id != "" {
			entry["id"] = id
		}
		if len(entry) == 0 {
			return ""
		}
		raw, _ := json.Marshal(entry)
		return string(raw)
	}
	return ""
}

func sanitizeTimelinePreview(value string) string {
	value = strings.TrimSpace(stripThinkingTags(value))
	value = stripDeliveryFileTokens(value)
	value = tools.ScrubCredentials(value)
	return tracing.TruncateMid(value, runTimelinePreviewLimit)
}

var deliveryFileTokenRe = regexp.MustCompile(`([?&])ft=[^)\]'"<>\s&]+`)

func stripDeliveryFileTokens(value string) string {
	if !strings.Contains(value, "ft=") {
		return value
	}
	value = deliveryFileTokenRe.ReplaceAllStringFunc(value, func(match string) string {
		if strings.HasPrefix(match, "&") {
			return ""
		}
		return "?"
	})
	value = strings.ReplaceAll(value, "?&", "?")
	value = strings.ReplaceAll(value, "?)", ")")
	value = strings.ReplaceAll(value, "?]", "]")
	value = strings.TrimSuffix(value, "?")
	return strings.TrimSuffix(value, "&")
}

func timelineMetadata(event AgentEvent) map[string]any {
	metadata := map[string]any{"event_type": event.Type}
	if event.RunKind != "" {
		metadata["run_kind"] = event.RunKind
	}
	if event.DelegationID != "" {
		metadata["delegation_id"] = event.DelegationID
	}
	if event.TeamID != "" {
		metadata["team_id"] = event.TeamID
	}
	if event.TeamTaskID != "" {
		metadata["team_task_id"] = event.TeamTaskID
	}
	if event.ParentAgentID != "" {
		metadata["parent_agent_id"] = event.ParentAgentID
	}
	if event.SenderID != "" {
		metadata["sender_id"] = event.SenderID
	}
	if payloadBool(event.Payload, "is_error") {
		metadata["is_error"] = true
	}
	return metadata
}

func payloadString(payload any, key string) string {
	switch m := payload.(type) {
	case map[string]string:
		return m[key]
	case map[string]any:
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	return ""
}

func payloadBool(payload any, key string) bool {
	if m, ok := payload.(map[string]any); ok {
		if v, ok := m[key].(bool); ok {
			return v
		}
	}
	return false
}

func payloadAnyString(payload any) string {
	switch v := payload.(type) {
	case string:
		return v
	case map[string]string:
		if c := v["content"]; c != "" {
			return c
		}
		if m := v["message"]; m != "" {
			return m
		}
	case map[string]any:
		for _, key := range []string{"content", "message", "status", "step"} {
			if s, ok := v[key].(string); ok && s != "" {
				return s
			}
		}
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func payloadJSON(payload any, key string) string {
	if m, ok := payload.(map[string]any); ok {
		if v, has := m[key]; has {
			raw, _ := json.Marshal(v)
			return string(raw)
		}
	}
	return ""
}

func payloadTraceIDs(payload any) (*uuid.UUID, *uuid.UUID) {
	traceID := parsePayloadUUID(payload, "trace_id", "traceId")
	spanID := parsePayloadUUID(payload, "span_id", "spanId")
	return traceID, spanID
}

func parsePayloadUUID(payload any, keys ...string) *uuid.UUID {
	for _, key := range keys {
		if v := payloadString(payload, key); v != "" {
			if parsed, err := uuid.Parse(v); err == nil {
				return &parsed
			}
		}
	}
	return nil
}

func parseOptionalUUID(value string) (*uuid.UUID, bool) {
	if value == "" {
		return nil, false
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, false
	}
	return &parsed, true
}

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}
