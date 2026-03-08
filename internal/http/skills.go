package http

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

const maxSkillUploadSize = 20 << 20 // 20 MB

var slugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// SkillsHandler handles skill management HTTP endpoints.
type SkillsHandler struct {
	skills  *pg.PGSkillStore
	baseDir string // filesystem base for skill content
	token   string
	msgBus  *bus.MessageBus
}

// NewSkillsHandler creates a handler for skill management endpoints.
func NewSkillsHandler(skills *pg.PGSkillStore, baseDir, token string, msgBus *bus.MessageBus) *SkillsHandler {
	return &SkillsHandler{skills: skills, baseDir: baseDir, token: token, msgBus: msgBus}
}

// emitCacheInvalidate broadcasts a cache invalidation event if msgBus is set.
func (h *SkillsHandler) emitCacheInvalidate(kind, key string) {
	if h.msgBus == nil {
		return
	}
	h.msgBus.Broadcast(bus.Event{
		Name:    protocol.EventCacheInvalidate,
		Payload: bus.CacheInvalidatePayload{Kind: kind, Key: key},
	})
}

// RegisterRoutes registers all skill management routes on the given mux.
func (h *SkillsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/skills", h.authMiddleware(h.handleList))
	mux.HandleFunc("POST /v1/skills/upload", h.authMiddleware(h.handleUpload))
	mux.HandleFunc("GET /v1/skills/{id}", h.authMiddleware(h.handleGet))
	mux.HandleFunc("PUT /v1/skills/{id}", h.authMiddleware(h.handleUpdate))
	mux.HandleFunc("DELETE /v1/skills/{id}", h.authMiddleware(h.handleDelete))
	mux.HandleFunc("POST /v1/skills/{id}/grants/agent", h.authMiddleware(h.handleGrantAgent))
	mux.HandleFunc("DELETE /v1/skills/{id}/grants/agent/{agentID}", h.authMiddleware(h.handleRevokeAgent))
	mux.HandleFunc("POST /v1/skills/{id}/grants/user", h.authMiddleware(h.handleGrantUser))
	mux.HandleFunc("DELETE /v1/skills/{id}/grants/user/{userID}", h.authMiddleware(h.handleRevokeUser))
	mux.HandleFunc("GET /v1/agents/{agentID}/skills", h.authMiddleware(h.handleListAgentSkills))
	mux.HandleFunc("GET /v1/skills/{id}/versions", h.authMiddleware(h.handleListVersions))
	mux.HandleFunc("GET /v1/skills/{id}/files/{path...}", h.authMiddleware(h.handleReadFile))
	mux.HandleFunc("GET /v1/skills/{id}/files", h.authMiddleware(h.handleListFiles))
}

func (h *SkillsHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.token != "" {
			if extractBearerToken(r) != h.token {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		userID := extractUserID(r)
		if userID != "" {
			ctx := store.WithUserID(r.Context(), userID)
			r = r.WithContext(ctx)
		}
		next(w, r)
	}
}

func (h *SkillsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	skills := h.skills.ListSkills()
	writeJSON(w, http.StatusOK, map[string]interface{}{"skills": skills})
}

func (h *SkillsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	skill, ok := h.skills.GetSkill(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	writeJSON(w, http.StatusOK, skill)
}

func (h *SkillsHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill ID"})
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	// Prevent changing sensitive fields
	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "file_path")

	if err := h.skills.UpdateSkill(id, updates); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.skills.BumpVersion()
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *SkillsHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill ID"})
		return
	}

	if err := h.skills.DeleteSkill(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.skills.BumpVersion()
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// handleUpload processes a ZIP file upload containing a skill (must have SKILL.md at root).
func (h *SkillsHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	userID := store.UserIDFromContext(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "X-GoClaw-User-Id header required"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSkillUploadSize)

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required: " + err.Error()})
		return
	}
	defer file.Close()

	// Save to temp file for zip processing
	tmp, err := os.CreateTemp("", "skill-upload-*.zip")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create temp file"})
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save upload"})
		return
	}
	fileHash := fmt.Sprintf("%x", hasher.Sum(nil))

	// Open as zip
	zr, err := zip.OpenReader(tmp.Name())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ZIP file"})
		return
	}
	defer zr.Close()

	// Validate: must have SKILL.md at root or inside a single top-level directory.
	// Many ZIP tools wrap contents in a folder (e.g. "my-skill/SKILL.md").
	var skillMD *zip.File
	var stripPrefix string
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		if name == "SKILL.md" {
			skillMD = f
			stripPrefix = ""
			break
		}
		// Allow one level of directory nesting: "dirname/SKILL.md"
		parts := strings.SplitN(name, "/", 3)
		if len(parts) == 2 && parts[1] == "SKILL.md" && !f.FileInfo().IsDir() {
			skillMD = f
			stripPrefix = parts[0] + "/"
			break
		}
	}
	if skillMD == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ZIP must contain SKILL.md at root (or inside a single top-level directory)"})
		return
	}

	// Read and parse SKILL.md frontmatter
	skillContent, err := readZipFile(skillMD)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read SKILL.md"})
		return
	}

	name, description, slug, frontmatter := parseSkillFrontmatter(skillContent)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "SKILL.md must have a name in frontmatter"})
		return
	}
	if slug == "" {
		slug = slugify(name)
	}
	if !slugRegexp.MatchString(slug) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug must be a valid slug (lowercase letters, numbers, hyphens only)"})
		return
	}

	// Determine version (always increment — includes archived skills so re-upload gets v2+)
	version := h.skills.GetNextVersion(slug)

	// Extract to filesystem: baseDir/slug/version/
	destDir := filepath.Join(h.baseDir, slug, fmt.Sprintf("%d", version))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create skill directory"})
		return
	}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Skip symlinks in ZIP — prevent directory escape attacks
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		// Strip wrapper directory prefix if ZIP had one
		entryName := strings.TrimPrefix(f.Name, "./")
		if stripPrefix != "" {
			entryName = strings.TrimPrefix(entryName, stripPrefix)
			if entryName == "" {
				continue
			}
		}
		// Skip macOS/system artifacts
		if isSystemArtifact(entryName) {
			continue
		}
		// Security: prevent path traversal
		name := filepath.Clean(entryName)
		if strings.Contains(name, "..") {
			continue
		}
		destPath := filepath.Join(destDir, name)
		if !strings.HasPrefix(destPath, destDir+string(filepath.Separator)) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			continue
		}
		data, err := readZipFile(f)
		if err != nil {
			continue
		}
		os.WriteFile(destPath, []byte(data), 0644)
	}

	// Save metadata to DB
	desc := description
	skill := pg.SkillCreateParams{
		Name:        name,
		Slug:        slug,
		Description: &desc,
		OwnerID:     userID,
		Visibility:  "internal",
		Version:     version,
		FilePath:    destDir,
		FileSize:    size,
		FileHash:    &fileHash,
		Frontmatter: frontmatter,
	}

	id, err := h.skills.CreateSkillManaged(r.Context(), skill)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save skill: " + err.Error()})
		return
	}

	h.skills.BumpVersion()
	slog.Info("skill uploaded", "id", id, "slug", slug, "version", version, "size", header.Size)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      id,
		"slug":    slug,
		"version": version,
		"name":    name,
	})
}

