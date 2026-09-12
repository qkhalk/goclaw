---
title: "OAuth subscription providers: Claude Pro/Max + GitHub Copilot"
description: "Add two OpenClaw-parity OAuth subscription providers (Claude Pro/Max and GitHub Copilot) following the proven chatgpt_oauth pattern: web-UI login, DB-backed auto-refresh, multi-tenant."
status: completed
priority: P1
effort: "4d"
tags: [providers, oauth, subscriptions]
created: 2026-09-13
---

# OAuth subscription providers: Claude Pro/Max + GitHub Copilot

## Overview

GoClaw has exactly one OAuth subscription provider today: `chatgpt_oauth` (OpenAI Codex PKCE, paste-redirect, `DBTokenSource` auto-refresh). OpenClaw goes further: GitHub Copilot via device flow and Claude subscription reuse. This plan adds two new provider types — `claude_oauth` (Claude Pro/Max, PKCE + paste-back) and `copilot_oauth` (GitHub Copilot, device flow) — reusing the existing OAuth architecture end-to-end: access token in `llm_providers.api_key`, refresh token in `config_secrets` (`oauth.<provider>.refresh_token`), expiry + metadata in `llm_providers.settings` JSONB, on-demand refresh with a 5-minute margin, and the existing Providers-page OAuth section UX.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | `claude_oauth` provider type: login at claude.ai from the web UI, paste-back callback, auto-refresh, chat via Anthropic Messages API with the OAuth beta header | P1 |
| 2 | `copilot_oauth` provider type: GitHub device flow (user_code shown in UI), token validated against `copilot_internal/user`, chat via OpenAI-compat Copilot API | P1 |
| 3 | Zero regression to `chatgpt_oauth`: existing flows, tests, and the single-flow loopback limitation stay untouched | P1 |
| 4 | Security parity: refresh tokens AES-256-GCM in `config_secrets`, state CSRF checks, SSRF allowlist for Copilot endpoint resolution, secrets never logged | P0 |
| 5 | Surface parity: web UI (5 locales), CLI `goclaw auth status\|logout`, docs, CHANGELOG | P2 |

## Verified external endpoints (ground truth, cross-checked Sep 2026)

### Claude Pro/Max OAuth (public PKCE client, no client secret)

| What | Value |
|------|-------|
| Authorize | `https://claude.ai/oauth/authorize?code=true&client_id=<cid>&response_type=code&redirect_uri=https://console.anthropic.com/oauth/code/callback&scope=org%3Acreate_api_key+user%3Aprofile+user%3Ainference&code_challenge=<S256>&code_challenge_method=S256&state=<state>` |
| Client ID | `9d1c250a-e61b-44e9-96ed-4d3ce8e8c1af` (Claude Code public client; config-overridable) |
| Token / refresh | `POST https://platform.claude.com/v1/oauth/token` — form-encoded; grant `authorization_code` (code + code_verifier + redirect_uri + client_id) and `refresh_token` (client_id + refresh_token); response `{access_token, refresh_token, expires_in, scope}` |
| Inference | `POST https://api.anthropic.com/v1/messages` with `Authorization: Bearer <access_token>` + `anthropic-beta: oauth-2025-04-20`; do NOT send `x-api-key` or `service_tier` (Anthropic rejects `service_tier` on OAuth; NOTE: `middleware_service_tier.go` only skips injection when `AuthType == "oauth"` and `AnthropicProvider.middlewareConfig()` hardcodes `"api_key"` today — Phase 1 must wire this) |
| Account display (best-effort) | `GET https://api.anthropic.com/v1/users/me` with Bearer + `anthropic-version: 2023-06-01` → email |
| Callback shape | Console redirect page URL carries `code` and `state` in the URL fragment and/or displays raw `code#state` text — paste parser must accept full URL with `?`/`#` params AND the bare `code#state` string |

### GitHub Copilot OAuth (device flow — verified against OpenClaw source `extensions/github-copilot/login.ts` + `runtime-auth.ts`)

