---
name: github
description: >-
  Work with GitHub via the gh CLI through exec: issues, pull requests, checks,
  releases, and reviews — with auth checks, pagination, and clean error
  handling. Never leak tokens into logs. Keywords: gh cli, GitHub, PR, issue,
  release, CI checks, tao PR. Dùng khi cần thao tác với GitHub: tạo và theo
  dõi PR, issue, check, release bằng dòng lệnh.
license: MIT
version: 1
---

# GitHub

Drive GitHub from the agent via the `gh` CLI through `exec`: read and triage issues, open and manage pull requests, watch checks, and cut releases — safely, with pagination and strict token hygiene.

## When to use
- Creating or commenting on issues and PRs, requesting reviews, merging
- Checking CI status or failed logs for a branch or PR
- Drafting release notes and publishing or patching releases
- Cross-referencing issues with commits during delivery

## When NOT to use
- Pure local git operations — that is `git`
- Repos not hosted on GitHub — use that platform's CLI or API
- History surgery or branch recovery — that is `git`

## Workflow
1. Verify access: run `gh auth status`; if unauthenticated or missing scopes, stop and tell the user what to run — never handle tokens yourself.
2. Anchor context: `gh repo view --json nameWithOwner,defaultBranchRef` so every later call targets the right repo.
3. Issues: list/search with explicit `--state` and `--limit`; read the full issue body and comments before acting; comment or label only with clear intent.
4. PR flow: push commits first, then `gh pr create` with a conventional title, a body covering what/why/how-tested, and the base branch stated explicitly.
5. Checks: `gh pr checks`; while pending, poll with `wait` between passes; on failure, `gh run view --log-failed` and route to `debug`.
6. Reviews: request reviewers with context; answer review threads with new commits plus replies; never rewrite a reviewed branch without agreement.
7. Merge: require green checks and approvals, then merge with the project's merge style; delete the branch if that is the convention.
8. Releases: collect merged work since the previous tag (`gh release view` to find it), draft notes, then `gh release create` with the correct target; prerelease flag for beta/rc tags.
9. Paginate long listings (`--limit`, `--page`) and quote exact URLs and numbers in reports.

## Output
Concrete artifacts (PR/issue/release URLs) plus a report: actions taken, current status (checks, reviews), and any next human actions — each backed by a link.

## Routing
- Local branch, commit, or recovery work -> `git`
- End-to-end delivery pipeline -> `ship`
- CI failure triage -> pull logs here, then `debug` or `fix`
- Substantial release-note writing -> `docs`

## Guardrails
- Never print, log, or store tokens; authentication belongs to the user's own `gh` login
- Read before write: fetch issue/PR state before commenting, labeling, or merging
- Destructive actions (close, delete, force-push reviewed branches, publish a release) require explicit user intent
- Quote exact URLs and numbers; ambiguity here causes wrong-target actions
- Respect rate limits: paginate and never poll checks in a tight loop
