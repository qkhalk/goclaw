---
name: common
description: >-
  Baseline engineering etiquette shared by every task: TDD discipline, small diffs, clear naming,
  explicit error handling, git hygiene, and asking sharp questions. Use as the default conduct
  layer when writing, reviewing, or refactoring code of any kind. Keywords: best practices, code
  quality, tdd, clean code, git hygiene, small commits, error handling, chuan code, ky luat lap
  trinh. Dùng khi viết hay sửa code để giữ chuẩn chất lượng chung cho mọi tác vụ.
license: MIT
version: 1
---

# Common

Shared engineering patterns that apply to every coding task regardless of domain or language.

## When to use
- Before and during any implementation, refactor, or review.
- When output quality feels sloppy: giant diffs, vague names, silently swallowed errors.
- As the baseline etiquette underlying specialized skills like `cook`, `fix`, or `test`.

## When NOT to use
- Read-only exploration with no artifacts produced.
- Pure documentation or conversation tasks with no code impact.

## Workflow
1. **TDD discipline:** for bug fixes, write the failing test first; for features, write the acceptance test before the happy path. Red, then green, then refactor — never code first and maybe test later.
2. **Small diffs:** one logical change per commit. If a diff grows beyond ~200 lines, look for a seam to split along. Separate "move" commits from "change" commits.
3. **Naming:** names state intent (`retryWithBackoff`, not `doStuff2`). Match the codebase's existing vocabulary; a consistent conventional name beats a clever novel one.
4. **Error handling:** every error is handled or deliberately propagated — never discarded. Wrap with context so the failure chain reads top-down (e.g. "loading config: <cause>").
5. **Git hygiene:** imperative commit subjects under ~70 characters, body explains why. Never mix generated files with hand edits; never commit secrets; verify what you stage.
6. **Asking good questions:** when blocked, ask one question with concrete options and your recommendation, not an open-ended plea — the `ask` skill has the full method.
7. **Finish cleanly:** run the project's build, vet, and tests before declaring done; report what you ran, not what you assume.

## Output
Code and commits that follow the patterns above, plus a short self-check when handing work to `review`: tests added, diff size, and the naming and error-handling choices made.

## Routing
- Implementation-heavy work → apply alongside `cook` or `fix`.
- Verifying the patterns held → `review` and `test`.
- Recording a non-obvious trade-off for posterity → `decision-log`.

## Guardrails
- These patterns never override a project's own conventions (AGENTS.md, CONTRIBUTING.md) — the codebase wins.
- Do not gold-plate: apply the lightest pattern that prevents the actual failure mode.
