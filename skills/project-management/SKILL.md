---
name: project-management
description: >-
  Progress tracking for multi-step work: milestone breakdown, status reports, risk and dependency
  registers, and recognizing when a plan is slipping — with concrete re-scoping moves. Use when
  coordinating a multi-phase project, reporting progress, or deciding to cut scope. Keywords:
  project tracking, milestones, status report, risks, dependencies, deadline, slip, quan ly du
  an, bao cao tien do. Dùng khi cần theo dõi và báo cáo tiến độ dự án nhiều giai đoạn.
license: MIT
version: 1
---

# Project Management

Keep multi-step work observable: what is done, what is at risk, and what to cut when reality diverges from plan.

## When to use
- Coordinating work with multiple phases, milestones, or contributors.
- The user asks for a status update, timeline, or "are we on track".
- Deadlines loom and scope must be renegotiated.

## When NOT to use
- Fine-grained task mechanics inside one plan → todo lists and `plans-kanban`.
- Cross-session goal continuity → `codex-goal`.

## Workflow
1. **Decompose:** break the project into milestones with concrete, verifiable done-definitions and rough effort estimates; record dependencies between them.
2. **Baseline:** capture the milestone list with target dates (or relative ordering when no dates exist) in a tracking file in the workspace.
3. **Track execution:** as work completes, update status (not started, in progress, done, blocked) with one-line evidence; refresh at the end of every work session.
4. **Maintain a risk register:** each risk with likelihood, impact, early-warning signal, and mitigation; make dependencies explicit ("M3 blocked until the API contract is frozen").
5. **Detect slip early:** compare actual progress against the baseline at each checkpoint. Signals: milestones taking over 150% of estimate, blocked items aging, growing rework. Name the slip plainly.
6. **Re-scope when slipping:** in order of preference — cut scope (drop or defer milestones), lower the quality bar on non-critical items, extend the timeline, add help. Present the trade-off via one `ask` question with options; never silently descope.
7. **Report:** deliver the status report — headline first (on track, at risk, slipping), milestone table, top risks with mitigations, decisions needed. Keep detail behind the headline.

## Output
A current tracking file (milestones, statuses, risks, dependencies) plus stakeholder-ready status reports — anyone reading them knows project health in under a minute.

## Routing
- Plan file structure and per-phase detail → `plans-kanban`.
- Goal registry for cross-session continuity → `codex-goal`.
- A scope decision is required → `ask`, then record the outcome in `decision-log`.

## Guardrails
- Never report optimistic status to avoid conflict; a truthful "at risk" beats a late surprise.
- Descope explicitly and with user consent — silent cuts destroy trust.
- Status updates come from verified evidence (tests, merges, demos), not from hope.
