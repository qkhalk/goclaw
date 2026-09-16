---
name: web-design-guidelines
description: >-
  Audit UI code against web interface guidelines: accessibility (contrast, focus,
  keyboard), interaction states, typography rhythm, spacing scale, mobile behavior —
  output a prioritized findings list with concrete fixes. Use when reviewing UI
  quality before shipping. Keywords: UI audit, accessibility, WCAG, contrast, focus
  states, typography, spacing, responsive, interaction states, kiểm tra giao diện,
  đánh giá UI, audit giao diện. Dùng khi cần audit, đánh giá chất lượng giao diện
  trước khi bàn giao.
license: MIT
version: 1
---

# Web Design Guidelines

Audit a UI against established web interface guidelines and produce a prioritized,
evidence-based findings list with concrete fixes — no vague grades.

## When to use
- Reviewing screens or components before shipping a UI change.
- Auditing an existing page for accessibility and interaction-quality gaps.
- Producing a fix backlog from a UI quality sweep.

## When NOT to use
- Building or restyling the UI — use frontend-design or frontend-development.
- Applying the GoClaw in-house mobile checklist specifically — use ui-ux-pro-max.
- Performance or bundle audits — use react-best-practices.

## Workflow
1. Define scope: the routes, screens, or components under review; read their code
   and CSS (read_file). Capture screenshots via the browser for visual evidence.
2. Walk the checklist in order, pass or finding per rule:
   - Semantics: real elements (`button`, `a`, `input`), landmarks, heading order.
   - Keyboard: everything operable, logical tab order, visible focus indicators.
   - Contrast: text >= 4.5:1, large text and UI edges >= 3:1, both themes.
   - Interaction states: hover/focus/active/disabled/loading on every control.
   - Typography: consistent scale, line length ~45-75 characters, line-height fit.
   - Spacing: consistent 4/8px rhythm; no one-off margins without a scale reason.
   - Touch: targets >= 44px; inputs >= 16px on mobile; `h-dvh` full-height layout.
   - Responsive: usable at 360px; tables scroll horizontally; no fixed grids.
   - Forms: labels, inline errors, preserved input, submit feedback.
   - Motion: `prefers-reduced-motion` respected; no essential info in animation only.
3. Classify each finding: BLOCKER (unusable or unreachable content), HIGH
   (accessibility violation), MEDIUM (inconsistency), LOW (polish).
4. For each finding record: `file:line`, what fails, why it matters, and the minimal
   concrete fix (exact class, attribute, or style change).
5. Sort by severity and close with a recommended fix order.

## Output
- A findings report: numbered, severity-tagged, each with location, evidence
  (code line or screenshot), and a specific fix. A rule with no violation is
  marked pass with evidence — never "mostly OK".

## Routing
- Applying the fixes: frontend-development (or fix for targeted patches).
- GoClaw in-house mobile/UI checklist: ui-ux-pro-max.
- Restyling after findings: frontend-design or ui-styling.
- Shipping once clean: ship.

## Guardrails
- Every finding needs evidence: `file:line` or a screenshot — no speculation.
- Do not modify code during the audit unless explicitly asked.
- Respect the existing design system; flag deviations, don't invent a new one.
- Report both themes (light/dark) and mobile viewport, not just desktop defaults.
