---
name: tanstack
description: >-
  Work with the TanStack ecosystem: Start, Query, Form, Table, Router — when to choose it
  over Next.js or Remix, server functions, and type-safe data fetching. Use when building
  client-heavy React apps with caching, tables, forms, or type-safe routing. Keywords:
  TanStack, React Query, Table, Form, Router, Start. Dùng khi cần React Query, bảng dữ
  liệu, form, router type-safe, hoặc so sánh TanStack với Next.js.
license: MIT
version: 1
---

# TanStack

Choose and apply the right TanStack library — Query, Router, Table, Form, Start — with type-safe data flow, and know when a meta-framework like Next.js is the better fit.

## When to use
- Client-heavy or SPA-style React apps where a meta-framework is overkill
- Server-state caching: fetching, retries, background refresh, optimistic updates
- Complex tables (sorting, grouping, virtualization) or multi-step forms with validation
- Teams that want end-to-end type safety from route params to query payloads

## When NOT to use
- Content, SEO-critical, or mostly-static sites → a meta-framework such as Next.js
- App Router structure, caching rules, monorepo layout → `web-frameworks`
- Mobile form factors → `mobile-development`

## Workflow
1. Decide the stack first: need SSR/SEO/streaming and file conventions → Next.js or TanStack Start; pure API-driven app with light SEO needs → Vite + Router + Query.
2. Model server state with Query: stable hierarchical query keys (`['users', id, 'posts', filters]`); set `staleTime` per data class, not one global value.
3. Centralize fetchers: one typed client per backend; derive query hooks and types from the same schema source to keep end-to-end types.
4. Handle mutations with `useMutation`: optimistic updates plus rollback on error, invalidation by key after success; never mirror server state into local `useState`.
5. Add Router loaders where data must exist before render; hydrate loader results into the Query cache with the same keys so there is one source of truth.
6. Use Table headless-only: own the markup and styling; add virtualization beyond a few hundred rows; keep column definitions typed and co-located.
7. Use Form for validation: schema-driven (Zod/Valibot), field errors from the same schema the API validates with; submit via mutations.
8. In Start, put server-only logic in server functions; treat them like public endpoints — validate and authorize inputs; keep secrets out of bundled client code.
9. Test the data layer with a Query-aware test wrapper and mocked fetchers; test tables and forms via user events, not snapshot tests.

## Output
- A stack decision note (or confirmation), a query-key map, caching/invalidation plan, and typed hook/table/form scaffolding applied to the codebase.

## Routing
- Next.js App Router structure, caching, monorepos → `web-frameworks`
- API contract or backend reliability questions → `backend-development`
- Review pass on the implementation → `review`; pre-merge checklist → `ship`

## Guardrails
- One cache only: never duplicate server state into stores or component state.
- Do not swallow mutation errors; surface them and roll back optimistic writes.
- Keep `staleTime`/refetch policy per data class; global refetch storms are a bug.
- Authorize inside server functions; client route guards are UX, not security.
