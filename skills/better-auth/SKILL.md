---
name: better-auth
description: >-
  Implement authentication with Better Auth: email/password, OAuth, magic links, session
  management, plugins such as 2FA and organizations, database adapters, and migration from
  other auth systems. Use when adding or replacing auth in a TypeScript app. Keywords:
  Better Auth, OAuth, sessions, 2FA, organizations, magic link. Dùng khi thêm đăng nhập,
  xác thực, OAuth, hai lớp, quản lý phiên, hoặc chuyển sang Better Auth.
license: MIT
version: 1
---

# Better Auth

Add or migrate authentication in TypeScript apps with Better Auth: credential and social login, sessions, plugins, database adapters, and a safe migration path off legacy systems.

## When to use
- Adding email/password, OAuth, or magic-link sign-in to a TS/JS app
- Replacing an in-house or legacy auth system (e.g., an old session table)
- Needing 2FA, organizations/teams, or admin features via plugins
- Choosing how users and sessions persist (SQL adapters: PostgreSQL, MySQL, SQLite)

## When NOT to use
- Auth design for the goclaw gateway itself → follow repo conventions, not this skill
- Deep auth architecture (token rotation policy, SSO federation) → `backend-development`, `security-audit`
- Runtimes without a Better Auth server/client story

## Workflow
1. Instantiate the server with the matching database adapter; point it at the existing users table or let it own its schema — decide before first run because migrations follow.
2. Configure credential login: email/password with minimum-length and breach checks enabled; add user fields via `additionalFields` rather than side tables.
3. Add social providers: register OAuth apps, set redirect callbacks per environment, and never commit client secrets — read them from env.
4. Pick the session strategy: cookie-based sessions with secure/sameSite attributes; set expiry and refresh windows; expose only non-sensitive session fields to the client.
5. Layer plugins only when needed: two-factor (TOTP first), organization (teams, roles, invitations), magic link; each adds tables and client methods — re-run its migration step.
6. Wire the client: mount the auth client, derive session state in the app shell, and gate routes/components off that state; handle loading and error states.
7. Migrating from another system: map legacy password hashes with a rehash-on-login shim, migrate users and sessions preserving IDs, and keep the old validator active until the cutover checklist completes.
8. Test the full matrix: each provider, expired/invalid sessions, 2FA enrollment and recovery codes, and permission checks for organization roles.

## Output
- Working auth configuration (server + client), the exact schema/migrations applied, the env var list (names only), and a migration/cutover checklist when applicable.

## Routing
- Threat-model or security review of the flow → `security-audit`
- Session/storage backend design → `backend-development`, `databases`
- Framework integration details → `web-frameworks` (Next.js) or `tanstack`

## Guardrails
- Secrets live in env/secret stores, never in code or committed config.
- Validate redirect/callback URLs against an allowlist to prevent open redirects.
- Log auth failures without passwords, tokens, or personal data.
- Every privileged action re-checks authorization server-side; a session is not permission.
