package telegram

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

type staticLister struct{ infos []skills.Info }

func (l staticLister) ListSkills(context.Context) []skills.Info { return l.infos }

func TestTelegramCommandName(t *testing.T) {
	cases := map[string]string{
		"security-audit": "security_audit",
		"ssl-audit":      "ssl_audit",
		"dns-audit":      "dns_audit",
		"loadtest":       "loadtest",
		"cook":           "cook",
		"has space":      "",
		"":               "",
	}
	for in, want := range cases {
		if got := telegramCommandName(in); got != want {
			t.Errorf("telegramCommandName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkillMenuCommands_SanitizesHyphens(t *testing.T) {
	cmds := skillMenuCommands(context.Background(), []string{"security-audit", "loadtest", "bad slug!"}, staticLister{})
	if len(cmds) != 2 {
		t.Fatalf("got %d commands, want 2 (invalid slug skipped): %+v", len(cmds), cmds)
	}
	if cmds[0].Command != "security_audit" {
		t.Errorf("first command = %q, want security_audit", cmds[0].Command)
	}
	if cmds[1].Command != "loadtest" {
		t.Errorf("second command = %q, want loadtest", cmds[1].Command)
	}
}

func TestTestingMenuSkillSlugs(t *testing.T) {
	if got := testingMenuSkillSlugs(nil); len(got) != len(defaultTestingMenuSkills) {
		t.Errorf("nil config must yield defaults, got %v", got)
	}
	if got := testingMenuSkillSlugs([]string{}); len(got) != 0 {
		t.Errorf("empty config must disable the testing menu, got %v", got)
	}
	if got := testingMenuSkillSlugs([]string{"loadtest"}); len(got) != 1 || got[0] != "loadtest" {
		t.Errorf("explicit config must win, got %v", got)
	}
}

func TestMenuMerge_Disjoint(t *testing.T) {
	// Simulates the startup merge (channel.go): a slug present in both lists
	// appears once in the final menu.
	first := skillMenuCommands(context.Background(), []string{"loadtest"}, nil)
	second := skillMenuCommands(context.Background(), []string{"loadtest", "recon"}, nil)

	commands := first
	seen := map[string]struct{}{}
	for _, cmd := range commands {
		seen[cmd.Command] = struct{}{}
	}
	for _, cmd := range second {
		if _, dup := seen[cmd.Command]; dup {
			continue
		}
		seen[cmd.Command] = struct{}{}
		commands = append(commands, cmd)
	}

	count := 0
	for _, cmd := range commands {
		if cmd.Command == "loadtest" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("loadtest appears %d times after merge, want 1", count)
	}
	if len(commands) != 2 {
		t.Errorf("merged menu = %+v, want [loadtest recon]", commands)
	}
}
