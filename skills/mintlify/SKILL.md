---
name: mintlify
description: >-
  Document products with the Mintlify framework: MDX page structure, docs.json
  navigation config, API playground blocks, components, and deployment — plus
  judgment on when Mintlify fits versus plain markdown docs. Use when building
  or migrating hosted product documentation with an API reference. Keywords:
  mintlify docs, mdx, docs.json, api playground, documentation site. Dùng khi
  cần viết tài liệu sản phẩm bằng Mintlify, cấu hình điều hướng, hoặc tích
  hợp API playground.
license: MIT
version: 1
---

# Mintlify

Build a hosted docs site where navigation, MDX components, and an API
playground are configured in code — or say plainly when plain markdown would
serve better.

## When to use
- Product/user-facing documentation hosted as a polished site.
- An API reference with try-it-out playground is wanted.
- Migrating existing docs into (or auditing) a Mintlify project.

## When NOT to use
- Docs that live in the repo for contributors (README, CONTRIBUTING) —
  plain markdown; use `docs` instead.
- Looking up a library's official documentation → `docs-seeker`.
- Visual design of the product itself → `ui-ux-pro-max`.

## Key structures
- `docs.json` at the project root: `$schema`, site `name`, theme settings,
  `navigation` with `tabs` → `anchors`/`groups` → `pages`. The nav IS the
  information architecture; every page must be listed or it will not build.
- Pages are MDX with frontmatter: `title`, `description` (feeds SEO and
  previews), optional `icon`, `sidebar` position.
- Components: `Card`/`CardGroup` for entry points, `Steps` for procedures,
  `Tabs` for language variants, `Accordion` for FAQs, `Callout` for
  warnings, `CodeGroup` for multi-language snippets, `ParamField` /
  `ResponseField` for API params.
- API playground: point the `openapi` field in `docs.json` at an OpenAPI
  spec (file path or URL); reference endpoints in pages to get runnable
  requests.

## Workflow
1. Assess fit: if the docs must be hosted, styled, searchable, and include
   an API playground, proceed; otherwise recommend plain markdown and stop.
2. Design the navigation tree first (groups mirror user journeys, not the
   org chart); write it into `docs.json`.
3. Write pages as MDX: one task or concept per page, frontmatter complete,
   components used for structure rather than raw HTML.
4. Wire the API reference: validate the OpenAPI spec, add the `openapi`
   field, and create endpoint pages.
5. Preview locally with the project's Mintlify CLI and fix broken links and
   unlisted pages; broken MDX or missing nav entries fail the build.
6. Deploy via the Git-connected host (push to the tracked branch) or export
   a static build; confirm the live nav matches step 2.

## Output
A building Mintlify project: `docs.json` nav tree, MDX pages, configured
playground (if applicable), and a preview/deploy note.

## Routing
- General writing quality and repo docs → `docs`.
- API endpoint details need verification → `docs-seeker`.
- Docs IA is part of a broader design effort → `design`.

## Guardrails
- Never hand-write the OpenAPI-derived pages when a spec exists; the spec is
  the source of truth.
- Keep custom MDX components minimal; exotic components break on framework
  upgrades.
- Validate every internal link and nav entry before declaring done — silent
  404s are the default failure mode.
