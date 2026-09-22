package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	mcpcatalog "github.com/nextlevelbuilder/goclaw/mcp"
)

// Dynamic catalog: new tool servers pushed to the catalog repo (mcp/<tool>/
// manifest.json at the latest release tag) appear in every existing install's
// Store WITHOUT a gateway upgrade. The embedded catalog stays the offline
// fallback and always wins on name conflicts (curated, ships with the
// binary).
//
// Fetching happens on a background ticker — handleCatalog never blocks on
// the network. Unauthenticated GitHub API budget: 3 calls per refresh at a
// 15-minute interval, well under the 60/hr limit.

const (
	dynamicCatalogRefreshEvery = 15 * time.Minute
	dynamicCatalogHTTPTimeout  = 10 * time.Second
	dynamicCatalogMaxTools     = 50
	dynamicCatalogMaxManifest  = 64 << 10 // 64KB per manifest
)

// dynamicCatalogState is the immutable snapshot served to readers.
type dynamicCatalogState struct {
	tag     string
	entries []mcpcatalog.Manifest
	fetched time.Time
}

type dynamicCatalogCache struct {
	mu     sync.RWMutex
	state  *dynamicCatalogState // nil until the first successful fetch
	client *http.Client
}

func (h *MCPInstallHandler) dynamicCatalog() *dynamicCatalogCache {
	if h.dynCatalog == nil {
		h.dynCatalog = &dynamicCatalogCache{client: &http.Client{Timeout: dynamicCatalogHTTPTimeout}}
	}
	return h.dynCatalog
}

// StartDynamicCatalogRefresh begins the background refresh loop. Call once
// at gateway startup; never blocks, never returns. Tests construct the
// handler directly and do not call this.
func (h *MCPInstallHandler) StartDynamicCatalogRefresh() {
	h.dynamicCatalog() // init client eagerly
	go func() {
		h.refreshDynamicCatalog()
		ticker := time.NewTicker(dynamicCatalogRefreshEvery)
		defer ticker.Stop()
		for range ticker.C {
			h.refreshDynamicCatalog()
		}
	}()
}

// refreshDynamicCatalog fetches the latest release tag of the catalog repo
// and parses every mcp/<tool>/manifest.json found there. Failures keep the
// previous snapshot (or none — the embedded catalog covers that case).
func (h *MCPInstallHandler) refreshDynamicCatalog() {
	c := h.dynamicCatalog()
	ctx, cancel := context.WithTimeout(context.Background(), 3*dynamicCatalogHTTPTimeout)
	defer cancel()

	repo := strings.TrimPrefix(mcpcatalog.DefaultRepo, "https://github.com/") // "owner/repo"
	api := "https://api.github.com/repos/" + repo

	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := c.getJSON(ctx, api+"/releases/latest", &rel); err != nil || rel.TagName == "" {
		slog.Warn("mcp_catalog.dynamic_fetch", "error", err, "step", "latest-release")
		return
	}
	// "v4.9.5" is a safe path segment by construction, but guard anyway.
	if !mcpcatalog.RefPattern.MatchString(rel.TagName) {
		slog.Warn("mcp_catalog.dynamic_fetch", "tag", rel.TagName, "reason", "unexpected tag shape")
		return
	}

	var dirs []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := c.getJSON(ctx, api+"/contents/mcp?ref="+rel.TagName, &dirs); err != nil {
		slog.Warn("mcp_catalog.dynamic_fetch", "error", err, "step", "list-mcp-dir")
		return
	}

	embedded := map[string]bool{}
	for _, m := range mcpcatalog.Entries() {
		embedded[m.Name] = true
	}

	entries := make([]mcpcatalog.Manifest, 0, len(dirs))
	for _, d := range dirs {
		if d.Type != "dir" || embedded[d.Name] || !isSlugLike(d.Name) {
			continue
		}
		if len(entries) >= dynamicCatalogMaxTools {
			break
		}
		var mf struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if err := c.getJSON(ctx, api+"/contents/mcp/"+d.Name+"/manifest.json?ref="+rel.TagName, &mf); err != nil {
			slog.Warn("mcp_catalog.dynamic_fetch", "error", err, "step", "manifest", "tool", d.Name)
			continue
		}
		if mf.Encoding != "base64" || len(mf.Content) > dynamicCatalogMaxManifest {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(mf.Content)
		if err != nil {
			continue
		}
		var m mcpcatalog.Manifest
		if err := json.Unmarshal(raw, &m); err != nil || m.Validate() != nil {
			slog.Warn("mcp_catalog.dynamic_fetch", "reason", "invalid manifest", "tool", d.Name)
			continue
		}
		if m.Name != d.Name {
			slog.Warn("mcp_catalog.dynamic_fetch", "reason", "manifest name != folder", "tool", d.Name, "name", m.Name)
			continue
		}
		// Dynamic entries pin the tag they were discovered at, so the
		// manifest and the installed code always come from the same source.
		if m.Ref == "" {
			m.Ref = rel.TagName
		}
		entries = append(entries, m)
	}

	c.mu.Lock()
	c.state = &dynamicCatalogState{tag: rel.TagName, entries: entries, fetched: time.Now()}
	c.mu.Unlock()
	slog.Info("mcp_catalog.dynamic_fetch", "tag", rel.TagName, "tools", len(entries))
}

// dynamicSnapshot returns the current dynamic state (nil-safe copy).
func (h *MCPInstallHandler) dynamicSnapshot() *dynamicCatalogState {
	c := h.dynamicCatalog()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// findDynamic resolves a name against the dynamic catalog (embedded always
// wins, so callers check mcpcatalog.Find first).
func (h *MCPInstallHandler) findDynamic(name string) *mcpcatalog.Manifest {
	st := h.dynamicSnapshot()
	if st == nil {
		return nil
	}
	for _, m := range st.entries {
		if m.Name == name {
			cp := m
			return &cp
		}
	}
	return nil
}

func (c *dynamicCatalogCache) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github api %s: status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// isSlugLike is the loose folder-name check for dynamic listing (manifest
// validation re-checks everything anyway).
func isSlugLike(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
