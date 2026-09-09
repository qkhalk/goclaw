package telegram

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// Text coalescing: the Telegram client splits an outbound message longer than
// 4096 chars into several consecutive messages (<100ms apart), and each part
// previously reached the agent as its own run — the agent answered only the
// first fragment. The coalescer buffers plain-text messages per
// chat|sender|topic and flushes them as ONE dispatch after a short silence
// window, reusing the album-flush multi-member publish path (which already
// seeds merged_message_ids for the consumer dedup).
//
// Only plain text is buffered: media/album messages keep their own paths
// (albumAggregator + the consumer-side media debounce floor).

const (
	textCoalesceDefaultMs = 1000
	textCoalesceMaxMsgs   = 10
	textCoalesceMaxRunes  = 32_000
)

// textCoalesceWindow resolves the silence window from config:
// unset (nil) = default 1s, 0/negative = disabled, otherwise the configured value.
func textCoalesceWindow(cfg config.TelegramConfig) time.Duration {
	if cfg.TextCoalesceMs == nil {
		return textCoalesceDefaultMs * time.Millisecond
	}
	if *cfg.TextCoalesceMs <= 0 {
		return 0
	}
	return time.Duration(*cfg.TextCoalesceMs) * time.Millisecond
}

type textBuffer struct {
	rctx     resolvedMessageContext // first part's context (metadata/reply identity)
	msgs     []*telego.Message
	contents []string // each part's gated content, arrival order
	runes    int
	timer    *time.Timer
}

type textCoalescer struct {
	mu      sync.Mutex
	window  time.Duration
	buffers map[string]*textBuffer
	stopped bool
	ch      *Channel // pollCtx source for timer flushes
	// dispatch is Channel.dispatchResolvedMessage; a field (not a method ref
	// on ch) so tests can capture flushed dispatches without a live bus.
	dispatch func(ctx context.Context, rctx resolvedMessageContext, msgs []*telego.Message)
}

func newTextCoalescer(ch *Channel, window time.Duration) *textCoalescer {
	return &textCoalescer{
		window:   window,
		buffers:  make(map[string]*textBuffer),
		ch:       ch,
		dispatch: ch.dispatchResolvedMessage,
	}
}

// handles reports whether this dispatch (single member, plain text without any
// attachments) is eligible for coalescing.
func (t *textCoalescer) handles(members []*telego.Message) bool {
	if t == nil || t.window <= 0 || len(members) != 1 {
		return false
	}
	m := members[0]
	return m.Text != "" && !messageHasAttachment(m) && m.MediaGroupID == ""
}

// messageHasAttachment reports whether the message carries any media, location,
// contact, poll, etc. Plain text messages have none of these.
func messageHasAttachment(m *telego.Message) bool {
	return m.Photo != nil || m.Audio != nil || m.Video != nil ||
		m.Document != nil || m.Voice != nil || m.VideoNote != nil ||
		m.Sticker != nil || m.Animation != nil || m.Contact != nil ||
		m.Location != nil || m.Venue != nil || m.Poll != nil
}

// push buffers a plain-text part and (re)arms the silence timer. The first
// part's rctx owns the flush identity; each part contributes its content.
// Overflow (message-count or rune cap) flushes early so nothing stalls.
func (t *textCoalescer) push(rctx resolvedMessageContext, m *telego.Message) {
	key := rctx.localKey + "|" + rctx.senderID
	content := rctx.content

	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		t.dispatch(t.ch.pollCtx, rctx, []*telego.Message{m})
		return
	}
	buf, ok := t.buffers[key]
	if !ok {
		buf = &textBuffer{rctx: rctx}
		t.buffers[key] = buf
	}
	buf.msgs = append(buf.msgs, m)
	buf.contents = append(buf.contents, content)
	buf.runes += len([]rune(content))

	if len(buf.msgs) >= textCoalesceMaxMsgs || buf.runes >= textCoalesceMaxRunes {
		delete(t.buffers, key)
		if buf.timer != nil {
			buf.timer.Stop()
		}
		t.mu.Unlock()
		slog.Debug("telegram text coalescer: overflow flush", "key", key, "msgs", len(buf.msgs))
		t.emit(key, buf)
		return
	}

	if buf.timer != nil {
		buf.timer.Stop()
	}
	window := t.window
	// Timer flushes must participate in handlerWg so Stop()'s
	// handlerWg.Wait() cannot return while a flush dispatch is still
	// running — same invariant the album aggregator's flushFn documents.
	buf.timer = time.AfterFunc(window, func() {
		t.ch.handlerWg.Add(1)
		defer t.ch.handlerWg.Done()
		t.flushKey(key)
	})
	buffered := len(buf.msgs)
	t.mu.Unlock()

	slog.Debug("telegram text coalescer: buffered",
		"key", key, "buffered", buffered, "window_ms", window.Milliseconds())
}

// flushKey drains one buffer and emits it as a single merged dispatch.
func (t *textCoalescer) flushKey(key string) {
	t.mu.Lock()
	buf, ok := t.buffers[key]
	if ok {
		delete(t.buffers, key)
	}
	t.mu.Unlock()
	if !ok || len(buf.msgs) == 0 {
		return
	}
	t.emit(key, buf)
}

// emit publishes the buffer through the normal single-dispatch path. With >1
// parts the first rctx carries the newline-joined content, so downstream sees
// one user message with the full text — identical shape to an album flush
// (members[0] identity + merged_message_ids seeding in dispatchResolvedMessage).
func (t *textCoalescer) emit(key string, buf *textBuffer) {
	rctx := buf.rctx
	if len(buf.contents) > 1 {
		rctx.content = strings.Join(buf.contents, "\n")
	}
	slog.Debug("telegram text coalescer: flush", "key", key, "msgs", len(buf.msgs))
	t.dispatch(t.ch.pollCtx, rctx, buf.msgs)
}

// FlushAll drains every pending buffer synchronously (channel Stop path).
// After FlushAll the coalescer passes new pushes straight through.
func (t *textCoalescer) FlushAll() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	pending := make([]*textBuffer, 0, len(t.buffers))
	keys := make([]string, 0, len(t.buffers))
	for k, buf := range t.buffers {
		if buf.timer != nil {
			buf.timer.Stop()
		}
		pending = append(pending, buf)
		keys = append(keys, k)
	}
	t.buffers = make(map[string]*textBuffer)
	t.mu.Unlock()

	for i, buf := range pending {
		slog.Debug("telegram text coalescer: stop flush", "key", keys[i], "msgs", len(buf.msgs))
		t.emit(keys[i], buf)
	}
}
