// Package mail implements a thin Gmail API client (list/search/get/modify)
// used by the cloud agent tools. Deliberately narrow: no permanent-delete
// path exists anywhere in this package — archive/trash are label operations.
package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const gmailBase = "https://gmail.googleapis.com/gmail/v1/users/me"

// MessageSummary is the compact search-result shape handed to the model.
type MessageSummary struct {
	ID       string `json:"id"`
	ThreadID string `json:"thread_id"`
	From     string `json:"from"`
	Subject  string `json:"subject"`
	Date     string `json:"date"`
	Snippet  string `json:"snippet"`
}

// MessageDetail is the full read shape: body text (truncated by the caller)
// plus attachment listing.
type MessageDetail struct {
	MessageSummary
	To                   string            `json:"to"`
	Labels               []string          `json:"labels"`
	Body                 string            `json:"body"`
	Attachments          []AttachmentInfo  `json:"attachments"`
	ListUnsubscribe      string            `json:"list_unsubscribe,omitempty"`
	ListUnsubscribePost  string            `json:"list_unsubscribe_post,omitempty"`
}

// AttachmentInfo lists (never downloads) an attachment.
type AttachmentInfo struct {
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	Size     int64  `json:"size"`
}

// Client is a Gmail API client bound to one account's token source.
type Client struct {
	httpClient *http.Client
}

// NewClient builds a Gmail client for an OAuth token source.
func NewClient(ts oauth2.TokenSource) *Client {
	return &Client{httpClient: oauth2.NewClient(context.Background(), ts)}
}

// GetProfile returns the account's email address (connectivity test).
func (c *Client) GetProfile(ctx context.Context) (email string, err error) {
	var out struct {
		EmailAddress string `json:"emailAddress"`
	}
	err = c.get(ctx, "/profile", &out)
	return out.EmailAddress, err
}

// Search lists message summaries matching the Gmail search query
// (same syntax as the Gmail search box, e.g. "from:x is:unread newer_than:1d").
func (c *Client) Search(ctx context.Context, query string, maxResults int, pageToken string) ([]MessageSummary, string, error) {
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 25
	}
	q := url.Values{"q": {query}, "maxResults": {fmt.Sprintf("%d", maxResults)}}
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	var wl struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := c.get(ctx, "/messages?"+q.Encode(), &wl); err != nil {
		return nil, "", err
	}
	summaries := make([]MessageSummary, 0, len(wl.Messages))
	for _, m := range wl.Messages {
		// metadata format: enough for the summary (headers), avoids N full-body downloads.
		detail, err := c.Get(ctx, m.ID, true)
		if err != nil {
			continue // skip messages that vanished between list and get
		}
		summaries = append(summaries, detail.MessageSummary)
	}
	return summaries, wl.NextPageToken, nil
}

// Get fetches one message. metadataOnly skips body download (unsubscribe
// analysis only needs headers).
func (c *Client) Get(ctx context.Context, id string, metadataOnly bool) (*MessageDetail, error) {
	path := "/messages/" + url.PathEscape(id)
	if metadataOnly {
		path += "?format=metadata"
	}
	var raw struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
		Snippet  string `json:"snippet"`
		LabelIDs []string `json:"labelIds"`
		Payload  struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			MimeType string       `json:"mimeType"`
			Body     rawBody      `json:"body"`
			Parts    []rawPart    `json:"parts"`
		} `json:"payload"`
	}
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}

	detail := &MessageDetail{Labels: raw.LabelIDs}
	detail.ID = raw.ID
	detail.ThreadID = raw.ThreadID
	detail.Snippet = raw.Snippet

	headers := map[string]string{}
	for _, h := range raw.Payload.Headers {
		headers[strings.ToLower(h.Name)] = h.Value
	}
	detail.From = headers["from"]
	detail.To = headers["to"]
	detail.Subject = headers["subject"]
	detail.Date = headers["date"]
	detail.ListUnsubscribe = headers["list-unsubscribe"]
	detail.ListUnsubscribePost = headers["list-unsubscribe-post"]

	if !metadataOnly {
		detail.Body = extractTextBody(raw.Payload.MimeType, raw.Payload.Body.Data, raw.Payload.Parts, 0)
		for _, p := range collectAttachmentParts(raw.Payload.Parts) {
			detail.Attachments = append(detail.Attachments, AttachmentInfo{
				Filename: p.Filename, MIME: p.MimeType, Size: p.Body.Size,
			})
		}
	}
	return detail, nil
}

// Modify applies label add/remove operations (archive = remove INBOX,
// trash = add TRASH, mark_read = remove UNREAD).
func (c *Client) Modify(ctx context.Context, id string, addLabelIDs, removeLabelIDs []string) error {
	body := map[string][]string{}
	if len(addLabelIDs) > 0 {
		body["addLabelIds"] = addLabelIDs
	}
	if len(removeLabelIDs) > 0 {
		body["removeLabelIds"] = removeLabelIDs
	}
	return c.post(ctx, "/messages/"+url.PathEscape(id)+"/modify", body, nil)
}

