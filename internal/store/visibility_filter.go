package store

import (
	"context"
	"strings"
)

// IsSkillVisibleTo returns true if the caller identified by ctx can discover
// the given skill. Rules:
//   - System skills are visible to everyone.
//   - Empty or "public" visibility is treated as public (legacy rows default
//     to "public" for safety since older stores did not enforce the field).
//   - "private" skills are only visible to the owner. Three identity strings
//     are considered (actor, user, sender) to match the same identities
//     isOwnerOfSkill checks for backward compatibility (#915).
//   - "internal" skills require explicit grants, which are evaluated in
//     ListAccessible; this baseline helper hides them.
//
// Admin/master-scope bypass is the caller's responsibility — this helper
// reflects the non-privileged baseline.
func IsSkillVisibleTo(ctx context.Context, ownerID, visibility string, isSystem bool) bool {
	if isSystem {
		return true
	}
	// Normalize to defend against historical rows with mixed case / whitespace
	// that bypassed the write-path normalizer.
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case "", "public":
		return true
	case "private":
		if ownerID == "" {
			// No owner recorded — treat as public (historical data).
			return true
		}
		actorID := ActorIDFromContext(ctx)
		userID := UserIDFromContext(ctx)
		senderID := SenderIDFromContext(ctx)
		return ownerID == actorID || ownerID == userID || ownerID == senderID
	case "internal":
		return false
	default:
		// Unknown enum value: fail closed (hide).
		return false
	}
}

// IsStatusDiscoverable reports whether a skill status is discoverable through
// the non-privileged skill list. Only published skills are discoverable; the
// review lifecycle states (draft/pending_review/approved/rejected/suspended)
// are hidden until an admin approves them into published. Legacy rows with an
// empty or "active" status remain discoverable for back-compat.
func IsStatusDiscoverable(status string) bool {
	switch status {
	case "", SkillStatusPublished, SkillStatusLegacyActive:
		return true
	default:
		return false
	}
}

// FilterVisibleSkills returns skills the caller can discover. Uses
// IsSkillVisibleTo for each entry and gates on status: a skill sitting in a
// review-lifecycle state (draft, pending_review, approved, rejected,
// suspended) is not discoverable until it reaches published. Admin/master
// scope bypass is the caller's responsibility.
func FilterVisibleSkills(ctx context.Context, skills []SkillInfo) []SkillInfo {
	out := make([]SkillInfo, 0, len(skills))
	for _, s := range skills {
		if !IsStatusDiscoverable(s.Status) {
			continue
		}
		if IsSkillVisibleTo(ctx, s.OwnerID, s.Visibility, s.IsSystem) {
			out = append(out, s)
		}
	}
	return out
}
