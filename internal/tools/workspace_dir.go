package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Workspace limits shared across workspace interceptor.
const (
	maxFileSizeBytes = 10 * 1024 * 1024 // 10MB
	maxFilesPerScope = 100
)

// WorkspaceDir returns the disk directory for a team workspace scope.
// Pattern: {dataDir}/teams/{teamID}/{chatID}/
// chatID is the system-derived userID (stable across WS reconnects).
// Creates directory with 0750 if not exists.
func WorkspaceDir(dataDir string, teamID uuid.UUID, chatID string) (string, error) {
	if chatID == "" {
		chatID = "_default"
	}
	dir := filepath.Join(dataDir, "teams", teamID.String(), chatID)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("failed to create workspace dir: %w", err)
	}
	return dir, nil
}

// blockedExtensions lists executable file types that are not allowed in team workspaces.
var blockedExtensions = map[string]bool{
	".exe": true, ".sh": true, ".bat": true, ".cmd": true,
	".ps1": true, ".com": true, ".msi": true, ".scr": true,
}
