---
name: interview-docs
description: >-
  Extract a user's vision into durable documentation through structured
  questioning: README, ADRs, glossary, and roadmap — capturing decisions and
  their rationale, not just features. Use when a project exists only in the
  owner's head, or docs are stale and the source of truth is unavailable.
  Keywords: elicitation, ADR, vision capture, knowledge extraction. Dùng khi
  cần phỏng vấn để trích xuất tầm nhìn dự án thành tài liệu bền vững như
  README, ADR, từ điển thuật ngữ và lộ trình.
license: MIT
version: 1
---

# Interview Docs

Turn what a founder or maintainer knows into documents a stranger (or a future
agent) can act on. The deliverable is written rationale, not a transcript.

## When to use
- A project has little or no documentation but an available owner.
- Onboarding is painful and decisions live in chat history or memory.
- Before a handover, audit, or the start of delegated development.

## When NOT to use
- The docs exist and only need updating or restructuring → `docs`.
- Answers can be found in code or git history — read before asking.
- Recording one specific decision → `decision-log`.

## Workflow
1. Survey first: `list_files` + `read_file` on existing docs, README,
   config, and schemas. Never ask what you can read. Note contradictions
   and gaps as question seeds.
2. Build a question queue across five topics: purpose and users; success
   criteria; constraints and non-negotiables; vocabulary (terms, entities,
   names); decisions already made and WHY (including rejected alternatives).
3. Interview in small batches: one topic per turn via `ask_options` (concrete
   choices) or open questions when choices do not exist. Confirm
   interpretations by restating answers in one line.
4. Write immediately: after each answer, update the target doc with
   `write_file` / `edit` while context is fresh. Do not batch all writing
   to the end.
5. Produce the four artifacts as applicable:
   - README: what this is, who it serves, how to run/operate it.
   - ADRs: one per significant decision — context, decision, consequences,
     status, date.
   - Glossary: term, definition, where it appears in code/config.
   - Roadmap: now / next / later, each item with its motivating reason.
6. Close the loop: show the docs summary to the owner, correct errors,
   and mark open questions that remain unanswered.

## Output
Committed docs (README, ADRs, glossary, roadmap) in the repo, plus a list of
unresolved questions. Every captured decision includes its rationale and date.

## Routing
- Docs need ongoing maintenance structure → `docs`.
- Single decisions deserve their own record → `decision-log`.
- The roadmap becomes executable work → `plan`.

## Guardrails
- Ask before assuming: an invented rationale is worse than a marked gap.
- Keep the owner's answers verbatim in your notes; paraphrase only in the
  final docs, and flag anything you generalized.
- Respect interview fatigue: 3-5 questions per turn maximum, skip what the
  repo already answers.
