package mail

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func b64(s string) string {
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(s))
}

// newFakeGmail spins an httptest server answering Gmail reads; it records every
// request so tests can assert exactly which endpoints were hit.
func newFakeGmail(t *testing.T, messageJSON string) (*Client, *[]http.Request, *httptest.Server) {
	t.Helper()
	var calls []http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, *r)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, messageJSON)
	}))
	t.Cleanup(srv.Close)

	// Point the client at the fake server by monkey-patching the transport
	// host: simplest is a token source-less client whose transport rewrites
	// the URL host.
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.Host = ""
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		resp.Request = req
		return resp, nil
	})}
	return &Client{httpClient: client}, &calls, srv
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const sampleMessage = `{
  "id": "msg-1",
  "threadId": "th-1",
  "snippet": "cheap deals",
  "labelIds": ["INBOX", "UNREAD", "CATEGORY_PROMOTIONS"],
  "payload": {
    "mimeType": "multipart/alternative",
    "headers": [
      {"name": "From", "value": "News <news@example.com>"},
      {"name": "To", "value": "me@gmail.com"},
      {"name": "Subject", "value": "Weekly deals"},
      {"name": "Date", "value": "Fri, 12 Sep 2026 09:00:00 +0700"},
      {"name": "List-Unsubscribe", "value": "<https://example.com/unsub?u=1>, <mailto:unsub@example.com>"},
      {"name": "List-Unsubscribe-Post", "value": "List-Unsubscribe=One-Click"}
    ],
    "parts": [
      {"mimeType": "text/plain", "body": {"data": "` + "PLAINBODY" + `", "size": 9}},
      {"mimeType": "text/html", "body": {"data": "` + "HTMLBODY" + `", "size": 8}},
      {"mimeType": "application/pdf", "filename": "doc.pdf", "body": {"data": "", "size": 1234}}
    ]
  }
}`

func TestGetParsesBodyAndAttachments(t *testing.T) {
	sample := strings.ReplaceAll(sampleMessage, "PLAINBODY", b64("Hello plain world"))
	sample = strings.ReplaceAll(sample, "HTMLBODY", b64("<p>html</p>"))
	client, _, _ := newFakeGmail(t, sample)

	detail, err := client.Get(context.Background(), "msg-1", false)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.Body != "Hello plain world" {
		t.Fatalf("body = %q", detail.Body)
	}
	if detail.From != "News <news@example.com>" || detail.Subject != "Weekly deals" {
		t.Fatalf("headers wrong: %+v", detail.MessageSummary)
	}
	if len(detail.Attachments) != 1 || detail.Attachments[0].Filename != "doc.pdf" {
		t.Fatalf("attachments = %+v", detail.Attachments)
	}
	if detail.ListUnsubscribe == "" || detail.ListUnsubscribePost == "" {
		t.Fatalf("unsubscribe headers missing: %+v", detail)
	}
}

func TestAnalyzeUnsubscribeRFC8058(t *testing.T) {
	detail := &MessageDetail{
		ListUnsubscribe:     "<https://example.com/unsub?u=1>, <mailto:unsub@example.com>",
		ListUnsubscribePost: "List-Unsubscribe=One-Click",
	}
	plan, err := AnalyzeUnsubscribe(detail)
	if err != nil {
		t.Fatalf("AnalyzeUnsubscribe: %v", err)
	}
	if plan.Mechanism != UnsubRFC8058 || plan.URL != "https://example.com/unsub?u=1" || !plan.OneClick {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestAnalyzeUnsubscribeHTTPGetFallback(t *testing.T) {
	plan, err := AnalyzeUnsubscribe(&MessageDetail{ListUnsubscribe: "<https://example.com/unsub>"})
	if err != nil {
		t.Fatalf("AnalyzeUnsubscribe: %v", err)
	}
	if plan.Mechanism != UnsubHTTPGet {
		t.Fatalf("mechanism = %s", plan.Mechanism)
	}
}

func TestAnalyzeUnsubscribeMailtoOnly(t *testing.T) {
	plan, err := AnalyzeUnsubscribe(&MessageDetail{ListUnsubscribe: "<mailto:unsub@example.com>"})
	if err != nil {
		t.Fatalf("AnalyzeUnsubscribe: %v", err)
	}
	if plan.Mechanism != UnsubMailto {
		t.Fatalf("mechanism = %s", plan.Mechanism)
	}
	if err := ExecuteUnsubscribe(context.Background(), http.DefaultClient, plan); err == nil {
		t.Fatal("mailto execution must be rejected in v1")
	}
}

func TestAnalyzeUnsubscribeNoHeader(t *testing.T) {
	if _, err := AnalyzeUnsubscribe(&MessageDetail{}); err == nil {
		t.Fatal("expected error for missing header")
	}
}

func TestExecuteRFC8058PostsCorrectBody(t *testing.T) {
	var gotMethod, gotBody, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	plan := &UnsubscribePlan{Mechanism: UnsubRFC8058, URL: srv.URL, OneClick: true}
	if err := ExecuteUnsubscribe(context.Background(), srv.Client(), plan); err != nil {
		t.Fatalf("ExecuteUnsubscribe: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s", gotMethod)
	}
	if !strings.Contains(gotBody, "List-Unsubscribe=One-Click") {
		t.Fatalf("body = %q", gotBody)
	}
	if !strings.Contains(gotContentType, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type = %q", gotContentType)
	}
}

func TestExecuteHTTPGetDoesNotFollowRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.Redirect(w, r, "https://elsewhere.example.com", http.StatusFound)
			return
		}
		w.WriteHeader(500)
	}))
	defer srv.Close()

	plan := &UnsubscribePlan{Mechanism: UnsubHTTPGet, URL: srv.URL}
	// A redirect must surface as an error (no auto-follow), not silently "OK".
	if err := ExecuteUnsubscribe(context.Background(), srv.Client(), plan); err == nil {
		t.Fatal("expected redirect to be treated as non-confirmation")
	}
}



func TestStripHTML(t *testing.T) {
	got := StripHTML("<p>Hello &amp; welcome</p><br/>to <b>the</b> world&nbsp;")
	if got != "Hello & welcome to the world" {
		t.Fatalf("StripHTML = %q", got)
	}
}
