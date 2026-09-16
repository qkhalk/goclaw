---
name: design
description: >-
  General design assistance beyond UI: system design sketches, information
  architecture, design docs, and design-review checklists with explicit
  trade-offs. Use when shaping how something works or is organized before
  building it — not for pixel-level UI work. Keywords: system design,
  information architecture, design doc, design review. Dùng khi cần thiết kế
  hệ thống, cấu trúc thông tin, viết design doc, hoặc rà soát thiết kế tổng
  thể.
license: MIT
version: 1
---

# Design

Design is deciding what a thing is before building it: boundaries, flows,
data, and failure behavior — recorded in an artifact others can critique.

## When to use
- A feature or system needs a sketch: components, responsibilities, data
  flow, interfaces.
- Content or data needs structure: information architecture, navigation,
  taxonomy, URL/slug design.
- A design exists and needs a structured review pass.
- A design decision needs a short written rationale for the team.

## When NOT to use
- Pixel-level or interaction-specific UI work → `ui-ux-pro-max`.
- A formal, decision-ready architecture proposal with migration and risk
  register → `architect`.
- Reviewing implemented code → `review`.

## Workflow
1. Frame: one sentence for what this must achieve, one for what it must not
   become. List constraints (scale, latency, team size, existing stack).
2. Choose the artifact the request actually needs:
   - System sketch: components, responsibilities, data flow (express as a
     Mermaid diagram in markdown), explicit interfaces between parts.
   - Information architecture: inventories, grouping, naming, navigation
     tree, and where each future item goes.
   - Design doc: problem, goals/non-goals, chosen approach, alternatives
     rejected and why, open questions.
   - Review checklist: run a design under review against coherence,
     boundaries, failure modes, data ownership, and evolution cost.
3. Generate one primary option and at least one serious alternative; state
   the trade-off that separates them.
4. Self-review the primary option: where does it break first under load,
   change, or misuse? Fix or note it.
5. Write the artifact with decisions bolded and open questions listed at
   the end.

## Output
The chosen artifact (sketch, IA tree, design doc, or filled review checklist)
with trade-offs explicit and open questions enumerable.

## Routing
- Formal architecture proposal with migration/rollback → `architect`.
- UI-specific visuals and interaction design → `ui-ux-pro-max`.
- Sequencing the implementation → `plan`; recording the decision →
  `decision-log`.

## Guardrails
- Every boundary must have a stated reason; "we always do it this way" is
  not a reason.
- Name what is out of scope (non-goals) to prevent silent scope creep.
- Prefer reversible decisions; flag irreversible ones for explicit sign-off.
