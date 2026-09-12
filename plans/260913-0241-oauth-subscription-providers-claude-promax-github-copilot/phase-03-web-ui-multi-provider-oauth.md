---
phase: 3
title: "Web UI multi-provider OAuth"
status: pending
priority: P1
effort: "1d"
dependencies: [1, 2]
---

# Phase 3: Web UI multi-provider OAuth

## Overview

Generalize the Providers-page OAuth experience from chatgpt-only to three flavors, keeping the existing UX muscle memory: paste-redirect for Claude (identical to chatgpt), display-a-code device flow for Copilot.

## Requirements

- Functional: Add-Provider dialog supports `claude_oauth` + `copilot_oauth` (no api_key field, alias + display name, OAuth section); provider list + detail show auth state for all three types; setup wizard step keeps working.
- Non-functional: mobile rules (44px targets, `text-base md:text-sm` inputs, full-screen dialog) already inherited from the shared dialog; i18n keys in all 5 locales (en, vi, zh, ko, ru); no new page routes.

## Architecture

- `ui/web/src/pages/providers/provider-oauth-section.tsx` — add `flavor: "chatgpt" | "claude" | "copilot"` prop:
  - Endpoint base becomes `/v1/auth/${path}/` where path = `chatgpt|claude|copilot`.
  - `chatgpt`/`claude`: identical flow (start → `window.open(auth_url)` → poll status 2s/6min → paste-redirect fallback → callback).
  - `copilot`: start returns `{user_code, verification_uri}`; render the code in a monospace box with a copy button (≥44px touch target), button "Open GitHub" (`window.open(verification_uri)`), then poll status until authenticated; no paste input.
- `provider-form-dialog.tsx` (line ~64): `isOAuth = ["chatgpt_oauth","claude_oauth","copilot_oauth"].includes(providerType)`; alias suggestion constants `DEFAULT_CLAUDE_OAUTH_ALIAS = "claude-pro"` and `DEFAULT_COPILOT_OAUTH_ALIAS = "github-copilot"` in `constants/providers.ts` via the existing `suggestUniqueProviderAlias`.
- `constants/providers.ts`: two new PROVIDER_TYPES entries — labels "Claude Subscription (OAuth)" and "GitHub Copilot (OAuth)", empty apiBase (OAuth resolves it).
- Status hooks: `hooks/use-chatgpt-oauth-provider-statuses.ts` currently hard-filters `provider_type === "chatgpt_oauth"` (line 22) — generalize to an `OAUTH_PROVIDER_TYPES` set so list badges cover all three; the quota hook stays chatgpt-only (the `/quota` endpoint exists only there).
- `provider-detail/provider-overview-helpers.ts`: `NO_API_KEY_TYPES` += the two new types (currently `{"claude_cli","acp","chatgpt_oauth"}`, line 6) AND `NO_EMBEDDING_TYPES` += both (lines 9-14; chatgpt_oauth is in both lists — parity requires both).
- Setup wizard (`pages/setup/step-provider.tsx`, OAuthSection at line ~188): pass the flavor through.
- i18n — `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/providers.json` (nested keys, per repo convention):
  - New: `oauth.signInWithClaude`, `oauth.signInWithCopilot`, `oauth.deviceCode`, `oauth.deviceCodeHint` ("Enter this code at github.com/login/device"), `oauth.copyCode`, `oauth.waitingForGithub`.
  - Reuse existing `oauth.pasteUrlPlaceholder`, `oauth.waiting`, `oauth.starting`, etc. where identical.

## Related Code Files

- Modify: `ui/web/src/pages/providers/provider-oauth-section.tsx`, `ui/web/src/pages/providers/provider-form-dialog.tsx`, `ui/web/src/pages/providers/hooks/use-chatgpt-oauth-provider-statuses.ts`, `ui/web/src/pages/providers/provider-detail/provider-overview-helpers.ts`, `ui/web/src/pages/setup/step-provider.tsx`, `ui/web/src/constants/providers.ts`, `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/providers.json`
- Create: none
- Delete: none

## Implementation Steps

1. OAuthSection `flavor` param + claude path (endpoint swap only — parametrize, do not copy the component).
2. Copilot device-code UI block (code display + copy + open + poll-only).
3. Form dialog + constants + alias suggestions.
4. Statuses hook generalization + list/detail badges.
5. Setup wizard passthrough.
6. i18n keys ×5 locales.
7. `pnpm build` (tsc + vite) green.

## Todo

- [ ] Steps 1-7 above
- [ ] Manual UI smoke: add each of the three OAuth types; confirm chatgpt flow unchanged

## Success Criteria

All three OAuth types addable from the web UI end-to-end against a dev gateway; existing chatgpt UI flow visually unchanged; mobile viewport verified at 375px.

## Risk Assessment

- **Flavor param refactor regresses chatgpt UX.** Mitigation: parametrize endpoint paths only; keep the paste flow code path shared; manual smoke before commit.
