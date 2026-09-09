package telegram

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

type coalescedDispatch struct {
	rctx resolvedMessageContext
	msgs []*telego.Message
}

// dispatchCapture records flushed dispatches from timer goroutines safely.
type dispatchCapture struct {
	mu   sync.Mutex
	all  []coalescedDispatch
	done chan struct{}
}

func newDispatchCapture() *dispatchCapture {
	return &dispatchCapture{done: make(chan struct{}, 64)}
}

func (c *dispatchCapture) add(_ context.Context, rctx resolvedMessageContext, msgs []*telego.Message) {
	c.mu.Lock()
	c.all = append(c.all, coalescedDispatch{rctx: rctx, msgs: msgs})
	c.mu.Unlock()
	select {
	case c.done <- struct{}{}:
	default:
	}
}

func (c *dispatchCapture) snapshot() []coalescedDispatch {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]coalescedDispatch(nil), c.all...)
}

func (c *dispatchCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.all)
}

func testRctx(localKey, senderID, content string) resolvedMessageContext {
	return resolvedMessageContext{
		localKey:  localKey,
		senderID:  senderID,
		chatID:    100,
		chatIDStr: "100",
		content:   content,
	}
}

func newTextTestCoalescer(window time.Duration) (*textCoalescer, *dispatchCapture) {
	capture := newDispatchCapture()
	t := newTextCoalescer(&Channel{}, window)
	t.dispatch = capture.add
	return t, capture
}

func textMsg(id int, text string) *telego.Message {
	return &telego.Message{MessageID: id, Text: text}
}

