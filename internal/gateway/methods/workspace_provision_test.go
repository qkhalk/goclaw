package methods

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProvisionWorkspaceRoot_CreatesNested(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws", "nested", "root")
	if err := provisionWorkspaceRoot(root); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected directory at %s, got err=%v", root, err)
	}
}

func TestProvisionWorkspaceRoot_ExistingDirPasses(t *testing.T) {
	root := t.TempDir()
	// Pre-existing content must survive.
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := provisionWorkspaceRoot(root); err != nil {
		t.Fatalf("expected success on existing dir, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); err != nil {
		t.Fatalf("existing content removed: %v", err)
	}
}

func TestProvisionWorkspaceRoot_BlockedByFileParent(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "file")
	if err := os.WriteFile(blocker, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(blocker, "child")
	if err := provisionWorkspaceRoot(root); err == nil {
		t.Fatal("expected error when a path parent is a regular file")
	}
}
