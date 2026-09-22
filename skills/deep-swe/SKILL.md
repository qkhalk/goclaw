---
name: deep-swe
description: >-
  Deep software-engineering discipline for hard tasks: slow down, form
  hypotheses, read before write, prove correctness with tests, and fix root
  causes instead of symptoms. A checklist mode, not a pipeline. Use when a bug
  resurfaced after previous fixes or the change is subtle. Keywords: root
  cause, hypothesis, rigor, hard bug, nguyen nhan goc. Dùng khi gặp lỗi khó
  hoặc vấn đề phức tạp cần phân tích kỹ lưỡng trước khi sửa.
license: MIT
version: 1
---

# Deep Swe

Deep software-engineering mode for hard problems: slow down, form explicit hypotheses, read before writing, prove correctness with tests, and fix root causes rather than symptoms. A discipline checklist, not a pipeline.

## When to use
- A bug that resurfaced after two or more "fixed" attempts
- Changes touching concurrency, state machines, caching, or data integrity
- Symptoms spanning layers (UI + API + DB) with no obvious culprit
- Any fix you cannot yet explain end-to-end

## When NOT to use
- Trivial, obvious fixes — ship the small diff directly
- Greenfield feature building with a clear plan — `cook` is fine
- You lack access or knowledge entirely — escalate instead of guessing slower

## Workflow
1. Slow down deliberately: declare the hypothesis phase; no edits allowed until step 6.
2. Write the problem statement: expected vs actual behavior, exact reproduction steps, and evidence (logs, traces, responses).
3. Read before write: trace the relevant code path end-to-end; identify every branch, early return, and state mutation on that path — citing lines without control-flow understanding is how bugs survive.
4. Form 2-4 competing hypotheses, each falsifiable: "X happens because Y; if Y, observation Z should hold."
5. Rank hypotheses by falsification cost: test the cheapest first with temporary logging, targeted commands, or real data inspection.
6. Identify the root cause: keep asking why until the answer is a mechanism; verify the mechanism explains ALL observed symptoms.
7. Design the fix at the cause: prefer explicit configuration and simple mechanisms over runtime heuristics; check the fix does not shift the problem elsewhere.
8. Prove it: write a failing test reproducing the root cause first, then make it pass with the fix; add a regression guard.
9. Sweep for the same bug class: grep for the pattern elsewhere and fix or flag sibling occurrences.
10. Clean up: remove all temporary instrumentation, then summarize mechanism -> fix -> evidence.

## Output
A root-cause writeup: problem statement, hypotheses with how each was falsified or confirmed, the mechanism, the fix diff with its reproducing test, and sibling occurrences found.

## Routing
- Reproduction tooling or environment issues -> `debug`
- The fix grows into a feature -> switch to `goclaw-kit` plus `cook`
- Reviewing the final diff -> `review`
- A recurring bug class across the project -> record it in `decision-log`

## Guardrails
- No edits before a stated mechanism hypothesis exists — fixes without mechanisms are superstition
- Never declare done on "seems to work": name the test that proves it
- One variable at a time: never change multiple things between observations
- Timebox the loop: if every hypothesis is falsified and no mechanism emerges, escalate with evidence instead of shotgunning changes
- Temporary instrumentation must be removed before shipping
