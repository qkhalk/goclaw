// Catalog parity for the Telegram interactive-UX keys (pickers, skills,
// ask_options, language, status). A missing translation would silently fall
// back to English — this test keeps all five catalogs complete, mirroring
// git_keys_parity_test.go (which predates the ko catalog and skips it).
package i18n

import (
	"strings"
	"testing"
)

// telegramKeys lists every telegram.* key group used by the channel package.
var telegramKeys = []string{
	MsgTGPickerExpired,
	MsgTGThinkingTitle,
	MsgTGThinkingCurrent,
	MsgTGThinkingAgentDef,
	MsgTGThinkingDefault,
	MsgTGThinkingSet,
	MsgTGThinkingCleared,
	MsgTGReasoningTitle,
	MsgTGReasoningOn,
	MsgTGReasoningOff,
	MsgTGDevTitle,
	MsgTGDevOn,
	MsgTGDevOff,
	MsgTGDevEnabled,
	MsgTGDevDisabled,
	MsgTGSkillsNone,
	MsgTGSkillsTitle,
	MsgTGSkillsHint,
	MsgTGSkillsNoDesc,
	MsgTGSkillsUnavailable,
	MsgTGSkillsRunHint,
	MsgTGSkillsBack,
	MsgTGAskOther,
	MsgTGAskOtherHint,
	MsgTGAskAnswered,
	MsgTGLangUnavailable,
	MsgTGLangCurrent,
	MsgTGLangSetHint,
	MsgTGLangUnsupported,
	MsgTGLangSet,
	MsgTGStatusUnavailable,
	MsgTGStatusUptime,
	MsgTGStatusUptimeUnknown,
	MsgTGStatusSysUptime,
	MsgTGStatusAgentModel,
	MsgTGStatusSessUpdShort,
	MsgTGStatusNoSession,
	MsgTGStatusSessFull,
	MsgTGStatusCost,
	MsgTGStatusCostNA,
	MsgTGStatusTokens,
	MsgTGStatusCtxPct,
	MsgTGStatusCtx,
	MsgTGStatusCtxUnknown,
	MsgTGStatusCompactions,
	MsgTGStatusThinkMode,
	MsgTGStatusQueue,
	MsgTGStatusFullHint,
}

func TestI18nCatalogs_HasTelegramKeys(t *testing.T) {
	for _, locale := range []string{LocaleEN, LocaleVI, LocaleZH, LocaleKO, LocaleRU} {
		for _, key := range telegramKeys {
			msg := lookup(locale, key)
			if msg == key {
				t.Errorf("locale=%s key=%s falls back to key string (translation missing)", locale, key)
				continue
			}
			if msg == "" {
				t.Errorf("locale=%s key=%s is empty", locale, key)
			}
		}
	}
}

// verbCases mirrors the argument shapes the channel passes to i18n.T. A
// translated template with mismatched/dropped verbs renders %!s(MISSING)
// noise into chat, so every catalog must format cleanly.
var verbCases = []struct {
	key    string
	inputs []any
}{
	{MsgTGThinkingSet, []any{"high"}},
	{MsgTGThinkingCurrent, []any{"low"}},
	{MsgTGThinkingAgentDef, []any{"adaptive"}},
	{MsgTGSkillsTitle, []any{7, 1, 1}},
	{MsgTGSkillsUnavailable, []any{"nmap missing"}},
	{MsgTGSkillsRunHint, []any{"security-audit"}},
	{MsgTGAskAnswered, []any{"Which DB?", "Postgres"}},
	{MsgTGLangCurrent, []any{"vi"}},
	{MsgTGLangSetHint, []any{"en · vi · zh"}},
	{MsgTGLangUnsupported, []any{"xx", "en · vi · zh"}},
	{MsgTGLangSet, []any{"vi"}},
	{MsgTGStatusUptime, []any{"2d 3h"}},
	{MsgTGStatusSysUptime, []any{"5m"}},
	{MsgTGStatusAgentModel, []any{"main", "openai/gpt-5"}},
	{MsgTGStatusSessUpdShort, []any{"5m"}},
	{MsgTGStatusSessFull, []any{"agent:1", "5m"}},
	{MsgTGStatusCost, []any{0.1234}},
	{MsgTGStatusTokens, []any{"1k", "2k"}},
	{MsgTGStatusCtxPct, []any{"1k", "100k", 1}},
	{MsgTGStatusCtx, []any{"100k"}},
	{MsgTGStatusCompactions, []any{2}},
	{MsgTGStatusThinkMode, []any{"low", "dev"}},
	{MsgTGStatusQueue, []any{"main", 1, 4, 0}},
}

func TestI18nTelegramKeys_FormatVerbsStable(t *testing.T) {
	for _, tc := range verbCases {
		for _, locale := range []string{LocaleEN, LocaleVI, LocaleZH, LocaleKO, LocaleRU} {
			out := T(locale, tc.key, tc.inputs...)
			if strings.Contains(out, "%!") {
				t.Errorf("locale=%s key=%s produced format noise: %q", locale, tc.key, out)
			}
		}
	}
}
