// Package installer installs MCP tool servers from git into a managed
// directory and registers them as stdio MCP servers (Tool Store tier 3).
//
// Pipeline per install: preflight (runtime + git + URL allowlist) → sparse
// git clone pinned to a tag/commit → dependency install (npm/pip, skipped
// for zero-dep tools) → smoke test (real MCP initialize + tools/list) →
// register into the mcp_servers registry. Every step reports progress; any
// failure rolls back partial files and marks the package row failed.
//
// Security posture: only github.com repositories are accepted, refs must be
// explicit tags/commits, and the registered server command stays a bare
// allowlisted runtime name (internal/mcp validation) with the installed
// entry file as its argument — package managers never run at server start,
// only inside this installer.
package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/mcp"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Step names one pipeline phase; the HTTP job surfaces them as progress.
type Step string

const (
	StepPreflight Step = "preflight"
	StepClone     Step = "clone"
	StepDeps      Step = "deps"
	StepSmoke     Step = "smoke"
	StepRegister  Step = "register"
)

// Progress receives pipeline updates (rough percent + last log line).
type Progress func(step Step, pct int, line string)

// Request describes one install. Catalog entries are resolved by the HTTP
// layer (which merges manifests); custom installs pass everything explicitly.
type Request struct {
	Name        string // slug — becomes the mcp_servers registry name
	DisplayName string
	Source      string // "catalog" | "custom"
	Repo        string // https://github.com/owner/repo
	Ref         string // tag or commit; empty → gateway version tag
	Subdir      string // tool folder inside the repo; empty → mcp/<name>
	Runtime     string // "node" | "python"
	Entry       string // entry file relative to the tool folder
	InstallRoot string // tenant-scoped dir that will hold <name>/
	CreatedBy   string // requesting user (audited on the server row)
}

// Result is what a successful install produced.
type Result struct {
	Package store.MCPInstalledPackage
	Tools   []mcp.ToolInfo
}

// Installer runs install/uninstall pipelines against the stores.
type Installer struct {
	Servers  store.MCPServerStore
	Installs store.MCPInstallStore
	// Version is the running gateway version, used to pin the default
	// install ref (catalog manifests ship with the binary).
	Version string
	// EvictServer, when set, drops pooled connections for a server before
	// uninstall/re-register so stale processes cannot linger.
	EvictServer func(tenantID uuid.UUID, serverName string)
}

// installTimeout bounds the whole pipeline; sub-steps get their own slices.
const (
	installTimeout = 10 * time.Minute
	cloneTimeout   = 5 * time.Minute
	depsTimeout    = 5 * time.Minute
)

// Install runs the full pipeline. It creates the package row (installing)
// right after preflight, so a crashed install leaves a repairable trace,
// and flips it to installed/failed at the end.
func (in *Installer) Install(ctx context.Context, req Request, progress Progress) (*Result, error) {
	if progress == nil {
		progress = func(Step, int, string) {}
	}
	ctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()

	if err := in.preflight(ctx, &req, progress); err != nil {
		return nil, err
	}

	pkg := store.MCPInstalledPackage{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Source:      req.Source,
		Repo:        req.Repo,
		Ref:         req.Ref,
		Runtime:     req.Runtime,
		Entry:       req.Entry,
		InstallDir:  filepath.Join(req.InstallRoot, req.Name),
		Status:      store.MCPInstallStatusInstalling,
	}
	if err := in.Installs.UpsertPackage(ctx, &pkg); err != nil {
		return nil, fmt.Errorf("persist package row: %w", err)
	}

	res, err := in.run(ctx, &req, &pkg, progress)
	if err != nil {
		errMsg := err.Error()
		pkg.Status = store.MCPInstallStatusFailed
		pkg.Error = &errMsg
		_ = in.Installs.UpsertPackage(ctx, &pkg)
		return nil, err
	}
	return res, nil
}

// run executes clone → deps → smoke → register.
func (in *Installer) run(ctx context.Context, req *Request, pkg *store.MCPInstalledPackage, progress Progress) (*Result, error) {
	sha, err := in.clone(ctx, req, pkg, progress)
	if err != nil {
		_ = os.RemoveAll(pkg.InstallDir)
		return nil, fmt.Errorf("clone: %w", err)
	}
	pkg.CommitSHA = sha

	if err := in.installDeps(ctx, req, pkg, progress); err != nil {
		_ = os.RemoveAll(pkg.InstallDir)
		return nil, fmt.Errorf("deps: %w", err)
	}

	command, args, env, tools, err := in.smoke(ctx, req, pkg, progress)
	if err != nil {
		_ = os.RemoveAll(pkg.InstallDir)
		return nil, fmt.Errorf("smoke test: %w", err)
	}

	if err := in.register(ctx, req, command, args, env, progress); err != nil {
		_ = os.RemoveAll(pkg.InstallDir)
		return nil, fmt.Errorf("register: %w", err)
	}

	pkg.Status = store.MCPInstallStatusInstalled
	pkg.Error = nil
	pkg.ToolCount = len(tools)
	if err := in.Installs.UpsertPackage(ctx, pkg); err != nil {
		return nil, fmt.Errorf("persist installed package: %w", err)
	}
	progress(StepRegister, 100, fmt.Sprintf("installed %s@%s (%d tools)", pkg.Name, shortSHA(sha), len(tools)))
	return &Result{Package: *pkg, Tools: tools}, nil
}

