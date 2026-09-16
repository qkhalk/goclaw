---
name: brainstorm
description: >-
  Structured brainstorming that diverges wide, surfaces hidden constraints, then
  converges to a scored shortlist. Use when a task is open-ended, several
  candidate approaches are possible, or a plan needs alternatives before
  committing. Keywords: ideation, diverge, converge, options, decision matrix,
  creativity. Dùng khi cần brainstorm, lấy ý tưởng, tìm nhiều hướng giải pháp,
  hoặc so sánh các phương án trước khi quyết định.
license: MIT
version: 1
---

# Brainstorm

Produce a ranked shortlist of ideas with explicit trade-offs instead of acting
on the first idea that comes to mind. Divergence first, convergence second.

## When to use
- The request is open-ended ("how should we...") with no single obvious answer.
- A plan or design feels stuck, weak, or premature and needs alternatives.
- The user asks for ideas, options, names, angles, or creative directions.
- Multiple approaches look plausible and need a fair comparison.

## When NOT to use
- The task has one correct answer or is a pure execution step.
- An approved plan or requirements doc already fixes the approach.
- The user needs an implementation now; go straight to the relevant skill.

## Workflow
1. Restate the goal in one sentence and list hard constraints (deadline, stack,
   budget, non-negotiables) gathered from the request and any codebase files
   you inspected with `read_file` or `list_files`.
2. Surface hidden constraints the user did not state; ask via `ask_options`
   only when a wrong guess would invalidate the whole session.
3. Diverge: generate at least 8-12 raw ideas without judging. Mix categories:
   boring and proven, cheap and fast, ambitious, and inverted ("what if the
   opposite were true?").
4. Converge: drop ideas that violate a hard constraint. Keep 3-5 candidates.
5. Score survivors in a matrix: impact, effort, risk, reversibility (1-5
   each), weighted toward the stated goal.
6. For each of the top 3, write a 2-3 sentence rationale: why it scores well
   and what evidence would make you reject it.
7. Recommend one option explicitly; do not hedge.

## Output
A one-page shortlist: ranked table (idea, scores, total), rationale per idea,
the constraints used, and a single clear recommendation.

## Routing
- The chosen idea needs an implementation plan → `plan` or `architect`.
- The decision itself should be recorded → `decision-log`.
- The user only wants a recommendation between fixed options → `advise`.

## Guardrails
- Never present one idea as the only option when real alternatives exist.
- Do not let scoring fake precision; state the assumptions behind weights.
- Timebox divergence; iterate with more ideas rather than padding the list
  with near-duplicates.
