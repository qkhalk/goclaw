package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// KnowledgeGraphSearchTool provides graph-based search for agents.
type KnowledgeGraphSearchTool struct {
	kgStore store.KnowledgeGraphStore
}

// NewKnowledgeGraphSearchTool creates a new KnowledgeGraphSearchTool.
func NewKnowledgeGraphSearchTool() *KnowledgeGraphSearchTool {
	return &KnowledgeGraphSearchTool{}
}

// SetKGStore sets the KnowledgeGraphStore for this tool.
func (t *KnowledgeGraphSearchTool) SetKGStore(ks store.KnowledgeGraphStore) {
	t.kgStore = ks
}

func (t *KnowledgeGraphSearchTool) Name() string { return "knowledge_graph_search" }

func (t *KnowledgeGraphSearchTool) Description() string {
	return "Search the knowledge graph for entities and their relationships. Use this to find connections between people, projects, tasks, and concepts. Supports entity search by name/type and graph traversal to discover multi-hop relationships."
}

func (t *KnowledgeGraphSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query for entity names or descriptions",
			},
			"entity_type": map[string]interface{}{
				"type":        "string",
				"description": "Filter by entity type (person, project, task, event, concept, location, organization)",
			},
			"entity_id": map[string]interface{}{
				"type":        "string",
				"description": "Entity ID to traverse from (for relationship discovery)",
			},
			"max_depth": map[string]interface{}{
				"type":        "number",
				"description": "Maximum traversal depth (default 2, max 3)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *KnowledgeGraphSearchTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	if t.kgStore == nil {
		return NewResult("Knowledge graph is not enabled for this agent.")
	}

	agentID := store.AgentIDFromContext(ctx)
	if agentID == uuid.Nil {
		return ErrorResult("agent context not available")
	}
	userID := store.UserIDFromContext(ctx)

	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("query parameter is required")
	}

	entityID, _ := args["entity_id"].(string)
	maxDepth := 2
	if md, ok := args["max_depth"].(float64); ok && md > 0 {
		maxDepth = int(md)
		if maxDepth > 3 {
			maxDepth = 3
		}
	}

	// Traversal mode: entity_id provided
	if entityID != "" {
		return t.executeTraversal(ctx, agentID.String(), userID, entityID, maxDepth)
	}

	// Search mode
	return t.executeSearch(ctx, agentID.String(), userID, query, args)
}

func (t *KnowledgeGraphSearchTool) executeTraversal(ctx context.Context, agentID, userID, entityID string, maxDepth int) *Result {
	results, err := t.kgStore.Traverse(ctx, agentID, userID, entityID, maxDepth)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph traversal failed: %v", err))
	}
	if len(results) == 0 {
		return NewResult(fmt.Sprintf("No connected entities found from entity_id=%q within depth %d.", entityID, maxDepth))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Graph traversal from %q (max depth %d):\n\n", entityID, maxDepth))
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("- [depth %d] %s (%s)", r.Depth, r.Entity.Name, r.Entity.EntityType))
		if r.Via != "" {
			sb.WriteString(fmt.Sprintf(" via %q", r.Via))
		}
		if r.Entity.Description != "" {
			sb.WriteString(fmt.Sprintf("\n  %s", r.Entity.Description))
		}
		if len(r.Path) > 0 {
			sb.WriteString(fmt.Sprintf("\n  path: %s", strings.Join(r.Path, " → ")))
		}
		sb.WriteString("\n")
	}
	return NewResult(sb.String())
}

func (t *KnowledgeGraphSearchTool) executeSearch(ctx context.Context, agentID, userID, query string, args map[string]interface{}) *Result {
	entities, err := t.kgStore.SearchEntities(ctx, agentID, userID, query, 10)
	if err != nil {
		return ErrorResult(fmt.Sprintf("entity search failed: %v", err))
	}
	if len(entities) == 0 {
		return NewResult(fmt.Sprintf("No entities found matching %q.", query))
	}

	// Optional type filter (post-search)
	entityType, _ := args["entity_type"].(string)
	if entityType != "" {
		filtered := entities[:0]
		for _, e := range entities {
			if e.EntityType == entityType {
				filtered = append(filtered, e)
			}
		}
		entities = filtered
		if len(entities) == 0 {
			return NewResult(fmt.Sprintf("No entities of type %q found matching %q.", entityType, query))
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d entities matching %q:\n\n", len(entities), query))
	for _, e := range entities {
		sb.WriteString(fmt.Sprintf("- %s [%s] (id: %s)\n", e.Name, e.EntityType, e.ExternalID))
		if e.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", e.Description))
		}

		// Fetch relations to show connections
		relations, err := t.kgStore.ListRelations(ctx, agentID, userID, e.ID)
		if err == nil && len(relations) > 0 {
			sb.WriteString("  Relations:\n")
			for _, rel := range relations {
				sb.WriteString(fmt.Sprintf("    %s → %s → %s\n", rel.SourceEntityID, rel.RelationType, rel.TargetEntityID))
			}
		}
	}
	return NewResult(sb.String())
}
