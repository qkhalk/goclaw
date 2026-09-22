package installer

import (
	"path/filepath"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestValidateRepoURL(t *testing.T) {
	ok := []string{
		"https://github.com/qkhalk/goclaw",
		"https://github.com/qkhalk/goclaw.git",
		"https://github.com/owner/repo.sub-1_x",
	}
	for _, repo := range ok {
		if err := ValidateRepoURL(repo); err != nil {
			t.Errorf("ValidateRepoURL(%q) = %v, want nil", repo, err)
		}
	}

	bad := map[string]string{
		"":                                  "empty",
		"https://gitlab.com/owner/repo":     "non-github host",
		"http://github.com/owner/repo":      "not https",
		"https://evil.com/github.com/x/y":   "spoofed host",
		"https://user:pass@github.com/o/r":  "userinfo",
		"https://github.com/owner/repo?x=1": "query",
		"https://github.com/owner":          "one path segment",
		"https://github.com/a/b/c/d":        "too many segments",
	}
	for repo, why := range bad {
		if err := ValidateRepoURL(repo); err == nil {
			t.Errorf("ValidateRepoURL(%q) = nil, want error (%s)", repo, why)
		}
	}
}

func TestDefaultRef(t *testing.T) {
	cases := []struct {
		version string
		want    string
		wantErr bool
	}{
		{version: "v4.9.4", want: "v4.9.4"},
		{version: "v4.9.4+424+abc123", want: "v4.9.4"},
		{version: "dev", wantErr: true},
		{version: "", wantErr: true},
		{version: "1.2.3", wantErr: true},
	}
	for _, c := range cases {
		in := &Installer{Version: c.version}
		got, err := in.defaultRef()
		if c.wantErr {
			if err == nil {
				t.Errorf("defaultRef(%q) = %q, want error", c.version, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("defaultRef(%q) error: %v", c.version, err)
		} else if got != c.want {
			t.Errorf("defaultRef(%q) = %q, want %q", c.version, got, c.want)
		}
	}
}

// serverCommand must keep the command a bare allowlisted runtime name with
// the installed entry as an absolute path arg — that shape is what
// internal/mcp validation enforces on the registry.
func TestServerCommand(t *testing.T) {
	pkg := &store.MCPInstalledPackage{InstallDir: "/data/mcp/media-probe"}

	cmd, args, env := serverCommand(&Request{Runtime: "node", Entry: "src/index.js"}, pkg)
	if cmd != "node" {
		t.Errorf("node command = %q, want bare 'node'", cmd)
	}
	wantEntry := filepath.Join(pkg.InstallDir, "src/index.js")
	if len(args) != 1 || args[0] != wantEntry {
		t.Errorf("node args = %v, want [%s]", args, wantEntry)
	}
	if env["PATH"] == "" {
		t.Error("env must carry PATH so spawned children (e.g. ffprobe) resolve")
	}

	cmd, args, env = serverCommand(&Request{Runtime: "python", Entry: "server.py"}, pkg)
	if cmd != "python3" {
		t.Errorf("python command = %q, want bare 'python3'", cmd)
	}
	if env["PYTHONPATH"] != "" {
		t.Errorf("PYTHONPATH set without vendor dir: %q", env["PYTHONPATH"])
	}
}

func TestPreflight_RejectsBadRepoBeforeRuntimeLookup(t *testing.T) {
	// A non-github repo must be rejected in preflight before any runtime
	// probe, so hosts without node/python still get the security rejection.
	in := &Installer{Version: "v4.9.4"}
	req := &Request{
		Name: "x", Repo: "https://gitlab.com/a/b", Runtime: "node",
		Entry: "a.js", InstallRoot: t.TempDir(),
	}
	if err := in.preflight(t.Context(), req, nil); err == nil {
		t.Fatal("preflight accepted a non-github repo")
	}
}

func TestPreflight_RejectsMissingInstallRootAndEntryEscape(t *testing.T) {
	in := &Installer{Version: "v4.9.4"}
	bad := []Request{
		{Name: "x", Repo: "https://github.com/a/b", Runtime: "node", Entry: "a.js", InstallRoot: ""},
		{Name: "x", Repo: "https://github.com/a/b", Runtime: "node", Entry: "../a.js", InstallRoot: "/tmp"},
		{Name: "bad name", Repo: "https://github.com/a/b", Runtime: "node", Entry: "a.js", InstallRoot: "/tmp"},
	}
	for i, req := range bad {
		if err := in.preflight(t.Context(), &req, nil); err == nil {
			t.Errorf("case %d: preflight accepted invalid request %+v", i, req)
		}
	}
}