func (h *SkillsHandler) handleListAgentSkills(w http.ResponseWriter, r *http.Request) {
	agentIDStr := r.PathValue("agentID")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	skills, err := h.skills.ListWithGrantStatus(r.Context(), agentID)
	if err != nil {
		slog.Error("failed to list skills with grant status", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list skills"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"skills": skills})
}

// handleListVersions returns all available version numbers for a skill.
func (h *SkillsHandler) handleListVersions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill ID"})
		return
	}

	_, slug, currentVersion, ok := h.skills.GetSkillFilePath(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}

	slugDir := filepath.Join(h.baseDir, slug)
	entries, err := os.ReadDir(slugDir)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"versions": []int{currentVersion},
			"current":  currentVersion,
		})
		return
	}

	var versions []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := strconv.Atoi(e.Name())
		if err != nil || v < 1 {
			continue
		}
		versions = append(versions, v)
	}
	sort.Ints(versions)
	if len(versions) == 0 {
		versions = []int{currentVersion}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"versions": versions,
		"current":  currentVersion,
	})
}

// handleListFiles returns all files in a skill version directory.
func (h *SkillsHandler) handleListFiles(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill ID"})
		return
	}

	_, slug, currentVersion, ok := h.skills.GetSkillFilePath(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}

	version := currentVersion
	if v := r.URL.Query().Get("version"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid version"})
			return
		}
		version = parsed
	}

	versionDir := filepath.Join(h.baseDir, slug, strconv.Itoa(version))
	if _, err := os.Stat(versionDir); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version not found"})
		return
	}

	type fileEntry struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		IsDir bool   `json:"isDir"`
		Size  int64  `json:"size"`
	}

	var files []fileEntry
	filepath.WalkDir(versionDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(versionDir, path)
		if rel == "." {
			return nil
		}
		// Skip system artifacts (__MACOSX, .DS_Store, etc.)
		if isSystemArtifact(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip symlinks — prevent escape from skill directory
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		entry := fileEntry{
			Path:  rel,
			Name:  d.Name(),
			IsDir: d.IsDir(),
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				entry.Size = info.Size()
			}
		}
		files = append(files, entry)
		return nil
	})

	if files == nil {
		files = []fileEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
}

// handleReadFile reads a single file from a skill version directory.
func (h *SkillsHandler) handleReadFile(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill ID"})
		return
	}

	relPath := r.PathValue("path")
	if relPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	if strings.Contains(relPath, "..") {
		slog.Warn("security.skill_files_traversal", "path", relPath)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	_, slug, currentVersion, ok := h.skills.GetSkillFilePath(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}

	version := currentVersion
	if v := r.URL.Query().Get("version"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid version"})
			return
		}
		version = parsed
	}

	versionDir := filepath.Join(h.baseDir, slug, strconv.Itoa(version))
	absPath := filepath.Join(versionDir, filepath.Clean(relPath))

	// Verify resolved path is within the version directory
	if !strings.HasPrefix(absPath, versionDir+string(filepath.Separator)) {
		slog.Warn("security.skill_files_escape", "resolved", absPath, "root", versionDir)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	// Use Lstat to detect symlinks — reject them to prevent directory escape
	info, err := os.Lstat(absPath)
	if err != nil || info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		slog.Warn("security.skill_files_symlink", "path", absPath)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	// Skip system artifacts
	if isSystemArtifact(relPath) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read file"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": string(data),
		"path":    relPath,
		"size":    info.Size(),
	})
}
