package methods

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// buildSuggestTree creates:
//
//	root/
//	  alpha/
//	    inner/
//	  Beta/
//	  .hidden/
//	  notes.txt
func buildSuggestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"alpha/inner", "Beta", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return root
}

func TestSuggestDirectoriesEmptyQueryListsBaseChildren(t *testing.T) {
	root := buildSuggestTree(t)
	got := suggestDirectories(root, "", 0)
	want := []string{
		filepath.ToSlash(filepath.Join(root, "alpha")) + "/",
		filepath.ToSlash(filepath.Join(root, "Beta")) + "/",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSuggestDirectoriesPrefixMatchCaseInsensitive(t *testing.T) {
	root := buildSuggestTree(t)
	got := suggestDirectories(root, "BE", 0)
	want := []string{filepath.ToSlash(filepath.Join(root, "Beta")) + "/"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSuggestDirectoriesTrailingSlashListsChildren(t *testing.T) {
	root := buildSuggestTree(t)
	got := suggestDirectories(root, filepath.Join(root, "alpha")+"/", 0)
	want := []string{filepath.ToSlash(filepath.Join(root, "alpha", "inner")) + "/"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSuggestDirectoriesHiddenSkippedUnlessTyped(t *testing.T) {
	root := buildSuggestTree(t)
	if got := suggestDirectories(root, ".", 0); len(got) != 1 {
		t.Fatalf("typed dot should surface .hidden, got %v", got)
	}
}

func TestSuggestDirectoriesRelativeResolvesUnderBase(t *testing.T) {
	root := buildSuggestTree(t)
	got := suggestDirectories(root, "al", 0)
	want := []string{filepath.ToSlash(filepath.Join(root, "alpha")) + "/"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSuggestDirectoriesRejectsDotDot(t *testing.T) {
	root := buildSuggestTree(t)
	if got := suggestDirectories(root, "../", 0); len(got) != 0 {
		t.Fatalf(".. segment must yield no suggestions, got %v", got)
	}
	if got := suggestDirectories(root, "alpha/../..", 0); len(got) != 0 {
		t.Fatalf("embedded .. must yield no suggestions, got %v", got)
	}
}

func TestSuggestDirectoriesMissingDirYieldsEmpty(t *testing.T) {
	root := buildSuggestTree(t)
	if got := suggestDirectories(root, filepath.Join(root, "nope")+"/x", 0); len(got) != 0 {
		t.Fatalf("missing dir must yield empty, got %v", got)
	}
}

func TestSuggestDirectoriesFilesExcluded(t *testing.T) {
	root := buildSuggestTree(t)
	got := suggestDirectories(root, "notes", 0)
	if len(got) != 0 {
		t.Fatalf("files must not be suggested, got %v", got)
	}
}

func TestSuggestDirectoriesLimitClamp(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"d1", "d2", "d3", "d4", "d5"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if got := suggestDirectories(root, "", 3); len(got) != 3 {
		t.Fatalf("limit 3 should return 3, got %v", got)
	}
	// Negative limit falls back to the default, not a panic.
	if got := suggestDirectories(root, "", -1); len(got) == 0 {
		t.Fatalf("negative limit should fall back to default, got %v", got)
	}
}
