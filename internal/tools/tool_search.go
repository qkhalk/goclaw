package tools

import (
	"context"
	"encoding/json"
	"log/slog"
)

// ToolSearchName is the meta-tool name for native deferred tool discovery.
const ToolSearchName = "tool_search"

// toolSearchEntry is one row of the tool_search response.
type toolSearchEntry struct {
	Name        string  `json:"name"`
	Group       string  `json:"group,omitempty"`
	Description string  `json:"description"`
	Score       float64 `json:"-"`
}

// ToolSearchTool is the meta-tool for deferred NATIVE builtin tools. Registered
// by Registry.ApplyDeferredMode when the visible tool count exceeds the
// configured threshold (tools.deferred). Discovered tools are promoted back
// into the registry and become available on the next agent loop iteration —
// mirroring the MCP manager's mcp_tool_search behavior.
//
// When MCP search mode is also active, the MCP side registers a UNIFIED
// "tool_search" (internal/mcp tool_search_unified.go) that overwrites this
// native-only variant, covering both deferred sets with one meta-tool.
type ToolSearchTool struct {
	reg   *Registry
	index *BM25Index
}

func (t *ToolSearchTool) Name() string { return ToolSearchName }

func (t *ToolSearchTool) Description() string {
	return "Search for available builtin tools by keyword. " +
		"IMPORTANT: Some builtin tools (filesystem helpers, media readers, " +
		"automation, messaging, and more) are NOT loaded by default to keep your " +
		"context small. Before performing an operation you do not see in your " +
		"tool list, you MUST search here first to discover available tools. " +
		"Use English keywords describing what you need " +
		"(e.g. 'read pdf', 'schedule task', 'convert audio'). " +
		"Discovered tools become immediately available for use."
}

func (t *ToolSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "English keywords describing the operation you need (e.g. 'read pdf document', 'schedule cron job', 'send message to channel')",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of tools to return (default: 5)",
			},
		},
		"required": []string{"query"},
	}
}

// rebuildIndex builds the BM25 index over the current deferred native set.
func (t *ToolSearchTool) rebuildIndex() {
	docs := t.reg.DeferredSearchDocs()
	t.index = NewBM25Index()
	t.index.Build(docs)
	slog.Debug("tool_search.index_built", "tools", len(docs))
}

func (t *ToolSearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("query parameter is required")
	}

	maxResults := 5
	if mr, ok := args["max_results"].(float64); ok && int(mr) > 0 {
		maxResults = int(mr)
	}

	hits := t.reg.searchDeferredTools(query, maxResults)

	slog.Info("tool_search", "query", query, "results", len(hits))

	if len(hits) == 0 {
		return NewResult("No builtin tools found matching: " + query +
			"\nProceed with other available tools.")
	}

	entries := make([]toolSearchEntry, len(hits))
	names := make([]string, len(hits))
	for i, h := range hits {
		entries[i] = toolSearchEntry{
			Name:        h.Name,
			Group:       h.Source,
			Description: h.Description,
			Score:       h.Score,
		}
		names[i] = h.Name
	}
	t.reg.ActivateDeferredTools(names)

	data, _ := json.MarshalIndent(map[string]any{
		"tools": entries,
		"count": len(entries),
	}, "", "  ")

	return NewResult(string(data) +
		"\n\nThe above tools are now activated and available for use. " +
		"Call them directly by name to perform your operation.")
}
