---
name: plans-kanban
description: >-
  Manage a plans/ directory as a kanban workflow: plan file naming, draft/active/done status
  markers, phase files, progress reports, and generating a kanban-style status board from the
  folder. Use when tracking multiple plans, resuming work, or reporting project state. Keywords:
  kanban, plans directory, status board, plan tracking, phases, progress, bang tien do, quan ly
  plan. Dùng khi cần theo dõi tiến độ nhiều plan dưới dạng bảng kanban.
license: MIT
version: 1
---

# Plans Kanban

Turn the `plans/` directory into a visible, reliable kanban board the agent can read and update.

## When to use
- Multiple plans exist and their status is unclear or scattered.
- Resuming work after a break: which plan is active, which phase is next.
- The user asks "where are we?" or wants a status board.

## When NOT to use
- A single in-flight plan with an obvious next step — just continue it.
- Richer stakeholder reporting across risks and dates → `project-management`.

## Workflow
1. **Inventory:** list the `plans/` tree and read each plan's status marker (first ~20 lines only). File conventions: `YYYY-MM-DD-slug/` directory or `YYYY-MM-DD-slug.md` per plan; a status field on the first line — `draft | active | blocked | done | abandoned`; phases as `phase-1.md`, `phase-2.md` each with Context, checkbox Tasks, and Verification; reports under `reports/` inside the plan unit.
2. **Classify** into columns: backlog (draft), in-progress (active), blocked, done.
3. **Pick the work:** for the active plan, open its current phase file and confirm the next unchecked task — that is your work queue.
4. **Update as reality changes:** mark `done` only when the plan's verification steps pass; mark `blocked` with a one-line reason and owner; never leave chat-only status.
5. **Produce the board** for the user as a compact table: Plan, Status, Current phase, Next action, Blocker.
6. **Housekeeping:** archive done plans into `plans/archive/`; prune abandoned ones with explicit user confirmation.

## Output
A kanban-style status table plus updated status markers in the files themselves — after the pass, the folder always reflects reality.

## Routing
- Setting up the plans/ layout in a fresh project → `bootstrap`, then `folder-context`.
- Goal-level tracking beyond plan files → `codex-goal`.
- Formal progress reports with risks and dates → `project-management`.

## Guardrails
- Status lives in the plan file, not only in chat — chat is lost, files persist.
- Never mark a phase done without executing its stated verification.
- One active plan per workstream unless the user explicitly parallelizes.
