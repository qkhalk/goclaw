package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/mcp/installer"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	mcpcatalog "github.com/nextlevelbuilder/goclaw/mcp"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// MCPInstallHandler serves the Tool Store's MCP-server catalog and runs
// install/uninstall jobs (tier 3: tool servers fetched from github.com at
// the pinned ref). Install runs as a background job because clone + deps
// take minutes; the UI polls GET /v1/mcp/install/{job}.
type MCPInstallHandler struct {
	installer *installer.Installer
	tenants   store.TenantStore
	msgBus    *bus.MessageBus
	dataDir   string
	version   string

	mu      sync.Mutex
	jobs    map[string]*mcpInstallJob
	running bool // single-flight: one install pipeline at a time

	dynCatalog *dynamicCatalogCache // lazily initialized; background-refreshed
}

// NewMCPInstallHandler wires the installer surface. dataDir is the resolved
// base data directory; installs land under its tenant-scoped mcp/ folder.
func NewMCPInstallHandler(servers store.MCPServerStore, installs store.MCPInstallStore, tenants store.TenantStore, msgBus *bus.MessageBus, dataDir, version string) *MCPInstallHandler {
	return &MCPInstallHandler{
		installer: &installer.Installer{
			Servers:  servers,
			Installs: installs,
			Version:  version,
		},
		tenants: tenants,
		msgBus:  msgBus,
		dataDir: dataDir,
		version: version,
		jobs:    map[string]*mcpInstallJob{},
	}
}

// SetPoolEvictor wires pool eviction so uninstalls/re-registers cannot leave
// a stale spawned server process behind.
func (h *MCPInstallHandler) SetPoolEvictor(e MCPPoolEvictor) {
	h.installer.EvictServer = e.EvictServer
}

// mcpInstallJob is the in-memory progress record for one install run.
// Package rows in the DB carry the durable state; jobs only live for the
// HTTP polling surface and are capped to keep memory bounded. The JSON
// state lives in a nested struct so snapshots copy it without dragging the
// mutex along (go vet: copies lock value).
type mcpInstallJob struct {
	mu    sync.Mutex
	state mcpInstallJobState
}

