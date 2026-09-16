---
name: backend-development
description: >-
  Design and review backend APIs and services: authN/authZ, caching, queues, rate limiting,
  idempotency, and pagination with concrete trade-offs. Use when building or reviewing
  server-side endpoints, data flows, or reliability patterns. Keywords: backend, REST,
  GraphQL, auth, cache, queue, rate limit, idempotency, pagination. Dùng khi thiết kế hoặc
  review backend, API, xác thực, cache, hàng đợi, giới hạn tốc độ, phân trang.
license: MIT
version: 1
---

# Backend Development

Design and review backend services with technology-agnostic patterns for API shape, authentication, caching, async work, and reliability — stating the trade-off behind every choice.

## When to use
- Designing a new REST or GraphQL API, or reshaping existing endpoints
- Deciding where a cache belongs (client, CDN, app, DB) and how to invalidate it
- Making retries and writes safe with idempotency keys and dedupe windows
- Adding rate limiting, backpressure, or pagination to an existing service
- Reviewing backend code for missing auth checks, unsafe retries, or N+1 queries

## When NOT to use
- Schema, index, or query-performance work — see `databases`
- CI/CD, containers, monitoring — see `devops`; release mechanics — see `deploy`
- Security review of auth flows, secrets, or attack surface — see `security-audit`
- Pure frontend structure — see `web-frameworks` or `tanstack`

## Workflow
1. Fix the contract first: list resources, verbs, auth scope, status codes, and the error envelope before writing handlers.
2. Pick the style with trade-offs: REST for resource CRUD and CDN cacheability; GraphQL when clients need flexible shapes — then add query depth/complexity limits to bound cost.
3. Design authN/authZ: authenticate at the edge, authorize per resource, deny by default; derive tenant and role from the session, never from client input.
4. Make every retriable write idempotent: require a client idempotency key on order- or payment-like mutations, persist the first outcome, and replay the stored response.
5. Place caches deliberately: short TTL at the CDN for public reads, app-level cache with explicit tag invalidation for hot entities; state the staleness budget in the design.
6. Push slow or fan-out work onto a queue: at-least-once delivery, idempotent handlers, dead-letter queue, bounded retries with jittered backoff.
7. Rate limit at the boundary per key, user, and IP; return 429 with a retry hint; keep limits configurable per environment.
8. Paginate with opaque cursors for large or user-facing lists; cap page size server-side; reserve offset pagination for small admin tables.
9. Validate input at the boundary, log with correlation IDs, and return stable error codes without internal details.

## Output
- An API design or review document: contract table, auth matrix, cache/queue/limit decisions with trade-offs, and a numbered risk list with file:line references for reviews.

## Routing
- Database schema, indexes, slow queries → `databases`
- Pipeline, containers, observability → `devops`; release/rollback steps → `deploy`
- Attack-surface or secrets review → `security-audit`
- Implementing the change here → `cook` (new feature) or `fix` (bug), then `test`

## Guardrails
- Never store or log plaintext credentials, tokens, or API keys.
- Authorization is server-side only; hiding UI elements is not access control.
- Describe schema changes with a rollback plan; no destructive one-step migrations.
- Prefer proven primitives; treat novel infrastructure as a risk to justify, not a default.
