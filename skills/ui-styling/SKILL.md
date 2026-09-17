---
name: ui-styling
description: >-
  Style applications with shadcn/ui + Tailwind CSS: design tokens, theme variables,
  component composition on Radix primitives, responsive utilities, dark mode theming,
  and extend-vs-customize decisions. Use when theming an app, building pages from
  existing components, or customizing component looks. Keywords: shadcn, Tailwind,
  design tokens, CSS variables, Radix, dark mode, theme, variant, styling, giao diện,
  chủ đề, màu sắc, tùy biến component. Dùng khi cần style, theme hoặc tùy biến
  component với shadcn/ui và Tailwind CSS.
license: MIT
version: 1
---

# UI Styling

Theme and style apps built on shadcn/ui + Tailwind CSS: tokens in one place,
components composed from Radix primitives, customizations that survive upgrades.

## When to use
- Theming an app: colors, radius, fonts, dark mode via design tokens.
- Composing pages from existing shadcn/ui components.
- Deciding whether to extend a component (variants) or build a new one.
- Fixing styling bugs: broken responsive layout, theme leaks, portal issues.

## When NOT to use
- Greenfield visual design with no component library — use frontend-design.
- General accessibility or code-quality audits — use web-design-guidelines.
- Project-specific mobile rule checklists — use ui-ux-pro-max.

## Workflow
1. Inspect the setup first: Tailwind config/CSS entry, existing CSS variables,
   installed shadcn components (list_files, read_file). Match what is already there.
2. Map design tokens: colors as HSL CSS variables under `:root` and `.dark`, plus
   radius, fonts, and spacing — defined once, consumed via Tailwind theme mapping.
3. Compose from primitives: build with existing shadcn components (Dialog, Popover,
   Select, Tabs) before writing new widgets; never fight Radix behavior with manual
   event code.
4. Decide extend vs customize: small look-and-feel differences become variants via
   cva/`cn` merge on the component; a new component only when composition truly
   fails. Record the decision.
5. Write responsive utilities mobile-first: `grid-cols-1 sm:grid-cols-2 lg:...`
   patterns; tables wrapped in `overflow-x-auto` with a min-width.
6. Theme dark mode through tokens only; support `prefers-color-scheme` plus class
   toggle; verify every surface in both themes.
7. Respect Radix pitfalls: custom dropdowns portaled into a Dialog need
   `pointer-events-auto`; dialogs go full-screen on mobile, centered on desktop.
8. Verify at 360px width and in both themes (screenshot via browser when possible).

## Output
- Styled components/pages that consume tokens only, work in light and dark themes,
  and are mobile-safe from 360px up.
- Notes on which components were extended vs newly created, and why.

## Routing
- Fresh visual direction or landing pages: frontend-design.
- Audit of the result: web-design-guidelines; in-house mobile checklist: ui-ux-pro-max.
- Feature implementation beyond styling: frontend-development.

## Guardrails
- No hard-coded hex/rgb inside components — route everything through tokens.
- No `!important` wars; fix specificity and composition instead.
- Preserve Radix accessibility behavior; do not bypass focus management.
- Keep dialogs full-screen on mobile and centered with zoom on desktop.