func waitDispatches(t *testing.T, d *dispatchCapture, want int) []coalescedDispatch {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if d.count() >= want {
			return d.snapshot()
		}
		select {
		case <-d.done:
		case <-time.After(5 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for %d dispatches, got %d", want, d.count())
	return nil
}

func TestTextCoalesceWindow(t *testing.T) {
	if got := textCoalesceWindow(config.TelegramConfig{}); got != 1000*time.Millisecond {
		t.Fatalf("nil config window = %v, want 1s", got)
	}
	zero := 0
	if got := textCoalesceWindow(config.TelegramConfig{TextCoalesceMs: &zero}); got != 0 {
		t.Fatalf("explicit 0 window = %v, want 0 (disabled)", got)
	}
	custom := 2500
	if got := textCoalesceWindow(config.TelegramConfig{TextCoalesceMs: &custom}); got != 2500*time.Millisecond {
		t.Fatalf("custom window = %v, want 2.5s", got)
	}
}

func TestTextCoalescer_Handles(t *testing.T) {
	t0, _ := newTextTestCoalescer(time.Second)
	if t0.handles([]*telego.Message{textMsg(1, "hi"), textMsg(2, "there")}) {
		t.Fatal("multi-member dispatch (album flush) must not be coalesced")
	}
	if !t0.handles([]*telego.Message{textMsg(1, "hi")}) {
		t.Fatal("plain text must be coalesced")
	}
	mediaMsg := &telego.Message{MessageID: 2, Text: "caption", Photo: []telego.PhotoSize{{FileID: "p"}}}
	if t0.handles([]*telego.Message{mediaMsg}) {
		t.Fatal("media-bearing message must not be coalesced")
	}
	if t0.handles([]*telego.Message{&telego.Message{MessageID: 3, MediaGroupID: "album-1", Text: "x"}}) {
		t.Fatal("album member must not be coalesced")
	}
	if t0.handles([]*telego.Message{&telego.Message{MessageID: 4}}) {
		t.Fatal("empty message must not be coalesced")
	}

	disabled, _ := newTextTestCoalescer(0)
	if disabled.handles([]*telego.Message{textMsg(1, "hi")}) {
		t.Fatal("disabled coalescer must not handle anything")
	}
	var nilCoalescer *textCoalescer
	if nilCoalescer.handles([]*telego.Message{textMsg(1, "hi")}) {
		t.Fatal("nil coalescer must not handle anything")
	}
}

func TestTextCoalescer_MergesSplitParts(t *testing.T) {
	co, d := newTextTestCoalescer(300 * time.Millisecond)

	co.push(testRctx("100", "42", "part one"), textMsg(1, "part one"))
	co.push(testRctx("100", "42", "part two"), textMsg(2, "part two"))
	co.push(testRctx("100", "42", "part three"), textMsg(3, "part three"))

	got := waitDispatches(t, d, 1)
	if len(got) != 1 {
		t.Fatalf("got %d dispatches, want 1 merged dispatch", len(got))
	}
	if len(got[0].msgs) != 3 {
		t.Fatalf("dispatched %d msgs, want 3", len(got[0].msgs))
	}
	if got[0].rctx.content != "part one\npart two\npart three" {
		t.Fatalf("merged content = %q, want newline-joined parts", got[0].rctx.content)
	}
	if got[0].rctx.senderID != "42" {
		t.Fatalf("flush rctx senderID = %q, want first part's", got[0].rctx.senderID)
	}
}

func TestTextCoalescer_KeyIsolation(t *testing.T) {
	co, d := newTextTestCoalescer(40 * time.Millisecond)

	co.push(testRctx("100", "42", "alice one"), textMsg(1, "alice one"))
	co.push(testRctx("100", "77", "bob one"), textMsg(2, "bob one"))
	co.push(testRctx("200", "42", "other chat"), textMsg(3, "other chat"))

	got := waitDispatches(t, d, 3)
	if len(got) != 3 {
		t.Fatalf("got %d dispatches, want 3 (no cross-key merging)", len(got))
	}
	seen := map[string]int{}
	for _, disp := range got {
		seen[disp.rctx.content] = len(disp.msgs)
	}
	for _, want := range []string{"alice one", "bob one", "other chat"} {
		if n := seen[want]; n != 1 {
			t.Fatalf("content %q dispatched with %d msgs, want 1", want, n)
		}
	}
}

func TestTextCoalescer_OverflowFlushesEarly(t *testing.T) {
	co, d := newTextTestCoalescer(time.Hour) // timer would never fire

	for i := 1; i <= textCoalesceMaxMsgs; i++ {
		co.push(testRctx("100", "42", "m"), textMsg(i, "m"))
	}

	got := waitDispatches(t, d, 1)
	if len(got[0].msgs) != textCoalesceMaxMsgs {
		t.Fatalf("overflow dispatched %d msgs, want %d", len(got[0].msgs), textCoalesceMaxMsgs)
	}
}

func TestTextCoalescer_FlushAllAndStoppedPassthrough(t *testing.T) {
	co, d := newTextTestCoalescer(time.Hour)

	co.push(testRctx("100", "42", "pending"), textMsg(1, "pending"))
	co.FlushAll()
	got := d.snapshot()
	if len(got) != 1 || got[0].rctx.content != "pending" {
		t.Fatalf("FlushAll must emit pending buffer, got %+v", got)
	}

	// After FlushAll (channel stopped), pushes pass straight through.
	co.push(testRctx("100", "42", "after stop"), textMsg(2, "after stop"))
	got = waitDispatches(t, d, 2)
	last := got[len(got)-1]
	if len(last.msgs) != 1 || last.rctx.content != "after stop" {
		t.Fatalf("post-stop push must dispatch immediately, got %+v", last)
	}

	// FlushAll is idempotent.
	co.FlushAll()
	if d.count() != 2 {
		t.Fatalf("second FlushAll must not re-emit, got %d dispatches", d.count())
	}
}

func TestTextCoalescer_ConcurrentPushSingleFlush(t *testing.T) {
	co, d := newTextTestCoalescer(300 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			co.push(testRctx("100", "42", "part"), textMsg(n, "part"))
		}(i)
	}
	wg.Wait()

	got := waitDispatches(t, d, 1)
	if len(got) != 1 {
		t.Fatalf("got %d dispatches, want 1", len(got))
	}
	if len(got[0].msgs) != 5 {
		t.Fatalf("dispatched %d msgs, want 5", len(got[0].msgs))
	}
	if want := strings.Repeat("part\n", 4) + "part"; got[0].rctx.content != want {
		t.Fatalf("merged content = %q", got[0].rctx.content)
	}
}
