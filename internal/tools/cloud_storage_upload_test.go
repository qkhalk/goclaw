package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/cloud"
	"github.com/nextlevelbuilder/goclaw/internal/cloud/storage"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// fakeUploadProvider records UploadAccount calls for cloud_upload tests.
type fakeUploadProvider struct {
	uploaded   []string // "dir|name|remote"
	failUpload bool
}

func (f *fakeUploadProvider) AgentAccount(context.Context, string, cloud.AgentAccess) (*store.CloudAccount, error) {
	return &store.CloudAccount{ID: "acct-1", Provider: "s3", Email: "bucket@r2"}, nil
}
func (f *fakeUploadProvider) ListAccount(context.Context, *store.CloudAccount, string, int) ([]storage.ListEntry, error) {
	return nil, nil
}
func (f *fakeUploadProvider) AboutAccount(context.Context, *store.CloudAccount) (*storage.AboutInfo, error) {
	return &storage.AboutInfo{}, nil
}
func (f *fakeUploadProvider) FetchAccount(context.Context, *store.CloudAccount, string, string, int64) (string, error) {
	return "", nil
}
func (f *fakeUploadProvider) MkdirAccount(context.Context, *store.CloudAccount, string) error { return nil }
func (f *fakeUploadProvider) WriteAccount(context.Context, *store.CloudAccount, string, string) error {
	return nil
}
func (f *fakeUploadProvider) UploadAccount(_ context.Context, _ *store.CloudAccount, localDir, localName, remotePath string) error {
	if f.failUpload {
		return fmt.Errorf("provider boom")
	}
	f.uploaded = append(f.uploaded, localDir+"|"+localName+"|"+remotePath)
	return nil
}
func (f *fakeUploadProvider) CopyAccount(context.Context, *store.CloudAccount, string, string) error {
	return nil
}
func (f *fakeUploadProvider) MoveAccount(context.Context, *store.CloudAccount, string, string) error {
	return nil
}
func (f *fakeUploadProvider) DeleteAccount(context.Context, *store.CloudAccount, string, bool) error {
	return nil
}
func (f *fakeUploadProvider) PublicLinkAccount(context.Context, *store.CloudAccount, string) (*storage.PublicLinkInfo, error) {
	return &storage.PublicLinkInfo{}, nil
}

func newUploadTestTool(t *testing.T) (*cloudUploadTool, *fakeUploadProvider, string) {
	t.Helper()
	ws := t.TempDir()
	prov := &fakeUploadProvider{}
	return &cloudUploadTool{parent: NewCloudStorageTools(prov, ws, 1)}, prov, ws
}

func TestCloudUploadSuccess(t *testing.T) {
	tool, prov, ws := newUploadTestTool(t)
	local := filepath.Join(ws, "clip.mp4")
	if err := os.WriteFile(local, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := tool.Execute(context.Background(), map[string]any{
		"path":        "clip.mp4",
		"remote_path": "test/clip.mp4",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %v", res)
	}
	if len(prov.uploaded) != 1 {
		t.Fatalf("uploads = %v, want 1", prov.uploaded)
	}
	parts := strings.SplitN(prov.uploaded[0], "|", 3)
	if parts[0] != ws || parts[1] != "clip.mp4" || parts[2] != "test/clip.mp4" {
		t.Fatalf("upload args = %v", parts)
	}
}

func TestCloudUploadWorkspaceEscapeRejected(t *testing.T) {
	tool, prov, ws := newUploadTestTool(t)
	// A symlink inside the workspace pointing OUTSIDE must be rejected.
	outside := filepath.Join(t.TempDir(), "secret.mp4")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "evil.mp4")); err != nil {
		t.Skip("symlinks unavailable on this platform")
	}
	res := tool.Execute(context.Background(), map[string]any{
		"path":        "evil.mp4",
		"remote_path": "test/evil.mp4",
	})
	if !res.IsError {
		t.Fatal("symlink escape was not rejected")
	}
	if len(prov.uploaded) != 0 {
		t.Fatal("upload happened despite escape")
	}
}

func TestCloudUploadValidation(t *testing.T) {
	tool, prov, ws := newUploadTestTool(t)
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"missing path", map[string]any{"remote_path": "a/b"}, "path is required"},
		{"missing remote", map[string]any{"path": "a"}, "remote_path is required"},
		{"missing local file", map[string]any{"path": "nope.bin", "remote_path": "x/nope.bin"}, "not found"},
	}
	for _, tc := range cases {
		res := tool.Execute(context.Background(), tc.args)
		if !res.IsError || !strings.Contains(fmt.Sprint(res), tc.want) {
			t.Errorf("%s: res = %v, want error containing %q", tc.name, res, tc.want)
		}
	}
	// Directory instead of file.
	dir := filepath.Join(ws, "adir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	res := tool.Execute(context.Background(), map[string]any{"path": "adir", "remote_path": "x/adir"})
	if !res.IsError || !strings.Contains(fmt.Sprint(res), "not a regular file") {
		t.Errorf("dir upload: res = %v, want not-a-regular-file error", res)
	}
	// Oversize (cap is 1 MB in the fixture).
	big := filepath.Join(ws, "big.bin")
	if err := os.WriteFile(big, make([]byte, 1<<20+1), 0o644); err != nil {
		t.Fatal(err)
	}
	res = tool.Execute(context.Background(), map[string]any{"path": "big.bin", "remote_path": "x/big.bin"})
	if !res.IsError || !strings.Contains(fmt.Sprint(res), "upload cap") {
		t.Errorf("oversize: res = %v, want cap error", res)
	}
	if len(prov.uploaded) != 0 {
		t.Fatalf("uploads recorded during validation failures: %v", prov.uploaded)
	}
}
