package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestDroppedTotalCountsFullQueueOverrun verifies the drop accounting: events
// published while the queue is full are lost but tallied, and a growing
// DroppedTotal is observable without inspecting logs.
func TestDroppedTotalCountsFullQueueOverrun(t *testing.T) {
	release := make(chan struct{})
	bus := NewDomainEventBus(Config{
		QueueSize:     2,
		WorkerCount:   1,
		RetryAttempts: 1,
		RetryDelay:    time.Millisecond,
		DedupTTL:      time.Minute,
	})
	bus.Start(context.Background())

	// Block the only worker so the queue stays full while we overrun it.
	var handled atomic.Int32
	bus.Subscribe(EventRunCompleted, func(_ context.Context, _ DomainEvent) error {
		handled.Add(1)
		<-release
		return nil
	})

	bus.Publish(DomainEvent{Type: EventRunCompleted, SourceID: "first"})
	time.Sleep(50 * time.Millisecond) // let the worker pick it up and block

	// Overrun the 2-slot queue: 2 land in the buffer, the rest must drop.
	for i := 0; i < 20; i++ {
		bus.Publish(DomainEvent{Type: EventRunCompleted, SourceID: "burst"})
	}
	// Allow the publishes to be observed (non-blocking select in Publish).
	time.Sleep(50 * time.Millisecond)

	dropped := bus.DroppedTotal()
	if dropped == 0 {
		t.Fatal("DroppedTotal = 0, want > 0 after queue overrun")
	}
	if dropped > 20 {
		t.Fatalf("DroppedTotal = %d, want ≤ 20", dropped)
	}

	close(release)
	_ = bus.Drain(time.Second)
}
