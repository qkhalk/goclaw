package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// failingSessionStore records AddMessage/Save calls so tests can assert what a
// failed run persisted. History is served from a settable slice.
type failingSessionStore struct {
	*nopSessionStore
	added []providers.Message
	saves int
}

func (s *failingSessionStore) AddMessage(_ context.Context, _ string, msg providers.Message) {
	s.added = append(s.added, msg)
}

func (s *failingSessionStore) Save(_ context.Context, _ string) error {
	s.saves++
	return nil
}

func TestPersistFailedTurnInput_PersistsUnflushedUserMessage(t *testing.T) {
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: "đoạn văn dài người dùng gửi"}
	var persisted bool

	loop.persistFailedTurnInput(context.Background(), req, nil, &persisted)

	if !persisted {
		t.Fatal("persisted flag must be set")
	}
	if len(sessions.added) != 1 || sessions.added[0].Role != "user" || sessions.added[0].Content != req.Message {
		t.Fatalf("persisted messages = %#v, want the user input", sessions.added)
	}
	if sessions.saves != 1 {
		t.Fatalf("Save calls = %d, want 1", sessions.saves)
	}
}

func TestPersistFailedTurnInput_SkipsWhenAlreadyFlushed(t *testing.T) {
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: "hello"}
	persisted := true // first FlushMessages already wrote the input

	loop.persistFailedTurnInput(context.Background(), req, nil, &persisted)

	if len(sessions.added) != 0 {
		t.Fatalf("persisted messages = %#v, want none (already flushed)", sessions.added)
	}
}

func TestPersistFailedTurnInput_SkipsResumeAndHiddenInput(t *testing.T) {
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	loop := &Loop{id: "fox", sessions: sessions}
	var persisted bool

	// Resume runs: the checkpoint stage of the earlier attempt already flushed.
	loop.persistFailedTurnInput(context.Background(),
		&RunRequest{SessionKey: "s1", Message: "hello"}, &pipeline.RunState{}, &persisted)
	// HideInput / empty message: nothing user-visible to record.
	loop.persistFailedTurnInput(context.Background(),
		&RunRequest{SessionKey: "s1", Message: "hello", HideInput: true}, nil, &persisted)
	loop.persistFailedTurnInput(context.Background(),
		&RunRequest{SessionKey: "s1", Message: ""}, nil, &persisted)

	if len(sessions.added) != 0 {
		t.Fatalf("persisted messages = %#v, want none", sessions.added)
	}
}

func TestPersistFailedTurnInput_DedupsIdenticalTrailingUserTurn(t *testing.T) {
	// Verifier continuation / fresh-fallback resume: the same message is already
	// the latest user turn in history (assistant replies may follow it).
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	sessions.history = []providers.Message{
		{Role: "user", Content: "novel text"},
		{Role: "assistant", Content: "incomplete reply"},
	}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: "novel text"}
	var persisted bool

	loop.persistFailedTurnInput(context.Background(), req, nil, &persisted)

	if len(sessions.added) != 0 {
		t.Fatalf("persisted messages = %#v, want none (identical turn already in history)", sessions.added)
	}
	if sessions.saves != 0 {
		t.Fatalf("Save calls = %d, want 0", sessions.saves)
	}
}

func TestPersistFailedTurnInput_PersistsWhenTrailingUserTurnDiffers(t *testing.T) {
	// History's latest user turn is from an earlier conversation turn: the
	// failed input must still land, even if a re-send repeats later.
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	sessions.history = []providers.Message{
		{Role: "user", Content: "older turn"},
		{Role: "assistant", Content: "reply"},
	}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: "new failed input"}
	var persisted bool

	loop.persistFailedTurnInput(context.Background(), req, nil, &persisted)

	if len(sessions.added) != 1 || sessions.added[0].Content != "new failed input" {
		t.Fatalf("persisted messages = %#v, want the failed input", sessions.added)
	}
}

func TestPersistFailedTurnInput_DedupsEnrichedMediaTurn(t *testing.T) {
	// The media stage replaced the raw text with the enriched form before the
	// failure; history already holds that enriched turn (verifier continuation
	// / resume fresh-fallback). Dedup must compare the EFFECTIVE input, not the
	// raw message, or a duplicate enriched turn gets appended.
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	sessions.history = []providers.Message{
		{Role: "user", Content: `enriched <media:image path=".uploads/photo.png">`},
	}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: `<media:image url="attachment://photo.png">`}
	req.enrichedInputMessage = providers.Message{
		Role:    "user",
		Content: `enriched <media:image path=".uploads/photo.png">`,
	}
	req.hasEnrichedInputMessage = true
	var persisted bool

	loop.persistFailedTurnInput(context.Background(), req, nil, &persisted)

	if len(sessions.added) != 0 {
		t.Fatalf("persisted messages = %#v, want none (enriched turn already in history)", sessions.added)
	}
}

func TestPersistFailedTurnInput_UsesEnrichedMediaMessage(t *testing.T) {
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: `<media:image url="attachment://photo.png">`}
	req.enrichedInputMessage = providers.Message{
		Role:      "user",
		Content:   `enriched <media:image path=".uploads/photo.png">`,
		MediaRefs: []providers.MediaRef{{ID: "m1"}},
	}
	req.hasEnrichedInputMessage = true
	var persisted bool

	loop.persistFailedTurnInput(context.Background(), req, nil, &persisted)

	if len(sessions.added) != 1 || sessions.added[0].Content != req.enrichedInputMessage.Content {
		t.Fatalf("persisted messages = %#v, want enriched input", sessions.added)
	}
	if len(sessions.added[0].MediaRefs) != 1 {
		t.Fatalf("persisted MediaRefs = %#v, want enriched ref", sessions.added[0].MediaRefs)
	}
}

func TestPersistFailedTurnInput_CancelledContextStillPersists(t *testing.T) {
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: "cancelled run input"}
	var persisted bool

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Even a cancelled context must record the input — the error path runs
	// after run aborts, and context.WithoutCancel keeps the store call alive.
	loop.persistFailedTurnInput(ctx, req, nil, &persisted)

	if len(sessions.added) != 1 {
		t.Fatalf("persisted messages = %#v, want the input despite cancelled ctx", sessions.added)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("sanity: context should be cancelled")
	}
}

func TestPipelineCallbacksShareFlushFlagWithSet(t *testing.T) {
	sessions := &failingSessionStore{nopSessionStore: &nopSessionStore{}}
	loop := &Loop{id: "fox", sessions: sessions}
	req := &RunRequest{SessionKey: "s1", Message: "hello"}

	cb := loop.pipelineCallbacks(req, &runState{})
	if cb.userMsgPersisted == nil {
		t.Fatal("pipelineCallbackSet.userMsgPersisted must be non-nil")
	}
	if *cb.userMsgPersisted {
		t.Fatal("flag must start false")
	}
	// First flush flips the shared flag.
	if err := cb.flushMessages(context.Background(), "s1", nil); err != nil {
		t.Fatal(err)
	}
	if !*cb.userMsgPersisted {
		t.Fatal("flag must flip after first flush")
	}
	// The failed-run persist then becomes a no-op.
	loop.persistFailedTurnInput(context.Background(), req, nil, cb.userMsgPersisted)
	if len(sessions.added) != 1 {
		t.Fatalf("persisted messages = %#v, want exactly one copy", sessions.added)
	}
}
