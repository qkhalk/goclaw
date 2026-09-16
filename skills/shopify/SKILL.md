---
name: shopify
description: >-
  Build Shopify apps and integrations: checkout, admin, and POS extensions, GraphQL Admin
  API, Polaris UI, Liquid themes, App Bridge, and embedded app auth. Use when developing,
  reviewing, or debugging anything on the Shopify platform. Keywords: Shopify, Liquid,
  Polaris, App Bridge, GraphQL Admin API, webhooks. Dùng khi làm app Shopify, extension,
  theme Liquid, tích hợp API Shopify.
license: MIT
version: 1
---

# Shopify

Build and review Shopify solutions across the surface map: app extensions, Admin GraphQL API, Polaris UI, Liquid themes, and embedded-app authentication.

## When to use
- Creating or changing an app: admin, checkout, or POS extensions; embedded admin app
- Querying or mutating shop data through the GraphQL Admin API
- Customizing storefront themes with Liquid, sections, and settings
- Debugging OAuth/token flows, webhook payloads, or App Bridge behavior

## When NOT to use
- Generic e-commerce backend questions unrelated to the platform → `backend-development`
- Payment reconciliation and gateway logic details → `payment-integration`
- Storefront scraping of shops without API access → `scraping` (respect ToS)

## Workflow
1. Classify the need: merchant UI (admin app/extension), checkout behavior (checkout extensions with strict component limits), storefront look (theme), or back-office automation (API + webhooks). The surface dictates the toolchain.
2. Prefer extensions over scripts: use Functions for cart/discount logic where required; check the API version release notes for deprecated fields before writing queries.
3. Handle embedded-app auth: OAuth authorization-code flow or token exchange, session tokens via App Bridge for fetches inside admin; never treat URL params alone as identity.
4. Use the GraphQL Admin API with bulk operations for large datasets (export/update thousands of objects); paginate with cursors and stay within rate-cost budgets.
5. Register webhooks for events you must react to; verify the HMAC signature before processing; handle deliveries idempotently — Shopify retries them.
6. Build admin UI with Polaris components inside App Bridge; keep deep links and resource pickers native instead of custom modals.
7. For themes: edit sections/blocks with schema settings, avoid hardcoded copy, test on a duplicate theme, and keep JSON templates under version control.
8. Test with a development store and CLI dev preview; before submission, confirm the GDPR-mandated webhooks (customer data request/delete, shop redact) are registered.
9. Review checklist: API version pinning, webhook verification, scope minimization, billing API integration, and documented extension limits.

## Output
- A working app/extension/theme change plus a note listing surfaces touched, required scopes, webhooks registered, and the API version used.

## Routing
- Payment provider logic, webhook reconciliation → `payment-integration`
- Backend service design around the app's own server → `backend-development`
- Submission/shipping checklist → `ship`; code review pass → `review`

## Guardrails
- Never request broader access scopes than the feature needs.
- Verify every webhook signature; reject unsigned or replayed deliveries.
- Do not write customer data into logs or analytics payloads.
- Test theme changes on a duplicate theme; never edit the live theme directly.
