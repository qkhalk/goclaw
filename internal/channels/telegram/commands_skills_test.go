package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

type fakeSkillsLister struct {
	infos []skills.Info
}

func (f *fakeSkillsLister) ListSkills(_ context.Context) []skills.Info {
	return f.infos
}

func TestMenuSkillSlugs_DefaultsWhenNil(t *testing.T) {
	if got := menuSkillSlugs(nil); len(got) != len(defaultMenuSkills) {
		t.Fatalf("menuSkillSlugs(nil) = %v, want defaults %v", got, defaultMenuSkills)
	}
	custom := []string{"security-audit"}
	if got := menuSkillSlugs(custom); len(got) != 1 || got[0] != "security-audit" {
		t.Fatalf("menuSkillSlugs(custom) = %v, want %v", got, custom)
	}
}

func TestSkillMenuCommands_FiltersInvalidSlugs(t *testing.T) {
	got := skillMenuCommands(context.Background(), []string{"cook", "ui-ux-pro-max", "Plan", "", strings.Repeat("a", 33)}, nil)
	if len(got) != 1 || got[0].Command != "cook" {
		t.Fatalf("commands = %+v, want only cook (invalid slugs skipped)", got)
	}
	if got[0].Description != "Run skill: cook" {
		t.Fatalf("fallback description = %q, want %q", got[0].Description, "Run skill: cook")
	}
}

func TestSkillMenuCommands_UsesListerDescriptions(t *testing.T) {
	lister := &fakeSkillsLister{infos: []skills.Info{
		{Slug: "cook", Description: "Implement plans with verification.\nSecond line ignored."},
	}}
	got := skillMenuCommands(context.Background(), []string{"cook"}, lister)
	if len(got) != 1 {
		t.Fatalf("commands = %+v, want 1 entry", got)
	}
	if got[0].Description != "Implement plans with verification." {
		t.Fatalf("description = %q, want first line of description", got[0].Description)
	}
}

func TestSkillMenuCommands_TruncatesLongDescriptions(t *testing.T) {
	long := strings.Repeat("d", 400)
	lister := &fakeSkillsLister{infos: []skills.Info{{Slug: "cook", Description: long}}}
	got := skillMenuCommands(context.Background(), []string{"cook"}, lister)
	// truncateStr(400 → 255) appends "…" → 256 chars total, Telegram's hard cap.
	if len([]rune(got[0].Description)) > 256 {
		t.Fatalf("description length = %d runes, want ≤256", len([]rune(got[0].Description)))
	}
	if !strings.HasSuffix(got[0].Description, "…") {
		t.Fatalf("description = %q, want ellipsis suffix", got[0].Description)
	}
}

func TestBuildSkillsListHTML_EscapesSortsAndFooter(t *testing.T) {
	html := buildSkillsListHTML([]skills.Info{
		{Slug: "zz", Description: "has <tags> & amps"},
		{Slug: "cook", Description: "Implement plans"},
		{Slug: "empty"},
	})
	for _, want := range []string{
		"<b>Available skills</b>",
		"<code>/cook</code> — Implement plans",
		"<code>/zz</code> — has &lt;tags&gt; &amp; amps",
		"<code>/empty</code> — (no description)",
		"Run a skill by starting a message with its command",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("list HTML missing %q:\n%s", want, html)
		}
	}
	if strings.Index(html, "/cook") > strings.Index(html, "/zz") {
		t.Fatalf("skills not sorted by slug:\n%s", html)
	}
}

func TestBuildSkillsListHTML_CapsAtLimit(t *testing.T) {
	infos := make([]skills.Info, maxSkillsInList+7)
	for i := range infos {
		infos[i] = skills.Info{Slug: "skill" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)), Description: "d"}
	}
	html := buildSkillsListHTML(infos)
	if got := strings.Count(html, "</code> — "); got != maxSkillsInList {
		t.Fatalf("rendered %d entries, want cap %d", got, maxSkillsInList)
	}
	if !strings.Contains(html, "… and 7 more") {
		t.Fatalf("missing hidden-count note:\n%s", html[len(html)-200:])
	}
}

func TestFilterSkillsByWhitelist(t *testing.T) {
	infos := []skills.Info{{Slug: "cook"}, {Slug: "plan"}, {Slug: "fix"}}

	if got := filterSkillsByWhitelist(infos, nil); len(got) != 3 {
		t.Fatalf("nil whitelist changed list: %+v", got)
	}
	got := filterSkillsByWhitelist(infos, []string{"cook"})
	if len(got) != 1 || got[0].Slug != "cook" {
		t.Fatalf("whitelist [cook] = %+v, want only cook", got)
	}
	if got := filterSkillsByWhitelist(infos, []string{}); len(got) != 0 {
		t.Fatalf("empty whitelist = %+v, want none", got)
	}
}

