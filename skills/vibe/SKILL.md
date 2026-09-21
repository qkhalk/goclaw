---
name: vibe
description: >-
  End-to-end autonomous feature pipeline: take an issue or requirement through
  recon, plan, implement, test, review, and ship by chaining existing skills,
  with checkpoints so the run is resumable. Use when a task is fully specified
  and approved for autonomous execution. Keywords: end-to-end, pipeline,
  feature delivery, automation, triển khai trọn gói. Dùng khi cần chạy trọn
  vòng đời một tính năng từ yêu cầu đến khi hoàn thiện.
license: MIT
version: 1
---

# Vibe

Drive a fully-specified task through the whole delivery pipeline — recon, plan, implement, test, review, ship — by chaining the project's existing skills, checkpointing progress after every phase so the run is auditable and resumable.

## When to use
- A requirement is complete with acceptance criteria and approved for autonomous execution
- You own a feature from idea to merge and hold a long-run budget
- The user asks for "the whole thing" and grants uninterrupted run time

## When NOT to use
- Requirements are vague — stop and ask via `ask_options`, or run `goal-warmup` first
- A one-line fix — a direct edit plus `test` is cheaper than a pipeline
- Any step still needs human sign-off that has not been given
- The repo is unfamiliar and no recon exists — start with `scout` explicitly

## Workflow
1. Validate scope: restate the requirement, acceptance criteria, and budget. If any is missing, stop and ask.
2. Recon: run `scout` (or reuse a fresh recon report) to get the file set and risks.
3. Plan: hand the recon to `goclaw-kit`; receive a phased plan with file-level steps and verification steps.
4. Warmup checkpoint: write the outcome contract (goal-warmup style) beside the plan before writing code.
5. Implement phase by phase with `cook`; after each phase append a journal checkpoint: phase, files touched, tests run, next step.
6. Test: run the project's test commands or the `test` skill; a failing gate means loop back to `fix`, never skip forward.
7. Review: call `review` on the diff; treat blocking findings as loop-backs to step 5.
8. Ship: when tests and review pass, run `ship` (branch, commit, PR) per repo convention.
9. Final report: what shipped, evidence (test names, PR link), deviations from plan, open items.

## Output
A merged or PR-ready change plus a delivery journal: plan reference, per-phase checkpoints, test and review evidence, and a final summary a human can audit in two minutes.

## Routing
- Step-level work: `scout`, `goclaw-kit`, `cook`, `test`, `fix`, `review`, `ship` do the actual work
- Long-horizon governance, budgets, checkpoints -> `mission` or `goal-warmup`
- Parallelizable subtasks discovered mid-run -> split with `team`
- Platform complexity in this repo -> `go-claw-engineer`

## Guardrails
- Never skip test or review gates to save time — the gates are the contract
- Stop for human approval before destructive operations, scope changes, new dependencies, or exceeding budget
- One pipeline run per task; never interleave two runs in one journal
- Checkpoint after every phase; a crash must never lose more than one phase
- Two consecutive loop-backs on the same phase: stop and escalate instead of thrashing
