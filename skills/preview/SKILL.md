---
name: preview
description: >-
  Verify frontend work by actually viewing it: serve the page, screenshot with web_browse, check
  console errors, compare against the design intent, and iterate until it matches. The trust-but-
  verify loop for UI changes. Use after implementing any visual change, layout, or component.
  Keywords: screenshot, visual check, verify ui, browser preview, console errors, frontend QA,
  kiem tra giao dien, xem truoc. Dùng khi cần kiểm tra trực quan giao diện sau khi sửa UI.
license: MIT
version: 1
---

# Preview

Close the loop on UI work: look at what you built before claiming it works.

## When to use
- After implementing any visual change: components, layouts, styling, responsive behavior.
- Before handing frontend work to the user or to `review`.
- Debugging "it looks wrong" reports — reproduce visually first.

## When NOT to use
- Backend-only changes with no UI surface — run tests instead.
- The project's established end-to-end suite already covers the change — prefer running it.

## Workflow
1. **Serve:** start the dev server (or build and preview) with `exec`; note the local URL. Reuse a server that is already running.
2. **Open and capture:** use `web_browse` to load the page and take a screenshot at a desktop viewport; inspect the image.
3. **Check the console:** capture console errors and failed network requests. A pixel-perfect page with console errors is not done.
4. **Verify against intent:** compare the screenshot with the design, spec, or the user's description — spacing, hierarchy, alignment, plus empty, loading, and error states.
5. **Exercise states:** click through the interactive paths you touched — open the dialog, submit the form — and check the required responsive breakpoints (at minimum mobile width and desktop).
6. **Iterate:** fix what you saw, reload, re-screenshot. Repeat until screenshots match intent and the console is clean.
7. **Report:** state what you verified with evidence (screenshot descriptions, console status) and what remains unverified.

## Output
Visual confirmation: screenshots matching design intent, zero console errors on touched pages, and an honest list of states and breakpoints checked versus skipped.

## Routing
- The design itself is unclear or the result looks off → `ui-ux-pro-max` for design guidance, then re-preview.
- A systematic quality pass on the component code → `review`.
- Functional bugs found while clicking → `debug`.

## Guardrails
- Never claim UI work is done from code alone — unviewed UI is unverified UI.
- Do not tune screenshots to hide problems; report real rendering, including ugly cases.
- Preview against safe test data, never real user data, especially in multi-tenant apps.