// --- Step 1: preflight ---

func (in *Installer) preflight(ctx context.Context, req *Request, progress Progress) error {
	if req.Name == "" || strings.ContainsAny(req.Name, " /\\") {
		return fmt.Errorf("invalid package name %q", req.Name)
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Name
	}
	if req.Source == "" {
		req.Source = "custom"
	}
	if req.InstallRoot == "" {
		return errors.New("install root not set")
	}
	if err := ValidateRepoURL(req.Repo); err != nil {
		slog.Warn("security.mcp.install_rejected", "reason", err.Error(), "repo", req.Repo, "name", req.Name)
		return err
	}
	if req.Ref == "" {
		ref, err := in.defaultRef()
		if err != nil {
			return err
		}
		req.Ref = ref
	}
	if req.Subdir == "" {
		req.Subdir = "mcp/" + req.Name
	}
	req.Subdir = filepath.ToSlash(filepath.Clean(req.Subdir))
	if req.Subdir == "." || strings.HasPrefix(req.Subdir, "../") {
		return fmt.Errorf("invalid subdir %q", req.Subdir)
	}

	var runtimeBin string
	switch req.Runtime {
	case "node":
		bin, err := exec.LookPath("node")
		if err != nil {
			return errors.New("node runtime not found on this host")
		}
		runtimeBin = bin
	case "python":
		bin, err := exec.LookPath("python3")
		if err != nil {
			return errors.New("python3 runtime not found on this host")
		}
		runtimeBin = bin
	default:
		return fmt.Errorf("unsupported runtime %q (node or python)", req.Runtime)
	}
	_ = runtimeBin // resolved again at register time; preflight just checks presence

	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git not found on this host")
	}
	entry := filepath.FromSlash(req.Entry)
	if entry == "" || filepath.IsAbs(entry) || strings.Contains(entry, "..") {
		return fmt.Errorf("invalid entry %q", req.Entry)
	}

	progress(StepPreflight, 5, fmt.Sprintf("preflight ok: %s @ %s (%s)", req.Repo, req.Ref, req.Runtime))
	return nil
}

// defaultRef derives the install ref from the running version so catalog
// installs always match the manifests shipped in the binary.
func (in *Installer) defaultRef() (string, error) {
	v := strings.SplitN(in.Version, "+", 2)[0]
	if v == "" || v == "dev" || !strings.HasPrefix(v, "v") {
		return "", fmt.Errorf("cannot pin install ref: gateway version %q has no release tag; specify an explicit ref", in.Version)
	}
	return v, nil
}

var githubRepoRe = regexp.MustCompile(`^/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?$`)

// ValidateRepoURL enforces the github.com-only allowlist (no arbitrary
// hosts, no userinfo, no query/fragment).
func ValidateRepoURL(repo string) error {
	if repo == "" {
		return errors.New("repo URL is required")
	}
	u, err := url.Parse(repo)
	if err != nil {
		return fmt.Errorf("invalid repo URL: %w", err)
	}
	if u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("repo must be an https://github.com/owner/repo URL (got %q)", repo)
	}
	if !githubRepoRe.MatchString(u.Path) {
		return fmt.Errorf("repo path must be owner/repo (got %q)", u.Path)
	}
	return nil
}

// --- Step 2: sparse clone ---

