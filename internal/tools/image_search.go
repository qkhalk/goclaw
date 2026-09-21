package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ImageSearchTool searches free-licensed stock photography (Openverse's
// CC index, falling back to Wikimedia Commons) so the video designer agent
// can source real imagery for image scenes instead of defaulting to
// color-only storyboards. Read-only, keyless, no exec.
type ImageSearchTool struct {
	client *http.Client
}

// NewImageSearchTool creates an ImageSearchTool.
func NewImageSearchTool() *ImageSearchTool {
	return &ImageSearchTool{client: &http.Client{Timeout: 12 * time.Second}}
}

func (t *ImageSearchTool) Name() string { return "image_search" }

func (t *ImageSearchTool) Description() string {
	return "Search free-licensed stock photos by keyword (Openverse CC index; falls back to Wikimedia Commons). " +
		"Returns direct image URLs with title, license and source. Use the URLs as the source of image scenes in video storyboards."
}

func (t *ImageSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search keywords — English works best (2-5 words, e.g. 'halong bay sunset').",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max results (default 6, max 12).",
			},
		},
		"required": []string{"query"},
	}
}

// stockImage is one normalized search result.
type stockImage struct {
	URL     string
	Title   string
	License string
	Source  string
}

// Execute runs the search: Openverse first, Wikimedia Commons as fallback
// when Openverse errors or comes back empty.
func (t *ImageSearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	q, _ := args["query"].(string)
	q = strings.TrimSpace(q)
	if q == "" {
		return ErrorResult("image_search: query is required")
	}
	limit := 6
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
		if limit > 12 {
			limit = 12
		}
	}

	images, ovErr := t.searchOpenverse(ctx, q, limit)
	if len(images) == 0 {
		wiki, wkErr := t.searchWikimedia(ctx, q, limit)
		if len(wiki) > 0 {
			images = wiki
		} else if ovErr != nil && wkErr != nil {
			return ErrorResult(fmt.Sprintf("image_search failed (openverse: %v; wikimedia: %v)", ovErr, wkErr))
		}
	}
	if len(images) == 0 {
		return NewResult(fmt.Sprintf("image_search: no usable results for %q — fall back to color scenes.", q))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Stock photos for %q (free licenses — check the license before commercial use):\n", q)
	for i, img := range images {
		fmt.Fprintf(&b, "%d. %s\n   title: %s | license: %s | via %s\n", i+1, img.URL, img.Title, img.License, img.Source)
	}
	b.WriteString("\nUse a direct URL above as the \"source\" of an image scene in the storyboard.")
	return NewResult(b.String())
}

// getJSON is a small helper with a browser-ish UA (some CDNs 403 bare Go
// clients) and context propagation.
func (t *ImageSearchTool) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 GoClaw/1.0")
	req.Header.Set("Accept", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *ImageSearchTool) searchOpenverse(ctx context.Context, q string, limit int) ([]stockImage, error) {
	u := "https://api.openverse.org/v1/images/?q=" + url.QueryEscape(q) + fmt.Sprintf("&page_size=%d", limit)
	var out struct {
		Results []struct {
			URL                string `json:"url"`
			Title              string `json:"title"`
			License            string `json:"license"`
			Provider           string `json:"provider"`
			ForeignLandingURL  string `json:"foreign_landing_url"`
		} `json:"results"`
	}
	if err := t.getJSON(ctx, u, &out); err != nil {
		return nil, fmt.Errorf("openverse: %w", err)
	}
	var images []stockImage
	for _, r := range out.Results {
		if !strings.HasPrefix(r.URL, "https://") {
			continue
		}
		license := r.License
		if license == "" {
			license = "unknown"
		}
		images = append(images, stockImage{
			URL:     r.URL,
			Title:   r.Title,
			License: "CC " + license,
			Source:  r.Provider,
		})
		if len(images) >= limit {
			break
		}
	}
	return images, nil
}

func (t *ImageSearchTool) searchWikimedia(ctx context.Context, q string, limit int) ([]stockImage, error) {
	u := "https://commons.wikimedia.org/w/api.php?action=query&format=json" +
		"&generator=search&gsrnamespace=6&gsrlimit=" + fmt.Sprint(limit) +
		"&gsrsearch=" + url.QueryEscape(q) +
		"&prop=imageinfo&iiprop=url&iiurlwidth=1600"
	var out struct {
		Query struct {
			Pages map[string]struct {
				Title     string `json:"title"`
				Imageinfo []struct {
					ThumbURL string `json:"thumburl"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := t.getJSON(ctx, u, &out); err != nil {
		return nil, fmt.Errorf("wikimedia: %w", err)
	}
	// Map iteration order is random — sort by title for stable output.
	keys := make([]string, 0, len(out.Query.Pages))
	for k := range out.Query.Pages {
		keys = append(keys, k)
	}
	// numeric-ish sort keeps result order deterministic
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if out.Query.Pages[keys[j]].Title < out.Query.Pages[keys[i]].Title {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var images []stockImage
	for _, k := range keys {
		p := out.Query.Pages[k]
		if len(p.Imageinfo) == 0 || p.Imageinfo[0].ThumbURL == "" {
			continue
		}
		// Strip the tracking params Commons appends to thumburl.
		clean := strings.SplitN(p.Imageinfo[0].ThumbURL, "?", 2)[0]
		images = append(images, stockImage{
			URL:     clean,
			Title:   strings.TrimPrefix(p.Title, "File:"),
			License: "free license — verify on Commons",
			Source:  "Wikimedia Commons",
		})
	}
	return images, nil
}

// Ensure compile-time interface compliance.
var _ Tool = (*ImageSearchTool)(nil)
