---
name: frontend-design
description: >-
  Design and build beautiful, professionally designed responsive web interfaces: layout
  systems, visual hierarchy, typography, micro-animations, gradients, glassmorphism,
  noise textures, dark mode. Use when building landing pages, marketing sites,
  dashboards, hero sections, portfolios, or any UI that must not look AI-generated.
  Keywords: frontend design, CSS, HTML, responsive, animation, typography, dark mode,
  UI polish, thiết kế giao diện, giao diện đẹp, landing page, trang marketing, hiệu ứng.
  Dùng khi cần tạo hoặc nâng cấp giao diện web đẹp, chuyên nghiệp, chuẩn responsive.
license: MIT
version: 1
---

# Frontend Design

Design and build web interfaces that look professionally designed, not generated: strong
visual hierarchy, deliberate typography, and tasteful decoration in one deliverable.

## When to use
- Building a landing page, marketing site, hero section, or portfolio from scratch.
- Restyling an existing page that looks generic, flat, or machine-made.
- Creating a single-file HTML demo or standalone component with high visual polish.
- Adding dark mode, micro-animations, or decorative depth (gradients, glass, noise).

## When NOT to use
- shadcn/ui + Tailwind theming inside an existing design system — use ui-styling.
- Accessibility or code-quality compliance is the goal — use web-design-guidelines.
- Backend logic, APIs, or data work dominates — use cook or a backend skill.

## Workflow
1. Clarify the deliverable: single-file HTML or framework component? Audience, brand,
   references, tone. If unspecified, pick one bold aesthetic direction and state it.
2. Choose a distinct style, not a default: type-driven, editorial, luxury/refined,
   playful, or brutalist. Write a 3-line design rationale before any code.
3. Set foundations: font pairing (display + body), a 5-7 color palette as CSS custom
   properties, a spacing scale (4/8px), and a consistent radius system.
4. Design the layout: max-width container, grid or flex composition, mobile-first
   breakpoints. Establish one dominant focal element per viewport.
5. Layer decoration deliberately: mesh or linear gradients, noise via inline SVG
   data-URI, glass panels (backdrop-filter with fallback), hairline borders, layered
   shadows. Decoration supports hierarchy — it never competes with content.
6. Add motion: hover/focus micro-interactions (150-250ms ease-out), scroll reveals via
   IntersectionObserver, and one signature animation. Always gate on
   `prefers-reduced-motion`.
7. Implement dark mode with tokens (`prefers-color-scheme` plus optional toggle) —
   never hard-coded colors scattered through rules.
8. Write files with write_file, then verify: open the page via web_browse or exec,
   capture screenshots, and read_image to critique your own output.
9. Polish pass: check contrast, alignment rhythm, empty states, and that every
   interactive element has hover, focus, and active states.

## Output
- One self-contained HTML file (inline CSS/JS) or a component file, responsive from
  360px to desktop, with dark mode and reduced-motion support.
- A short summary of the chosen aesthetic direction and key design decisions.

## Routing
- Component libraries and theme tokens inside an app: hand off to ui-styling.
- Accessibility/quality audit of the result: web-design-guidelines or ui-ux-pro-max.
- A showcase page presenting shipped work: show-off.
- Effects beyond CSS: shader or threejs.

## Guardrails
- Never claim pixel-perfection from code alone — visually verify via screenshot.
- No heavy runtime dependencies by default; vanilla CSS/JS unless the user asks.
- Use placeholder gradients/SVG for imagery unless real assets are provided.
- Keep reduced-motion, keyboard focus, and 16px minimum input font size intact.
