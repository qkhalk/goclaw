package cmd

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/bootstrap"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// buildEnsureUserFiles creates the per-user file seeding callback.
// Seeds per-user context files on first chat (new user profile).
func buildEnsureUserFiles(as store.AgentStore, configPermStore store.ConfigPermissionStore) agent.EnsureUserFilesFunc {
	return func(ctx context.Context, agentID uuid.UUID, userID, agentType, workspace, channel string) (string, error) {
		isNew, effectiveWs, err := as.GetOrCreateUserProfile(ctx, agentID, userID, workspace, channel)
		if err != nil {
			return effectiveWs, err
		}

		// Seed context files:
		// - isNew: always seed (first-time user)
		// - !isNew: only seed when user has NO context files at all.
		//   This handles the EnsureUserProfile (HTTP API) pre-creation case where profile
		//   exists but context files were never seeded. We check for zero files rather than
		//   calling SeedUserFiles unconditionally to avoid re-seeding BOOTSTRAP.md after
		//   auto-cleanup (which DELETEs the row — SeedUserFiles would treat it as missing).
		needSeed := isNew
		if !isNew {
			existing, qErr := as.GetUserContextFiles(ctx, agentID, userID)
			if qErr == nil && len(existing) == 0 {
				needSeed = true
			}
		}
		if needSeed {
			if _, seedErr := bootstrap.SeedUserFiles(ctx, as, agentID, userID, agentType); seedErr != nil {
				slog.Warn("failed to seed user context files", "error", seedErr, "agent", agentID, "user", userID)
			}
		}

		// Auto-add first group member as a file writer (bootstrap the allowlist).
		// Only needed for truly new profiles — existing groups already have their writers.
		if isNew && configPermStore != nil && (strings.HasPrefix(userID, "group:") || strings.HasPrefix(userID, "guild:")) {
			senderID := store.SenderIDFromContext(ctx)
			if senderID != "" {
				parts := strings.SplitN(senderID, "|", 2)
				numericID := parts[0]
				senderUsername := ""
				if len(parts) > 1 {
					senderUsername = parts[1]
				}
				meta, _ := json.Marshal(map[string]string{"displayName": "", "username": senderUsername})
				if addErr := configPermStore.Grant(ctx, &store.ConfigPermission{
					AgentID:    agentID,
					Scope:      userID,
					ConfigType: "file_writer",
					UserID:     numericID,
					Permission: "allow",
					Metadata:   meta,
				}); addErr != nil {
					slog.Warn("failed to auto-add group file writer", "error", addErr, "sender", numericID, "group", userID)
				}
			}
		}

		return effectiveWs, nil
	}
}

// buildBootstrapCleanup creates a callback that removes BOOTSTRAP.md for a user.
// Used as a safety net after enough conversation turns, in case the LLM
// didn't clear BOOTSTRAP.md itself. Idempotent — no-op if already cleared.
func buildBootstrapCleanup(as store.AgentStore) agent.BootstrapCleanupFunc {
	return func(ctx context.Context, agentID uuid.UUID, userID string) error {
		return as.DeleteUserContextFile(ctx, agentID, userID, bootstrap.BootstrapFile)
	}
}

// buildContextFileLoader creates the per-request context file loader callback.
// Delegates to the ContextFileInterceptor for type-aware routing.
func buildContextFileLoader(intc *tools.ContextFileInterceptor) agent.ContextFileLoaderFunc {
	return func(ctx context.Context, agentID uuid.UUID, userID, agentType string) []bootstrap.ContextFile {
		return intc.LoadContextFiles(ctx, agentID, userID, agentType)
	}
}
