package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// recordingSnapshotStore is a minimal CheckpointSnapshotStore double that
// records appends so the snapshot-history wiring is asserted without a DB.
type recordingSnapshotStore struct {
	mu      sync.Mutex
	appends []store.CheckpointSnapshot
}

func (s *recordingSnapshotStore) AppendCheckpointSnapshot(_ context.Context, snap *store.CheckpointSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appends = append(s.appends, *snap)
	return nil
}

func (s *recordingSnapshotStore) ListCheckpointSnapshots(_ context.Context, opts store.CheckpointSnapshotListOpts) ([]store.CheckpointSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Newest-first for the run, mirroring the real stores.
	var out []store.CheckpointSnapshot
	for i := len(s.appends) - 1; i >= 0; i-- {
		if s.appends[i].RunID == opts.RunID {
			out = append(out, s.appends[i])
			if len(out) >= opts.Limit {
				break
			}
		}
	}
	return out, nil
}

func (s *recordingSnapshotStore) GetCheckpointSnapshot(_ context.Context, _ string, _ int) (*store.CheckpointSnapshot, error) {
	return nil, nil
}

func (s *recordingSnapshotStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.appends)
}

func TestCheckpointAppendsSnapshotHistory(t *testing.T) {
	runs := &recordingRunsStore{}
	snaps := &recordingSnapshotStore{}
	l := &Loop{runsStore: runs, snapshotsStore: snaps}
	u := newRunRecordUpdater(context.Background(), l, "run-snap-1")
	if u == nil {
		t.Fatal("expected updater")
	}
	defer u.terminal(context.Background(), store.AgentRunStatusCompleted, "")

	state := &pipeline.RunState{}
	state.Iteration = 5
	if err := u.checkpoint(context.Background(), store.AgentRunStatusRunning, state); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	state.Iteration = 10
	if err := u.checkpoint(context.Background(), store.AgentRunStatusRunning, state); err != nil {
		t.Fatalf("second checkpoint failed: %v", err)
	}

	if got := snaps.count(); got != 2 {
		t.Fatalf("snapshot appends = %d, want 2", got)
	}
	latest, err := snaps.ListCheckpointSnapshots(context.Background(), store.CheckpointSnapshotListOpts{RunID: "run-snap-1", Limit: 1})
	if err != nil || len(latest) != 1 {
		t.Fatalf("list snapshots: %v (%d rows)", err, len(latest))
	}
	if latest[0].Seq != 2 {
		t.Fatalf("newest seq = %d, want 2", latest[0].Seq)
	}
	if latest[0].Status != store.CheckpointSnapshotRunning {
		t.Fatalf("status = %q, want %q", latest[0].Status, store.CheckpointSnapshotRunning)
	}
	if latest[0].Iteration != 10 {
		t.Fatalf("iteration = %d, want 10", latest[0].Iteration)
	}
	if len(latest[0].Snapshot) == 0 {
		t.Fatal("snapshot body empty")
	}
}

func TestCheckpointNilSnapshotStoreStillWritesRecord(t *testing.T) {
	// Snapshot history disabled (nil) → checkpoint DB write unaffected.
	runs := &recordingRunsStore{}
	l := &Loop{runsStore: runs}
	u := newRunRecordUpdater(context.Background(), l, "run-snap-2")
	if u == nil {
		t.Fatal("expected updater")
	}
	defer u.terminal(context.Background(), store.AgentRunStatusCompleted, "")

	if err := u.checkpoint(context.Background(), store.AgentRunStatusRunning, &pipeline.RunState{}); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
}

func TestAppendSnapshotUnknownStatusFallsBackToRunning(t *testing.T) {
	runs := &recordingRunsStore{}
	snaps := &recordingSnapshotStore{}
	l := &Loop{runsStore: runs, snapshotsStore: snaps}
	u := newRunRecordUpdater(context.Background(), l, "run-snap-3")
	if u == nil {
		t.Fatal("expected updater")
	}
	defer u.terminal(context.Background(), store.AgentRunStatusCompleted, "")

	u.appendSnapshot(context.Background(), "bogus-status", json.RawMessage(`{}`), 1)
	if got := snaps.count(); got != 1 {
		t.Fatalf("snapshot appends = %d, want 1", got)
	}
	if snaps.appends[0].Status != store.CheckpointSnapshotRunning {
		t.Fatalf("status = %q, want fallback %q", snaps.appends[0].Status, store.CheckpointSnapshotRunning)
	}
}
