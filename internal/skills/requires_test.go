package skills

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakePaths replaces the package-level lookPath with a stub that resolves
// only the listed binaries, restoring the real one after the test.
func fakePaths(t *testing.T, available []string) {
	t.Helper()
	set := make(map[string]bool, len(available))
	for _, b := range available {
		set[b] = true
	}
	prev := lookPath
	lookPath = func(bin string) (string, error) {
		if set[bin] {
			return "/usr/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = prev })
}

func TestParseRequires(t *testing.T) {
	cases := []struct {
		name string
		fm   string
		want *SkillRequires
	}{
		{"no block", "name: x\ndescription: y", nil},
		{"empty block", "requires:\nname: x", nil},
		{
			"nested lists",
			"name: x\nrequires:\n  bins:\n    - ffmpeg\n    - jq\n  os:\n    - linux\nallowed-tools:\n  - Bash",
			&SkillRequires{Bins: []string{"ffmpeg", "jq"}, OS: []string{"linux"}},
		},
		{
			"scalar fallback",
			"requires:\n  bins: ffmpeg, jq\n  os: linux",
			&SkillRequires{Bins: []string{"ffmpeg", "jq"}, OS: []string{"linux"}},
		},
		{
			"stops at next top-level key",
			"requires:\n  bins:\n    - ffmpeg\nversion: \"2\"\n  os:\n    - linux",
			&SkillRequires{Bins: []string{"ffmpeg"}},
		},
		{
			"unknown subkey stops attribution",
			"requires:\n  bins:\n    - ffmpeg\n  unknown:\n    - x",
			&SkillRequires{Bins: []string{"ffmpeg"}},
		},
		{
			"items containing colons",
			"requires:\n  bins:\n    - python3.11",
			&SkillRequires{Bins: []string{"python3.11"}},
		},
		{
			"requires before other keys",
			"description: d\nrequires:\n  os:\n    - darwin\nname: x",
			&SkillRequires{OS: []string{"darwin"}},
		},
	}
	for _, tc := range cases {
		got := parseRequires(tc.fm)
		if tc.want == nil {
			if got != nil {
				t.Errorf("%s: parseRequires = %+v, want nil", tc.name, got)
			}
			continue
		}
		if got == nil || strings.Join(got.Bins, ",") != strings.Join(tc.want.Bins, ",") || strings.Join(got.OS, ",") != strings.Join(tc.want.OS, ",") {
			t.Errorf("%s: parseRequires = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestParseRequiresCRLF(t *testing.T) {
	got := parseRequires("requires:\r\n  bins:\r\n    - ffmpeg\r\n  os:\r\n    - linux\r\n")
	if got == nil || len(got.Bins) != 1 || got.Bins[0] != "ffmpeg" || len(got.OS) != 1 {
		t.Fatalf("CRLF parse = %+v", got)
	}
}

func TestCheckRequires(t *testing.T) {
	fakePaths(t, []string{"ffmpeg"})

	if got := checkRequires(&SkillRequires{Bins: []string{"ffmpeg"}}, "linux"); got != "" {
		t.Errorf("present bin: %q", got)
	}
	if got := checkRequires(&SkillRequires{Bins: []string{"ffmpeg", "jq"}}, "linux"); got != "missing required binary: jq" {
		t.Errorf("missing bin: %q", got)
	}
	if got := checkRequires(&SkillRequires{OS: []string{"linux"}}, "linux"); got != "" {
		t.Errorf("matching os: %q", got)
	}
	if got := checkRequires(&SkillRequires{OS: []string{"linux"}}, "darwin"); got != "unsupported OS: darwin (requires linux)" {
		t.Errorf("os mismatch: %q", got)
	}
	if got := checkRequires(&SkillRequires{OS: []string{"macos"}}, "darwin"); got != "" {
		t.Errorf("macos alias: %q", got)
	}
	// Bins checked before OS; empty block always OK.
	if got := checkRequires(nil, "win32"); got != "" {
		t.Errorf("nil requires: %q", got)
	}
}

func TestOSSupported(t *testing.T) {
	if !osSupported([]string{runtime.GOOS}, runtime.GOOS) {
		t.Error("same GOOS must be supported")
	}
	if osSupported([]string{"windows"}, "darwin") {
		t.Error("mismatch must be unsupported")
	}
	if osSupported([]string{""}, "linux") {
		t.Error("empty entries must be skipped")
	}
}

// writeSkill creates <dir>/<slug>/SKILL.md with the given content.
func writeSkill(t *testing.T, dir, slug, frontmatter string) string {
	t.Helper()
	skillDir := filepath.Join(dir, slug)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	content := "---\n" + frontmatter + "\n---\n\nBody of " + slug + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestLoaderRequiresGating(t *testing.T) {
	fakePaths(t, []string{"ffmpeg"})
	workspace := t.TempDir()
	skillsDir := filepath.Join(workspace, "skills") // loader scans <workspace>/skills
	writeSkill(t, skillsDir, "ok-skill", "name: ok-skill\ndescription: fine\n")
	writeSkill(t, skillsDir, "missing-bin", "name: missing-bin\ndescription: needs jq\nrequires:\n  bins:\n    - jq\n")
	writeSkill(t, skillsDir, "wrong-os", "name: wrong-os\ndescription: plan9 only\nrequires:\n  os:\n    - plan9\n")

	l := NewLoader(workspace, "", "")
	skills := l.ListSkills(t.Context())
	if len(skills) != 3 {
		t.Fatalf("ListSkills = %d skills, want 3 (gated skills stay listed)", len(skills))
	}
	byName := make(map[string]Info, len(skills))
	for _, s := range skills {
		byName[s.Slug] = s
	}
	if byName["ok-skill"].UnavailableReason != "" {
		t.Errorf("ok-skill should be available, got %q", byName["ok-skill"].UnavailableReason)
	}
	if byName["ok-skill"].Requires != nil {
		t.Errorf("ok-skill should have no requires, got %+v", byName["ok-skill"].Requires)
	}
	if byName["missing-bin"].UnavailableReason != "missing required binary: jq" {
		t.Errorf("missing-bin reason = %q", byName["missing-bin"].UnavailableReason)
	}
	if byName["wrong-os"].UnavailableReason == "" {
		t.Error("wrong-os must be flagged unavailable")
	}

	// Auto-injection skips unavailable skills; explicit allowlist honors pins.
	auto := l.LoadForContext(t.Context(), nil)
	if strings.Contains(auto, "Body of missing-bin") || strings.Contains(auto, "Body of wrong-os") {
		t.Error("unavailable skills must be skipped by auto-injection")
	}
	if !strings.Contains(auto, "Body of ok-skill") {
		t.Error("available skill body missing from auto-injection")
	}
	pinned := l.LoadForContext(t.Context(), []string{"missing-bin"})
	if !strings.Contains(pinned, "Body of missing-bin") {
		t.Error("explicit allowlist pin must be honored even when unavailable")
	}

	// Summary annotates the unavailable ones.
	summary := l.BuildSummary(t.Context(), nil)
	if !strings.Contains(summary, `unavailable="true"`) {
		t.Error("summary must flag unavailable skills")
	}
	if !strings.Contains(summary, "missing required binary: jq") {
		t.Error("summary must carry the reason")
	}
}

func TestLoaderGatingReevaluatedOnRescan(t *testing.T) {
	// jq missing at first scan, "installed" before the second — gating must
	// re-evaluate on every ListSkills call (no stale availability cache).
	fakePaths(t, nil)
	workspace := t.TempDir()
	skillsDir := filepath.Join(workspace, "skills")
	writeSkill(t, skillsDir, "gated", "name: gated\ndescription: needs jq\nrequires:\n  bins:\n    - jq\n")

	l := NewLoader(workspace, "", "")
	if got := l.ListSkills(t.Context())[0]; got.UnavailableReason == "" {
		t.Fatal("expected unavailable on first scan")
	}

	fakePaths(t, []string{"jq"}) // "install" jq
	if got := l.ListSkills(t.Context())[0]; got.UnavailableReason != "" {
		t.Fatalf("expected available after rescan, got %q", got.UnavailableReason)
	}
}
