---
name: handover
description: >-
  Compose a complete work handover for another agent or teammate: continuation
  contract plus orchestration notes — who does what, priorities, acceptance
  criteria, access map. Use when transferring ownership of in-flight work.
  Keywords: handover, transition, ownership transfer, onboarding brief, ban
  giao cong viec. Dùng khi cần bàn giao toàn bộ công việc và trách nhiệm cho
  người hoặc agent khác.
license: MIT
version: 1
---

# Handover

Assemble a full ownership-transfer package: the technical continuation contract (`handoff` format) plus orchestration notes — workstreams, owners, priorities, acceptance criteria — so the receiver can lead the work, not just resume it.

## When to use
- Transferring a workstream permanently to another agent or teammate
- Splitting one agent's responsibilities across a team
- Decommissioning a session that owns live tasks (leave, migration)
- A `team` run ends and results must be handed to a maintainer

## When NOT to use
- Temporary pause with the same owner — `handoff` alone is enough
- Status reporting only — `watzup`
- Process reflection — `retro`

## Workflow
1. Inventory everything owned: branches, open PRs, plan files, journals, scheduled `cron` entries, in-flight sessions via `sessions_list`.
2. Produce the technical contract in `handoff` format: goal, decisions, state, gotchas, next steps.
3. Add orchestration notes: each open workstream with owner, priority (now/next/later), acceptance criteria, and current blocker.
4. Add the access map: repos, environments, where credentials live (names only, never values), dashboards, and who to ask.
5. Add the promises log: commitments made to humans the receiver must honor (reviews owed, replies pending, demos promised).
6. Compose the document with `write_file` (e.g., `handovers/<date>-<slug>.md`), linking the raw handoff file.
7. Walk through it once as the receiver: is every acceptance criterion testable? every blocker actionable?
8. Deliver: link the file in the final message and notify the receiver with `message` if reachable.

## Output
A handover document: technical contract (handoff format), workstreams table (task, owner, priority, acceptance, blocker), access map, and promises log. The receiver can act without asking a single question.

## Routing
- Underlying technical detail -> generated with `handoff`
- Status snapshot for the receiver -> embed a fresh `watzup` digest
- The receiver will run a long autonomous arc -> prepend a `goal-warmup` contract
- Formal plan continuation -> link the relevant `goclaw-kit` phase files

## Guardrails
- Never include secret values; point to the secret store, not the contents
- Every workstream needs an owner and an acceptance criterion — no orphan bullets
- Mark assumptions explicitly so guesses never read as facts
- Keep to ~100 lines; link evidence rather than inlining it
- Date, author, and name the receiver at the top
