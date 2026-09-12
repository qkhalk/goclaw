package methods

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

const (
	suggestDirsLimitDefault = 20
	suggestDirsLimitMax     = 50
)

// handleSuggestDirs lists child directories for path autocomplete in the
// workspace creation form (workspace.suggestDirs). Terminal tab-completion
// semantics: the query splits into a directory prefix plus a trailing partial
// segment; the prefix directory is read and child directories matching the
// partial (case-insensitive) are returned as absolute slash paths with a
// trailing slash. An empty query lists the sandbox base path so users can
// discover the default location instead of typing it blind. Requires operator
// role — the same gate as workspace.create, which already accepts arbitrary
// absolute root paths, so this leaks nothing create does not.
func (m *WorkspaceMethods) handleSuggestDirs(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return
	}
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "workspace.suggestDirs requires operator role")))
		return
	}
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"directories": suggestDirectories(m.basePath, params.Query, params.Limit),
	}))
}

// suggestDirectories is the pure core of workspace.suggestDirs. It never
// creates, writes, or follows into directories — a single os.ReadDir on the
// resolved prefix. Read errors (missing dir, permissions) yield an empty
// list, not an error: the client just shows no suggestions.
func suggestDirectories(basePath, rawQuery string, limit int) []string {
	if limit <= 0 {
		limit = suggestDirsLimitDefault
	}
	if limit > suggestDirsLimitMax {
		limit = suggestDirsLimitMax
	}
	dir, partial, ok := splitPathQuery(basePath, strings.TrimSpace(rawQuery))
	if !ok {
		return []string{}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Debug("workspace.suggest_dirs_read_failed", "dir", dir, "error", err)
		return []string{}
	}
	partialLower := strings.ToLower(partial)
	showHidden := strings.HasPrefix(partial, ".")
	var names []string
	for _, e := range entries {
		// Symlinks report IsDir()==false from ReadDir, so linked directories
		// are skipped — suggestions only ever name real directories.
		name := e.Name()
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(name, ".") && !showHidden {
			continue
		}
		if partial != "" && !strings.HasPrefix(strings.ToLower(name), partialLower) {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	if len(names) > limit {
		names = names[:limit]
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.ToSlash(filepath.Join(dir, name))+"/")
	}
	return out
}

// splitPathQuery resolves a typed query to the directory to list plus the
// partial last segment to filter by. Empty query targets basePath with an
// empty partial. A leading ~ expands to the user home. Relative queries join
// basePath, mirroring sanitizeRootPath. Any ".." segment is rejected so the
// listing cannot be pointed above an explicitly typed absolute path.
func splitPathQuery(basePath, query string) (dir, partial string, ok bool) {
	if query == "" {
		return filepath.Clean(basePath), "", true
	}
	expanded := filepath.ToSlash(query)
	if expanded == "~" || strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", false
		}
		expanded = filepath.ToSlash(filepath.Join(home, strings.TrimPrefix(expanded, "~")))
	}
	// Reject ".." on the raw input BEFORE resolution: Join/Clean would
	// silently collapse it, turning "../x" into a listing of basePath's
	// parent instead of a rejection.
	for _, seg := range strings.Split(expanded, "/") {
		if seg == ".." {
			return "", "", false
		}
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.ToSlash(filepath.Join(basePath, expanded))
	}
	idx := strings.LastIndex(expanded, "/")
	dir = filepath.Clean(expanded[:idx+1])
	partial = expanded[idx+1:]
	return dir, partial, true
}
