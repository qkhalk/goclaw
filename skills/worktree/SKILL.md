---
name: worktree
description: >-
  Isolated git worktree management for parallel feature work: create, use, and
  clean up worktrees so parallel tasks never disturb the main checkout. Use
  when running two tasks at once, keeping a long build alive while coding, or
  reviewing changes on clean files. Keywords: git worktree, wt/ branches,
  isolation, parallel checkouts. Dùng khi cần làm việc song song trên nhiều
  nhánh mà không ảnh hưởng thư mục làm việc chính.
license: MIT
version: 1
---

# Worktree

Create, use, and clean up isolated git worktrees so parallel feature work never dirties the main tree. One worktree per workstream, branch named `wt/<slug>`.

## When to use
- Two or more independent tasks that would collide in one checkout
- A long-running build, test, or job needs a stable tree while coding continues
- Comparing an implementation against another branch side by side
- Reviewing large changes without stashing local work in progress

## When NOT to use
- Simple single-task work — a normal branch is enough
- Tooling or submodules in the repo are known to break across worktrees
- The change is tiny — swapping branches costs less than a new checkout
- Disk space is constrained (each worktree is a full checkout)

## Workflow
1. Decide branch vs worktree: if any other task must keep running in this directory, use a worktree.
2. Pick one short slug and use it everywhere: branch `wt/<slug>`, directory `../<repo>-wt-<slug>`.
3. Create with `exec`: `git worktree add ../<repo>-wt-<slug> -b wt/<slug>` (omit `-b` to reuse an existing branch).
4. Copy or link untracked local needs (env files, local config) into the worktree before building; never commit them.
5. Install dependencies and run the build/test loop inside the worktree directory only.
6. Track lifecycle with `git worktree list`; journal which slug owns which task and its status.
7. When done: commit or hand off the branch, then `git worktree remove ../<repo>-wt-<slug>` and `git worktree prune`.
8. Before pruning an abandoned worktree, verify no uncommitted work remains in it.

## Output
A branch `wt/<slug>` with committed work, the main checkout left untouched, and a journal entry recording worktree path, branch, owning task, and status (active/merged/removed).

## Routing
- Integrating a finished worktree branch -> `ship` or `git`
- Deep repo knowledge needed inside the worktree -> `go-claw-engineer`
- Coordinating many worktrees across agents -> `team` or `orchestrate`
- Deciding what to build in the worktree first -> `plan`

## Guardrails
- Never remove a worktree with uncommitted changes; check status in that directory first
- One branch per worktree; git refuses checking out the same branch twice — do not fight it
- Never rebase or rename a shared branch from inside a worktree without agreement
- Use absolute paths in all reports so agents never confuse worktrees
- Prune stale worktrees promptly; abandoned checkouts confuse other agents
