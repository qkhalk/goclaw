---
name: issue-to-plan
description: >-
  Turn a GitHub issue or bug report into an implementation plan: extract
  testable acceptance criteria, define scope in/out, surface risks, and list
  file-level changes — then hand off to the plan skill. Use when an issue is
  approved for development. Keywords: issue, acceptance criteria, scoping,
  implementation plan, ke hoach. Dùng khi cần chuyển một issue hoặc báo cáo
  lỗi thành kế hoạch triển khai cụ thể.
license: MIT
version: 1
---

# Issue To Plan

Convert a GitHub issue (or any bug report) into an implementation-ready brief: extract testable acceptance criteria, bound the scope, surface risks, and enumerate file-level changes — then hand the brief to `plan`.

## When to use
- An issue has been triaged and approved for development
- A bug report needs a fix strategy before anyone touches code
- Multiple issues must be sized and sequenced into a milestone

## When NOT to use
- A one-line obvious fix — just fix it and reference the issue
- The issue is a question or discussion, not actionable work
- The request is still ambiguous — ask the reporter first via `ask_options` or an issue comment

## Workflow
1. Fetch the issue (via `gh` through `exec`, or `web_fetch` for other trackers); read title, body, comments, and linked PRs/issues.
2. Extract acceptance criteria: rewrite the request as numbered, testable statements (given/when/where possible); flag untestable ones as open questions.
3. Scope: separate the minimal must-fix from reported-but-separable asks; write explicit out-of-scope items.
4. Recon the code: run `scout`-style read-only sweeps to find affected files, entry points, tests, and schema or config touchpoints; cite absolute paths.
5. Assess impact: behavior changes, API/UI contract changes, migrations, backward compatibility, security implications.
6. List risks and unknowns, each with a mitigation: a spike, a question for the reporter, or a feature flag.
7. Draft the change list: file by file (path -> change summary -> verification), ordered by dependency.
8. Estimate size (S/M/L) and note whether it splits into parallel streams (making it `team` material).
9. Hand off: invoke `plan` with this brief as input to produce the full plan document; link the issue in the plan header.

## Output
An issue brief: source link, restated problem, testable acceptance criteria, scope in/out, file-level change list, risks with mitigations, and a size estimate — ready to become a plan document.

## Routing
- Full plan document production -> `plan` (this brief is its input)
- Implementation after planning -> `cook`; verification -> `test`; delivery -> `ship`
- A bug needs reproduction before scoping -> `debug`
- Contract/API changes detected -> flag `go-claw-engineer` and the repo's parity rules

## Guardrails
- Every acceptance criterion must be objectively testable; rewrite or question the rest
- Never invent requirements absent from the issue — park them as open questions
- Cite evidence for every claimed file: a grep hit or a read, never memory
- Keep the minimal fix minimal; scope creep needs explicit user sign-off
- Link the issue in everything downstream so the trail stays traceable
