---
name: ship
description: "Use when the user asks to commit, ship, send a PR, merge, publish changes, or watch CI: splitting work into clean conventional commits, pushing a branch, opening a pull request with a real description, waiting for checks to go green, and merging only under guard rails. Triggers: commit this, push, open a PR, ship it, merge when green, release this, 'commit giúp tôi', 'tạo PR', gộp code. Do NOT use for writing the code itself (just do it), for reviewing someone else's PR (use the review skill), or for repo tagging/release automation the user already scripted."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - change_description
  - target_repo
outputs:
  - pr_url
  - merge_report
allowed-tools:
  - exec
  - read_file
  - list_files
  - web_fetch
quality-gates:
  - tree_clean_and_on_branch
  - conventional_commits_used
  - checks_green_before_merge
  - pr_body_has_why_and_test_notes
deps:
  - system:git
  - system:gh
---

# Ship (guarded commit → PR → CI → merge)

Turn finished work into a reviewable, traceable change: clean commits, a PR a
human can read, checks verified green, merge only when the guards pass. The
git history is a communication medium — every commit message should survive
being read a year later with no other context.

## Gate 0: pre-flight (before any commit)

1. `git status --porcelain` — if dirty with work NOT part of this change,
   stop and ask the user what to include.
2. `git branch --show-current` — never commit directly to `main`/`master`.
   If on the default branch, create one:
   `git checkout -b <type>/<slug>` (e.g. `feat/user-export`,
   `fix/ws-reconnect`).
3. `git diff --stat` + read the changed files (`read_file`) — confirm the
   diff matches what the user asked to ship. Surprise files (lockfiles from
   another change, `.env`, local configs) get excluded, not committed.
4. Secrets gate: `git diff --cached` after staging must contain no keys,
   tokens, passwords, connection strings. If found, stop and tell the user.
   A leaked secret in history needs rotation, not just deletion.
5. Run the project's own checks before committing (build + tests — for a Go
   project: `go build ./... && go test ./...`). Red tests are fixed, not
   shipped "to see if CI catches them".

## Commits: split by reason, not by file

1. Group the diff into logical units — one commit per independent reason a
   reviewer would care about (a feature, its test, its docs; a refactor
   separate from a bugfix it exposed).
2. Conventional Commit format:
   ```text
   <type>(<scope>): <imperative summary, <=72 chars, no period>

   <what and WHY — the constraint or bug that motivated it,
   the alternative rejected and the reason. Wrap at 72.>
   ```
   Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`.
3. Stage per group (`git add <paths>`), commit, repeat. Verify with
   `git log --oneline -n5` that the story reads cleanly bottom-up.
4. Body honest, no filler: no attribution footers, no "updated files",
   no pasted AI disclaimers. If a commit needs "and also misc fixes",
   it is two commits.

## PR: a page, not a placeholder

1. Push: `git push -u origin HEAD`.
2. Open with the CLI:
   ```sh
   gh pr create --title "<same convention as commit subject>" --body-file <file>
   ```
3. PR body written with `exec` heredoc or via `read_file` on a temp note;
   it must contain:
   - **What** — one paragraph, plain language.
   - **Why** — the problem/ticket this addresses.
   - **How** — the approach in 3–6 bullets, plus anything deliberately
     NOT done.
   - **Testing** — exact commands run and their results; manual steps for
     the reviewer if any.
   - **Risk & rollback** — the blast radius and the revert command.
4. Link the issue (`Fixes #N`) only when the change actually closes it.

## CI watch: verify, don't assume

1. `gh pr checks` — list check status. Pending is fine; failed is a signal.
2. Wait for the run: `gh pr checks --watch` (or `gh run watch <id>`). For
   long queues, poll `gh pr checks` on a spacing instead of hammering.
3. On failure: `gh run view <id> --log-failed`, read the actual error,
   fix locally, `git commit --amend` or follow-up commit (prefer follow-up
   once others have seen the branch), push again. Never merge red.
4. Flaky-looking failure (network/timeouts in unrelated jobs)? Re-run once:
   `gh run rerun <id>`. Fails twice → treat as real.

## Guarded merge

Merge ONLY when every guard holds:

| Guard | Check |
|-------|-------|
| Checks green | `gh pr checks` shows all pass, none pending |
| Review | required approvals present (or user explicitly overrides in chat) |
| Conflicts | `git fetch origin && git merge-tree` clean, or rebase first |
| Scope | diff still matches what the user approved — re-skim `gh pr diff` |

- Default to the repo's existing merge style; if none, prefer squash for
  small single-purpose PRs, merge commit for multi-commit histories worth
  preserving.
- Delete the branch after merge (`gh pr merge --delete-branch`) unless the
  user keeps branches.
- Report back: PR URL, what merged, what to watch post-merge (migration,
  deploy, dashboard).

## Anti-patterns

- Committing to `main` because "it's a small fix" — small fixes get the
  fastest review; that's exactly when the guard is cheapest.
- One mega-commit `feat: changes` covering refactor + feature + formatting.
- Message says WHAT (`fix: edit user.go`) with no WHY — the diff already
  says what.
- `git push --force` to a shared branch. If a rebase must be pushed to a
  branch others touched, `--force-with-lease` and only after telling the
  user.
- Merging on "CI is probably fine" — `gh pr checks` is one command.
- Committing generated artifacts, `.env*`, node_modules, or editor dirs —
  check `.gitignore` covers them before the first commit.
- Amending a pushed commit others have reviewed — add a follow-up commit.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `gh` not authenticated | `gh auth status`; ask the user to `gh auth login` |
| Push rejected (non-fast-forward) | `git fetch origin`, rebase onto the branch, re-run checks |
| Checks stuck queued >20 min | Report to the user with the run URL; don't merge while pending |
| Secret already committed | Stop, tell the user immediately — rotation decision is theirs; rewriting history is their call |
| User says "just merge it" while red | State the failing check name in one line, then follow their explicit instruction |
