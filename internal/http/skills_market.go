package http

// skills_market.go — bundled-skill market endpoints on the SkillsHandler.
//
// GET    /v1/skills/market                    (viewer+)  catalog with installed flags
// POST   /v1/skills/market/install            (master)   install slugs (+optional grants)
// DELETE /v1/skills/market/installed/{slug}   (master)   uninstall a bundled skill
// POST   /v1/skills/market/update/{slug}      (master)   refresh from bundled source
//
// Mutations require master scope (not just tenant admin): bundled rows are
// seeded into the master tenant and are global to the deployment, so a
// tenant admin must not be able to uninstall a bundled skill for everyone.
// Mutating handlers serialize on one shared market lock — installs compute
// version directories (nextVersion + CopyDir) that race under concurrency.
//
// Installs are local directory copies, so they run synchronously and return
// a plain JSON result — no background job layer. A very large slug list
// (100+) takes proportionally longer (one small copy + upsert per skill)
// but stays well within normal request budgets.

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// marketFor builds a Market bound to the caller's tenant-scoped skills-store
// directory. Returns nil when the handler's store does not implement the
// needed surfaces (file-backed stores in tests).
func (h *SkillsHandler) marketFor(r *http.Request) *skills.Market {
	manage, ok := h.skills.(skills.MarketManageStore)
	if !ok {
		return nil
	}
	seedStore, ok := h.skills.(skills.SystemSkillStore)
	if !ok {
		return nil
	}
	bundledDir := h.bundledDir
	if bundledDir == "" {
		bundledDir = skills.ResolveBundledSkillsDir()
	}
	if bundledDir == "" {
		return nil
	}
	return skills.NewMarket(bundledDir, h.tenantSkillsDir(r), seedStore, manage)
}

// validMarketSlug is a light path-traversal guard; the market itself only
// operates on slugs that exist in the bundled catalog, which is built from
// ReadDir and therefore cannot contain separators.
func validMarketSlug(slug string) bool {
	if slug == "" || len(slug) > 128 || strings.ContainsAny(slug, "/\\") || strings.Contains(slug, "..") {
		return false
	}
	return true
}

func (h *SkillsHandler) handleMarketList(w http.ResponseWriter, r *http.Request) {
	market := h.marketFor(r)
	if market == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill market unavailable (no bundled skills directory or store support)"})
		return
	}
	rows, err := market.Catalog(r.Context())
	if err != nil {
		slog.Error("market catalog failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"skills":     rows,
		"total":      len(rows),
		"bundledDir": market.BundledDir(),
	})
}

func (h *SkillsHandler) handleMarketInstall(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	locale := store.LocaleFromContext(r.Context())
	userID := store.UserIDFromContext(r.Context())

	var req struct {
		Slugs         []string `json:"slugs"`
		GrantAgentIDs []string `json:"grantAgentIds"`      // camelCase (desktop convention)
		GrantAgents   []string `json:"grant_agent_ids"`    // snake_case alias
	}
	if !bindJSON(w, r, locale, &req) {
		return
	}
	if len(req.Slugs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slugs is required"})
		return
	}
	for _, slug := range req.Slugs {
		if !validMarketSlug(slug) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid slug: " + slug})
			return
		}
	}

	grantIDs := make([]uuid.UUID, 0, len(req.GrantAgentIDs)+len(req.GrantAgents))
	for _, raw := range append(append([]string{}, req.GrantAgentIDs...), req.GrantAgents...) {
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id: " + raw})
			return
		}
		grantIDs = append(grantIDs, id)
	}

	lock := h.skillUploadLock("market")
	lock.Lock()
	defer lock.Unlock()
	market := h.marketFor(r)
	if market == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill market unavailable (no bundled skills directory or store support)"})
		return
	}
	res, err := market.Install(r.Context(), req.Slugs, grantIDs, userID)
	if err != nil {
		slog.Warn("market install failed", "slugs", req.Slugs, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if len(res.Installed) > 0 {
		h.emitCacheInvalidate(bus.CacheKindSkills, "", store.TenantIDFromContext(r.Context()))
		h.emitCacheInvalidate(bus.CacheKindSkillGrants, "", uuid.Nil)
	}
	emitAudit(h.msgBus, r, "skill.market_installed", "skill", strings.Join(res.Installed, ","))
	writeJSON(w, http.StatusOK, res)
}

func (h *SkillsHandler) handleMarketUninstall(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	slug := r.PathValue("slug")
	if !validMarketSlug(slug) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid slug"})
		return
	}
	lock := h.skillUploadLock("market")
	lock.Lock()
	defer lock.Unlock()
	market := h.marketFor(r)
	if market == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill market unavailable (no bundled skills directory or store support)"})
		return
	}
	if err := market.Uninstall(r.Context(), slug); err != nil {
		slog.Warn("market uninstall failed", "slug", slug, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.emitCacheInvalidate(bus.CacheKindSkills, "", store.TenantIDFromContext(r.Context()))
	h.emitCacheInvalidate(bus.CacheKindSkillGrants, "", uuid.Nil)
	emitAudit(h.msgBus, r, "skill.market_uninstalled", "skill", slug)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *SkillsHandler) handleMarketUpdate(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	slug := r.PathValue("slug")
	if !validMarketSlug(slug) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid slug"})
		return
	}
	lock := h.skillUploadLock("market")
	lock.Lock()
	defer lock.Unlock()
	market := h.marketFor(r)
	if market == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "skill market unavailable (no bundled skills directory or store support)"})
		return
	}
	if err := market.Update(r.Context(), slug); err != nil {
		slog.Warn("market update failed", "slug", slug, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.emitCacheInvalidate(bus.CacheKindSkills, "", store.TenantIDFromContext(r.Context()))
	emitAudit(h.msgBus, r, "skill.market_updated", "skill", slug)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// registerMarketRoutes mounts the market routes. Called from
// RegisterRoutes; kept separate so the market surface stays readable.
func (h *SkillsHandler) registerMarketRoutes(mux *http.ServeMux) {
	// Catalog read is viewer+ (same floor as the skills list).
	mux.HandleFunc("GET /v1/skills/market", h.authMiddleware(h.handleMarketList))
	// Installs/uninstalls mutate master-tenant (global) skill rows — admin
	// role floor here, master-scope enforcement inside the handlers.
	mux.HandleFunc("POST /v1/skills/market/install", h.tenantAdminMiddleware(h.handleMarketInstall))
	mux.HandleFunc("DELETE /v1/skills/market/installed/{slug}", h.tenantAdminMiddleware(h.handleMarketUninstall))
	mux.HandleFunc("POST /v1/skills/market/update/{slug}", h.tenantAdminMiddleware(h.handleMarketUpdate))
}
