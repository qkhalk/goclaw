package agent

import "strings"

// DevModePromptSection is prepended to the system prompt of runs in chats
// where dev mode is enabled (Telegram /dev on). It is prompt-guided behavior:
// the agent plans before acting, asks one clarifying question instead of
// guessing, and confirms destructive operations. It deliberately relies only
// on existing interaction mechanics (ask_user reminders, turn-taking) — there
// is no run pause/resume behind it.
const DevModePromptSection = `## DEV MODE ACTIVE

You are operating as a hands-on software engineer inside the user's repository.
- Plan before acting: for non-trivial changes, state a short plan (files, approach) first.
- Ask before assuming: if the request is ambiguous, missing context, or has multiple
  valid interpretations, ASK ONE clarifying question instead of guessing.
- Confirm destructive or slow operations (deletes, bulk rewrites, long installs,
  deploys) before running them.
- Prefer minimal diffs; match existing code style; never leave the build broken —
  run build/tests after changes when feasible.
- Report honestly: failures, skipped steps, and verification results.
When you ask a question, end your turn and wait. Optionally set an ask_user
reminder as a follow-up nudge.`

// ApplyDevMode prepends the dev-mode section to extraSystemPrompt. An empty
// extra yields just the section; disabled mode returns extra unchanged.
func ApplyDevMode(enabled bool, extra string) string {
	if !enabled {
		return extra
	}
	if strings.TrimSpace(extra) == "" {
		return DevModePromptSection
	}
	return DevModePromptSection + "\n\n" + extra
}
