---
name: handoff
description: >-
  Write a compact continuation contract so a fresh agent or session can resume
  work without context loss: goal, decisions, state, gotchas, next steps in
  one markdown file. Use when a session is ending, context is nearly full, or
  work pauses mid-task. Keywords: handoff, continuation, resume, context
  transfer, ban giao. Dùng khi cần bàn giao công việc dở dang cho phiên hoặc
  agent tiếp theo.
license: MIT
version: 1
---

# Handoff

Write a continuation contract — a compact markdown file capturing goal, decisions, current state, gotchas, and next steps — so a fresh context can resume exactly where this one stops, with zero questions.

## When to use
- A session is about to end with work unfinished
- Context is nearly exhausted and a fresh session will continue
- Pausing long work deliberately (end of day, waiting on review)
- Before risky experiments that might wreck the current context

## When NOT to use
- Work is fully done — a final report suffices
- Transferring ownership with orchestration detail — that is `handover`
- Only summarizing what happened, no resumption needed — that is `watzup`

## Workflow
1. Pick a location by convention: `<workspace>/handoffs/<date>-<slug>.md` or the plan's own directory; create it with `write_file`.
2. Goal block: one paragraph — the outcome, acceptance criteria, and budget.
3. Decisions: each irreversible or costly choice with a one-line rationale and date.
4. State: what is done (with paths, branch, commit), what is in progress, what is untouched; use checklists so resumption can tick items off.
5. Gotchas: non-obvious traps — pre-existing failing tests, environment quirks, files that must not be touched, commands that hang.
6. Next steps: ordered and concrete, starting with the exact first command or read for the next agent.
7. Context pointers: key files (absolute paths), relevant skills, prior reports, evidence links.
8. Verify by re-reading as a stranger: could you resume with zero questions? Fix gaps now.
9. Point the next session at the file explicitly in your final message.

## Output
One markdown file, roughly 40-80 lines, with sections: Goal, Decisions, State (checklists), Gotchas, Next steps, Context pointers. Self-contained — no "see chat above".

## Routing
- Adding owners, priorities, acceptance per person -> expand into `handover`
- Fresh context also needs a status picture -> generate a `watzup` digest and link it
- Work continues under a formal plan -> store the handoff beside the `plan` phase files
- The next arc is long and autonomous -> prepend a `goal-warmup` contract

## Guardrails
- Compact over complete: the file must be readable in two minutes
- Absolute paths only; relative paths rot between sessions
- No secrets in the file — reference env var names, never values
- Do not paste code blocks; link files and line ranges instead
- Date and author the file so stale handoffs are detectable
