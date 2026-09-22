package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRCClientURLJoin guards the rc path join: do() must always request
// <base>/<path> — a missing separator produced
// "http://127.0.0.1:36123core/version" and broke the whole storage layer.
func TestRCClientURLJoin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/core/version") {
			t.Errorf("path = %q, want /core/version prefix", r.URL.Path)
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer srv.Close()

	// Deliberately pass a base without a trailing slash (the rcd supervisor
	// constructs it as "http://127.0.0.1:<port>").
	rc := NewRCClient(srv.URL, "u", "p")
	if _, err := rc.CoreVersion(context.Background()); err != nil {
		t.Fatalf("CoreVersion: %v", err)
	}
}

// TestRCClientConfigListRemotes guards the response shape: rclone wraps the
// names in {"remotes": [...]} — a bare []string decode fails on every call.
func TestRCClientConfigListRemotes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remotes":["goclaw-abc:","goclaw-def:"]}`))
	}))
	defer srv.Close()

	rc := NewRCClient(srv.URL, "u", "p")
	remotes, err := rc.ConfigListRemotes(context.Background())
	if err != nil {
		t.Fatalf("ConfigListRemotes: %v", err)
	}
	if len(remotes) != 2 || remotes[0] != "goclaw-abc:" {
		t.Fatalf("remotes = %v, want [goclaw-abc: goclaw-def:]", remotes)
	}
}

// --- Phase 4 wrapper tests: param shapes, auth, and error mapping against a
// fake rcd. The param shapes are load-bearing: rclone's rc handlers read
// fs/remote as separate params (rc.GetFsAndRemote) and sync/copy reads ONLY
// srcFs/dstFs — a wrapper that puts the path in the wrong key either 500s on
// file paths (fs.ErrorIsFile from the fs cache) or, worse, syncs the remote's
// entire root instead of the requested subtree.

// rcCapture records what the client actually sent to the fake rcd.
type rcCapture struct {
	url      string
	path     string
	user     string
	pass     string
	body     map[string]any
	rawBody  []byte
	status   int
	response string
}

// newFakeRCD starts an httptest server answering every rc call with the given
// status/JSON body and storing the last request in the returned capture.
func newFakeRCD(t *testing.T, cap *rcCapture, user, pass string) *RCClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cap.path = r.URL.Path
		cap.user, cap.pass, _ = r.BasicAuth()
		cap.rawBody = body
		cap.body = map[string]any{}
		_ = json.Unmarshal(body, &cap.body)
		u, p, _ := r.BasicAuth()
		if u != user || p != pass {
			cap.status = http.StatusUnauthorized
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		cap.status = http.StatusOK
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(cap.response))
	}))
	t.Cleanup(srv.Close)
	cap.url = srv.URL
	return NewRCClient(srv.URL, user, pass)
}

// TestRCClientSinglePathOpsParamShape pins fs/remote as separate params: the
// fs spec is the bare remote root ("<remote>:") and the path rides in
// "remote". Composing the path into "fs" makes rclone return fs.ErrorIsFile
// for file paths (rc.GetFsAndRemote → fs cache) — see the wrapper comments.
func TestRCClientSinglePathOpsParamShape(t *testing.T) {
	cases := []struct {
		name string
		call func(rc *RCClient) error
		op   string
	}{
		{"mkdir", func(rc *RCClient) error { return rc.OperationsMkdir(context.Background(), "goclaw-x", "dir/sub") }, "operations/mkdir"},
		{"rmdir", func(rc *RCClient) error { return rc.OperationsRmdir(context.Background(), "goclaw-x", "dir/empty") }, "operations/rmdir"},
		{"deletefile", func(rc *RCClient) error { return rc.OperationsDeleteFile(context.Background(), "goclaw-x", "dir/file.txt") }, "operations/deletefile"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &rcCapture{response: `{}`}
			rc := newFakeRCD(t, cap, "u", "p")
			if err := tc.call(rc); err != nil {
				t.Fatalf("%s: %v", tc.op, err)
			}
			if cap.path != "/"+tc.op {
				t.Fatalf("path = %q, want %q", cap.path, "/"+tc.op)
			}
			if got := cap.body["fs"]; got != "goclaw-x:" {
				t.Fatalf("fs = %v, want goclaw-x: (bare root — path in fs breaks file ops)", got)
			}
			wantRemote := map[string]string{
				"operations/mkdir": "dir/sub", "operations/rmdir": "dir/empty", "operations/deletefile": "dir/file.txt",
			}[tc.op]
			if got := cap.body["remote"]; got != wantRemote {
				t.Fatalf("remote = %v, want %q", got, wantRemote)
			}
		})
	}
}

