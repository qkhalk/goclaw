package storage

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// WebDAVBackend implements Backend against a generic WebDAV server
// (Nextcloud, ownCloud, Synology, Apache/Nginx DAV, ...). Auth is HTTP Basic
// over the user-supplied endpoint, so — like S3Backend — the OAuth
// TokenSource plumbing of the other backends is unused.
//
// RFC 4918 semantics worth noting:
//   - DELETE on a collection deletes RECURSIVELY, so Delete pre-checks
//     Depth-1 for children to preserve the house "only empty dirs" rule.
//   - PUT replaces any existing resource, so Upload overwrites by default.
//   - MOVE/COPY overwrite the Destination only when the Overwrite header
//     says so — it is sent explicitly (T) to match the replace semantics.
type WebDAVBackend struct {
	endpoint string // e.g. https://cloud.example.com/remote.php/dav/files/alice
	username string
	password string
	client   *http.Client
}

// Compile-time guard.
var _ Backend = (*WebDAVBackend)(nil)

// WebDAVCreds carries the static configuration of one WebDAV account.
type WebDAVCreds struct {
	Endpoint string // collection URL that acts as the account root
	Username string
	Password string
}

// NewWebDAVBackend builds a WebDAV backend rooted at the endpoint collection.
func NewWebDAVBackend(_ context.Context, creds WebDAVCreds) *WebDAVBackend {
	return &WebDAVBackend{
		endpoint: strings.TrimRight(creds.Endpoint, "/"),
		username: creds.Username,
		password: creds.Password,
		client:   &http.Client{},
	}
}

// davProp is one file/collection entry of a multistatus response.
type davProp struct {
	Href          string `xml:"href"`
	Displayname   string `xml:"displayname"`
	ContentLength int64  `xml:"propstat>prop>getcontentlength"`
	ContentType   string `xml:"propstat>prop>getcontenttype"`
	LastModified  string `xml:"propstat>prop>getlastmodified"`
	Collection    struct {
		XMLName xml.Name
	} `xml:"propstat>prop>resourcetype>collection"`
}

type davMultistatus struct {
	Responses []davProp `xml:"response"`
}

// davURL joins the endpoint with our "/"-rooted path. Each segment is
// percent-encoded; the endpoint is taken as-is (it may already contain a
// encoded base path).
func (b *WebDAVBackend) davURL(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	parts := make([]string, 0, len(segs))
	for _, s := range segs {
		if s == "" {
			continue
		}
		parts = append(parts, url.PathEscape(s))
	}
	return b.endpoint + "/" + strings.Join(parts, "/")
}

// davReq builds an authenticated request with the standard DAV headers.
func (b *WebDAVBackend) davReq(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.davURL(path), body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(b.username, b.password)
	req.Header.Set("User-Agent", "GoClaw")
	return req, nil
}

// davDo runs the request and maps 404 to ErrNotFound.
func (b *WebDAVBackend) davDo(req *http.Request) (*http.Response, error) {
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		drainClose(resp)
		return nil, ErrNotFound
	}
	return resp, nil
}

func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