type mcpInstallJobState struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     string         `json:"status"` // running | done | error
	Step       installer.Step `json:"step,omitempty"`
	Progress   int            `json:"progress"`
	Log        []string       `json:"log,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}

const (
	mcpInstallJobLogCap = 40
	mcpInstallJobCap    = 40
)

func (j *mcpInstallJob) snapshot() mcpInstallJobState {
	j.mu.Lock()
	defer j.mu.Unlock()
	cp := j.state
	cp.Log = append([]string(nil), j.state.Log...)
	return cp
}

func (j *mcpInstallJob) update(step installer.Step, pct int, line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.state.Step = step
	j.state.Progress = pct
	j.state.Log = append(j.state.Log, time.Now().Format("15:04:05")+" "+line)
	if len(j.state.Log) > mcpInstallJobLogCap {
		j.state.Log = j.state.Log[len(j.state.Log)-mcpInstallJobLogCap:]
	}
}

func (j *mcpInstallJob) finish(status, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now()
	j.state.Status = status
	j.state.Error = errMsg
	j.state.FinishedAt = &now
}

func (j *mcpInstallJob) status() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state.Status
}

// RecoverStaleInstalls marks rows stuck at "installing" as failed. Called
// once at gateway startup: an install interrupted by a restart or a crashed
// job would otherwise 409 every retry ("still installing") forever.
func (h *MCPInstallHandler) RecoverStaleInstalls() {
	if !h.storesReady() {
		return
	}
	ctx := store.WithCrossTenant(context.Background())
	pkgs, err := h.installer.Installs.ListPackages(ctx)
	if err != nil {
		slog.Warn("mcp_install.recover_stale", "error", err)
		return
	}
	for _, p := range pkgs {
		if p.Status != store.MCPInstallStatusInstalling {
			continue
		}
		errMsg := "interrupted by gateway restart"
		p.Status = store.MCPInstallStatusFailed
		p.Error = &errMsg
		if err := h.installer.Installs.UpsertPackage(ctx, &p); err != nil {
			slog.Warn("mcp_install.recover_stale", "name", p.Name, "error", err)
			continue
		}
		slog.Info("mcp_install.recovered_stale", "name", p.Name)
	}
}

// RegisterRoutes registers the installer API. Catalog/installed reads are
// viewer+; install/uninstall are admin+ and tenant-gated (installs register
// servers into the caller's tenant).
func (h *MCPInstallHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/mcp/catalog", h.auth(h.handleCatalog))
	mux.HandleFunc("GET /v1/mcp/installed", h.auth(h.handleListInstalled))
	mux.HandleFunc("GET /v1/mcp/install/{jobID}", h.auth(h.handleJobStatus))
	mux.HandleFunc("POST /v1/mcp/install", h.adminAuth(h.handleInstall))
	mux.HandleFunc("DELETE /v1/mcp/installed/{name}", h.adminAuth(h.handleUninstall))
}

func (h *MCPInstallHandler) auth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth("", next)
}

func (h *MCPInstallHandler) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(permissions.RoleAdmin, next)
}

// storesReady reports whether the handler was wired with its stores. The
// gateway only registers the handler when they exist; this guard turns any
// stray registration into a clean 503 instead of a nil-pointer panic.
func (h *MCPInstallHandler) storesReady() bool {
	return h.installer != nil && h.installer.Installs != nil && h.installer.Servers != nil
}

// catalogEntry merges an embedded manifest with its installed state.
type catalogEntry struct {
	mcpcatalog.Manifest
	Repo      string                     `json:"repo"` // resolved (manifest pin or default)
	Installed bool                       `json:"installed"`
	Dynamic   bool                       `json:"dynamic"` // discovered from the catalog repo's latest release (not embedded)
	Package   *store.MCPInstalledPackage `json:"package,omitempty"`
}

func (h *MCPInstallHandler) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if !h.storesReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "installer not configured"})
		return
	}

	pkgs, err := h.installer.Installs.ListPackages(r.Context())
	if err != nil {
		slog.Error("mcp_install.list_packages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list installed packages"})
		return
	}
	byName := map[string]store.MCPInstalledPackage{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}

	entries := make([]catalogEntry, 0)
	seen := map[string]bool{}
	addEntry := func(m mcpcatalog.Manifest, dynamic bool) {
		if seen[m.Name] {
			return
		}
		seen[m.Name] = true
		e := catalogEntry{Manifest: m, Dynamic: dynamic}
		if m.Repo != "" {
			e.Repo = m.Repo
		} else {
			e.Repo = mcpcatalog.DefaultRepo
		}
		if p, ok := byName[m.Name]; ok {
			e.Installed = true
			pkg := p
			e.Package = &pkg
		}
		entries = append(entries, e)
	}
	for _, m := range mcpcatalog.Entries() {
		addEntry(m, false)
	}
	// Dynamic entries from the catalog repo's latest release tag appear
	// without a gateway upgrade; embedded curated entries win on conflict.
	if st := h.dynamicSnapshot(); st != nil {
		for _, m := range st.entries {
			addEntry(m, true)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entries":      entries,
		"default_repo": mcpcatalog.DefaultRepo,
	})
}

func (h *MCPInstallHandler) handleListInstalled(w http.ResponseWriter, r *http.Request) {
	if !h.storesReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "installer not configured"})
		return
	}
	pkgs, err := h.installer.Installs.ListPackages(r.Context())
	if err != nil {
		slog.Error("mcp_install.list_packages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list installed packages"})
		return
	}
	if pkgs == nil {
		pkgs = []store.MCPInstalledPackage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": pkgs})
}

// installRequest is either {"name": "<catalog entry>"} or a full custom
// GitHub install spec.
type installRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Repo        string `json:"repo"`
	Ref         string `json:"ref"`
	Subdir      string `json:"subdir"`
	Runtime     string `json:"runtime"`
	Entry       string `json:"entry"`
}

func (h *MCPInstallHandler) handleInstall(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	if !requireTenantAdmin(w, r, h.tenants) {
		return
	}

	if !h.storesReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "installer not configured"})
		return
	}

	var req installRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidJSON)})
		return
	}

	// Resolve catalog entry when only a name is given; otherwise every
	// custom field must be present and valid on its own.
	ireq := installer.Request{CreatedBy: store.UserIDFromContext(r.Context())}
	var catManifest *mcpcatalog.Manifest
	if m := mcpcatalog.Find(req.Name); m != nil {
		catManifest = m
	} else {
		catManifest = h.findDynamic(req.Name)
	}
	if req.Repo == "" && catManifest != nil {
		m := catManifest
		ireq.Name = m.Name
		ireq.DisplayName = m.DisplayName
		ireq.Source = "catalog"
		if m.Repo != "" {
			ireq.Repo = m.Repo
		} else {
			ireq.Repo = mcpcatalog.DefaultRepo
		}
		ireq.Ref = m.Ref
		ireq.Subdir = m.Subdir
		ireq.Runtime = m.Runtime
		ireq.Entry = m.Entry
	} else {
		ireq.Name = req.Name
		ireq.DisplayName = req.DisplayName
		ireq.Source = "custom"
		ireq.Repo = req.Repo
		ireq.Ref = req.Ref
		ireq.Subdir = req.Subdir
		ireq.Runtime = req.Runtime
		ireq.Entry = req.Entry
	}

	if ireq.Name == "" || !isValidSlug(ireq.Name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidSlug, "name")})
		return
	}
	if ireq.Repo == "" || ireq.Runtime == "" || ireq.Entry == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgRequired, "repo, runtime, entry")})
		return
	}

	// Tenant-scoped install root (master tenant → <dataDir>/mcp).
	tenantID := store.TenantIDFromContext(r.Context())
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	slug := store.TenantSlugFromContext(r.Context())
	ireq.InstallRoot = config.TenantScopedDir(h.dataDir, tenantID.String(), slug) + "/mcp"

	// Guard: package already installed (or a failed row from a previous run
	// the admin must clear first), and one install at a time globally — the
	// pipeline spawns package managers and clones; overlapping runs on a
	// 512MB host are how OOMs happen.
	if existing, err := h.installer.Installs.GetPackageByName(r.Context(), ireq.Name); err == nil && existing != nil {
		if existing.Status == store.MCPInstallStatusInstalled {
			writeJSON(w, http.StatusConflict, map[string]string{"error": ireq.Name + " is already installed — uninstall first"})
			return
		}
		if existing.Status == store.MCPInstallStatusInstalling {
			writeJSON(w, http.StatusConflict, map[string]string{"error": ireq.Name + " is still installing"})
			return
		}
	}

	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "another install is in progress"})
		return
	}
	h.running = true
	h.mu.Unlock()

	job := &mcpInstallJob{state: mcpInstallJobState{
		ID:        uuid.NewString(),
		Name:      ireq.Name,
		Status:    "running",
		StartedAt: time.Now(),
	}}
	h.mu.Lock()
	h.jobs[job.state.ID] = job
	h.mu.Unlock()

	go h.runJob(job, ireq, tenantID)

	emitAudit(h.msgBus, r, "mcp_package.install_started", "mcp_package", ireq.Name)
	slog.Info("mcp_install.started", "name", ireq.Name, "repo", ireq.Repo, "ref", ireq.Ref, "job", job.state.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.state.ID})
}

// runJob executes the pipeline detached from the request and always clears
// the single-flight flag.
func (h *MCPInstallHandler) runJob(job *mcpInstallJob, ireq installer.Request, tenantID uuid.UUID) {
	defer func() {
		h.mu.Lock()
		h.running = false
		// Hard cap: evict the oldest FINISHED jobs until we are back under
		// the cap, regardless of age — the map must never grow unbounded.
		for len(h.jobs) > mcpInstallJobCap {
			oldestID := ""
			var oldest time.Time
			for id, j := range h.jobs {
				if j.status() == "running" {
					continue
				}
				st := j.snapshot().StartedAt
				if oldestID == "" || st.Before(oldest) {
					oldestID, oldest = id, st
				}
			}
			if oldestID == "" {
				break // only running jobs left — cannot shrink now
			}
			delete(h.jobs, oldestID)
		}
		h.mu.Unlock()
	}()

	// The installer reads/writes package rows with the tenant in ctx; the
	// request ctx dies with the response, so run detached with the tenant.
	ctx := store.WithTenantID(context.Background(), tenantID)
	res, err := h.installer.Install(ctx, ireq, job.update)
	if err != nil {
		job.finish("error", err.Error())
		slog.Error("mcp_install.failed", "name", ireq.Name, "job", job.state.ID, "error", err)
		return
	}
	job.finish("done", "")
	slog.Info("mcp_install.done", "name", ireq.Name, "job", job.state.ID, "sha", res.Package.CommitSHA, "tools", res.Package.ToolCount)
	h.emitCacheInvalidate()
}

func (h *MCPInstallHandler) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	job, ok := h.jobs[r.PathValue("jobID")]
	h.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
}

func (h *MCPInstallHandler) handleUninstall(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	if !requireTenantAdmin(w, r, h.tenants) {
		return
	}
	if !h.storesReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "installer not configured"})
		return
	}
	name := r.PathValue("name")

	// Never race a live pipeline: RemoveAll of the install dir mid-npm, or a
	// DeletePackage colliding with the final UpsertPackage, corrupts state.
	h.mu.Lock()
	running := h.running
	h.mu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "an install is in progress — try again after it finishes"})
		return
	}
	if pkg, err := h.installer.Installs.GetPackageByName(r.Context(), name); err == nil && pkg.Status == store.MCPInstallStatusInstalling {
		writeJSON(w, http.StatusConflict, map[string]string{"error": name + " is still installing"})
		return
	}

	tenantID := store.TenantIDFromContext(r.Context())
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	if err := h.installer.Uninstall(r.Context(), tenantID, name); err != nil {
		slog.Error("mcp_install.uninstall", "name", name, "error", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "package", name)})
		return
	}
	h.emitCacheInvalidate()
	emitAudit(h.msgBus, r, "mcp_package.uninstalled", "mcp_package", name)
	slog.Info("mcp_install.uninstalled", "name", name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}

func (h *MCPInstallHandler) emitCacheInvalidate() {
	if h.msgBus == nil {
		return
	}
	h.msgBus.Broadcast(bus.Event{
		Name:    protocol.EventCacheInvalidate,
		Payload: bus.CacheInvalidatePayload{Kind: bus.CacheKindMCP},
	})
}
