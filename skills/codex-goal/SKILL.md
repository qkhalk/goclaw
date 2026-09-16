---
name: codex-goal
description: >-
  Long-horizon goal management for coding agents: a goal registry in the workspace, milestones
  with done-definitions, and resuming partially-complete goals across sessions. Use for work
  spanning many sessions, after context loss, or when a project keeps stalling without a north
  star. Keywords: goal tracking, milestones, long-running, resume, registry, done criteria, muc
  tieu, quan ly muc tieu. Dùng khi quản lý mục tiêu dài hạn cần tiếp tục qua nhiều phiên làm việc.
license: MIT
version: 1
---

# Codex Goal

Keep long-horizon goals alive across sessions with a durable registry and unambiguous done-definitions.

## When to use
- Work spans many sessions or will outlive the current context window.
- Resuming after context loss: what was the goal, what is done, what is next.
- A project keeps drifting or restarting because "done" was never defined.

## When NOT to use
- Single-session tasks — a plain todo list is enough.
- Stakeholder-facing status reporting → `project-management`; plan-file mechanics → `plans-kanban`.

## Workflow
1. **Register:** keep a `goals.md` (or `goals/` directory) at the workspace root — one section per goal with statement, why, done-definition, milestones M1..Mn, and status. Done-definitions are checkable facts ("integration tests green", "user confirms in writing"), never vibes ("basically working"). Draft it and confirm with one `ask` question if the user's goal is vague.
2. **Milestonize:** split into 2-6 milestones, each independently verifiable and sized for 1-3 sessions; link each to its plan file in `plans/` for phase detail.
3. **Work:** execute the current milestone through the appropriate chain (usually `goclaw-kit` → `cook` → `test`).
4. **Checkpoint:** at each milestone boundary, update `goals.md` status and write a short progress note — what changed, evidence, next step — so any future session can resume cold.
5. **Resume:** at session start with an unfinished goal, read `goals.md` first, then the current milestone's plan file and last progress note. Rebuild state from files, never from memory.
6. **Close:** when the done-definition is fully satisfied, mark done with evidence links and propose archiving.

## Output
A current `goals.md` registry plus per-milestone progress notes; the invariant is that a cold-start session can resume in under two minutes of reading.

## Routing
- Plan and phase file mechanics → `plans-kanban`; stakeholder status → `project-management`.
- A milestone proves to be the wrong direction → `decision-log` the pivot, then update the registry.

## Guardrails
- Never mark a milestone done without executing its verification.
- Done-definitions change only with user agreement — loosening them silently defeats the system.
- The registry lives in workspace files, not in conversation memory.
