---
name: git
description: >-
  Safe git operations via exec: branch hygiene, staging discipline,
  conventional commits, rebase-vs-merge decisions, and recovery from common
  mistakes — never force-pushing shared branches. Use for any branch, commit,
  merge, or history cleanup work. Keywords: git, branch, commit, rebase,
  merge, conflict, recovery, khoi phuc. Dùng khi cần thao tác git an toàn:
  tạo nhánh, commit, gộp, hoặc khôi phục sau lỗi.
license: MIT
version: 1
---

# Git

Perform safe git operations via `exec`: clean branches, disciplined staging, conventional commit messages, sane merge/rebase choices, and recovery from common messes — protecting shared history at all times.

## When to use
- Creating branches, staging, committing, merging, or rebasing
- Cleaning up stale local branches or untangling a wrong-branch commit
- Writing or fixing commit messages to project convention
- Recovering from a bad merge or detached-HEAD confusion

## When NOT to use
- GitHub-side work (PRs, issues, checks) — that is `github`
- Shipping a feature end-to-end — that is `ship`
- Project commit conventions — check `docs`/CONTRIBUTING first, then apply this skill

## Workflow
1. Check state before any mutation: `git status`, current branch, `git log --oneline -5`, remote tracking.
2. Branch hygiene: name branches by type (`feat/`, `fix/`, `chore/` + slug); branch from the correct, freshly fetched base.
3. Stage deliberately: add by explicit path, never a blanket add; review the staged diff before committing; separate unrelated changes into separate commits.
4. Commit messages: conventional style — `type(scope): imperative summary` under ~72 characters, body explains why, references the issue or task id.
5. Rebase vs merge: rebase your own unpushed local work for linear history; merge for shared or integrated branches; never rebase commits others may have built on.
6. Wrong-branch commit: if unpushed, move it — reset this branch and re-commit (or cherry-pick) on the right one; if pushed, prefer a forward revert over rewriting history.
7. Bad merge on a shared branch: revert the merge commit; note that re-merging later may require reverting the revert.
8. Clean up: delete merged local branches (verify with the merged-branches list first) and prune stale remote-tracking refs.
9. Confirm final state: clean status, log reads correctly, and the push target is exactly what you intended.

## Output
A repo in the intended state (branch, commits, or recovery applied) plus a short report: operations run, commits created (hashes and messages), and anything the user must know (e.g., a revert landed on a shared branch).

## Routing
- PRs, issues, checks, releases on the hosting side -> `github`
- Full delivery flow from branch to PR -> `ship`
- History analysis for retrospectives -> `retro`
- Commit discipline inside an implementation run -> part of `cook`

## Guardrails
- Never force-push shared branches (main, dev, integration, others' branches); rewriting only ever touches your own unshared work, and you say so
- Never commit secrets or env files; if one slipped in, rotate the credential first — history rewriting is a last resort needing explicit approval
- Destructive operations (hard reset, discard, forced branch delete) only after verifying nothing unmerged is lost
- Run `git status` before and after every mutation
- Prefer reversible operations: new commits over rewrites, reverts over deletions