// clone fetches only the tool subfolder at the pinned ref and moves it into
// place, returning the resolved commit SHA.
func (in *Installer) clone(ctx context.Context, req *Request, pkg *store.MCPInstalledPackage, progress Progress) (string, error) {
	if err := os.MkdirAll(req.InstallRoot, 0o755); err != nil {
		return "", fmt.Errorf("create install root: %w", err)
	}
	tmp, err := os.MkdirTemp(req.InstallRoot, ".tmp-"+req.Name+"-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)
	progress(StepClone, 20, "cloning "+req.Repo+" @ "+req.Ref)

	cctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	steps := [][]string{
		{"git", "clone", "--depth", "1", "--branch", req.Ref, "--filter=blob:none", "--no-checkout", req.Repo, tmp},
		{"git", "-C", tmp, "sparse-checkout", "init", "--cone"},
		{"git", "-C", tmp, "sparse-checkout", "set", req.Subdir},
		{"git", "-C", tmp, "checkout"},
	}
	for _, args := range steps {
		if out, err := runCmd(cctx, args[0], args[1:], ""); err != nil {
			return "", fmt.Errorf("%s: %w: %s", args[1], err, tail(out, 300))
		}
	}

	out, err := runCmd(cctx, "git", []string{"-C", tmp, "rev-parse", "HEAD"}, "")
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if len(sha) < 7 {
		return "", fmt.Errorf("unexpected commit SHA %q", sha)
	}

	src := filepath.Join(tmp, filepath.FromSlash(req.Subdir))
	if _, err := os.Stat(filepath.Join(src, filepath.FromSlash(req.Entry))); err != nil {
		return "", fmt.Errorf("entry %s not found in %s@%s", req.Entry, req.Subdir, req.Ref)
	}

	// Replace any previous install atomically enough for our purposes.
	if err := os.RemoveAll(pkg.InstallDir); err != nil {
		return "", fmt.Errorf("clear old install: %w", err)
	}
	if err := os.Rename(src, pkg.InstallDir); err != nil {
		return "", fmt.Errorf("move into place: %w", err)
	}
	progress(StepClone, 35, "cloned @ "+shortSHA(sha))
	return sha, nil
}

// --- Step 3: dependencies ---

// installDeps installs npm/pip dependencies when the tool declares any.
// Zero-dep tools (no lockfile and no dependencies block) skip this step —
// nothing touches the network.
func (in *Installer) installDeps(ctx context.Context, req *Request, pkg *store.MCPInstalledPackage, progress Progress) error {
	switch req.Runtime {
	case "node":
		return in.npmDeps(ctx, pkg, progress)
	case "python":
		return in.pipDeps(ctx, pkg, progress)
	}
	return nil
}

func (in *Installer) npmDeps(ctx context.Context, pkg *store.MCPInstalledPackage, progress Progress) error {
	pkgJSONPath := filepath.Join(pkg.InstallDir, "package.json")
	raw, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return fmt.Errorf("package.json missing: %w", err)
	}
	var pkgJSON struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(raw, &pkgJSON); err != nil {
		return fmt.Errorf("parse package.json: %w", err)
	}
	if len(pkgJSON.Dependencies) == 0 && len(pkgJSON.DevDependencies) == 0 {
		progress(StepDeps, 55, "no npm dependencies — skipping")
		return nil
	}
	args := []string{"install", "--omit=dev", "--no-audit", "--no-fund", "--loglevel=error"}
	if _, err := os.Stat(filepath.Join(pkg.InstallDir, "package-lock.json")); err == nil {
		args = []string{"ci", "--omit=dev", "--no-audit", "--no-fund", "--loglevel=error"}
	}
	progress(StepDeps, 45, "npm "+args[0]+" …")
	npm, err := exec.LookPath("npm")
	if err != nil {
		return errors.New("npm not found on this host")
	}
	dctx, cancel := context.WithTimeout(ctx, depsTimeout)
	defer cancel()
	if out, err := runCmd(dctx, npm, args, pkg.InstallDir); err != nil {
		return fmt.Errorf("npm %s: %w: %s", args[0], err, tail(out, 300))
	}
	// Reclaim disk: the download cache can dwarf the install itself.
	_, _ = runCmd(dctx, npm, []string{"cache", "prune", "--force"}, "")
	progress(StepDeps, 55, "npm deps installed")
	return nil
}

func (in *Installer) pipDeps(ctx context.Context, pkg *store.MCPInstalledPackage, progress Progress) error {
	requirements := filepath.Join(pkg.InstallDir, "requirements.txt")
	pyproject := filepath.Join(pkg.InstallDir, "pyproject.toml")
	_, reqErr := os.Stat(requirements)
	_, pyErr := os.Stat(pyproject)
	if reqErr != nil && pyErr != nil {
		progress(StepDeps, 55, "no python dependencies — skipping")
		return nil
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		return errors.New("python3 not found on this host")
	}
	// Install into an in-tree vendor dir: the registered command stays the
	// bare "python3" (validation allowlist) and PYTHONPATH points here.
	vendor := filepath.Join(pkg.InstallDir, "vendor")
	args := []string{"-m", "pip", "install", "--no-cache-dir", "--target", vendor}
	if reqErr == nil {
		args = append(args, "-r", requirements)
	} else {
		args = append(args, ".")
	}
	progress(StepDeps, 45, "pip install …")
	dctx, cancel := context.WithTimeout(ctx, depsTimeout)
	defer cancel()
	if out, err := runCmd(dctx, python, args, pkg.InstallDir); err != nil {
		return fmt.Errorf("pip install: %w: %s", err, tail(out, 300))
	}
	progress(StepDeps, 55, "python deps installed")
	return nil
}