func newSkillsTestChannel(t *testing.T, lister SkillsLister, cfg config.TelegramConfig) (*Channel, *recordingTelegramCaller) {
	t.Helper()
	caller := &recordingTelegramCaller{}
	bot, err := telego.NewBot(
		"123456:abcdefghijklmnopqrstuvwxyzABCDE1234",
		telego.WithAPICaller(caller),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	ch := &Channel{
		BaseChannel:  channels.NewBaseChannel("telegram", nil, nil),
		bot:          bot,
		config:       cfg,
		skillsLister: lister,
	}
	return ch, caller
}

func TestHandleBotCommand_SkillsNilLister(t *testing.T) {
	ch, caller := newSkillsTestChannel(t, nil, config.TelegramConfig{})
	handled := ch.handleBotCommand(context.Background(), &telego.Message{}, 123, "123", "123", "/skills", "123", false, false, 0)
	if !handled {
		t.Fatal("/skills with nil lister must be handled (unavailable reply)")
	}
	if len(caller.calls) != 1 || caller.calls[0].method != "sendMessage" {
		t.Fatalf("calls = %+v, want one sendMessage", caller.calls)
	}
	if text, _ := caller.calls[0].body["text"].(string); text != "Skills are not available." {
		t.Fatalf("text = %q, want unavailable message", text)
	}
}

func TestHandleBotCommand_SkillsSendsList(t *testing.T) {
	lister := &fakeSkillsLister{infos: []skills.Info{
		{Slug: "cook", Description: "Implement plans"},
		{Slug: "security-audit", Description: "Audit web apps"},
	}}
	ch, caller := newSkillsTestChannel(t, lister, config.TelegramConfig{})
	handled := ch.handleBotCommand(context.Background(), &telego.Message{}, 123, "123", "123", "/skills", "123", false, false, 0)
	if !handled {
		t.Fatal("/skills must be handled")
	}
	if len(caller.calls) == 0 {
		t.Fatal("no messages sent")
	}
	joined := ""
	for _, call := range caller.calls {
		if call.method != "sendMessage" {
			t.Fatalf("unexpected method %q", call.method)
		}
		if pm, _ := call.body["parse_mode"].(string); pm != telego.ModeHTML {
			t.Fatalf("parse_mode = %q, want HTML", pm)
		}
		text, _ := call.body["text"].(string)
		joined += text
	}
	for _, want := range []string{"<code>/cook</code>", "<code>/security-audit</code>", "Audit web apps"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("sent list missing %q:\n%s", want, joined)
		}
	}
}

func TestHandleBotCommand_SkillsRespectsGroupWhitelist(t *testing.T) {
	lister := &fakeSkillsLister{infos: []skills.Info{
		{Slug: "cook", Description: "Implement plans"},
		{Slug: "plan", Description: "Design plans"},
	}}
	cfg := config.TelegramConfig{Groups: map[string]*config.TelegramGroupConfig{
		"-10042": {Skills: []string{"plan"}},
	}}
	ch, caller := newSkillsTestChannel(t, lister, cfg)
	handled := ch.handleBotCommand(context.Background(), &telego.Message{}, -10042, "-10042", "-10042", "/skills", "42", true, false, 0)
	if !handled {
		t.Fatal("/skills must be handled")
	}
	joined := ""
	for _, call := range caller.calls {
		text, _ := call.body["text"].(string)
		joined += text
	}
	if strings.Contains(joined, "/cook") {
		t.Fatalf("whitelisted list must not contain /cook:\n%s", joined)
	}
	if !strings.Contains(joined, "/plan") {
		t.Fatalf("whitelisted list missing /plan:\n%s", joined)
	}
}

func TestStartupMenuIncludesSkillEntries(t *testing.T) {
	commands := append(DefaultMenuCommands(), skillMenuCommands(context.Background(), menuSkillSlugs(nil), nil)...)
	if len(commands) > 100 {
		t.Fatalf("menu has %d commands, Telegram caps at 100", len(commands))
	}
	found := map[string]bool{}
	for _, cmd := range commands {
		found[cmd.Command] = true
	}
	for _, want := range []string{"skills", "help", "cook", "plan", "fix", "review", "test"} {
		if !found[want] {
			t.Fatalf("menu missing %q: %+v", want, commands)
		}
	}
}
