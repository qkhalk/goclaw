---
name: mobile-development
description: >-
  Cross-platform mobile guidance: React Native vs Flutter vs native Swift/Kotlin decision
  matrix, navigation, offline sync, push notifications, and release pipelines. Use when
  starting, reviewing, or debugging a mobile app or choosing a mobile stack. Keywords:
  React Native, Flutter, iOS, Android, navigation, push, offline sync. Dùng khi làm app
  mobile, chọn React Native hay Flutter hay native, push notification, đồng bộ offline.
license: MIT
version: 1
---

# Mobile Development

Choose a mobile stack with a concrete decision matrix and apply the patterns that dominate mobile quality: navigation, offline sync, push notifications, and release pipelines.

## When to use
- Deciding the stack for a new mobile app or a rewrite
- Designing navigation structure, deep links, or offline-first data flow
- Adding push notifications or preparing store release pipelines
- Reviewing a mobile app for platform-specific pitfalls

## When NOT to use
- Responsive web UI rules (viewport, touch targets) — those are web concerns, not app code
- Backend APIs the app consumes → `backend-development`
- Build/deployment infrastructure → `deploy`, `devops`

## Workflow
1. Decide the stack with the matrix: team knows React/TypeScript and UI is forms-and-lists → React Native; heavy custom UI, animations, consistent brand rendering → Flutter; deep platform integration (widgets, health, AR, background execution) or single-platform long-term → native Swift/Kotlin.
2. Structure navigation as typed routes with a single source of truth; define deep-link schemes early (they become public API); avoid nested stack sprawl.
3. Design data flow offline-first where it matters: a local database as the app's source of truth, a sync engine with an explicit conflict policy (last-write-wins vs merge), and pending/failed states in the UI.
4. Implement push: platform credentials (APNs/FCM), token registration tied to the user, notification vs silent/data messages, and permission UX that asks in context.
5. Keep platform differences explicit in code (file suffixes or platform checks), never scattered runtime conditionals; test both platforms before merge.
6. Set up release pipelines: signed builds in CI, staged rollout (internal → closed → production track), crash reporting gated as a release blocker, and a rollback story (previous binary or remote feature flags).
7. Manage app-review risk: privacy/data-collection disclosure, account-deletion requirement, and review notes with a demo account.
8. Review checklist: cold-start time, list virtualization, image caching, keyboard behavior, permission-denial handling, and platform background-task limits respected.

## Output
- A stack decision note with reasons, navigation/deep-link map, offline-sync and push design, and a release checklist tailored to the target stores.

## Routing
- Backend API design for the app → `backend-development`
- CI/build infrastructure → `deploy`, `devops`
- Web companion app patterns → `web-frameworks`, `tanstack`

## Guardrails
- Never store auth tokens in plaintext storage; use the platform keystore/keychain.
- Ask for permissions in context with a rationale screen; never on cold launch.
- Do not ship unfinished code paths; gate them with remote flags instead.
- Validate deep links server-side where they grant access to data.
