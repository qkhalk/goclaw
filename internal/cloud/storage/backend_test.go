package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// staticTS returns a token source with a fixed access token.
func staticTS() oauth2.TokenSource {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
}

func TestDriveListAndResolve(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch q := r.URL.Query().Get("q"); {
		case strings.Contains(q, "mimeType = 'application/vnd.google-apps.folder'"):
			// Walk step: resolve one folder segment under its parent.
			queries = append(queries, q)
			writeJSON(t, w, map[string]any{"files": []driveFile{{
				ID: "folder-1", Name: "Docs", MimeType: driveFolderMime,
			}}})
		case q != "":
			// Children listing of the resolved folder.
			writeJSON(t, w, map[string]any{"files": []driveFile{
				{ID: "f-1", Name: "a.txt", MimeType: "text/plain", Size: "12", ModifiedTime: "2026-01-02T03:04:05Z"},
				{ID: "d-2", Name: "sub", MimeType: driveFolderMime},
			}})
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	backup := driveAPIBase
	driveAPIBase = srv.URL
	defer func() { driveAPIBase = backup }()

	d := NewDriveBackend(context.Background(), staticTS())
	entries, err := d.List(context.Background(), "Docs", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 || entries[0].Name != "a.txt" || entries[0].Size != 12 || !entries[1].IsDir {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].ModTime != "2026-01-02T03:04:05Z" {
		t.Fatalf("modtime = %q", entries[0].ModTime)
	}
	if len(queries) == 0 || !strings.Contains(queries[0], "'root' in parents") {
		t.Fatalf("first query = %v, want root-parented walk", queries)
	}

	// A missing path maps to ErrNotFound.
	srvNotFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srvNotFound.Close()
	driveAPIBase = srvNotFound.URL
	defer func() { driveAPIBase = backup }()
	if _, err := d.List(context.Background(), "Missing", 10); err != ErrNotFound {
		t.Fatalf("missing dir err = %v, want ErrNotFound", err)
	}
}

func TestDriveUploadSimpleMultipart(t *testing.T) {
	var gotBody []byte
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		writeJSON(t, w, map[string]any{"id": "new-1"})
	}))
	defer srv.Close()

	backupAPI, backupUpload := driveAPIBase, driveUploadBase
	driveAPIBase, driveUploadBase = srv.URL, srv.URL
	defer func() { driveAPIBase, driveUploadBase = backupAPI, backupUpload }()

	// The overwrite pre-check hits the API base (empty result → create path).
	d := NewDriveBackend(context.Background(), staticTS())
	if err := d.Upload(context.Background(), "hello.txt", strings.NewReader("hi drive"), 8, "text/plain", time.Time{}); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !strings.Contains(gotCT, "multipart/related") {
		t.Fatalf("content type = %q, want multipart/related", gotCT)
	}
	if !strings.Contains(string(gotBody), "hi drive") || !strings.Contains(string(gotBody), `"name":"hello.txt"`) {
		t.Fatalf("multipart body = %q", gotBody)
	}
}

func TestGraphPathEscapingAndList(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI()) // raw (escaped) request form
		item := graphItem{}
		item.ID = "i-1"
		item.Name = "pic.png"
		item.Size = 5
		item.LastModifiedDateTime = "2026-03-04T05:06:07.1234567Z"
		item.File = &graphFile{MimeType: "image/png"}
		writeJSON(t, w, map[string]any{"value": []graphItem{item}})
	}))
	defer srv.Close()

	g := NewGraphBackend(context.Background(), staticTS(), "drive-1")
	g.base = srv.URL + "/drives/drive-1"

	// Hash in a folder name must be percent-encoded, slashes kept as separators.
	entries, err := g.List(context.Background(), "a#b/c d", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].IsDir || entries[0].ModTime != "2026-03-04T05:06:07Z" {
		t.Fatalf("entries = %+v", entries)
	}
	if !strings.Contains(paths[0], "root:/a%23b/c%20d:/children") {
		t.Fatalf("children path = %q", paths[0])
	}
}

func TestGraphUploadSimplePut(t *testing.T) {
	var body []byte
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		body, _ = io.ReadAll(r.Body)
		writeJSON(t, w, map[string]any{"id": "u-1"})
	}))
	defer srv.Close()

	g := NewGraphBackend(context.Background(), staticTS(), "drive-1")
	g.base = srv.URL + "/drives/drive-1"
	if err := g.Upload(context.Background(), "docs/note.txt", strings.NewReader("hello od"), 8, "text/plain", time.Time{}); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if string(body) != "hello od" {
		t.Fatalf("body = %q", body)
	}
	if !strings.Contains(query, "conflictBehavior=replace") {
		t.Fatalf("query = %q, want conflictBehavior=replace", query)
	}
}

func TestGraphUploadSessionChunking(t *testing.T) {
	var ranges []string
	var chunkURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createUploadSession") {
			writeJSON(t, w, map[string]any{"uploadUrl": chunkURL})
			return
		}
		ranges = append(ranges, r.Header.Get("Content-Range"))
		if cr := r.Header.Get("Content-Range"); strings.HasSuffix(cr, "/21") {
			writeJSON(t, w, map[string]any{"id": "done"}) // completion response
			return
		}
		w.WriteHeader(http.StatusAccepted)
		writeJSON(t, w, map[string]any{"nextExpectedRanges": []string{}})
	}))
	defer srv.Close()
	chunkURL = srv.URL + "/chunk"

	g := NewGraphBackend(context.Background(), staticTS(), "drive-1")
	g.base = srv.URL + "/drives/drive-1"

	// 5 MiB + 1 byte: above the simple-PUT cap → upload session; a 10 MiB chunk
	// buffer shrinks to the body, and the single chunk carries /total.
	total := int64(5<<20) + 1
	body := strings.Repeat("x", int(total))
	if err := g.Upload(context.Background(), "big.bin", strings.NewReader(body), total, "application/octet-stream", time.Time{}); err != nil {
		t.Fatalf("Upload session: %v", err)
	}
	if len(ranges) == 0 || !strings.HasSuffix(ranges[len(ranges)-1], "/"+itoa(total)) {
		t.Fatalf("ranges = %v, want final chunk with /%d", ranges, total)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
