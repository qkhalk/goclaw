---
name: bootstrap
description: >-
  Bootstrap a new project workspace: skeleton layout, README and AGENTS conventions, tooling
  selection, environment setup checklist, and first-commit hygiene. Use when starting a project
  from zero, initializing a repo, or rescuing a project that grew without structure. Keywords:
  new project, scaffold, init, setup, skeleton, readme, tooling, khoi tao du an, khoi tao repo.
  Dùng khi cần khởi tạo dự án mới hoặc thiết lập cấu trúc workspace từ đầu.
license: MIT
version: 1
---

# Bootstrap

Stand up a new project workspace with a sane skeleton, conventions, and tooling so future agents and humans navigate fast.

## When to use
- Starting a project from an empty directory or a bare repository.
- A project lacks README, conventions, or structure and work is becoming chaotic.
- The user asks to "set up", "initialize", or "scaffold" a new codebase.

## When NOT to use
- The project already has structure — extend it instead of re-imposing (use `folder-context`).
- Inside an established monorepo that has its own conventions.

## Workflow
1. Clarify the minimum with the user: language and framework preference, deploy target, anything non-negotiable (see `ask` if genuinely ambiguous).
2. Create the skeleton: `src/` (or the idiomatic equivalent), `tests/`, `docs/`, and a `plans/` directory if multi-phase work is expected.
3. Write a short README: what the project is, how to install, how to run, how to test. Under 60 lines; link out rather than inline.
4. Add an AGENTS or CONTRIBUTING conventions file: code style, commit style, test expectations, and always/never rules future agents must follow.
5. Select tooling: formatter, linter, and test runner first; add CI configuration only if a remote exists. Prefer boring mainstream choices with strong defaults.
6. Prepare the environment: commit an example env file with keys but no secret values, add ignore rules for env files and build output, document required services.
7. First-commit hygiene: one clean initial commit with the skeleton; verify the test runner actually executes (even on a trivial test) before claiming the setup works.

## Output
A runnable, testable skeleton: directory layout, README, conventions file, tooling config, ignore rules, and a passing first test — committed cleanly.

## Routing
- Multi-phase roadmap for the new project → `goclaw-kit` and `codex-goal`.
- Ongoing folder and file placement rules → `folder-context`.
- First feature on top of the skeleton → `cook`.

## Guardrails
- Do not install heavy dependencies speculatively; add tooling when a need is real.
- Never commit real secrets; example files carry placeholder values only.
- Match the ecosystem's idioms (the language's standard layout) over personal preference.
