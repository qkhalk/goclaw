---
name: frontend-development
description: >-
  Build production-quality frontend features: semantic HTML, accessible components,
  state management patterns, performance budgets (code splitting, lazy loading), and
  complete error/loading/empty states with form validation UX. Use when implementing
  frontend features, fixing UI bugs, or reviewing frontend code quality. Keywords:
  React, accessibility, a11y, performance, forms, validation, state, component,
  phát triển frontend, tối ưu hiệu năng, biểu mẫu, sửa lỗi UI. Dùng khi cần implement,
  sửa lỗi hoặc review code frontend chất lượng production.
license: MIT
version: 1
---

# Frontend Development

Engineer frontend features that are semantic, accessible, fast, and complete: every
async view handles loading, error, and empty; every form respects the user.

## When to use
- Implementing a new page, feature, or component in an existing frontend codebase.
- Fixing UI bugs: broken layouts, stale state, unhandled failures.
- Reviewing or hardening frontend code quality before ship.
- Adding form flows with validation and good submit UX.

## When NOT to use
- Pure visual design of a new look — use frontend-design.
- Component styling/theming within a design system — use ui-styling.
- Deep React performance tuning as the main goal — use react-best-practices.

## Workflow
1. Read the surrounding code first (list_files, read_file) and match existing stack,
   patterns, and conventions instead of introducing a parallel style.
2. Write semantic HTML: real `button`/`a`/`input` elements, landmark structure
   (`nav`, `main`, `section`), and a logical heading order.
3. Make it accessible: label every input, alt text on images, visible focus styles,
   keyboard operability, ARIA only where semantics are insufficient.
4. Model state deliberately: keep URL params as source of truth where navigation
   matters, colocate local state, avoid mirroring server data into local copies.
5. Implement all data states: loading (skeleton over spinner when layout is known),
   error (with a retry action), empty (with the next action for the user).
6. Build forms with care: validate on blur/submit (not per keystroke), show inline
   errors tied to inputs, preserve user input, disable submit only while pending,
   confirm success explicitly.
7. Respect performance budgets: code-split routes and heavy components, lazy-load
   below-fold media, fetch independent data in parallel (no request waterfalls).
8. Cover mobile: `h-dvh` instead of `h-screen`, 16px inputs, 44px touch targets,
   tables inside horizontal-scroll wrappers.
9. Verify: run the app (exec), exercise the flows, check the console for errors, and
   run lint/tests when the project has them.

## Output
- Working feature code integrated into the existing codebase, with loading/error/
  empty states implemented and responsive behavior verified.
- A summary of state decisions, validation rules, and any performance trade-offs.

## Routing
- Styling and theme tokens: ui-styling. Visual redesign: frontend-design.
- Accessibility/quality audit: web-design-guidelines or ui-ux-pro-max.
- React-specific performance deep-dive: react-best-practices.
- Automated coverage for the feature: test.

## Guardrails
- No new heavy dependencies without asking; prefer what the project already has.
- Do not change unrelated behavior while implementing or fixing.
- Never put secrets or tokens in client-side code.
- An async view without error and empty states is not done.
