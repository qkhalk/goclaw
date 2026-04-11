package eventbus

import (
	"log/slog"

	"github.com/google/uuid"
)

// validateAgentID is a publish-time observer that logs a warning when a DomainEvent
// carries a non-UUID AgentID. It does NOT block the publish — observability only.
//
// Motivation: PR #826 fixed multiple call sites that passed agent_key strings
// (e.g. "goctech-leader") into DomainEvent.AgentID, silently corrupting downstream
// consumers. This helper acts as a safety net to catch any future drift BEFORE
// the event reaches a consumer that parses the field as a UUID.
//
// Log field name: `non_uuid_agent_id` (intentionally distinct from the standard
// `agent_id` field used elsewhere) to avoid collision with observability tooling
// that parses `agent_id` as a UUID. See red-team finding H6.
//
// See docs/agent-identity-conventions.md (Phase 6) for the convention.
func validateAgentID(event DomainEvent) {
	if event.AgentID == "" {
		return // legitimate team-owned, tenant-scoped, or anonymous event
	}
	if _, err := uuid.Parse(event.AgentID); err != nil {
		slog.Warn("eventbus.non_uuid_agent_id",
			"event_type", event.Type,
			"non_uuid_agent_id", event.AgentID,
			"source_id", event.SourceID,
		)
	}
}
