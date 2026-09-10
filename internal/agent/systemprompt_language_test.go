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
