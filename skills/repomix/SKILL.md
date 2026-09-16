---
name: repomix
description: >-
  Package a repository into one AI-friendly markdown/XML document with
  include/exclude rules, token budgeting, and secrets redacted — for sharing
  full context with another model or agent. Use when targeted reads are not
  enough. Keywords: repomix, pack repo, context bundle, code snapshot, dong
  goi ma nguon. Dùng khi cần đóng gói toàn bộ repository thành một tài liệu
  duy nhất cho AI đọc.
license: MIT
version: 1
---

# Repomix

Pack a repository into one AI-friendly document (markdown or XML) with deliberate include/exclude rules, a token budget, and secrets redacted — so a distant model gets full context without repo access.

## When to use
- Handing a codebase slice to another model or agent with no repo access
- Asking an external LLM for a whole-repo review or migration assessment
- Cross-repo analysis (porting a feature) that needs two codebases packed
- Snapshotting "how the code looked" for comparison or archival

## When NOT to use
- The task touches 2-3 known files — targeted `read_file` is cheaper and fresher
- The repo is huge and the relevant slice is small — pack the slice, not the repo
- The consumer needs current truth — a pack is stale the moment it is written

## Workflow
1. Define the question the packed document must answer; scope follows the question.
2. Choose the include set: relevant directories, globs, and docs; exclude vendored deps, lockfiles, build outputs, media binaries, and generated code.
3. Set the token budget: estimate packed size (roughly bytes/4) and tighten excludes until it fits the target model's usable context with room for the answer.
4. Redact before packing: search the include set for secret patterns (API keys, tokens, passwords, env files, private config) and exclude those files or strip the values; never pack raw credentials.
5. Produce the pack — via the repomix CLI with `exec` when available, otherwise assemble it with `list_files` + `read_file` + `write_file`.
6. Add a manifest header: generation date, source commit hash, include/exclude rules, redaction notes.
7. Sanity-check the pack: key files present, no secret strings survived (grep the output), total size under budget.
8. Deliver the pack file path plus the manifest, and state what was deliberately left out.

## Output
One packed document with a manifest header (date, commit, rules, redactions), sized to the token budget, containing only reviewable source and docs.

## Routing
- The pack feeds a porting effort -> follow up with `xia`
- Whole-repo review of the findings -> `review`, or `security-audit` for the security lens
- The real need is fresh in-session repo knowledge -> `scout` instead

## Guardrails
- Secrets never leave the machine: redact first, pack second, then verify the output with a grep
- Do not send proprietary code to third-party services without authorization
- Exclude generated and vendored noise — it burns budget and hides signal
- Always record the commit hash; a pack without a commit is unverifiable
- Prefer scoped slices over whole repos when the question is scoped
