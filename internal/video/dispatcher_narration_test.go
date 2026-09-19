package video

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// narrationRefs turns whatever lands in the job assets dir into the worker's
// per-scene narration list — the parsing rules (prefix cut, index bounds,
// extension-agnostic suffixes, junk files) deserve direct coverage.
func TestNarrationRefsParsesAndDedupes(t *testing.T) {
	workspace := t.TempDir()
	disp := NewDispatcher(&config.VideoConfig{}, nil, nil, nil, workspace)
	jobID := "job-1"
	dir := disp.assetsDir(jobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"narration-2.wav":     "a",
		"narration-2.mp3":     "b", // duplicate index: lexicographically-first wins
		"narration-0.m4a":     "c",
		"narration-59.flac":   "d", // highest valid index
		"narration-70.wav":    "e", // out of range (>= maxScenes)
		"narration-x.wav":     "f", // non-numeric index
		"narration-.wav":      "g", // empty index
		"narration-2.wav.bak": "h", // suffix after ext — index parses as "2.wav" → skipped
		"other-3.wav":         "i", // wrong prefix
		"sub":                 "",  // dir, not a file
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if content == "" {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	refs := disp.narrationRefs(jobID)
	if len(refs) != 3 {
		t.Fatalf("got %d refs (%v), want 3", len(refs), refs)
	}
	want := []struct {
		idx  int
		name string
	}{
		{0, "narration-0.m4a"},
		{2, "narration-2.mp3"}, // .mp3 < .wav — deterministic dedupe
		{59, "narration-59.flac"},
	}
	for i, w := range want {
		got := refs[i]
		if got.SceneIndex != w.idx || filepath.Base(got.AudioPath) != w.name {
			t.Fatalf("refs[%d] = {%d %s}, want {%d %s}", i, got.SceneIndex, got.AudioPath, w.idx, w.name)
		}
	}
}

func TestNarrationRefsMissingDirReturnsNil(t *testing.T) {
	disp := NewDispatcher(&config.VideoConfig{}, nil, nil, nil, t.TempDir())
	if refs := disp.narrationRefs("no-such-job"); refs != nil {
		t.Fatalf("refs = %v, want nil", refs)
	}
}
