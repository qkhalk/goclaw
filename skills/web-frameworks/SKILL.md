---
name: web-frameworks
description: >-
  Build and review Next.js App Router apps and Turborepo monorepos: server components,
  data fetching, caching, shared UI packages, and CI. Use when creating or restructuring
  React framework apps or monorepo layouts. Keywords: Next.js, App Router, Turborepo,
  RSC, monorepo, caching. Dùng khi làm việc với Next.js, React framework, monorepo,
  server components, hoặc cấu trúc dự án frontend.
license: MIT
version: 1
---

# Web Frameworks

Build and review Next.js App Router applications and Turborepo monorepos with correct server/client boundaries, data fetching, caching, and shared packages.

## When to use
- Starting a Next.js App Router app or migrating from the pages router
- Splitting a frontend into a Turborepo monorepo with shared UI/config packages
- Debugging surprising re-renders, missing data updates, or wrong caching in App Router
- Setting up CI for a framework app (build, typecheck, lint, test, cache)

## When NOT to use
- Non-React stacks (SvelteKit, Nuxt, Angular) — apply the same ideas, not these recipes
- Heavy client-side tables/forms/caching — check `tanstack` for those libraries
- Backend API design — see `backend-development`

## Workflow
1. Map every route and mark it server or client first; push `"use client"` down to the smallest leaf (buttons, inputs), never to the page level.
2. Choose the fetching layer per route: server components fetch directly for first paint; route handlers only for webhooks or third-party callbacks; typed client wrappers for interactive views.
3. Configure caching explicitly: revalidate by tag or path after mutations; treat any `fetch` without a stated cache decision as a bug; keep per-user data uncached or user-scoped.
4. Structure the monorepo: `apps/*` for deployables, `packages/ui`, `packages/config`, `packages/db`; wire internal packages via workspace protocol and TypeScript project references.
5. Keep server-only code out of the client bundle: mark DB clients and secret readers as server-only; never import env-reading modules from client components.
6. Handle mutations with server actions or route handlers; revalidate exactly the tags touched; return typed errors the UI can render.
7. Set up CI: fast package-manager install, `turbo build/test/lint` with remote caching, typecheck as a blocking gate, preview deploy per PR.
8. Review checklist: streaming boundaries, loading/error UI per segment, image and font optimization, metadata, and no secret leakage into the client tree.

## Output
- Annotated route structure (server/client per segment), a caching and revalidation plan, monorepo layout with package boundaries, and CI configuration guidance.

## Routing
- Generic backend/API work → `backend-development`; deploying the app → `deploy`
- Complex client state, tables, forms → `tanstack`
- UI review pass → `review`; pre-merge shipping checklist → `ship`

## Guardrails
- Never place secrets in `NEXT_PUBLIC_*` variables or client components.
- Do not cache per-user data in shared caches; scope by user or bypass the cache.
- Prefer the framework's own primitives over bespoke loaders and routers.
- Keep each package's public API small; deep cross-package imports signal wrong boundaries.