// propfind runs a PROPFIND and decodes the multistatus body.
func (b *WebDAVBackend) propfind(ctx context.Context, path string, depth string) (*davMultistatus, error) {
	req, err := b.davReq(ctx, "PROPFIND", path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", depth)
	req.Header.Set("Content-Type", "application/xml")
	// An allprop body keeps request-size tiny; servers fall back to allprop
	// on an empty body anyway, but being explicit avoids Nextcloud quirks.
	req.Body = io.NopCloser(strings.NewReader(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:allprop/></d:propfind>`))
	resp, err := b.davDo(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		drainClose(resp)
		return nil, fmt.Errorf("webdav propfind %s: unexpected status %d", path, resp.StatusCode)
	}
	var ms davMultistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, fmt.Errorf("webdav propfind %s: decode: %w", path, err)
	}
	return &ms, nil
}

// davEntry converts one multistatus response into a ListEntry. The name is
// taken from the decoded href (displayname is unreliable across servers).
func davEntry(href, fallback string, p davProp) ListEntry {
	name := fallback
	if href != "" {
		if u, err := url.Parse(href); err == nil {
			h := u.Path
			h = strings.TrimSuffix(h, "/")
			if idx := strings.LastIndex(h, "/"); idx >= 0 {
				h = h[idx+1:]
			}
			if dec, err := url.PathUnescape(h); err == nil && dec != "" {
				name = dec
			}
		}
	}
	mod := ""
	if t, err := http.ParseTime(p.LastModified); err == nil {
		mod = t.UTC().Format(time.RFC3339)
	}
	return ListEntry{
		Name:    name,
		IsDir:   p.Collection.XMLName.Local == "collection",
		Size:    p.ContentLength,
		ModTime: mod,
	}
}

// List lists one directory level (Depth:1 — no recursive listing).
func (b *WebDAVBackend) List(ctx context.Context, dir string, limit int) ([]ListEntry, error) {
	if limit <= 0 {
		limit = 1000
	}
	ms, err := b.propfind(ctx, dir, "1")
	if err != nil {
		return nil, err
	}
	out := make([]ListEntry, 0, len(ms.Responses))
	// The first response is the requested collection itself — skip it.
	for i, p := range ms.Responses {
		if i == 0 && isSelf(p, dir) {
			continue
		}
		out = append(out, davEntry(p.Href, "", p))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func davUnescape(href string) string {
	if u, err := url.Parse(href); err == nil {
		return u.Path
	}
	return href
}

// isSelf reports whether a multistatus entry is the requested collection
// itself (href decodes to the request path, modulo trailing slash).
func isSelf(p davProp, path string) bool {
	h := strings.TrimSuffix(davUnescape(p.Href), "/")
	want := "/" + strings.Trim(path, "/")
	if want == "/" {
		return h == "" || h == "/"
	}
	return h == want
}

// Stat stats one path (Depth:0, single response).
func (b *WebDAVBackend) Stat(ctx context.Context, path string) (*StatInfo, error) {
	ms, err := b.propfind(ctx, path, "0")
	if err != nil {
		return nil, err
	}
	if len(ms.Responses) == 0 {
		return nil, ErrNotFound
	}
	p := ms.Responses[0]
	e := davEntry(p.Href, "", p)
	info := &StatInfo{
		Name:     e.Name,
		Size:     e.Size,
		ModTime:  e.ModTime,
		IsDir:    e.IsDir,
		MimeType: p.ContentType,
	}
	if info.IsDir {
		info.MimeType = ""
	}
	return info, nil
}

// About returns zeroed quota — generic WebDAV has no quota property.
func (b *WebDAVBackend) About(ctx context.Context) (*AboutInfo, error) {
	return &AboutInfo{}, nil
}

// Mkdir creates a collection (MKCOL; parents must exist per RFC 4918).
func (b *WebDAVBackend) Mkdir(ctx context.Context, dir string) error {
	req, err := b.davReq(ctx, "MKCOL", dir, nil)
	if err != nil {
		return err
	}
	resp, err := b.davDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusMethodNotAllowed { // 405 = already exists
		return nil // idempotent mkdir parity
	}
	if resp.StatusCode/100 != 2 {
		drainClose(resp)
		return fmt.Errorf("webdav mkdir %s: status %d", dir, resp.StatusCode)
	}
	return nil
}

// Delete removes a file or an EMPTY directory (WebDAV DELETE recurses, so a
// Depth-1 pre-check preserves the "only empty dirs" house rule).
func (b *WebDAVBackend) Delete(ctx context.Context, path string, isDir bool) error {
	if isDir {
		ms, err := b.propfind(ctx, path, "1")
		if err != nil {
			return err
		}
		if len(ms.Responses) > 1 {
			return ErrDirNotEmpty
		}
	}
	req, err := b.davReq(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}
	resp, err := b.davDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		drainClose(resp)
		return fmt.Errorf("webdav delete %s: status %d", path, resp.StatusCode)
	}
	return nil
}

// davCopyMove runs COPY or MOVE. Per RFC 4918 the request targets the SOURCE
// URL and the Destination header names the target (absolute URL).
func (b *WebDAVBackend) davCopyMove(ctx context.Context, method, from, to string) error {
	req, err := b.davReq(ctx, method, from, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Destination", b.davURL(to))
	req.Header.Set("Overwrite", "T")
	resp, err := b.davDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		drainClose(resp)
		return fmt.Errorf("webdav %s %s -> %s: status %d", strings.ToLower(method), from, to, resp.StatusCode)
	}
	return nil
}

// Move renames/moves via the WebDAV MOVE method.
func (b *WebDAVBackend) Move(ctx context.Context, from, to string) error {
	return b.davCopyMove(ctx, "MOVE", from, to)
}

// Copy copies a file server-side via the WebDAV COPY method.
func (b *WebDAVBackend) Copy(ctx context.Context, from, to string) error {
	return b.davCopyMove(ctx, "COPY", from, to)
}

// CopyURL fetches an http(s) URL through the SSRF guard and uploads the body.
func (b *WebDAVBackend) CopyURL(ctx context.Context, rawURL, path string) error {
	if !trimURLScheme(rawURL) {
		return errors.New("webdav copyurl: only http(s) URLs are supported")
	}
	resp, err := safeGet(ctx, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, size, err := spoolUnknownSize(resp.Body)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	return b.Upload(ctx, path, f, size, resp.Header.Get("Content-Type"), time.Time{})
}

// PublicLink is not supported — generic WebDAV has no share-link API.
func (b *WebDAVBackend) PublicLink(ctx context.Context, path string) (*PublicLinkInfo, error) {
	return nil, fmt.Errorf("webdav: share links are not supported by WebDAV servers")
}

// Open GETs the file, relaying the Range header when set.
func (b *WebDAVBackend) Open(ctx context.Context, path string, rangeHdr RangeHeader) (*Content, error) {
	req, err := b.davReq(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if rangeHdr != "" {
		req.Header.Set("Range", string(rangeHdr))
	}
	resp, err := b.davDo(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		drainClose(resp)
		return nil, fmt.Errorf("webdav open %s: status %d", path, resp.StatusCode)
	}
	status := http.StatusOK
	if resp.StatusCode == http.StatusPartialContent {
		status = http.StatusPartialContent
	}
	length := resp.ContentLength
	return &Content{
		ReadCloser:   resp.Body,
		Status:       status,
		Length:       length,
		MimeType:     resp.Header.Get("Content-Type"),
		ContentRange: resp.Header.Get("Content-Range"),
	}, nil
}

// Upload PUTs the body (RFC 4918: PUT replaces an existing resource; generic
// WebDAV has no client-controlled mtime, so modTime is accepted but not
// persisted — sync mirrors fall back to size comparison).
func (b *WebDAVBackend) Upload(ctx context.Context, path string, src io.Reader, size int64, contentType string, _ time.Time) error {
	req, err := b.davReq(ctx, "PUT", path, src)
	if err != nil {
		return err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := b.davDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		drainClose(resp)
		return fmt.Errorf("webdav upload %s: status %d", path, resp.StatusCode)
	}
	return nil
}
