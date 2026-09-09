package mcp

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// unifiedToolSearchEntry is one row of the unified tool_search response.
// Kind distinguishes deferred native builtin tools from deferred MCP bridge
// tools so the LLM knows how to call them (both are called by Name directly).
type unifiedToolSearchEntry struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"` // "builtin" | "mcp"
	Source       string  `json:"source,omitempty"`
	OriginalName string  `json:"original_name,omitempty"`
	Description  string  `json:"description"`
	Score        float64 `json:"-"`
}

// unifiedToolSearch is the meta-tool registered as "tool_search" when BOTH the
// native deferred-tools mode (tools.deferred) and the MCP search mode are
// active. It covers native + MCP deferred entries in one BM25 index — one
// meta-tool instead of two. Registered by NewMCPToolSearchTool in place of the
// classic mcp_tool_search; the resolver's Register call then overwrites the
// native-only tool_search with this unified variant.
type unifiedToolSearch struct {
	manager *Manager
}

func newUnifiedToolSearch(mgr *Manager) *unifiedToolSearch {
	return &unifiedToolSearch{manager: mgr}
}

func (t *unifiedToolSearch) Name() string { return "tool_search" }

func (t *unifiedToolSearch) Description() string {
	return "Search for available tools by keyword. " +
		"IMPORTANT: Some tools are NOT loaded by default — additional builtin " +
		"tools and external service integrations (databases, APIs, file systems, " +
		"messaging, etc. via MCP) are deferred to keep your context small. " +
		"Before performing an operation you do not see in your tool list, you MUST " +
		"search here first to discover available tools. " +
		"Use English keywords describing what you need " +
		"(e.g. 'database query', 'create issue', 'send email', 'read pdf'). " +
		"Discovered tools become immediately available for use."
}

func (t *unifiedToolSearch) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "English keywords describing the operation you need (e.g. 'create github issue', 'query postgres', 'send slack message')",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of tools to return (default: 5)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *unifiedToolSearch) Execute(ctx context.Context, args map[string]any) *tools.Result {
	query, _ := args["query"].(string)
	if query == "" {
		return tools.ErrorResult("query parameter is required")
	}

	maxResults := 5
	if mr, ok := args["max_results"].(float64); ok && int(mr) > 0 {
		maxResults = int(mr)
	}

	// Rebuild the combined index per call: both deferred sets shrink as tools
	// are activated, and both are small (tens of entries), so a rebuild is
	// cheap and always fresh.
	nativeDocs := t.manager.registry.DeferredSearchDocs()
	mcpDeferred := t.manager.DeferredToolInfos()

	docs := make([]tools.SearchDoc, 0, len(nativeDocs)+len(mcpDeferred))
	docs = append(docs, nativeDocs...)
	docs = append(docs, mcpSearchDocs(mcpDeferred)...)

	index := tools.NewBM25Index()
	index.Build(docs)
	hits := index.Search(query, maxResults)

	slog.Info("tool_search", "query", query, "results", len(hits),
		"indexed", index.DocCount())

	if len(hits) == 0 {
		return tools.NewResult("No tools found matching: " + query +
			"\nProceed with other available tools.")
	}

	// Partition hits by kind and activate on both sides. Native deferred names
	// are unique vs MCP registered names (mcp_<server>__<tool> prefix).
	nativeNames := make([]string, 0, len(nativeDocs))
	nativeSet := make(map[string]struct{}, len(nativeDocs))
	for _, d := range nativeDocs {
		nativeSet[d.Name] = struct{}{}
	}

	entries := make([]unifiedToolSearchEntry, 0, len(hits))
	var mcpNames []string
	for _, h := range hits {
		if _, isNative := nativeSet[h.Name]; isNative {
			nativeNames = append(nativeNames, h.Name)
			entries = append(entries, unifiedToolSearchEntry{
				Name:        h.Name,
				Kind:        "builtin",
				Source:      h.Source,
				Description: h.Description,
				Score:       h.Score,
			})
			continue
		}
		mcpNames = append(mcpNames, h.Name)
		entries = append(entries, unifiedToolSearchEntry{
			Name:         h.Name,
			Kind:         "mcp",
			Source:       h.Source,
			OriginalName: h.Title,
			Description:  h.Description,
			Score:        h.Score,
		})
	}

	t.manager.registry.ActivateDeferredTools(nativeNames)
	t.manager.ActivateTools(mcpNames)

	data, _ := json.MarshalIndent(map[string]any{
		"tools": entries,
		"count": len(entries),
	}, "", "  ")

	return tools.NewResult(string(data) +
		"\n\nThe above tools are now activated and available for use. " +
		"Call them directly by name to perform your operation.")
}
