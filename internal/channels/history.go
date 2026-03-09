// Package channels — Group pending history tracker.
//
// Tracks messages in group chats when the bot is NOT mentioned (requireMention=true).
// When the bot IS mentioned, accumulated context is prepended to the user message
// so the LLM has conversational context from the group.
//
// Supports optional DB persistence via PendingMessageStore with batched flush.
package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// maxHistoryKeys is the max number of distinct groups/topics tracked in RAM.
const maxHistoryKeys = 1000

// DefaultGroupHistoryLimit is the default pending message limit per group.
const DefaultGroupHistoryLimit = 50

const (
	flushInterval = 3 * time.Second // periodic flush interval
	flushBatchMax = 20              // flush when buffer reaches this size
)

// HistoryEntry represents a single tracked group message.
type HistoryEntry struct {
	Sender    string
	SenderID  string
	Body      string
	Timestamp time.Time
	MessageID string
}

// PendingHistory tracks group messages across multiple groups.
// Thread-safe for concurrent access from message handlers.
type PendingHistory struct {
	mu      sync.Mutex
	entries map[string][]HistoryEntry // historyKey → entries
	order   []string                  // insertion order for LRU eviction

	// Persistence (optional — nil means RAM-only)
	channelName string
	store       store.PendingMessageStore
	flushMu     sync.Mutex
	flushBuf    []store.PendingMessage
	flushSignal chan struct{}
	stopCh      chan struct{}
	stopped     chan struct{}

	// Compaction (optional — nil means no auto-compaction)
	compactionCfg *CompactionConfig

	// Compaction guard: per-key flag to prevent concurrent compactions
	compacting sync.Map // historyKey → bool
}

// NewPendingHistory creates a new RAM-only pending history tracker.
func NewPendingHistory() *PendingHistory {
	return &PendingHistory{entries: make(map[string][]HistoryEntry)}
}

// NewPersistentHistory creates a persistent history tracker with batched DB flush.
// Call StartFlusher() after creation and StopFlusher() on shutdown.
func NewPersistentHistory(channelName string, s store.PendingMessageStore) *PendingHistory {
	return &PendingHistory{
		entries:     make(map[string][]HistoryEntry),
		channelName: channelName,
		store:       s,
		flushSignal: make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		stopped:     make(chan struct{}),
	}
}

// IsPersistent returns true if this history is backed by a DB store.
func (ph *PendingHistory) IsPersistent() bool { return ph.store != nil }

// SetCompactionConfig sets the LLM compaction config. Call after creation.
func (ph *PendingHistory) SetCompactionConfig(cfg *CompactionConfig) {
	ph.compactionCfg = cfg
}

// LoadFromDB loads pending history from the database into RAM.
// Call once during Start() before the channel begins processing messages.
func (ph *PendingHistory) LoadFromDB(ctx context.Context) {
	if ph.store == nil {
		return
	}
	// List all distinct history keys for this channel, then load entries.
	// Since ListByKey requires a specific key, we need a ListByChannel method.
	// For now, the DB-backed history starts empty in RAM and accumulates.
	// Entries are persisted and available via compaction reads.
	// TODO: Add ListByChannel to PendingMessageStore for full startup warm.
}

// MakeHistory creates a PendingHistory — persistent if store is non-nil, RAM-only otherwise.
func MakeHistory(channelName string, s store.PendingMessageStore) *PendingHistory {
	if s != nil {
		return NewPersistentHistory(channelName, s)
	}
	return NewPendingHistory()
}

// Record adds a message to the pending history for a group.
// If limit ≤ 0, recording is disabled.
func (ph *PendingHistory) Record(historyKey string, entry HistoryEntry, limit int) {
	if limit <= 0 || historyKey == "" {
		return
	}

	var count int

	ph.mu.Lock()
	existing := ph.entries[historyKey]
	existing = append(existing, entry)
	count = len(existing) // capture pre-trim count so MaybeCompact sees threshold exceeded
	if len(existing) > limit {
		existing = existing[len(existing)-limit:]
	}
	ph.entries[historyKey] = existing
	ph.removeFromOrder(historyKey)
	ph.order = append(ph.order, historyKey)
	ph.evictOldKeys()
	ph.mu.Unlock()

	// Queue for DB persistence (batched flush)
	if ph.store != nil {
		ph.enqueueFlush(store.PendingMessage{
			ChannelName:   ph.channelName,
			HistoryKey:    historyKey,
			Sender:        entry.Sender,
			SenderID:      entry.SenderID,
			Body:          entry.Body,
			PlatformMsgID: entry.MessageID,
			CreatedAt:     entry.Timestamp,
		})
	}

	// Trigger compaction if threshold exceeded (background, non-blocking)
	ph.MaybeCompact(historyKey, count, ph.compactionCfg)
}

// BuildContext retrieves pending history for a group and formats it as context
// to prepend to the current message.
func (ph *PendingHistory) BuildContext(historyKey, currentMessage string, limit int) string {
	if limit <= 0 || historyKey == "" {
		return currentMessage
	}

	ph.mu.Lock()
	entries := ph.entries[historyKey]
	entriesCopy := make([]HistoryEntry, len(entries))
	copy(entriesCopy, entries)
	ph.mu.Unlock()

	if len(entriesCopy) == 0 {
		return currentMessage
	}

	var lines []string
	for _, e := range entriesCopy {
		ts := ""
		if !e.Timestamp.IsZero() {
			ts = fmt.Sprintf(" [%s]", e.Timestamp.Format("15:04"))
		}
		lines = append(lines, fmt.Sprintf("  %s%s: %s", e.Sender, ts, e.Body))
	}

	return fmt.Sprintf("[Chat messages since your last reply - for context]\n%s\n\n[Your current message]\n%s",
		strings.Join(lines, "\n"), currentMessage)
}

// GetEntries returns a copy of pending entries for a group.
func (ph *PendingHistory) GetEntries(historyKey string) []HistoryEntry {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	entries := ph.entries[historyKey]
	if len(entries) == 0 {
		return nil
	}
	result := make([]HistoryEntry, len(entries))
	copy(result, entries)
	return result
}

// Clear removes all pending history for a group.
// Called after the bot replies to that group.
func (ph *PendingHistory) Clear(historyKey string) {
	if historyKey == "" {
		return
	}

	ph.mu.Lock()
	delete(ph.entries, historyKey)
	ph.removeFromOrder(historyKey)
	ph.mu.Unlock()

	if ph.store != nil {
		// Remove pending flushes for this key
		ph.removeFromFlushBuf(historyKey)
		// Delete from DB
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ph.store.DeleteByKey(ctx, ph.channelName, historyKey); err != nil {
			slog.Warn("pending_history.clear_db_failed", "channel", ph.channelName, "key", historyKey, "error", err)
		}
	}
}

// removeFromOrder removes a key from the LRU order slice (caller must hold ph.mu).
func (ph *PendingHistory) removeFromOrder(key string) {
	for i, k := range ph.order {
		if k == key {
			ph.order = append(ph.order[:i], ph.order[i+1:]...)
			return
		}
	}
}

// evictOldKeys removes the oldest groups when exceeding maxHistoryKeys (caller must hold ph.mu).
func (ph *PendingHistory) evictOldKeys() {
	for len(ph.order) > maxHistoryKeys {
		oldest := ph.order[0]
		ph.order = ph.order[1:]
		delete(ph.entries, oldest)
	}
}
