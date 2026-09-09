package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

func TestDownloadOptsWithDefaults(t *testing.T) {
	got := DownloadOpts{}.withDefaults()
	if got.Timeout != DefaultDownloadTimeout {
		t.Errorf("Timeout = %v, want %v", got.Timeout, DefaultDownloadTimeout)
	}
	if got.MaxBytes != DefaultDownloadMaxBytes {
		t.Errorf("MaxBytes = %d, want %d", got.MaxBytes, DefaultDownloadMaxBytes)
	}

	custom := DownloadOpts{Timeout: 5 * time.Second, MaxBytes: 1024}.withDefaults()
	if custom.Timeout != 5*time.Second || custom.MaxBytes != 1024 {
		t.Errorf("custom opts mutated: %+v", custom)
	}

	// Explicit 0/negative MaxBytes falls back to the default (a 0-byte cap is useless).
	zero := DownloadOpts{MaxBytes: -1}.withDefaults()
	if zero.MaxBytes != DefaultDownloadMaxBytes {
		t.Errorf("negative MaxBytes = %d, want default", zero.MaxBytes)
	}
}

func TestDownloadBehaviorConfig(t *testing.T) {
	ctxID := proto.BrowserBrowserContextID("ctx-1")
	beh := downloadBehavior(ctxID, "/tmp/staging")
	if beh.Behavior != proto.BrowserSetDownloadBehaviorBehaviorAllow {
		t.Errorf("Behavior = %v, want allow (keeps original filenames)", beh.Behavior)
	}
	if beh.BrowserContextID != ctxID {
		t.Errorf("BrowserContextID = %v, want %v", beh.BrowserContextID, ctxID)
	}
	if beh.DownloadPath != "/tmp/staging" {
		t.Errorf("DownloadPath = %v, want /tmp/staging", beh.DownloadPath)
	}

	reset := resetDownloadBehavior(ctxID)
	if reset.Behavior != proto.BrowserSetDownloadBehaviorBehaviorDefault {
		t.Errorf("reset Behavior = %v, want default", reset.Behavior)
	}
	if reset.DownloadPath != "" {
		t.Errorf("reset DownloadPath = %q, want empty", reset.DownloadPath)
	}

	// Master/empty scope: empty context ID targets the default context.
	master := downloadBehavior("", "/tmp/x")
	if master.BrowserContextID != "" {
		t.Errorf("master BrowserContextID = %v, want empty", master.BrowserContextID)
	}
}

func TestScanStagingAndSettled(t *testing.T) {
	dir := t.TempDir()

	// Empty/missing dir scans cleanly.
	scan, err := scanStaging(dir)
	if err != nil {
		t.Fatalf("scanStaging: %v", err)
	}
	if len(scan.sizes) != 0 || scan.active {
		t.Fatalf("expected empty scan, got %+v", scan)
	}

	missing, err := scanStaging(filepath.Join(dir, "nope"))
	if err != nil || len(missing.sizes) != 0 {
		t.Fatalf("missing dir scan = %+v, %v", missing, err)
	}

	writeFile := func(name string, n int) {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, n), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	writeFile("report.pdf", 100)
	s1, err := scanStaging(dir)
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	if s1.active {
		t.Fatal("no partial expected yet")
	}
	// First sighting is never settled (needs 2 stable scans).
	if got := s1.settledFiles(nil); len(got) != 0 {
		t.Fatalf("first scan settled = %v, want empty", got)
	}

	// Partial file appears (Chrome renames the final away while writing) —
	// scan goes active, partial never settles.
	_ = os.Remove(filepath.Join(dir, "report.pdf"))
	writeFile("report.pdf.crdownload", 40)
	s2, _ := scanStaging(dir)
	if !s2.active {
		t.Fatal("partial expected to mark scan active")
	}
	if got := s2.settledFiles(s1); len(got) != 0 {
		t.Fatalf("settled during partial = %v, want empty", got)
	}

	// Partial completes: file renamed, size stable across scans → settled.
	_ = os.Remove(filepath.Join(dir, "report.pdf.crdownload"))
	writeFile("report.pdf", 0) // reset to exact final size
	_ = os.Remove(filepath.Join(dir, "report.pdf"))
	writeFile("report.pdf", 500)
	s3, _ := scanStaging(dir)
	if got := s3.settledFiles(s2); len(got) != 0 {
		t.Fatalf("renamed file cannot settle against pre-rename scan: %v", got)
	}
	s4, _ := scanStaging(dir)
	got := s4.settledFiles(s3)
	if len(got) != 1 || got["report.pdf"] != 500 {
		t.Fatalf("settled = %v, want {report.pdf:500}", got)
	}
	if s4.active {
		t.Fatal("completed download must not be active")
	}
	if s4.totalBytes != 500 {
		t.Fatalf("totalBytes = %d, want 500", s4.totalBytes)
	}
}