// ListLabels returns user labels (id + name).
func (c *Client) ListLabels(ctx context.Context) ([]map[string]string, error) {
	var out struct {
		Labels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := c.get(ctx, "/labels", &out); err != nil {
		return nil, err
	}
	labels := make([]map[string]string, 0, len(out.Labels))
	for _, l := range out.Labels {
		labels = append(labels, map[string]string{"id": l.ID, "name": l.Name})
	}
	return labels, nil
}

// --- HTTP plumbing ---

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gmailBase+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailBase+path, strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return &RateError{Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gmail api status %d: %s", resp.StatusCode, truncateStr(string(data), 200))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// RateError signals retryable throttling/upstream errors.
type RateError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *RateError) Error() string { return fmt.Sprintf("gmail api rate/status %d", e.Status) }

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	var secs int
	if _, err := fmt.Sscanf(v, "%d", &secs); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// --- MIME body extraction ---

type rawBody struct {
	Data string `json:"data"`
}

type rawPart struct {
	MimeType string     `json:"mimeType"`
	Filename string     `json:"filename"`
	Body     rawBodyExt `json:"body"`
	Parts    []rawPart  `json:"parts"`
}

type rawBodyExt struct {
	Data string `json:"data"`
	Size int64  `json:"size"`
}

const maxBodyDepth = 8

// extractTextBody prefers text/plain, falls back to text/html stripped of
// tags; nested multiparts are walked with a depth bound.
func extractTextBody(mimeType, bodyData string, parts []rawPart, depth int) string {
	if depth > maxBodyDepth {
		return ""
	}
	if bodyData != "" {
		if strings.HasPrefix(mimeType, "text/plain") {
			return decodeBodyData(mimeType, bodyData)
		}
		if len(parts) == 0 && strings.HasPrefix(mimeType, "text/html") {
			return StripHTML(decodeBodyData(mimeType, bodyData))
		}
	}
	for i := range parts {
		if got := extractTextBody(parts[i].MimeType, parts[i].Body.Data, parts[i].Parts, depth+1); got != "" {
			return got
		}
	}
	return ""
}

func collectAttachmentParts(parts []rawPart) []rawPart {
	var out []rawPart
	for _, p := range parts {
		if p.Filename != "" {
			out = append(out, p)
		}
		out = append(out, collectAttachmentParts(p.Parts)...)
	}
	return out
}

// decodeBodyData handles Gmail's base64url encoding and quoted-printable /
// charset transfer encodings.
func decodeBodyData(mimeType, data string) string {
	raw, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(data)
	if err != nil {
		// Some responses use standard base64 padding.
		if raw2, err2 := base64.StdEncoding.DecodeString(data); err2 == nil {
			raw = raw2
		} else {
			return ""
		}
	}
	text := string(raw)
	if strings.Contains(mimeType, "quoted-printable") {
		if decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(text))); err == nil {
			text = string(decoded)
		}
	}
	if cs, _, err := mime.ParseMediaType(mimeType); err == nil && cs != "" &&
		!strings.EqualFold(cs, "utf-8") && !strings.EqualFold(cs, "us-ascii") {
		if enc, ok := charsetDecoders[strings.ToLower(cs)]; ok {
			text = enc(text)
		}
	}
	return text
}

// charsetDecoders holds the few legacy charsets worth supporting; unknown
// charsets fall through as-is (fail-open to readable-ish text).
var charsetDecoders = map[string]func(string) string{
	"iso-8859-1": func(s string) string { return decodeLatin(s) },
	"windows-1252": func(s string) string { return decodeLatin(s) },
}

func decodeLatin(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		b[i] = s[i]
	}
	return string(b) // Go strings are byte-safe; Gmail mostly serves UTF-8 anyway
}

// StripHTML removes tags and collapses whitespace for a readable plain-text
// fallback of HTML-only bodies.
func StripHTML(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	inTag, inEntity := false, false
	var entity strings.Builder
	for _, r := range s {
		switch {
		case inTag:
			if r == '>' {
				inTag = false
				// A tag boundary acts as a word boundary in rendered HTML.
				sb.WriteRune(' ')
			}
		case r == '<':
			inTag = true
		case r == '&':
			inEntity = true
			entity.Reset()
		case inEntity:
			if r == ';' {
				inEntity = false
				sb.WriteRune(entityToRune(entity.String()))
			} else if entity.Len() < 10 {
				entity.WriteRune(r)
			} else {
				inEntity = false
				sb.WriteString("&" + entity.String())
			}
		default:
			sb.WriteRune(r)
		}
	}
	out := sb.String()
	out = strings.Join(strings.Fields(out), " ")
	return out
}

func entityToRune(name string) rune {
	switch strings.ToLower(name) {
	case "amp":
		return '&'
	case "lt":
		return '<'
	case "gt":
		return '>'
	case "quot":
		return '"'
	case "nbsp":
		return ' '
	}
	return ' '
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var ErrNotFound = errors.New("gmail: message not found")
