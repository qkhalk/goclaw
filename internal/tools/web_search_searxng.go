package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// --- SearXNG Search Provider (inheritance plan Phase 4) ---
//
// Talks to a self-hosted (or tenant-hosted) SearXNG instance's JSON API:
//   GET {baseURL}/search?q=<query>&format=json[&language=][&time_range=]
//
// The endpoint is per-tenant config (config_secrets key tools.web.searxng.url)
// instead of a fixed SaaS host, so no SSRF guard is applied here on purpose:
// pointing goclaw at a private/self-hosted SearXNG is the primary use case,
// and the URL is operator- or tenant-admin-configured (same trust level as
// the fixed provider endpoints' API keys).

type searxSearchProvider struct {
	baseURL    string
	maxResults int
	client     *http.Client
}

func newSearxSearchProvider(baseURL string, maxResults int) *searxSearchProvider {
	return &searxSearchProvider{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		maxResults: normalizeProviderMaxResults(maxResults),
		client:     &http.Client{Timeout: time.Duration(searchTimeoutSeconds) * time.Second},
	}
}

func (p *searxSearchProvider) Name() string { return searchProviderSearxNG }

func (p *searxSearchProvider) Search(ctx context.Context, params searchParams) ([]searchResult, error) {
	if p.baseURL == "" {
		return nil, fmt.Errorf("searxng URL is not configured")
	}

	q := url.Values{}
	q.Set("q", params.Query)
	q.Set("format", "json")
	if params.SearchLang != "" {
		q.Set("language", params.SearchLang)
	}
	// SearXNG has no arbitrary date-range filter; only the four coarse buckets.
	if tr := searxTimeRange(params.Freshness, params.MaxAgeDays); tr != "" {
		q.Set("time_range", tr)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/search?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", webSearchUserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng API returned %d: %s", resp.StatusCode, truncateStr(string(body), 200))
	}

	var searxResp struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Content       string `json:"content"`
			PublishedDate string `json:"publishedDate"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &searxResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	count := clampProviderResultCount(params.Count, p.maxResults)
	results := make([]searchResult, 0, len(searxResp.Results))
	for _, r := range searxResp.Results {
		results = append(results, searchResult{
			Title:         coalesceSearchText(r.Title, r.URL, "Untitled"),
			URL:           r.URL,
			Description:   truncateStr(r.Content, 240),
			PublishedDate: parseSearxPublishedDate(r.PublishedDate),
		})
	}

	if params.MaxAgeDays > 0 {
		// Server-side time_range is a coarse bucket (day/week/month/year);
		// tighten it client-side using publishedDate when the engine provides
		// one. Undated results are kept — they passed SearXNG's own filter.
		results = filterResultsByMaxAge(results, params.MaxAgeDays, time.Now())
	}
	if len(results) > count {
		results = results[:count]
	}
	return results, nil
}

// searxTimeRange maps the tool's freshness inputs onto SearXNG's four
// time_range buckets. An explicit freshness shortcut wins; a date-range
// freshness (unsupported natively) falls through to the maxAgeDays bucket.
func searxTimeRange(freshness string, maxAgeDays int) string {
	switch normalizeFreshness(freshness) {
	case "pd":
		return "day"
	case "pw":
		return "week"
	case "pm":
		return "month"
	case "py":
		return "year"
	}
	if maxAgeDays <= 0 {
		return ""
	}
	switch {
	case maxAgeDays <= 1:
		return "day"
	case maxAgeDays <= 7:
		return "week"
	case maxAgeDays <= 31:
		return "month"
	default:
		return "year"
	}
}

// parseSearxPublishedDate accepts the ISO-8601 variants SearXNG instances
// emit in practice ("2024-05-01T12:00:00Z", with offset, or date-only).
// Returns nil for empty/unparseable values.
func parseSearxPublishedDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	return nil
}

// filterResultsByMaxAge drops results older than maxAgeDays. Undated results
// survive (the filter only acts on providers that actually report dates).
func filterResultsByMaxAge(results []searchResult, maxAgeDays int, now time.Time) []searchResult {
	if maxAgeDays <= 0 || len(results) == 0 {
		return results
	}
	cutoff := now.AddDate(0, 0, -maxAgeDays)
	out := results[:0]
	for _, r := range results {
		if r.PublishedDate != nil && r.PublishedDate.Before(cutoff) {
			continue
		}
		out = append(out, r)
	}
	return out
}
