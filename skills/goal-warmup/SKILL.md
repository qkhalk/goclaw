---
name: goal-warmup
description: >-
  Preflight for long-running autonomous goals: write an outcome contract with
  success criteria, constraints, budget, checkpoints, and stop conditions, and
  confirm scope with the user before starting multi-hour unsupervised work.
  Use before any extended autonomous run. Keywords: goal, outcome contract,
  budget, checkpoints, autonomous, muc tieu. Dùng khi cần chốt hợp đồng mục
  tiêu trước khi chạy tự động trong thời gian dài.
license: MIT
version: 1
---

# Goal Warmup

Preflight any long-running autonomous goal: turn a vague ambition into an outcome contract — success criteria, constraints, budget, checkpoints, stop conditions — and confirm it with the user before burning hours of unsupervised work.

## When to use
- Before multi-hour or overnight runs (migrations, large refactors, bulk processing, `loop` runs)
- Before delegating a big arc to sub-agents (`mission`, `team`, `vibe`)
- When the user says "just handle it" and boundaries need to be in writing

## When NOT to use
- Short tasks under ~30 minutes — the contract costs more than it saves
- Interactive sessions with the user present throughout
- Genuine emergencies where asking first is the wrong move

## Workflow
1. Restate the goal as a measurable outcome: what artifact or state will exist when done, and how it will be verified.
2. Write success criteria: 3-7 testable statements, each mapping to a command, check, or observable state.
3. Define constraints: what must not change (public APIs, data, shared files), style and convention requirements, deadlines.
4. Set the budget: max wall-clock time, iteration count, and cost (tokens, sub-agent count) — hard numbers, never "reasonable effort".
5. Define checkpoints: every N iterations or M minutes, write a journal entry and decide continue / pivot / stop.
6. Write stop conditions: criterion proven impossible, budget exhausted, repeated identical failures, missing credentials or approvals.
7. Identify dependencies and permissions: tools, access, and approvals needed — obtain them before starting, not mid-run.
8. Present the contract compactly (one screen) and get explicit confirmation via `ask_options` (proceed / adjust / abort).
9. Save the contract beside the work (e.g., `plans/<slug>/contract.md`) and reference it in every checkpoint entry.

## Output
A one-screen outcome contract file (goal, criteria, constraints, budget, checkpoints, stop conditions) plus recorded user confirmation before execution starts.

## Routing
- Executing the contracted goal -> `vibe`, `team`, `mission`, or `loop` depending on shape
- Iterative optimize-measure cycles -> pair with `loop`; the contract bounds the loop
- Phased work needs a plan document -> `plan` consumes the contract
- Mid-run assessment -> checkpoint journal vs this contract

## Guardrails
- No contract, no run: never start a long autonomous arc without agreed criteria and budget
- Numbers beat adjectives: "max 20 iterations" survives disagreement, "reasonable" does not
- Stop conditions are commitments, not suggestions — honor them mid-run
- Scope changed? Pause and re-contract; do not silently drift
- Keep the contract findable: link it from journals and the final report
