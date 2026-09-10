package agent

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptLanguagePin(t *testing.T) {
	build := func(mode PromptMode, locale string) string {
		return BuildSystemPrompt(SystemPromptConfig{Mode: mode, UserLocale: locale})
	}

	t.Run("vietnamese locale pins language", func(t *testing.T) {
		prompt := build(PromptFull, "vi")
		if !strings.Contains(prompt, "## LANGUAGE — MANDATORY") {
			t.Error("missing LANGUAGE section")
		}
		if !strings.Contains(prompt, "The user's language is Vietnamese (`vi`).") {
			t.Error("missing Vietnamese language line")
		}
		if !strings.Contains(prompt, "Every reply MUST be written entirely in Vietnamese") {
			t.Error("missing mandatory directive")
		}
		if !strings.Contains(prompt, "reply in Vietnamese only.") {
			t.Error("reminder not updated to pinned language")
		}
		if strings.Contains(prompt, "match the user's language.") {
			t.Error("generic language reminder should be replaced when pinned")
		}
	})

	t.Run("regional variant normalizes", func(t *testing.T) {
		prompt := build(PromptFull, "vi-VN")
		if !strings.Contains(prompt, "The user's language is Vietnamese (`vi`).") {
			t.Error("vi-VN should normalize to vi")
		}
	})

	t.Run("chinese locale pins language", func(t *testing.T) {
		prompt := build(PromptFull, "zh")
		if !strings.Contains(prompt, "The user's language is Chinese (`zh`).") {
			t.Error("missing Chinese language line")
		}
	})

	t.Run("korean and russian locales pin language", func(t *testing.T) {
		if prompt := build(PromptFull, "ko"); !strings.Contains(prompt, "The user's language is Korean (`ko`).") {
			t.Error("missing Korean language line")
		}
		if prompt := build(PromptFull, "ru-RU"); !strings.Contains(prompt, "The user's language is Russian (`ru`).") {
			t.Error("ru-RU should normalize to ru")
		}
	})

	t.Run("unsupported locale pins nothing", func(t *testing.T) {
		prompt := build(PromptFull, "pt")
		if strings.Contains(prompt, "## LANGUAGE") {
			t.Error("unsupported locale must not pin a language")
		}
		if !strings.Contains(prompt, "match the user's language.") {
			t.Error("generic guidance should remain for unsupported locale")
		}
	})

	t.Run("empty locale pins nothing", func(t *testing.T) {
		prompt := build(PromptFull, "")
		if strings.Contains(prompt, "## LANGUAGE") {
			t.Error("empty locale must not pin a language")
		}
	})

	t.Run("task mode pins, minimal skips", func(t *testing.T) {
		if prompt := build(PromptTask, "vi"); !strings.Contains(prompt, "## LANGUAGE") {
			t.Error("task mode should pin language")
		}
		if prompt := build(PromptMinimal, "vi"); strings.Contains(prompt, "## LANGUAGE") {
			t.Error("minimal mode should skip language pin")
		}
	})
}

func TestResolvePinnedLanguage_MessageScriptWins(t *testing.T) {
	t.Run("vietnamese message overrides english client locale", func(t *testing.T) {
		// The production case: Telegram UI language "en", user writes Vietnamese.
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:       PromptFull,
			UserLocale: "en",
			UserMessage: "Phân tích phong cách hiện thực của Vũ Trọng Phụng qua đoạn trích " +
				"trận quần vợt trong tiểu thuyết Số Đỏ nhé",
		})
		if !strings.Contains(prompt, "The user's language is Vietnamese (`vi`).") {
			t.Error("Vietnamese script must win over an English client locale")
		}
		if strings.Contains(prompt, "language is English") {
			t.Error("must not pin English over a Vietnamese message")
		}
	})

	t.Run("english locale with plain english message pins nothing", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:       PromptFull,
			UserLocale: "en",
			UserMessage: "Analyze the realist style of this novelist across the extract, " +
				"covering irony and satire",
		})
		if strings.Contains(prompt, "## LANGUAGE") {
			t.Error("English must stay unpinned — models match the input language naturally")
		}
		if !strings.Contains(prompt, "match the user's language.") {
			t.Error("generic guidance should remain")
		}
	})

	t.Run("empty locale with vietnamese message pins vietnamese", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:        PromptFull,
			UserMessage: "Em ơi, thử phân tích cái đoạn văn này xem sao nhé",
		})
		if !strings.Contains(prompt, "The user's language is Vietnamese (`vi`).") {
			t.Error("detection must work without any client locale")
		}
	})

	t.Run("accentless vietnamese falls back to client locale", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:        PromptFull,
			UserLocale:  "vi",
			UserMessage: "phan tich phong cach hien thuc cua vu trong phung",
		})
		if !strings.Contains(prompt, "The user's language is Vietnamese (`vi`).") {
			t.Error("vi client locale must still pin when the message lacks diacritics")
		}
	})

	t.Run("cjk message pins chinese", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:        PromptFull,
			UserLocale:  "en",
			UserMessage: "请分析这段小说的讽刺艺术和现实主义风格",
		})
		if !strings.Contains(prompt, "The user's language is Chinese (`zh`).") {
			t.Error("CJK script must pin Chinese")
		}
	})

	t.Run("hangul message pins korean", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:        PromptFull,
			UserLocale:  "",
			UserMessage: "이 소설의 풍자 기법을 분석해 줘",
		})
		if !strings.Contains(prompt, "The user's language is Korean (`ko`).") {
			t.Error("hangul script must pin Korean")
		}
	})

	t.Run("japanese kana pins nothing", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:        PromptFull,
			UserLocale:  "en",
			UserMessage: "この小説の風刺的手法を分析してください",
		})
		if strings.Contains(prompt, "## LANGUAGE") {
			t.Error("Japanese is unsupported — must fall through to natural matching")
		}
	})

	t.Run("spanish accents do not pin vietnamese", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:        PromptFull,
			UserLocale:  "en",
			UserMessage: "¿Podrías analizar el estilo realista de esta novela, por favor?",
		})
		if strings.Contains(prompt, "Vietnamese") {
			t.Error("plain Spanish accents (á ó …) must not trigger a Vietnamese pin")
		}
	})

	t.Run("short message uses locale fallback", func(t *testing.T) {
		prompt := BuildSystemPrompt(SystemPromptConfig{
			Mode:        PromptFull,
			UserLocale:  "vi",
			UserMessage: "ok",
		})
		if !strings.Contains(prompt, "The user's language is Vietnamese (`vi`).") {
			t.Error("short message must fall back to the client locale pin")
		}
	})
}

func TestDetectLanguageFromText(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		want    string
		wantOK  bool
	}{
		{"vietnamese with ơ", "Em thử đọc đoạn văn này giúp nhé, cảm ơn nhiều", "vi", true},
		{"vietnamese tones only", "thế nào là hiện thực phê phán, giải thích giúp", "vi", true},
		{"english", "please analyze this passage for me thanks", "", false},
		{"french", "Peux-tu analyser ce passage de roman s'il te plaît", "", false},
		{"too short vi", "được", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang, ok := detectLanguageFromText(tc.text)
			if ok != tc.wantOK || (ok && lang.code != tc.want) {
				t.Fatalf("detectLanguageFromText(%q) = (%s, %v), want (%s, %v)", tc.text, lang.code, ok, tc.want, tc.wantOK)
			}
		})
	}
}
