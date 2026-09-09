package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// MCPToolSearchResult is a single result from a BM25 search over MCP tools.
// The JSON shape is part of the LLM-facing tool response contract — do not
// rename fields.
type MCPToolSearchResult struct {
	RegisteredName string  `json:"name"`
	OriginalName   string  `json:"original_name"`
	ServerName     string  `json:"server"`
	Description    string  `json:"description"`
	Score          float64 `json:"-"`
}

// mcpSearchDocs maps deferred bridge tools into shared BM25 search documents.
func mcpSearchDocs(deferred []*BridgeTool) []tools.SearchDoc {
	docs := make([]tools.SearchDoc, 0, len(deferred))
	for _, bt := range deferred {
		docs = append(docs, tools.SearchDoc{
			Name:        bt.registeredName,
			Source:      bt.serverName,
			Title:       bt.toolName,
			Description: bt.description,
		})
	}
	return docs
}

// mcpHits converts shared search hits back into the MCP result contract.
func mcpHits(hits []tools.SearchHit) []MCPToolSearchResult {
	out := make([]MCPToolSearchResult, len(hits))
	for i, h := range hits {
		out[i] = MCPToolSearchResult{
			RegisteredName: h.Name,
			OriginalName:   h.Title,
			ServerName:     h.Source,
			Description:    h.Description,
			Score:          h.Score,
		}
	}
	return out
}

// MCPToolSearchTool searches deferred MCP tools by keyword using BM25.
// Registered instead of individual MCP tools when the total count exceeds
// the inline threshold (search mode). Discovered tools are activated in
// the registry and become available on the next agent loop iteration.
//
// When the shared registry is already in NATIVE deferred mode (tools.deferred
// enabled and the native tool_search meta-tool is active), NewMCPToolSearchTool
// returns a UNIFIED tool named "tool_search" that covers both native and MCP
// deferred entries — one meta-tool instead of two. Without native deferred
// mode, the classic "mcp_tool_search" is returned (backward-compatible).
type MCPToolSearchTool struct {
	manager *Manager
	index   *tools.BM25Index
}

// NewMCPToolSearchTool creates an mcp_tool_search tool backed by BM25.
// The returned tools.Tool is named "mcp_tool_search" (classic mode) or
// "tool_search" (unified mode when the registry is in native deferred mode).
func NewMCPToolSearchTool(mgr *Manager) tools.Tool {
	if mgr.registry != nil && mgr.registry.IsDeferredMode() {
		return newUnifiedToolSearch(mgr)
	}
	t := &MCPToolSearchTool{
		manager: mgr,
		index:   tools.NewBM25Index(),
	}
	t.rebuildIndex()
	return t
}

func (t *MCPToolSearchTool) rebuildIndex() {
	deferred := t.manager.DeferredToolInfos()
	t.index.Build(mcpSearchDocs(deferred))
	slog.Debug("mcp_tool_search.index_built", "tools", len(deferred))
}

func (t *MCPToolSearchTool) Name() string { return "mcp_tool_search" }

func (t *MCPToolSearchTool) Description() string {
	return "Search for available external integration tools (MCP) by keyword. " +
		"IMPORTANT: You have access to external service integrations " +
		"(databases, APIs, file systems, messaging, etc.) through MCP tools " +
		"that are NOT loaded by default. Before performing any external service " +
		"operation, you MUST search here first to discover available tools. " +
		"Use English keywords describing what you need " +
		"(e.g. 'database query', 'create issue', 'send email'). " +
		"Discovered tools become immediately available for use."
}

func (t *MCPToolSearchTool) Parameters() map[string]any {
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

func (t *MCPToolSearchTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	query, _ := args["query"].(string)
	if query == "" {
		return tools.ErrorResult("query parameter is required")
	}

	maxResults := 5
	if mr, ok := args["max_results"].(float64); ok && int(mr) > 0 {
		maxResults = int(mr)
	}

	results := mcpHits(t.index.Search(query, maxResults))

	slog.Info("mcp_tool_search", "query", query, "results", len(results))

	if len(results) == 0 {
		return tools.NewResult(fmt.Sprintf(
			"No MCP tools found matching: %q\nProceed with other available tools.", query))
	}

	// Activate matched tools in the registry
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.RegisteredName
	}
	t.manager.ActivateTools(names)

	data, _ := json.MarshalIndent(map[string]any{
		"tools": results,
		"count": len(results),
	}, "", "  ")

	return tools.NewResult(string(data) +
		"\n\nThe above tools are now activated and available for use. " +
		"Call them directly by name to perform your operation.")
}
