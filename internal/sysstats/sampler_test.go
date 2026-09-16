package sysstats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotRanges(t *testing.T) {
	s := NewSampler()
	time.Sleep(300 * time.Millisecond) // let the priming sample land

	st := s.Snapshot(t.TempDir())

	if st.CPU.Cores <= 0 {
		t.Errorf("CPU.Cores = %d, want > 0", st.CPU.Cores)
	}
	if st.CPU.Percent < 0 || st.CPU.Percent > 100 {
		t.Errorf("CPU.Percent = %f, want in [0,100]", st.CPU.Percent)
	}
	if st.Memory.Total <= 0 {
		t.Errorf("Memory.Total = %d, want > 0", st.Memory.Total)
	}
	if st.Disk == nil {
		t.Fatal("Disk = nil, want stats for temp dir")
	}
	if st.Disk.UsedPercent < 0 || st.Disk.UsedPercent > 100 {
		t.Errorf("Disk.UsedPercent = %f, want in [0,100]", st.Disk.UsedPercent)
	}
	if st.Proc.Goroutines <= 0 {
		t.Errorf("Proc.Goroutines = %d, want > 0", st.Proc.Goroutines)
	}
	if st.Proc.GoVersion == "" {
		t.Error("Proc.GoVersion empty")
	}
}

func TestSnapshotCPURequiresTwoSamples(t *testing.T) {
	s := NewSampler()
	// No priming wait: first Snapshot may still report samples=0/1 —
	// the contract is only that samples grows monotonically.
	a := s.Snapshot("")
	b := s.Snapshot("")
	if b.Samples < a.Samples {
		t.Errorf("samples decreased: %d -> %d", a.Samples, b.Samples)
	}
}

func TestSnapshotMissingDiskPath(t *testing.T) {
	s := NewSampler()
	st := s.Snapshot(filepath.Join(os.TempDir(), "goclaw-definitely-missing-9f3a"))
	if st.Disk != nil {
		t.Errorf("Disk = %+v, want nil for missing path", st.Disk)
	}
	// Rest of the payload must still be populated.
	if st.Memory.Total <= 0 {
		t.Error("Memory.Total missing despite disk failure")
	}
}

func TestSnapshotJSONShape(t *testing.T) {
	s := NewSampler()
	st := s.Snapshot(t.TempDir())
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"cpu", "memory", "host", "proc", "samples"} {
		if _, ok := m[key]; !ok {
			t.Errorf("payload missing key %q", key)
		}
	}
}