func TestLargestFile(t *testing.T) {
	if name, _ := largestFile(nil); name != "" {
		t.Fatalf("empty map → %q, want empty", name)
	}
	name, size := largestFile(map[string]int64{"a": 1, "b": 9, "c": 9})
	// Deterministic tiebreak: lexicographically smaller name wins on equal size.
	if name != "b" || size != 9 {
		t.Fatalf("largest = %s:%d, want b:9", name, size)
	}
}

func TestSafeStagedFile(t *testing.T) {
	dir := t.TempDir()
	p, err := safeStagedFile(dir, "file.pdf")
	if err != nil {
		t.Fatalf("safeStagedFile: %v", err)
	}
	if p != filepath.Join(dir, "file.pdf") {
		t.Fatalf("path = %q", p)
	}
	if _, err := safeStagedFile(dir, ""); err == nil {
		t.Fatal("empty name must be rejected")
	}
	if _, err := safeStagedFile(dir, filepath.Join("..", "escape.txt")); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	if _, err := safeStagedFile(dir, ".."); err == nil {
		t.Fatal("bare .. must be rejected")
	}
}

func TestMimeTypeForFile(t *testing.T) {
	cases := map[string]string{
		"report.pdf":  "application/pdf",
		"archive.ZIP": "application/zip",
		"song.MP3":    "audio/mpeg",
		"noext":       "application/octet-stream",
		"weird.xyz":   "application/octet-stream",
	}
	for name, want := range cases {
		if got := mimeTypeForFile(name); got != want {
			t.Errorf("mimeTypeForFile(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestStageDownloadedFileWithoutWorkspace(t *testing.T) {
	// No workspace in ctx → the staging path is returned unchanged.
	res := &DownloadResult{FilePath: filepath.Join(t.TempDir(), "goclaw-dl-x", "f.bin"), FileName: "f.bin"}
	if got := StageDownloadedFile(context.Background(), res); got != res.FilePath {
		t.Fatalf("StageDownloadedFile = %q, want staging path %q", got, res.FilePath)
	}
	if got := StageDownloadedFile(context.Background(), nil); got != "" {
		t.Fatalf("nil result → %q, want empty", got)
	}
}

func TestStageDownloadedFileIntoMediaStore(t *testing.T) {
	ws := t.TempDir()
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "doc.pdf")
	if err := os.WriteFile(src, []byte("%PDF-fake"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	ctx := context.Background()
	ctx = tools.WithToolWorkspace(ctx, ws)
	ctx = tools.WithToolSessionKey(ctx, "session-A")

	res := &DownloadResult{FilePath: src, FileName: "doc.pdf", Bytes: 9}
	got := StageDownloadedFile(ctx, res)
	if got == src {
		t.Fatalf("expected staged path under media store, got src %q", got)
	}
	if filepath.Base(filepath.Dir(filepath.Dir(got))) != ".media" {
		t.Fatalf("staged path %q not under <ws>/.media/<sessionHash>/", got)
	}
	data, err := os.ReadFile(got)
	if err != nil || string(data) != "%PDF-fake" {
		t.Fatalf("staged content = %q, %v", data, err)
	}
}

func TestCleanupStaging(t *testing.T) {
	dir := t.TempDir()
	staging := filepath.Join(dir, "goclaw-dl-123")
	if err := os.MkdirAll(staging, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	CleanupStaging(&DownloadResult{FilePath: filepath.Join(staging, "f.bin")})
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging dir still exists after cleanup: %v", err)
	}

	// Non-goclaw-dl dirs are never removed.
	other := filepath.Join(dir, "important")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	CleanupStaging(&DownloadResult{FilePath: filepath.Join(other, "f.bin")})
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated dir must survive: %v", err)
	}

	CleanupStaging(nil) // must not panic
}