| What | Value |
|------|-------|
| Device code | `POST https://github.com/login/device/code` — form `client_id=Iv1.b507a08c87ecfe98&scope=read:user` → `{device_code, user_code, verification_uri, interval, expires_in}` |
| User step | Open `verification_uri` (`https://github.com/login/device`), user types `user_code` |
| Poll | `POST https://github.com/login/oauth/access_token` — `client_id + device_code + grant_type=urn:ietf:params:oauth:grant-type:device_code`; handle `authorization_pending` (wait interval), `slow_down` (interval += 5s), `expired_token`; `Accept: application/json` |
| Runtime validation | `GET https://api.github.com/copilot_internal/user` — `Authorization: Bearer <github_token>` → validates Copilot access and returns `endpoints.api` |
| API base | `endpoints.api` from above, else default `https://api.individual.githubcopilot.com`; **SSRF allowlist**: host must be `*.githubcopilot.com` or `copilot-proxy.githubusercontent.com` (GHE tenants: `copilot-api.<tenant>`), https only, no userinfo/query |
| Inference | OpenAI-compatible `POST {apiBase}/chat/completions` with `Authorization: Bearer <github_token>` (the ORIGINAL token — the retired `/v2/token` ephemeral exchange is not needed; matches OpenClaw Sept 2026 behavior) + `Copilot-Integration-Id: copilot-developer-cli` + editor-style User-Agent |
| Token lifetime | GitHub device-flow OAuth tokens are long-lived (no TTL); no refresh loop — revalidate on auth failure |

## Architecture

```
Web UI Providers page
  ├─ chatgpt_oauth   (existing; untouched)
  ├─ claude_oauth    PKCE → window.open(claude.ai/authorize) → paste console callback URL → /v1/auth/claude/{name}/callback
  └─ copilot_oauth   device flow → POST /v1/auth/copilot/{name}/start → UI shows user_code + opens github.com/login/device → backend poller completes → UI polls /status

Gateway
  internal/oauth/claude.go        PKCE + paste parse + exchange + refresh   (mirrors openai.go)
  internal/oauth/claude_token.go  ClaudeDBTokenSource (api_key + secrets + settings, on-demand refresh)
  internal/oauth/copilot.go       device flow state machine + /copilot_internal/user validation + SSRF allowlist
  internal/http/oauth_claude.go   /v1/auth/claude/{provider}/{status,start,callback,logout}
  internal/http/oauth_copilot.go  /v1/auth/copilot/{provider}/{status,start,logout}
  providers: WithAnthropicTokenSource + NewClaudeOAuthProvider; OpenAI TokenSource option for Copilot
```

Token storage is identical to `chatgpt_oauth` (no schema changes): access token in `llm_providers.api_key` (AES-256 encrypted at rest), refresh token in `config_secrets` key `oauth.<provider>.refresh_token` (PK includes tenant), metadata in `settings` JSONB (`expires_at`, `scopes`, `account_email`/`account_login`, `api_base` for Copilot).

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Claude Pro/Max OAuth backend](./phase-01-claude-oauth-backend.md) | Completed |
| 2 | [Phase 2: GitHub Copilot OAuth backend](./phase-02-github-copilot-oauth-backend.md) | Completed |
| 3 | [Phase 3: Web UI multi-provider OAuth](./phase-03-web-ui-multi-provider-oauth.md) | Completed |
| 4 | [Phase 4: Parity, docs, release](./phase-04-parity-docs-release.md) | Completed |

## Out of scope (explicit)

- OpenClaw's Claude-CLI credential reuse (`~/.claude` scraping) — GoClaw does its own OAuth login instead.
- Copilot GHE data-residency domains (the allowlist structure supports them; v1 wires github.com only).
- Proactive background token refresh scheduler (on-demand refresh with quota-driven `RouteEligibility` is the established model).
- Background token-quota polling for the new providers (chatgpt's `RouteEligibility` quota machinery stays chatgpt-only). NOT out of scope: the `AuthType: "oauth"` wiring in `AnthropicProvider.middlewareConfig()` (anthropic.go:126-135) — that is Phase 1 work, otherwise `service_tier`/fast-mode injection fires and Anthropic rejects OAuth requests.

## Open questions

None — endpoint set verified from public implementations; client_id and URLs are config-overridable in case Anthropic rotates them.

## Success Criteria

- [ ] Add Provider → "Claude Subscription (OAuth)": login in browser, paste callback, provider row created, agent completes a chat on Pro/Max quota; token auto-refreshes after expiry without re-login.
- [ ] Add Provider → "GitHub Copilot (OAuth)": UI shows user_code, GitHub authorize completes, provider row created with validated Copilot API base; agent completes a chat.
- [ ] `chatgpt_oauth` regression-free: existing unit tests pass; manual smoke of start/callback/logout unchanged.
- [ ] Refresh tokens only ever in `config_secrets` (AES-256-GCM); no token material in logs.
- [ ] `go build ./...`, `go build -tags sqliteonly ./...`, `go vet`, web `pnpm build` all green; new unit tests for paste parsing, device poller, SSRF allowlist, token source refresh.

<!-- slug: oauth-subscription-providers-claude-promax-github-copilot -->
