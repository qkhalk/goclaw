---
name: scenario
description: >-
  Generate edge cases across 12 dimensions — inputs, scale, concurrency,
  failure, security, locale, accessibility, time, network, data, integration,
  human — and shape them into a prioritized test-scenario matrix. Use when
  planning test coverage, hardening a feature, or auditing a design for
  unhandled situations. Keywords: edge cases, test matrix, boundary
  conditions, chaos scenarios. Dùng khi cần liệt kê kịch bản biên, edge case,
  hoặc lên ma trận kịch bản kiểm thử.
license: MIT
version: 1
---

# Scenario

Systematic edge-case generation: sweep every dimension so coverage is a
checked property, not a matter of inspiration.

## When to use
- Before writing tests for a new feature or API.
- A design/plan needs a hardening pass ("what happens when...?").
- Post-incident: enumerate the scenario class the incident belonged to.

## When NOT to use
- Implementing the tests themselves → `test`.
- Reviewing already-written code for defects → `review`.
- One known boundary case needs a quick fix → `fix`.

## The 12 dimensions
1. Inputs: empty, null, huge, malformed, unicode/emoji, adversarial.
2. Scale: 1 item, many items, pagination limits, unbounded growth.
3. Concurrency: duplicates, races, retries, idempotency, ordering.
4. Failure: dependency down, timeout, partial failure, recovery paths.
5. Security: unauthenticated, wrong tenant, privilege escalation attempts,
   oversized or hostile payloads.
6. Locale: RTL text, CJK, date/number formats, timezone edges, translations
   missing.
7. Accessibility: screen-reader flow, keyboard-only, high contrast, long
   labels.
8. Time: DST shifts, leap days, clock skew, end-of-month, long-running jobs
   crossing midnight.
9. Network: offline, slow, flaky, huge latency, interrupted uploads.
10. Data: migration from old shapes, mixed-version rows, orphaned
    references, schema drift.
11. Integration: counterpart API changes, version skew, contract breaks,
    sandbox vs production differences.
12. Human: fat-finger duplicates, back-button after submit, abandoned
    flows, permission changes mid-session.

## Workflow
1. Read the target (`read_file`, or the plan text) and restate its contract:
   expected inputs, outputs, and side effects.
2. Sweep the dimensions in order; for each, write 2-4 scenarios phrased as
   concrete events with an expected behavior ("retry after timeout must not
   double-charge"). Skip dimensions with a one-line justification.
3. Do one expansion pass: for each scenario ask "and what if that also
   fails?", merging results rather than exploding the list.
4. Score each scenario: severity if unhandled (1-5) x likelihood (1-5).
5. Emit the matrix sorted by score; mark the top 5 as must-test now.

## Output
A test-scenario matrix: ID, dimension, scenario, expected behavior, severity,
likelihood, priority. Plus the list of skipped dimensions and why.

## Routing
- Turning the matrix into real tests → `test`.
- Scenarios expose design gaps → back to `plan` or `architect`.
- Formal adversarial security deep-dive on a plan → `predict`.

## Guardrails
- "Invalid input causes an error" is not a scenario; state the input, the
  path, and the expected outcome.
- Describe dangerous classes abstractly (e.g. destructive database
  operations) without writing destructive commands.
- Keep the matrix bounded; prefer 40 sharp scenarios over 200 derivatives.
