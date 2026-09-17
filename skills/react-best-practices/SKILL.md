---
name: react-best-practices
description: >-
  Apply React performance and correctness rules: avoid request waterfalls, memoize
  correctly, stable keys, list virtualization, Suspense and streaming, bundle
  hygiene, and anti-pattern fixes. Use when writing, reviewing, or optimizing React
  components. Keywords: React, hooks, useMemo, useCallback, suspense, virtualization,
  bundle, re-render, performance, React tối ưu, hiệu năng React, anti-pattern.
  Dùng khi cần tối ưu hiệu năng hoặc review code React.
license: MIT
version: 1
---

# React Best Practices

Make React apps fast and correct by rule, not guesswork: kill waterfalls, render
less, split bundles — and measure before and after every change.

## When to use
- A component tree feels slow: janky scrolling, slow mounts, delayed data.
- Reviewing React code for performance and correctness anti-patterns.
- Large lists, heavy dashboards, or data-driven views that lag.

## When NOT to use
- General feature implementation — use frontend-development.
- Styling/theming questions — use ui-styling.
- Non-React stacks — this skill assumes React 18+.

## Workflow
1. Establish the symptom and baseline first: slow mount, scroll jank, or delayed
   content; use React Profiler or timing logs. Never optimize blind.
2. Kill request waterfalls: flatten sequential awaits into parallel fetches when
   independent, prefetch on intent (hover/focus) for likely navigation, and keep
   data at the lowest common ancestor that needs it.
3. Fix re-render storms: derive state during render instead of syncing with
   effects, move state down to isolate hot regions, pass components as children to
   avoid re-rendering everything, and memoize only components with proven
   expensive renders — `memo()` fed by unstable props is pure noise.
4. Fix lists: stable ID keys (never array index for filterable/reorderable lists)
   and virtualize anything beyond roughly 200 visible rows.
5. Clean up effects: no effects for derived data, no effect chains that set state
   in sequence, exhaustive but intentional dependency arrays.
6. Add Suspense boundaries around data-driven regions with skeleton fallbacks, and
   error boundaries per region — not only at the app root.
7. Audit the bundle: whole-library imports (date, icon, chart utilities), lazy-load
   routes and heavy editors, confirm with a bundle report.
8. Sweep classic anti-patterns: components defined inside components, unstable
   context values, new object/array props into memoized children on hot paths.
9. Re-measure after changes and report before/after numbers or a reasoned
   explanation when measurement is impractical.

## Output
- Patched components plus a summary: each change, the anti-pattern it fixed, and
  measured or reasoned impact (render counts, bundle delta, waterfall removed).

## Routing
- Implementing new features while at it: frontend-development.
- Styling-level fixes: ui-styling. Full codebase review: review.
- Regression protection for the fix: test.

## Guardrails
- Never memoize blindly — measure first or state the reasoning explicitly.
- Behavior must not change while optimizing; pure performance edits only.
- Virtualization must preserve keyboard scrolling, find-in-page, and a11y.
- Keep TypeScript types accurate; no `any` shortcuts to silence prop warnings.