// TestRCClientPublicLink decodes operations/publiclink's {"url": ...} reply.
func TestRCClientPublicLink(t *testing.T) {
	cap := &rcCapture{response: `{"url":"https://drive.google.com/uc?id=XYZ"}`}
	rc := newFakeRCD(t, cap, "u", "p")
	link, err := rc.OperationsPublicLink(context.Background(), "goclaw-x", "dir/report.pdf")
	if err != nil {
		t.Fatalf("OperationsPublicLink: %v", err)
	}
	if link.URL != "https://drive.google.com/uc?id=XYZ" {
		t.Fatalf("url = %q", link.URL)
	}
	if cap.body["fs"] != "goclaw-x:" || cap.body["remote"] != "dir/report.pdf" {
		t.Fatalf("params = %v, want fs=goclaw-x: remote=dir/report.pdf", cap.body)
	}
}

// TestRCClientOperationsCopyURL pins the copyurl shape: fs is the bare root,
// remote the FULL destination file path (including name), url verbatim.
func TestRCClientOperationsCopyURL(t *testing.T) {
	cap := &rcCapture{response: `{}`}
	rc := newFakeRCD(t, cap, "u", "p")
	err := rc.OperationsCopyURL(context.Background(), "https://example.com/a.png", "goclaw-x", "pics/a.png")
	if err != nil {
		t.Fatalf("OperationsCopyURL: %v", err)
	}
	if cap.path != "/operations/copyurl" {
		t.Fatalf("path = %q", cap.path)
	}
	if cap.body["url"] != "https://example.com/a.png" || cap.body["fs"] != "goclaw-x:" || cap.body["remote"] != "pics/a.png" {
		t.Fatalf("params = %v", cap.body)
	}
}

// TestRCClientPairOpsParamShape pins copyfile/movefile's four-param shape.
func TestRCClientPairOpsParamShape(t *testing.T) {
	cases := []struct {
		name string
		op   string
		call func(rc *RCClient) error
	}{
		{"copyfile", "operations/copyfile", func(rc *RCClient) error {
			return rc.OperationsCopyFile(context.Background(), "goclaw-x:", "a.txt", "/tmp/dst", "b.txt")
		}},
		{"movefile", "operations/movefile", func(rc *RCClient) error {
			return rc.OperationsMoveFile(context.Background(), "goclaw-x:", "a.txt", "goclaw-x:", "b/c.txt")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &rcCapture{response: `{}`}
			rc := newFakeRCD(t, cap, "u", "p")
			if err := tc.call(rc); err != nil {
				t.Fatalf("%s: %v", tc.op, err)
			}
			if cap.path != "/"+tc.op {
				t.Fatalf("path = %q, want %q", cap.path, "/"+tc.op)
			}
			want := map[string]string{
				"srcFs": "goclaw-x:", "srcRemote": "a.txt", "dstFs": "/tmp/dst", "dstRemote": "b.txt",
			}
			if tc.op == "operations/movefile" {
				want["dstFs"] = "goclaw-x:"
				want["dstRemote"] = "b/c.txt"
			}
			for k, v := range want {
				if cap.body[k] != v {
					t.Fatalf("%s = %v, want %q (body %v)", k, cap.body[k], v, cap.body)
				}
			}
		})
	}
}