// --- Step 4: smoke test ---

// smoke spawns the installed server exactly the way the registry will and
// requires it to complete the MCP handshake and expose at least one tool.
// It returns the registration shape (command/args/env) plus the tool list.
func (in *Installer) smoke(ctx context.Context, req *Request, pkg *store.MCPInstalledPackage, progress Progress) (string, []string, map[string]string, []mcp.ToolInfo, error) {
	command, args, env := serverCommand(req, pkg)
	progress(StepSmoke, 65, "smoke test: spawn + MCP initialize + tools/list")

	tools, err := mcp.DiscoverTools(ctx, "stdio", command, args, env, "", nil)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("server failed MCP handshake: %w", err)
	}
	if len(tools) == 0 {
		return "", nil, nil, nil, errors.New("server exposes no tools")
	}
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	progress(StepSmoke, 80, "tools: "+strings.Join(names, ", "))
	return command, args, env, tools, nil
}

// serverCommand builds the registry command for the installed server: bare
// allowlisted runtime name + absolute entry path (+ PYTHONPATH for vendored
// python deps). Env never carries gateway secrets.
func serverCommand(req *Request, pkg *store.MCPInstalledPackage) (string, []string, map[string]string) {
	env := map[string]string{"PATH": os.Getenv("PATH")}
	entry := filepath.Join(pkg.InstallDir, filepath.FromSlash(req.Entry))
	switch req.Runtime {
	case "python":
		if vendor := filepath.Join(pkg.InstallDir, "vendor"); dirExists(vendor) {
			env["PYTHONPATH"] = vendor
		}
		return "python3", []string{entry}, env
	default:
		return "node", []string{entry}, env
	}
}

// --- Step 5: register ---

func (in *Installer) register(ctx context.Context, req *Request, command string, args []string, env map[string]string, progress Progress) error {
	if err := mcp.ValidateServerConfig("stdio", command, args, ""); err != nil {
		return fmt.Errorf("registration rejected by command validation: %w", err)
	}
	argsJSON, _ := json.Marshal(args)
	envJSON, _ := json.Marshal(env)

	progress(StepRegister, 90, "registering MCP server "+req.Name)
	existing, err := in.Servers.GetServerByName(ctx, req.Name)
	if err == nil && existing != nil {
		updates := map[string]any{
			"display_name": req.DisplayName,
			"transport":    "stdio",
			"command":      command,
			"args":         json.RawMessage(argsJSON),
			"env":          json.RawMessage(envJSON),
			"enabled":      true,
			"timeout_sec":  120,
		}
		if err := in.Servers.UpdateServer(ctx, existing.ID, updates); err != nil {
			return fmt.Errorf("update existing server: %w", err)
		}
	} else {
		srv := &store.MCPServerData{
			Name:        req.Name,
			DisplayName: req.DisplayName,
			Transport:   "stdio",
			Command:     command,
			Args:        json.RawMessage(argsJSON),
			Env:         json.RawMessage(envJSON),
			Enabled:     true,
			TimeoutSec:  120,
			CreatedBy:   req.CreatedBy,
		}
		if err := in.Servers.CreateServer(ctx, srv); err != nil {
			return fmt.Errorf("create server: %w", err)
		}
	}
	return nil
}

// --- Uninstall ---

// Uninstall removes the server registration, its files, and the package
// row. Missing pieces are cleaned best-effort so uninstall is idempotent.
func (in *Installer) Uninstall(ctx context.Context, tenantID uuid.UUID, name string) error {
	pkg, err := in.Installs.GetPackageByName(ctx, name)
	if err != nil {
		return fmt.Errorf("package %q not found", name)
	}
	if in.EvictServer != nil {
		in.EvictServer(tenantID, name)
	}
	if srv, err := in.Servers.GetServerByName(ctx, name); err == nil && srv != nil {
		if err := in.Servers.DeleteServer(ctx, srv.ID); err != nil {
			return fmt.Errorf("delete server registration: %w", err)
		}
	}
	if err := os.RemoveAll(pkg.InstallDir); err != nil {
		return fmt.Errorf("remove install dir: %w", err)
	}
	if err := in.Installs.DeletePackage(ctx, name); err != nil {
		return fmt.Errorf("delete package row: %w", err)
	}
	return nil
}

// --- helpers ---

// runCmd runs a command with no shell, inheriting the environment (package
// managers need HOME/PASSWORD etc.), returning combined output.
func runCmd(ctx context.Context, name string, args []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
