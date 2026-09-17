---
name: show-off
description: >-
  Create a self-contained showcase HTML page presenting a result, project, or
  product attractively: hero, feature grid, live demo embeds, screenshots, bilingual
  copy, theme toggle — one portable file. Use when presenting shipped work, demos,
  or release highlights. Keywords: showcase, demo page, hero, feature grid,
  presentation, portfolio, one-file, landing, trang demo, trang giới thiệu, showcase
  sản phẩm. Dùng khi cần một trang HTML giới thiệu kết quả hoặc sản phẩm.
license: MIT
version: 1
---

# Show Off

Build one portable HTML page that presents a result or product attractively: a hero
that lands the value in five seconds, proof, features, and demos — all inline.

## When to use
- Presenting a shipped feature, project, or release to stakeholders or users.
- Packaging a demo or hackathon result into a shareable single file.
- Turning raw screenshots and notes into a polished presentation page.

## When NOT to use
- A general marketing site with multiple pages — use frontend-design.
- The result is a video — build it with html-video, then embed or link it here.
- Documentation or a README — use docs-style output instead.

## Workflow
1. Gather content: read the files, notes, or reports the user points to; extract
   what was built, key outcomes and metrics, screenshots, and links.
2. Define the story structure: hero (title, one-line value, primary CTA), proof
   strip (3-4 metrics), feature grid (3-6 cards with one-line benefits), screenshot
   or demo section, footer with links.
3. Pick a visual direction that matches the product's personality: one accent
   color, a strong display font, generous whitespace. Deliberate beats decorated.
4. Build a single portable HTML file: inline CSS/JS; embed small images as base64
   or inline SVG; link larger assets relatively so the folder stays portable.
5. Add live demo elements that survive offline: code snippets with copy buttons,
   inline widgets, or before/after sliders — the core story must not depend on a
   network service.
6. Support bilingual copy: EN + VI toggle with both texts stored in the file
   (data attributes or a JSON block); persist the choice.
7. Add a light/dark theme toggle defaulting to `prefers-color-scheme`, implemented
   with CSS tokens.
8. Verify responsive behavior from 360px to desktop and `prefers-reduced-motion`;
   screenshot in both themes and languages.
9. Deliver the file path plus suggested usage: open locally, attach to a release,
   or share directly.

## Output
- One self-contained `showcase.html` (or similarly named) file: bilingual,
  theme-toggled, mobile-ready, opening cleanly from the file system.
- A short list of the claims shown, so the user can verify accuracy.

## Routing
- Visual design foundations: frontend-design; component styling: ui-styling.
- Pre-share audit: web-design-guidelines.
- A launch video instead: html-video or remotion.

## Guardrails
- The file must truly be portable: no build step, no CDN requirement for core
  content, works from a double-clicked local file.
- Never embed secrets, internal URLs, or private data in the page.
- Honest claims only — never invent metrics or quotes.
- Keep total file size sane (embed small assets, link big ones).
