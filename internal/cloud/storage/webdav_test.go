package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// davServer is a minimal in-memory WebDAV stub: one root collection with a
// fixed child set, enough semantics (PROPFIND/MKCOL/DELETE/MOVE/COPY/GET/PUT)
// for the backend contract tests below.
type davServer struct {
	mu sync.Mutex
	// dirs and files: path (no leading slash) -> exists
	dirs  map[string]bool
	files map[string]string // path -> body
	srv   *httptest.Server
}

func newDavServer(t *testing.T) *davServer {
	t.Helper()
	d := &davServer{
		dirs:  map[string]bool{"": true, "docs": true},
		files: map[string]string{"docs/a.txt": "hello", "b.txt": "world"},
	}
	d.srv = httptest.NewServer(http.HandlerFunc(d.handle))
	t.Cleanup(d.srv.Close)
	return d
}

// auth checks the Basic credentials of the request.
func (d *davServer) auth(w http.ResponseWriter, r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	if !ok || u != "alice" || p != "s3cret" {
		w.Header().Set("WWW-Authenticate", `Basic realm="dav"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (d *davServer) isDir(p string) bool { return d.dirs[strings.Trim(p, "/")] }

func (d *davServer) exists(p string) bool {
	p = strings.Trim(p, "/")
	return p == "" || d.dirs[p] || func() bool { _, ok := d.files[p]; return ok }()
}

// children lists direct children of a dir path.
func (d *davServer) children(dir string) []string {
	dir = strings.Trim(dir, "/")
	var out []string
	for p := range d.dirs {
		if p != "" && parentOf(p) == dir {
			out = append(out, p)
		}
	}
	for p := range d.files {
		if parentOf(p) == dir {
			out = append(out, p)
		}
	}
	return out
}

func parentOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

func (d *davServer) handle(w http.ResponseWriter, r *http.Request) {
	if !d.auth(w, r) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	p := r.URL.Path
	switch r.Method {
	case "PROPFIND":
		if !d.exists(p) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		depth := r.Header.Get("Depth")
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`)
		writeResp := func(href string, isDir bool, size int64) {
			b.WriteString(`<d:response><d:href>` + href + `</d:href><d:propstat><d:prop>`)
			if isDir {
				b.WriteString(`<d:resourcetype><d:collection/></d:resourcetype>`)
			} else {
				b.WriteString(`<d:resourcetype/><d:getcontentlength>` + davItoa(size) + `</d:getcontentlength><d:getcontenttype>text/plain</d:getcontenttype>`)
			}
			b.WriteString(`<d:getlastmodified>Mon, 02 Jan 2006 15:04:05 GMT</d:getlastmodified>`)
			b.WriteString(`</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
		}
		selfSize := int64(0)
		if body, ok := d.files[strings.Trim(p, "/")]; ok {
			selfSize = int64(len(body))
		}
		writeResp(p, d.isDir(p), selfSize)
		if depth == "1" {
			for _, c := range d.children(p) {
				href := "/" + c
				if _, isF := d.files[c]; isF {
					writeResp(href, false, int64(len(d.files[c])))
				} else {
					writeResp(href, true, 0)
				}
			}
		}
		b.WriteString(`</d:multistatus>`)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, b.String())
	case "MKCOL":
		if d.exists(p) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !d.exists(parentOf(strings.Trim(p, "/"))) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		d.dirs[strings.Trim(p, "/")] = true
		w.WriteHeader(http.StatusCreated)
	case "DELETE":
		if !d.exists(p) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if d.isDir(p) && len(d.children(p)) > 0 {
			// Real WebDAV would recurse; refuse to mask backend pre-checks.
			w.WriteHeader(http.StatusConflict)
			return
		}
		d.remove(p)
		w.WriteHeader(http.StatusNoContent)
	case "MOVE", "COPY":
		dest := r.Header.Get("Destination")
		if dest == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// The stub shares the mux's server: strip scheme://host.
		destPath := dest[len("http://"):]
		if i := strings.Index(destPath, "/"); i >= 0 {
			destPath = destPath[i:]
		} else {
			destPath = "/"
		}
		if !d.exists(p) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if d.isDir(p) {
			d.dirs[strings.Trim(destPath, "/")] = true
			if r.Method == "MOVE" {
				d.remove(p)
			}
		} else {
			src := strings.Trim(p, "/")
			body, ok := d.files[src]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			d.files[strings.Trim(destPath, "/")] = body
			if r.Method == "MOVE" {
				d.remove(p)
			}
		}
		w.WriteHeader(http.StatusCreated)
	case "GET":
		body, ok := d.files[strings.Trim(p, "/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if rg := r.Header.Get("Range"); rg == "bytes=0-1" {
			w.Header().Set("Content-Range", "bytes 0-1/"+davItoa(int64(len(body))))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, body[:2])
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, body)
	case "PUT":
		if d.isDir(p) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, _ := io.ReadAll(r.Body)
		d.files[strings.Trim(p, "/")] = string(body)
		w.WriteHeader(http.StatusCreated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (d *davServer) remove(p string) {
	p = strings.Trim(p, "/")
	delete(d.dirs, p)
	delete(d.files, p)
}

func davItoa(n int64) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func newWebDAVBackend(t *testing.T, d *davServer) *WebDAVBackend {
	t.Helper()
	return NewWebDAVBackend(context.Background(), WebDAVCreds{
		Endpoint: d.srv.URL, Username: "alice", Password: "s3cret",
	})
}

func TestWebDAVBackendList(t *testing.T) {
	d := newDavServer(t)
	b := newWebDAVBackend(t, d)
	entries, err := b.List(context.Background(), "/", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	// Root must show docs/ + b.txt and NOT itself.
	if len(names) != 2 || names[0] != "docs" || names[1] != "b.txt" {
		t.Fatalf("root list = %v, want [docs b.txt]", names)
	}
	sub, err := b.List(context.Background(), "/docs", 0)
	if err != nil {
		t.Fatalf("list docs: %v", err)
	}
	if len(sub) != 1 || sub[0].Name != "a.txt" || sub[0].Size != 5 {
		t.Fatalf("docs list = %+v, want a.txt size 5", sub)
	}
}

func TestWebDAVBackendListNotFound(t *testing.T) {
	d := newDavServer(t)
	b := newWebDAVBackend(t, d)
	if _, err := b.List(context.Background(), "/missing", 0); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWebDAVBackendStat(t *testing.T) {
	d := newDavServer(t)
	b := newWebDAVBackend(t, d)
	info, err := b.Stat(context.Background(), "/docs/a.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Name != "a.txt" || info.Size != 5 || info.IsDir || info.MimeType != "text/plain" {
		t.Fatalf("stat = %+v", info)
	}
	if info.ModTime == "" {
		t.Fatal("modtime not parsed")
	}
	if _, err := b.Stat(context.Background(), "/nope.txt"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWebDAVBackendMkdirDeleteCopyMove(t *testing.T) {
	d := newDavServer(t)
	ctx := context.Background()
	b := newWebDAVBackend(t, d)

	if err := b.Mkdir(ctx, "/newdir"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Idempotent mkdir (405 = exists) must not error.
	if err := b.Mkdir(ctx, "/newdir"); err != nil {
		t.Fatalf("mkdir idempotent: %v", err)
	}
	// Non-empty dir delete is refused with ErrDirNotEmpty.
	if err := b.Delete(ctx, "/docs", true); err != ErrDirNotEmpty {
		t.Fatalf("delete non-empty = %v, want ErrDirNotEmpty", err)
	}
	// Empty dir delete works.
	if err := b.Delete(ctx, "/newdir", true); err != nil {
		t.Fatalf("delete empty dir: %v", err)
	}
	// Copy a file.
	if err := b.Copy(ctx, "/docs/a.txt", "/copy.txt"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got := d.files["copy.txt"]; got != "hello" {
		t.Fatalf("copy body = %q", got)
	}
	// Move it.
	if err := b.Move(ctx, "/copy.txt", "/moved.txt"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, ok := d.files["copy.txt"]; ok {
		t.Fatal("move left the source behind")
	}
	if got := d.files["moved.txt"]; got != "hello" {
		t.Fatalf("moved body = %q", got)
	}
}

func TestWebDAVBackendOpenUpload(t *testing.T) {
	d := newDavServer(t)
	ctx := context.Background()
	b := newWebDAVBackend(t, d)

	// Full body.
	c, err := b.Open(ctx, "/docs/a.txt", "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	body, _ := io.ReadAll(c)
	_ = c.Close()
	if string(body) != "hello" || c.Status != http.StatusOK {
		t.Fatalf("open body=%q status=%d", body, c.Status)
	}
	// Range relay.
	rc, err := b.Open(ctx, "/docs/a.txt", "bytes=0-1")
	if err != nil {
		t.Fatalf("open range: %v", err)
	}
	rbody, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(rbody) != "he" || rc.Status != http.StatusPartialContent {
		t.Fatalf("range body=%q status=%d", rbody, rc.Status)
	}
	// Upload overwrites.
	if err := b.Upload(ctx, "/docs/a.txt", strings.NewReader("bye!"), 4, "text/plain", time.Time{}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if got := d.files["docs/a.txt"]; got != "bye!" {
		t.Fatalf("upload body = %q", got)
	}
}

func TestWebDAVBackendPublicLinkUnsupported(t *testing.T) {
	d := newDavServer(t)
	b := newWebDAVBackend(t, d)
	if _, err := b.PublicLink(context.Background(), "/b.txt"); err == nil {
		t.Fatal("publiclink should be unsupported")
	}
}

func TestWebDAVBackendBadAuth(t *testing.T) {
	d := newDavServer(t)
	b := NewWebDAVBackend(context.Background(), WebDAVCreds{
		Endpoint: d.srv.URL, Username: "alice", Password: "wrong",
	})
	if _, err := b.List(context.Background(), "/", 0); err == nil {
		t.Fatal("bad credentials must fail")
	}
}
