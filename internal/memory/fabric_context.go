package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tokencount"
)

// maxFabricContextEntries bounds how many semantic memories are surfaced into
// the prompt section. Retrieval already ranks by authority/confidence/recency;
// the cap keeps one broad query from flooding the system prompt.
const maxFabricContextEntries = 8

// defaultFabricContextTokens caps the assembled section (same budget shape as
// the episodic auto-inject's 200-token L0 section, slightly larger because
// provenance suffixes add per-entry overhead).
const defaultFabricContextTokens = 250

// FabricContextParams selects which memories to surface and for whom.
// Identity fields act as the same hard gates SearchMemories applies.
type FabricContextParams struct {
	UserID      string
	AgentID     string
	WorkspaceID string
	SessionKey  string
	Scopes      []string
	Kinds       []string
	Limit       int
	// MaxTokens caps the assembled section. 0 = defaultFabricContextTokens.
	MaxTokens int
}

// FabricContextResult is the assembled prompt section plus counters for
// telemetry.
type FabricContextResult struct {
	Section  string
	Matched  int // rows retrieved before conflict filtering
	Injected int // entries that fit the token budget
}

// BuildFabricContext retrieves semantic memories and formats a provenance-
// bearing prompt section:
//
//	## Long-term memory
//	- fact content (source: session:sess_x, confidence 0.9)
//
// The provenance suffix is what lets the agent say "em đang nhớ điều này từ
// phiên trước" instead of asserting a recalled fact as its own knowledge.
// Contradicted rows are already filtered store-side (SearchMemories).
func BuildFabricContext(ctx context.Context, fabric store.MemoryFabricStore, params FabricContextParams) (*FabricContextResult, error) {
	if fabric == nil {
		return &FabricContextResult{}, nil
	}
	limit := params.Limit
	if limit <= 0 {
		limit = maxFabricContextEntries
	}
	results, err := fabric.SearchMemories(ctx, store.MemoryQuery{
		UserID:      params.UserID,
		AgentID:     params.AgentID,
		WorkspaceID: params.WorkspaceID,
		SessionKey:  params.SessionKey,
		Scopes:      params.Scopes,
		Kinds:       params.Kinds,
		Limit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("fabric context search: %w", err)
	}
	if len(results) == 0 {
		return &FabricContextResult{}, nil
	}

	maxTokens := params.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultFabricContextTokens
	}
	counter := tokencount.NewBudgetCounter()

	var sb strings.Builder
	sb.WriteString("## Long-term memory\n\n")
	sb.WriteString("Relevant remembered facts (cite the source when relying on them):\n")
	sectionTokens, _ := counter.CountText(sb.String())

	injected := 0
	for _, r := range results {
		if injected >= maxFabricContextEntries {
			break
		}
		entry := "- " + strings.TrimSpace(r.Content) + provenanceSuffix(r.Memory) + "\n"
		tokens, err := counter.CountText(entry)
		if err != nil {
			continue
		}
		if sectionTokens+tokens > maxTokens {
			continue
		}
		sectionTokens += tokens
		sb.WriteString(entry)
		injected++
	}
	if injected == 0 {
		return &FabricContextResult{Matched: len(results)}, nil
	}
	return &FabricContextResult{
		Section:  sb.String(),
		Matched:  len(results),
		Injected: injected,
	}, nil
}

// provenanceSuffix renders the memory's provenance metadata as a compact
// parenthetical. Confidence is omitted when it carries no signal (the 0.8
// write default); the source ref is omitted when absent.
func provenanceSuffix(m *store.Memory) string {
	var parts []string
	if m.SourceRef != nil && *m.SourceRef != "" {
		parts = append(parts, "source: "+*m.SourceRef)
	} else if m.SourceType == "manual" {
		parts = append(parts, "source: user")
	}
	if m.Confidence > 0 && m.Confidence != 0.8 {
		parts = append(parts, fmt.Sprintf("confidence %.2f", m.Confidence))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}
