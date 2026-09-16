---
name: stitch
description: >-
  Generate UI designs with Google Stitch AI from text prompts, export the result to
  HTML/Tailwind, and fall back to manual design when quota is exhausted. Use when
  needing quick high-fidelity mockups and design starting points before coding a UI.
  Keywords: Google Stitch, AI UI design, mockup, design to code, HTML export,
  Tailwind, wireframe, prototype, thiết kế UI bằng AI, mockup giao diện, tạo thiết
  kế. Dùng khi cần tạo nhanh mockup giao diện bằng Google Stitch AI.
license: MIT
version: 1
---

# Stitch

Generate UI designs with Google Stitch AI from well-crafted prompts, export them to
HTML/Tailwind, and degrade gracefully to manual design when quota runs out.

## When to use
- Quick high-fidelity mockups for a screen before committing to code.
- Exploring 2-3 visual directions for a UI before choosing one.
- Getting an HTML/Tailwind starting point that will be reworked into a real app.

## When NOT to use
- The user already has a design system and wants implementation — use ui-styling.
- Quota is known-exhausted and no sign-in is available — go straight to
  frontend-design with ui-ux-pro-max rules.
- Pixel-precise brand work where AI output would need full redraw anyway.

## Workflow
1. Check access first: open Stitch (stitch.withgoogle.com) via web_browse with the
   user's authenticated browser session; confirm sign-in and remaining generation
   quota before spending any.
2. Draft the prompt before generating: screen type, audience, style direction
   (palette, mood, references), key sections, and named components with layout
   hints. Specific prompts beat vague ones — never burn a generation on "a nice
   dashboard".
3. Generate, then screenshot the rendered design and critique it against the brief.
4. Iterate inside quota: change one axis per regeneration (layout, style, or
   content) instead of re-rolling everything.
5. Export: request the HTML/Tailwind export from Stitch and save it into the
   project with write_file.
6. Post-process the export: replace placeholder copy and images, fix responsive
   behavior, and map its styles onto the project's design tokens.
7. Fallback when quota is exhausted, sign-in fails, or Stitch is unreachable:
   design manually using frontend-design principles (typography, hierarchy,
   decorative layers, dark mode) and the ui-ux-pro-max mobile rules, matching the
   structure you would have requested from Stitch.
8. Deliver with a one-line note on which path produced the result and why.

## Output
- Either a cleaned-up Stitch HTML/Tailwind export or a manually built equivalent,
  plus a note on quota status and the prompts used (for reproducibility).

## Routing
- Manual design and polish: frontend-design; mobile rules: ui-ux-pro-max.
- Turning the mockup into a real themed app: ui-styling, frontend-development.
- Pre-ship audit of the result: web-design-guidelines.

## Guardrails
- Verify quota and sign-in before the first generation — never assume access.
- Treat exports as drafts: they are starting points, not production code.
- No credential handling: use the user's existing browser session only.
- Keep the prompts; re-generation without the original prompt wastes quota.
