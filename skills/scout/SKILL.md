---
name: scout
description: >-
  Fast codebase reconnaissance that maps a task to the minimal set of files,
  entry points, and risks before any work starts. Use when starting an
  unfamiliar task, scoping a bug, planning a change, or onboarding to a repo
  where you do not yet know the blast radius. Keywords: recon, codebase map,
  survey, entry points, hot paths, structure, khảo sát, thăm dò, sơ đồ mã
  nguồn, điểm vào. Dùng khi cần khảo sát nhanh codebase để tìm đúng file trước
  khi làm việc.
license: MIT
version: 1
---

# Scout

Fast, read-only reconnaissance: turn a task into the smallest set of files, entry points, and risks an agent must know before doing work. Produce a recon report, not a solution.

## When to use
- Starting any non-trivial task in an unfamiliar or large codebase
- A bug report that does not yet point to a file or function
- Before writing a plan or estimate that needs real file paths
- Verifying that a claimed symbol, config key, or endpoint actually exists
- Estimating blast radius before a refactor or port

## When NOT to use
- The task already names exact files and lines to change
- Pure writing, research, or ops work with no code involved
- Deep understanding of one algorithm (use a targeted read instead)
- Emergency hotfix where the fix location is already known

## Workflow
1. Restate the task in one sentence and extract 3-6 search terms (symbol names, domain nouns, error strings).
2. Run `list_files` on the repo root to identify project type (go.mod, package.json, pyproject) and top-level layout.
3. Fan out parallel read-only sweeps with `exec` grep/ripgrep for: entry points (main, init, route tables), the task's key symbols, config keys, and schema or table names.
4. Read only files that appeared in hits; skim structure first (types, exported functions), bodies second.
5. Trace one path end-to-end for the core flow: caller -> handler -> store -> response.
6. Note tests covering the area and where conventions live (docs, migrations, UI).
7. Check risks: deprecated patterns, missing tests, generated code, tight coupling, stale comments contradicting code.
8. Decide the minimal file set: files to change vs files to read for context only.
9. Write the recon report (see Output) and propose the next skill and first concrete step.

## Output
A concise recon report: (1) 5-line structure map, (2) key files as absolute paths with one-line roles, (3) core flow trace, (4) risks and unknowns, (5) suggested next skill and first step. Keep it under ~40 lines.

## Routing
- Fix shape is clear -> hand the report to `fix` or `cook` for implementation
- Task needs multi-phase planning -> hand to `goclaw-kit` with the report attached
- Deep platform-specific work in this repo -> `go-claw-engineer`
- Bug needing reproduction -> `debug`; packaging a whole repo instead -> `repomix`

## Guardrails
- Read-only: never edit, stage, or delete anything during recon
- Cap exploration: stop at the first coherent answer, not exhaustive reading
- Cite absolute paths only; never guess a file location without a grep hit
- Summarize file contents instead of pasting large dumps
- If docs and code disagree, trust the code and flag the discrepancy
