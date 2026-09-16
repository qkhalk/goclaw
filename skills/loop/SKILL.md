---
name: loop
description: >-
  Autonomous iterative improvement loop: define one metric and a stop
  condition, then iterate implement-measure-adjust with a hard iteration cap
  and a per-iteration journal, so the run stays auditable and stoppable. Use
  for optimization grinds and bounded improvement targets. Keywords: loop,
  iterate, metric, optimization, vong lap. Dùng khi cần lặp tự động: cải
  thiện - đo lường - điều chỉnh cho đến khi đạt chỉ số mục tiêu.
license: MIT
version: 1
---

# Loop

Run a controlled iterative improvement loop: pick one metric, define the stop condition and a hard iteration cap, then cycle implement -> measure -> adjust with a journal entry per iteration so the run stays auditable and stoppable.

## When to use
- Optimization grinds: reduce bundle size, cut latency, raise coverage, lower cost
- Batch-fixing a measurable class of issues (lint debt, small failing tests)
- Any "keep improving X until it hits Y" request, bounded by a contract

## When NOT to use
- No objective metric exists — you cannot steer without a number
- Each iteration needs user input — that is a conversation, not a loop
- Exploratory work where "better" is undefined — use `deep-swe` or `research`

## Workflow
1. Contract first (via `goal-warmup` if the run may be long): the metric with its exact measuring command, a target or acceptable range, max iterations, per-iteration time cap, and stop conditions.
2. Baseline: run the metric command and record the number and environment in the journal as iteration 0.
3. Plan iteration 1: the single highest-leverage change — one change per iteration so attribution stays clean.
4. Implement the change as a small, revertible diff (`cook`-style).
5. Measure: run the exact metric command from the contract and record the result.
6. Journal every iteration: number, change made, metric before/after, decision (keep/revert), next hypothesis — one append per iteration.
7. Adjust: keep improvements; revert flat or negative results and try a different mechanism — two flat iterations on the same mechanism means switch approach.
8. Enforce stops: target reached, iteration cap hit, time budget spent, or any contract stop condition ends the loop immediately.
9. Final report: metric trajectory from baseline to final, kept changes, reverted experiments, and residual ideas for a future loop.

## Output
A journal (one entry per iteration: change, metric delta, decision) plus a final report with the metric trajectory from baseline to final, kept changes, reverted experiments, and remaining opportunities.

## Routing
- Contract and budget for the loop -> `goal-warmup`
- Implementing each iteration's diff -> `cook`; verification -> `test`
- Multiple independent metrics or areas -> split into parallel streams via `team`
- A scheduled unattended run -> pair checkpoints with `cron`

## Guardrails
- The hard cap is law: at max iterations, stop and report — never "just one more"
- One change per iteration; two changes make attribution impossible
- Revert failures; never stack a regression beneath a later improvement
- Guard against metric gaming: verify the contract's quality constraints still pass every iteration
- Journal from iteration 0 — an unjournaled loop is unreviewable