// TestRCClientSyncCopyComposesPathsIntoFsSpecs is the regression guard for
// the worst possible wrapper bug: sync/copy reads ONLY srcFs/dstFs (rc.GetFsNamed)
// — if the paths were sent as srcRemote/dstRemote instead, rclone would
// silently sync the remote's ENTIRE root to the destination.
func TestRCClientSyncCopyComposesPathsIntoFsSpecs(t *testing.T) {
	cap := &rcCapture{response: `{"jobid": 42}`}
	rc := newFakeRCD(t, cap, "u", "p")
	jobID, err := rc.SyncCopy(context.Background(), "goclaw-src", "docs/folder", "goclaw-dst", "backup/folder", true)
	if err != nil {
		t.Fatalf("SyncCopy: %v", err)
	}
	if jobID != 42 {
		t.Fatalf("jobid = %d, want 42", jobID)
	}
	if cap.path != "/sync/copy" {
		t.Fatalf("path = %q", cap.path)
	}
	if got := cap.body["srcFs"]; got != "goclaw-src:docs/folder" {
		t.Fatalf("srcFs = %v, want goclaw-src:docs/folder", got)
	}
	if got := cap.body["dstFs"]; got != "goclaw-dst:backup/folder" {
		t.Fatalf("dstFs = %v, want goclaw-dst:backup/folder", got)
	}
	if cap.body["_async"] != true {
		t.Fatalf("_async = %v, want true", cap.body["_async"])
	}
	if _, ok := cap.body["srcRemote"]; ok {
		t.Fatalf("srcRemote must not be sent (ignored by sync/copy → whole-root sync): %v", cap.body)
	}
	if _, ok := cap.body["dstRemote"]; ok {
		t.Fatalf("dstRemote must not be sent (ignored by sync/copy → whole-root sync): %v", cap.body)
	}
}

// TestRCClientSyncCopySync returns 0 for a synchronous call.
func TestRCClientSyncCopySync(t *testing.T) {
	cap := &rcCapture{response: `{}`}
	rc := newFakeRCD(t, cap, "u", "p")
	jobID, err := rc.SyncCopy(context.Background(), "goclaw-src", "f", "goclaw-dst", "f", false)
	if err != nil {
		t.Fatalf("SyncCopy: %v", err)
	}
	if jobID != 0 {
		t.Fatalf("jobid = %d, want 0 (synchronous)", jobID)
	}
	if cap.body["_async"] != false {
		t.Fatalf("_async = %v, want false", cap.body["_async"])
	}
}

// TestRCClientJobStatus decodes job/status (fields rclone actually emits).
func TestRCClientJobStatus(t *testing.T) {
	cap := &rcCapture{response: `{"id":7,"finished":true,"success":false,"error":"boom","duration":1.5}`}
	rc := newFakeRCD(t, cap, "u", "p")
	job, err := rc.JobStatus(context.Background(), 7)
	if err != nil {
		t.Fatalf("JobStatus: %v", err)
	}
	if job.ID != 7 || !job.Finished || job.Success || job.Error != "boom" {
		t.Fatalf("job = %+v", job)
	}
	if got := cap.body["jobid"]; got != float64(7) {
		t.Fatalf("jobid param = %v, want 7", got)
	}
}

// TestRCClientStatFileShape guards operations/stat for FILES: the path must
// ride in "remote", not in "fs" — with it inside fs, rclone's fs cache fails
// the request with fs.ErrorIsFile before stat runs.
func TestRCClientStatFileShape(t *testing.T) {
	cap := &rcCapture{response: `{"item":{"Name":"f.txt","Size":3,"IsDir":false}}`}
	rc := newFakeRCD(t, cap, "u", "p")
	info, err := rc.OperationsStat(context.Background(), "goclaw-x", "dir/f.txt")
	if err != nil {
		t.Fatalf("OperationsStat: %v", err)
	}
	if info.Size != 3 || info.IsDir {
		t.Fatalf("info = %+v", info)
	}
	if cap.body["fs"] != "goclaw-x:" || cap.body["remote"] != "dir/f.txt" {
		t.Fatalf("params = %v, want fs=goclaw-x: remote=dir/f.txt", cap.body)
	}
}

// TestRCClientBasicAuthEnforced: wrong credentials → 401 → error.
func TestRCClientBasicAuthEnforced(t *testing.T) {
	cap := &rcCapture{response: `{}`}
	newFakeRCD(t, cap, "u", "p")
	wrong := NewRCClient(cap.url, "u", "wrong-pass")
	if err := wrong.OperationsMkdir(context.Background(), "goclaw-x", "d"); err == nil {
		t.Fatal("expected auth error, got nil")
	}
}

// TestRCClientErrorBodyTruncation: status != 200 → error carries a truncated
// body snippet (do() truncates at 200 bytes).
func TestRCClientErrorBodyTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", 500)))
	}))
	defer srv.Close()
	rc := NewRCClient(srv.URL, "u", "p")
	err := rc.OperationsDeleteFile(context.Background(), "goclaw-x", "f.txt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "status 500") {
		t.Fatalf("error %q missing status", msg)
	}
	if len(msg) > 400 {
		t.Fatalf("error %q not truncated (%d bytes)", msg, len(msg))
	}
}
