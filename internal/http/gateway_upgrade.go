package http

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/version"
)

const (
	defaultGatewayUpgradeScript = "/usr/local/bin/goclaw-upgrade-release"
	defaultGatewayUpgradeStatus = "/var/lib/goclaw/update-jobs/current.json"
	defaultGatewayUpgradeRepo   = "qkhalk/goclaw"
	gatewayUpgradeTokenHeader   = "X-GoClaw-Upgrade-Token"
	gatewayUpgradeRunningMaxAge = 30 * time.Minute
	githubReleasesMaxBytes      = 4 << 20
)

var gatewayUpgradeTagRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-(beta|rc)\.[0-9]+)?$`)

// plainTagRE matches clean release tags (vX.Y.Z) — the check only offers
// stable releases, never betas/rcs or fork-suffixed tags.
var plainTagRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

type gatewayUpgradeRunner interface {
	Start(tag string) error
}

type gatewayUpgradeCommandRunner struct {
	scriptPath string
}

func (r gatewayUpgradeCommandRunner) Start(tag string) error {
	cmd := exec.Command("sudo", "-n", r.scriptPath, tag)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Warn("gateway upgrade command exited with error", "error", err)
		}
	}()
	return nil
}

// GatewayUpgradeHandler triggers the host-local GoClaw release upgrade script.
// It never accepts arbitrary commands or URLs.
type GatewayUpgradeHandler struct {
	ScriptPath   string
	StatusPath   string
	TriggerToken string
	Repo         string // GitHub repo to check for releases ("owner/name")
	Version      string // the running gateway's version (cmd.Version, may include +build metadata)
	Runner       gatewayUpgradeRunner
	mu           sync.Mutex
}

func NewGatewayUpgradeHandlerFromEnv() *GatewayUpgradeHandler {
	scriptPath := strings.TrimSpace(os.Getenv("GOCLAW_UPGRADE_SCRIPT"))
	if scriptPath == "" {
		scriptPath = defaultGatewayUpgradeScript
	}
	statusPath := strings.TrimSpace(os.Getenv("GOCLAW_UPGRADE_STATUS_PATH"))
	if statusPath == "" {
		statusPath = defaultGatewayUpgradeStatus
	}
	repo := strings.TrimSpace(os.Getenv("GOCLAW_UPGRADE_REPO"))
	if repo == "" {
		repo = defaultGatewayUpgradeRepo
	}
	h := &GatewayUpgradeHandler{
		ScriptPath:   scriptPath,
		StatusPath:   statusPath,
		TriggerToken: os.Getenv("GOCLAW_UPGRADE_TRIGGER_TOKEN"),
		Repo:         repo,
	}
	h.Runner = gatewayUpgradeCommandRunner{scriptPath: h.ScriptPath}
	return h
}

func (h *GatewayUpgradeHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/system/gateway/upgrade/check", requireAuth(permissions.RoleAdmin, h.handleCheck))
	mux.HandleFunc("GET /v1/system/gateway/upgrade/status", requireAuth(permissions.RoleAdmin, h.handleStatus))
	mux.HandleFunc("POST /v1/system/gateway/upgrade", requireAuth(permissions.RoleAdmin, h.handleStart))
}

func (h *GatewayUpgradeHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	if !h.requireTriggerToken(w, r) {
		return
	}

	status, err := h.readStatus()
	if err != nil {
		slog.Error("gateway upgrade status read failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read gateway upgrade status"})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *GatewayUpgradeHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	if !enforcePackagesWriteLimit(w, r, "/v1/system/gateway/upgrade") {
		return
	}
	if !h.requireTriggerToken(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var req struct {
		Tag string `json:"tag"`
	}
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}
	tag := strings.TrimSpace(req.Tag)
	if !validGatewayUpgradeTag(tag) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tag must be latest or vMAJOR.MINOR.PATCH[-beta.N|-rc.N]"})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	status, err := h.readStatus()
	if err != nil {
		slog.Error("gateway upgrade status read failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read gateway upgrade status"})
		return
	}
	if gatewayUpgradeStatusRunning(status, time.Now().UTC()) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "gateway upgrade already running"})
		return
	}

	runner := h.Runner
	if runner == nil {
		runner = gatewayUpgradeCommandRunner{scriptPath: h.ScriptPath}
	}
	if err := h.writeRunningStatus(tag); err != nil {
		slog.Error("gateway upgrade status write failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write gateway upgrade status"})
		return
	}
	if err := runner.Start(tag); err != nil {
		slog.Error("gateway upgrade start failed", "error", err)
		_ = h.writeFailedStatus(tag, "failed to start gateway upgrade")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start gateway upgrade"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":       true,
		"accepted": true,
		"tag":      tag,
	})
}

func (h *GatewayUpgradeHandler) requireTriggerToken(w http.ResponseWriter, r *http.Request) bool {
	if h.TriggerToken != "" {
		provided := r.Header.Get(gatewayUpgradeTokenHeader)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(h.TriggerToken)) == 1 {
			return true
		}
	}
	// System-owner sessions (gateway token + configured owner ID) may trigger
	// from the web UI without the automation token; the trigger only ever
	// reaches the fixed script with a validated tag (never a URL or command).
	// Deliberately NOT IsOwnerRole: tenant owners also hold RoleOwner via
	// browser pairing, so that check would widen the bypass beyond the
	// system-owner credential path. Non-owner callers still need the token.
	if store.IsSystemOwner(r.Context()) {
		slog.Info("gateway upgrade trigger via system owner session", "path", r.URL.Path)
		return true
	}
	if h.TriggerToken == "" {
		slog.Warn("security.gateway_upgrade_token_unconfigured", "path", r.URL.Path)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway upgrade trigger token is not configured"})
		return false
	}
	slog.Warn("security.gateway_upgrade_token_denied", "path", r.URL.Path)
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "upgrade trigger token required"})
	return false
}

// --- GET /v1/system/gateway/upgrade/check ---

// githubReleaseAsset / githubCheckRelease mirror the slice of the GitHub
// Releases API the check needs.
type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubCheckRelease struct {
	TagName    string               `json:"tag_name"`
	HTMLURL    string               `json:"html_url"`
	Body       string               `json:"body"`
	Prerelease bool                 `json:"prerelease"`
	Draft      bool                 `json:"draft"`
	Assets     []githubReleaseAsset `json:"assets"`
}

// upgradeCheckClient fetches release listings; overridden in tests.
var upgradeCheckClient = &http.Client{Timeout: 30 * time.Second}

// upgradeCheckInfo is the response of the check endpoint.
type upgradeCheckInfo struct {
	Current    string `json:"current"`               // running version (build metadata stripped for display is up to the caller)
	Latest     string `json:"latest,omitempty"`      // newest stable release tag found
	Available  bool   `json:"available"`             // latest > current AND installable on this host
	Version    string `json:"version,omitempty"`     // tag of the offered update (== Latest)
	Asset      string `json:"asset,omitempty"`       // matching binary asset name for this GOOS/GOARCH
	ReleaseURL string `json:"release_url,omitempty"` // GitHub release page
	Reason     string `json:"reason,omitempty"`      // why available=false when a newer release exists
}

// installableAsset returns the release-fork binary archive of this host
// (.zip on Windows, .tar.gz elsewhere; "" when the release ships no match).
func installableAsset(assets []githubReleaseAsset) string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	suffix := fmt.Sprintf("-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
	for _, a := range assets {
		if strings.HasSuffix(a.Name, suffix) {
			return a.Name
		}
	}
	return ""
}

func (h *GatewayUpgradeHandler) handleCheck(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}
	current := strings.TrimSpace(h.Version)
	info := upgradeCheckInfo{Current: current}
	if current == "" || current == "dev" || h.Repo == "" {
		info.Reason = "version unknown — set GOCLAW_UPGRADE_REPO and build with -ldflags version"
		writeJSON(w, http.StatusOK, info)
		return
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=30", h.Repo)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build request failed"})
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := upgradeCheckClient.Do(req)
	if err != nil {
		slog.Warn("gateway upgrade check failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to query releases"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("gateway upgrade check failed", "status", resp.Status)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to query releases"})
		return
	}

	var releases []githubCheckRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, githubReleasesMaxBytes)).Decode(&releases); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to decode releases"})
		return
	}

	// Releases come newest-first; take the first stable, plain-tagged,
	// newer-than-current release that ships a binary archive for this host.
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease || !plainTagRE.MatchString(rel.TagName) {
			continue
		}
		if !version.IsNewer(rel.TagName, current) {
			break // newest stable is not newer — nothing to offer
		}
		info.Latest = rel.TagName
		info.Version = rel.TagName
		info.ReleaseURL = rel.HTMLURL
		if asset := installableAsset(rel.Assets); asset != "" {
			info.Available = true
			info.Asset = asset
		} else {
			info.Reason = fmt.Sprintf("release %s has no %s/%s binary archive", rel.TagName, runtime.GOOS, runtime.GOARCH)
		}
		break
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *GatewayUpgradeHandler) readStatus() (map[string]any, error) {
	path := h.statusPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{"state": "idle"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read upgrade status: %w", err)
	}
	if len(data) > 64*1024 {
		return nil, fmt.Errorf("upgrade status too large")
	}
	var status map[string]any
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("decode upgrade status: %w", err)
	}
	if status == nil {
		return map[string]any{"state": "idle"}, nil
	}
	return status, nil
}

func gatewayUpgradeStatusRunning(status map[string]any, now time.Time) bool {
	if status["state"] != "running" {
		return false
	}
	startedRaw, ok := status["startedAt"].(string)
	if !ok || strings.TrimSpace(startedRaw) == "" {
		return true
	}
	startedAt, err := time.Parse(time.RFC3339, startedRaw)
	if err != nil {
		return true
	}
	age := now.Sub(startedAt)
	if age < 0 || age <= gatewayUpgradeRunningMaxAge {
		return true
	}
	slog.Warn("gateway upgrade stale running status ignored", "started_at", startedRaw, "age", age.String())
	return false
}

func (h *GatewayUpgradeHandler) writeRunningStatus(tag string) error {
	return h.writeStatus(map[string]any{
		"jobId":        time.Now().UTC().Format("20060102T150405Z") + "-" + tag,
		"state":        "running",
		"requestedTag": tag,
		"resolvedTag":  "",
		"startedAt":    time.Now().UTC().Format(time.RFC3339),
		"finishedAt":   nil,
		"error":        nil,
	})
}

func (h *GatewayUpgradeHandler) writeFailedStatus(tag, reason string) error {
	return h.writeStatus(map[string]any{
		"jobId":        time.Now().UTC().Format("20060102T150405Z") + "-" + tag,
		"state":        "failed",
		"requestedTag": tag,
		"resolvedTag":  "",
		"startedAt":    time.Now().UTC().Format(time.RFC3339),
		"finishedAt":   time.Now().UTC().Format(time.RFC3339),
		"error":        reason,
	})
}

func (h *GatewayUpgradeHandler) writeStatus(status map[string]any) error {
	path := h.statusPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create upgrade status dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".current-*.json")
	if err != nil {
		return fmt.Errorf("create upgrade status tmp: %w", err)
	}
	tmpName := tmp.Name()
	encErr := json.NewEncoder(tmp).Encode(status)
	closeErr := tmp.Close()
	if encErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("encode upgrade status: %w", encErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close upgrade status tmp: %w", closeErr)
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod upgrade status tmp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace upgrade status: %w", err)
	}
	return nil
}

func (h *GatewayUpgradeHandler) statusPath() string {
	if h.StatusPath == "" {
		return defaultGatewayUpgradeStatus
	}
	return filepath.Clean(h.StatusPath)
}

func validGatewayUpgradeTag(tag string) bool {
	return tag == "latest" || gatewayUpgradeTagRE.MatchString(tag)
}
