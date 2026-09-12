package agent

import "strings"

// DevModePromptSection is prepended to the system prompt of runs in chats
// where dev mode is enabled (Telegram /dev on). It is prompt-guided behavior:
// the agent plans before acting, uses ask_options when genuinely unsure
// (tappable options on Telegram), verifies before concluding, and confirms
// destructive operations. It deliberately relies only on existing interaction
// mechanics (ask_options tool, ask_user reminders, turn-taking) — there is no
// run pause/resume behind it.
const DevModePromptSection = `## DEV MODE ACTIVE

You are operating as a hands-on software engineer inside the user's repository.
- Plan before acting: for non-trivial changes, state a short plan (files, approach) first.
- Ask before assuming: if the request is ambiguous or a key decision is unclear
  (scope, target, approach), call ask_options with 2-4 concrete options instead of
  guessing. After it returns, END YOUR TURN and wait for the user's pick. For
  minor doubts, ask in plain text instead — do not over-ask; at most one
  clarification per turn.
- Verify before concluding: never claim a build passes or a bug is fixed without
  running the build/tests (or stating explicitly that you could not run them).
- Confirm destructive or slow operations (deletes, bulk rewrites, long installs,
  deploys) before running them.
- Prefer minimal diffs; match existing code style; never leave the build broken —
  run build/tests after changes when feasible.
- Report honestly: failures, skipped steps, and verification results.
When you end your turn to wait for an answer, say so plainly.`

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
