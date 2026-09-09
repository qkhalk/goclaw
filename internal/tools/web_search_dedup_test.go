package tools

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// ptrTime is a small helper for dated search results in tests.
func ptrTime(t time.Time) *time.Time { return &t }

// searxTestServer serves canned SearXNG JSON payloads.
type searxTestServer struct {
	*httptest.Server
	respond func(w http.ResponseWriter)
}

func newTestServer(t *testing.T, payload func(query url.Values) string) *searxTestServer {
	t.Helper()
	s := &searxTestServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if payload != nil {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, payload(r.URL.Query()))
			return
		}
		s.respond(w)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestNormalizeResultURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{
			"strips utm params",
			"https://Example.com/page/?utm_source=news&utm_medium=x&id=7",
			"https://example.com/page?id=7",
		},
		{
			"strips fbclid and gclid",
			"https://example.com/a?fbclid=ABC&gclid=DEF&q=1",
			"https://example.com/a?q=1",
		},
		{
			"lowercases host only (path case preserved)",
			"https://EXAMPLE.com/Page",
			"https://example.com/Page",
		},
		{
			"strips trailing slash",
			"https://example.com/page/",
			"https://example.com/page",
		},
		{
			"root slash normalizes to bare origin",
			"https://example.com/",
			"https://example.com",
		},
		{
			"query order irrelevant",
			"https://example.com/s?b=2&a=1",
			"https://example.com/s?a=1&b=2",
		},
		{
			"keeps non-tracking params, drops fragment (dedup-key only)",
			"https://example.com/p?utm_term=z&keep=1#sec",
			"https://example.com/p?keep=1",
		},
		{
			"unparseable passthrough",
			"not a url at all",
			"not a url at all",
		},
	}
	for _, tc := range cases {
		if got := NormalizeResultURL(tc.in); got != tc.want {
			t.Errorf("%s: NormalizeResultURL(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestDedupeSearchResults(t *testing.T) {
	now := time.Now()
	in := []searchResult{
		{Title: "A", URL: "https://example.com/post?utm_source=rss"},
		{Title: "A dup", URL: "https://EXAMPLE.com/post"},
		{Title: "A trailing slash", URL: "https://example.com/post/"},
		{Title: "B", URL: "https://example.com/other", PublishedDate: &now},
		{Title: "no url", URL: ""},
		{Title: "B dated dup", URL: "https://example.com/other#x", PublishedDate: &now},
	}
	got := DedupeSearchResults(in)
	if len(got) != 2 {
		t.Fatalf("len = %d (%+v), want 2", len(got), got)
	}
	if got[0].Title != "A" {
		t.Errorf("first kept = %q, want A (order preserved, first wins)", got[0].Title)
	}
	if got[1].Title != "B" {
		t.Errorf("second kept = %q, want B", got[1].Title)
	}
	if got[1].URL != "https://example.com/other" {
		t.Errorf("URL must stay unmodified for display, got %q", got[1].URL)
	}

	if got := DedupeSearchResults(nil); got != nil {
		t.Errorf("nil input → %v, want nil", got)
	}
}

func TestSearxTimeRange(t *testing.T) {
	cases := []struct {
		freshness  string
		maxAgeDays int
		want       string
	}{
		{"", 0, ""},
		{"pd", 0, "day"},
		{"pw", 0, "week"},
		{"pm", 0, "month"},
		{"py", 0, "year"},
		// Explicit shortcut beats maxAgeDays bucket.
		{"pw", 90, "week"},
		// Date-range freshness is not natively supported → maxAgeDays bucket.
		{"2024-01-01to2024-02-01", 3, "week"},
		{"2024-01-01to2024-02-01", 0, ""},
		{"", 7, "week"},
		{"", 3, "week"},
		{"", 30, "month"},
		{"", 400, "year"},
	}
	for _, tc := range cases {
		if got := searxTimeRange(tc.freshness, tc.maxAgeDays); got != tc.want {
			t.Errorf("searxTimeRange(%q, %d) = %q, want %q", tc.freshness, tc.maxAgeDays, got, tc.want)
		}
	}
}

func TestParseSearxPublishedDate(t *testing.T) {
	if parseSearxPublishedDate("") != nil {
		t.Error("empty → nil expected")
	}
	if parseSearxPublishedDate("garbage") != nil {
		t.Error("garbage → nil expected")
	}
	cases := map[string]string{
		"2024-05-01T12:00:00Z":       "2024-05-01",
		"2024-05-01T12:00:00+02:00":  "2024-05-01",
		"2024-05-01T09:00:00":        "2024-05-01",
		"2024-05-01":                 "2024-05-01",
		"  2024-05-01T12:00:00Z   ":  "2024-05-01",
	}
	for in, wantDay := range cases {
		got := parseSearxPublishedDate(in)
		if got == nil || got.Format("2006-01-02") != wantDay {
			t.Errorf("parseSearxPublishedDate(%q) = %v, want day %s", in, got, wantDay)
		}
	}
}

func TestFilterResultsByMaxAge(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	results := []searchResult{
		{Title: "fresh", URL: "a", PublishedDate: ptrTime(now.Add(-2 * day))},
		{Title: "stale", URL: "b", PublishedDate: ptrTime(now.Add(-30 * day))},
		{Title: "undated", URL: "c"},
	}
	got := filterResultsByMaxAge(results, 7, now)
	if len(got) != 2 || got[0].Title != "fresh" || got[1].Title != "undated" {
		t.Fatalf("filtered = %+v, want fresh+undated", got)
	}
	// No-op paths.
	if got := filterResultsByMaxAge(results, 0, now); len(got) != 3 {
		t.Fatalf("maxAgeDays=0 must be a no-op, got %+v", got)
	}
	if got := filterResultsByMaxAge(nil, 7, now); got != nil {
		t.Fatalf("nil input must stay nil, got %+v", got)
	}
}

func TestSearxSearchRequest(t *testing.T) {
	var gotQuery, gotFormat, gotRange, gotLang string
	recent := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	srv := newTestServer(t, func(query url.Values) string {
		gotQuery, gotFormat, gotRange, gotLang = query.Get("q"), query.Get("format"), query.Get("time_range"), query.Get("language")
		return fmt.Sprintf(`{"results": [
			{"title": "R1", "url": "https://a.example/1", "content": "first", "publishedDate": %q},
			{"title": "", "url": "https://b.example/2", "content": "second"}
		]}`, recent)
	})
	defer srv.Close()

	p := newSearxSearchProvider(srv.URL, 10)
	params := searchParams{Query: "goclaw", Count: 5, SearchLang: "en", MaxAgeDays: 3}
	results, err := p.Search(t.Context(), params)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotQuery != "goclaw" || gotFormat != "json" || gotRange != "week" || gotLang != "en" {
		t.Fatalf("query params = %q %q %q %q", gotQuery, gotFormat, gotRange, gotLang)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2", results)
	}
	if results[0].Title != "R1" || results[0].Description != "first" {
		t.Errorf("r0 = %+v", results[0])
	}
	if results[0].PublishedDate == nil {
		t.Errorf("r0 publishedDate = nil, want %q", recent)
	}
	// Missing title falls back to URL.
	if results[1].Title != "https://b.example/2" {
		t.Errorf("r1 title fallback = %q", results[1].Title)
	}
}

func TestSearxSearchErrors(t *testing.T) {
	p := newSearxSearchProvider("", 5)
	if _, err := p.Search(t.Context(), searchParams{Query: "x"}); err == nil {
		t.Fatal("empty baseURL must error")
	}

	// Server that answers 503 → provider must surface an error.
	srv := newTestServer(t, nil)
	srv.respond = func(w http.ResponseWriter) { http.Error(w, "boom", http.StatusServiceUnavailable) }
	p2 := newSearxSearchProvider(srv.URL, 5)
	if _, err := p2.Search(t.Context(), searchParams{Query: "x"}); err == nil {
		t.Fatal("non-200 must error")
	}
}
