---
phase: 1
title: "Claude Pro/Max OAuth backend"
status: completed
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 1: Claude Pro/Max OAuth backend

## Overview

Add the `claude_oauth` provider type: PKCE login against claude.ai, paste-back callback exchange (console.anthropic.com redirect — no loopback server needed), refresh-token persistence, on-demand auto-refresh, and Anthropic-provider inference with the OAuth beta header.

## Requirements

- Functional: start login → `auth_url`; paste callback → tokens saved + provider registered per-tenant; `status`; `logout`; agent chat uses `Bearer` OAuth token + `anthropic-beta: oauth-2025-04-20`; auto-refresh when <5 min to expiry; stale-token fallback on refresh failure (same as chatgpt).
- Non-functional: no schema migration; secrets encrypted; endpoints/client_id/scope config-overridable; zero changes to `chatgpt_oauth` code paths.

## Architecture

Mirror `internal/oauth/openai.go` + `token.go` in the same `internal/oauth` package (reuse unexported `generatePKCE()` and generic `RefreshTokenSecretKey()`):

- `internal/oauth/claude.go`
  - Constants: `ClaudeAuthorizeURL = "https://claude.ai/oauth/authorize"`, `ClaudeTokenURL = "https://platform.claude.com/v1/oauth/token"`, `ClaudeClientID = "9d1c250a-e61b-44e9-96ed-4d3ce8e8c1af"`, `ClaudeScopes = "org:create_api_key user:profile user:inference"`, `ClaudeRedirectURI = "https://console.anthropic.com/oauth/code/callback"`, `ClaudeAPIBase = "https://api.anthropic.com"`, `OAuthBetaHeader = "oauth-2025-04-20"`.
  - `StartLoginClaude() *PendingClaudeLogin{AuthURL, verifier, state}` — pure URL builder, no loopback server (paste-only flow).
  - `ParseClaudeRedirectURL(raw string) (code, state string, err error)` — accepts (a) URL with `?code=&state=`, (b) URL with fragment `#code=&state=`, (c) bare `code#state` text (what the console page displays).
  - `ExchangeClaudeCode(ctx, code, verifier) (*ClaudeTokenResponse, error)` — form POST to token URL.
  - `RefreshClaudeToken(ctx, refreshToken) (*ClaudeTokenResponse, error)`.
  - `FetchClaudeAccountEmail(ctx, accessToken) string` — best-effort `/v1/users/me`, never fails the flow.
  - Token URL / client id / scopes resolved via a small `claudeOAuthConfig()` reading config overrides (see below) so tests can inject httptest servers.
- `internal/oauth/claude_token.go` — `ClaudeDBTokenSource` mirroring `DBTokenSource` (`token.go:61-471`), minus the OpenAI quota `RouteEligibility`:
  - `NewClaudeDBTokenSource(provStore, secretsStore, providerName)` + `WithTenantID` + `WithProviderMeta`.
  - `Token()`: cached until `expires_at − 5m`; loads row via `GetProviderByName`, asserts `provider_type == "claude_oauth"`; access token in `api_key`, refresh in `config_secrets` via existing generic `RefreshTokenSecretKey(name)` (`token.go:181-187`).
  - `SaveOAuthResult(ctx, resp)` — upsert `llm_providers` row `provider_type: "claude_oauth"`, `api_base = ClaudeAPIBase`, `settings{expires_at, scopes, account_email}`; rotated refresh token into secrets (pattern: `token.go:375-442`).
  - `Delete/Exists` — mirror `token.go:445-471`.
  - Refresh secret key is namespaced to avoid alias collisions with the OpenAI source: `ClaudeRefreshTokenSecretKey(name) = "oauth.claude." + name + ".refresh_token"` (the generic `RefreshTokenSecretKey` returns the legacy literal `oauth.openai-codex.refresh_token` when name is exactly `openai-codex`, token.go:183-185).
- `internal/providers/anthropic.go` — OAuth inference support:
  - New option `WithAnthropicTokenSource(ts providers.TokenSource)` (`TokenSource` interface: `internal/providers/types.go:31-33`).
  - When `tokenSource != nil`: request building uses `Authorization: Bearer <token>` instead of `x-api-key` (x-api-key set at anthropic.go:198), and `middlewareConfig()` (anthropic.go:126-135) returns `AuthType: "oauth"` — REQUIRED: the `service_tier`/fast-mode middlewares only skip injection when `AuthType == "oauth"` (middleware_service_tier.go:34,74) and Anthropic currently hardcodes `"api_key"`; only Codex sets oauth today (codex.go:128).
  - `anthropic-beta` header MERGE: `doRequest` already conditionally sets `anthropic-beta: interleaved-thinking-2025-05-14` (anthropic.go:201-206); the OAuth beta must be comma-merged with it, never overwriting or overwritten.
  - New constructor `NewClaudeOAuthProvider(name string, ts providers.TokenSource, apiBase, model string, registry ModelRegistry) *AnthropicProvider` composing the options.
- Provider plumbing:
  - `internal/store/provider_store.go`: `ProviderClaudeOAuth = "claude_oauth"` + `ValidProviderTypes` entry.
  - `cmd/gateway_providers.go`: `case store.ProviderClaudeOAuth:` → `NewClaudeDBTokenSource(...).WithTenantID(...)` → `NewClaudeOAuthProvider(p.Name, ts, p.APIBase, defaultModel, modelReg)` (place after the `ProviderChatGPTOAuth` case at `cmd/gateway_providers.go:403-410`; the empty-`api_key` guard sits BEFORE the switch — OAuth rows always carry an access token, matching chatgpt behavior).
  - `internal/http/providers.go`: identical case for tenant-scoped runtime registration (mirror `internal/http/providers.go:300-307`).
- Routes — `internal/http/oauth_claude.go` (new handler struct, do NOT rework the chatgpt one):
  - `GET  /v1/auth/claude/{provider}/status`
  - `POST /v1/auth/claude/{provider}/start` — body `{display_name}`; `already_authenticated` when `Exists && Token()` ok; per-tenant pending map keyed `tenant:user:provider` (reuse the `oauthFlowKey` pattern, `oauth.go:104-106`); NO 409 port conflict (no loopback); pending entries carry a 10-minute expiry checked lazily on access (no `waitForCallback` goroutine to clean up, so eviction must be explicit).
  - `POST /v1/auth/claude/{provider}/callback` — body `{redirect_url}`; `ParseClaudeRedirectURL` → state check → exchange → `SaveOAuthResult` → register provider for tenant → `{authenticated, provider_name, provider_id}`.
  - `POST /v1/auth/claude/{provider}/logout` — `Delete` + `UnregisterForTenant` + audit event.
  - Reuse existing helpers: `readAuth`, `writeOAuthProviderConflict` (`internal/http/oauth.go:131-138`).
- Config overrides (`internal/config/config_channels.go` + `config_load.go`): `ProvidersConfig.ClaudeOAuth {ClientID, AuthorizeURL, TokenURL, APIBase, Scopes}` with env overlay `GOCLAW_CLAUDE_OAUTH_CLIENT_ID` etc.; thread into `internal/oauth` via an options struct so empty = defaults above. (Anthropic has rotated these before — overrides are the escape hatch.)

## Related Code Files

- Create: `internal/oauth/claude.go`, `internal/oauth/claude_token.go`, `internal/oauth/claude_test.go`, `internal/oauth/claude_token_test.go`, `internal/http/oauth_claude.go`, `internal/http/oauth_claude_test.go`
- Modify: `internal/store/provider_store.go`, `internal/providers/anthropic.go`, `internal/providers/middleware_service_tier.go` (only if the strip doesn't cover the TokenSource path), `internal/config/config_channels.go`, `internal/config/config_load.go`, `cmd/gateway_providers.go`, `internal/http/providers.go`, `internal/http/oauth.go` (route mounting only), `internal/gateway/server.go` (handler wiring, mirror `SetOAuthHandler` at `server.go:749-750`)
- Delete: none

## Implementation Steps

1. `claude.go`: constants + `claudeOAuthConfig` overrides + `StartLoginClaude` + `ParseClaudeRedirectURL` (table-driven tests: query form, fragment form, raw `code#state`, `%23`-encoded `#`, trailing whitespace, wrong-state rejection; `strings.TrimSpace` the raw paste first).
2. `claude.go`: `ExchangeClaudeCode` + `RefreshClaudeToken` + `FetchClaudeAccountEmail` (httptest fake token server via override).
3. `claude_token.go`: `ClaudeDBTokenSource` save/refresh/delete + expiry-margin caching (sqlite in-memory store tests; race-test concurrent `Token()` with a fake refresh server — same style as `token_test.go`).
4. `anthropic.go`: `WithAnthropicTokenSource` + `middlewareConfig()` AuthType flip + Bearer/beta header path + `NewClaudeOAuthProvider`; extend anthropic provider tests with an OAuth-transport case asserting `Authorization` present, `x-api-key` absent, `service_tier` absent, and — with thinking enabled — `anthropic-beta` contains BOTH `oauth-2025-04-20` and `interleaved-thinking-2025-05-14`.
5. Plumbing: store constant, `NoEmbeddingTypes` += `claude_oauth` (provider_store.go:351-357), config overrides + env, `cmd/gateway_providers.go` + `internal/http/providers.go` cases.
6. `oauth_claude.go` routes + handler tests (start returns `auth_url` containing client_id + PKCE challenge; callback happy path with fake token server; logout; 409 on name taken by another provider type).
7. Wire handler into gateway server routing.

## Todo

- [x] Steps 1-7 above, in order
- [x] `go build ./...` + `go vet ./internal/... ./cmd/...` green
- [x] `go test ./internal/oauth/... ./internal/http/... -run "Claude" -race` green
- [x] `go build -tags sqliteonly ./...` green

## Success Criteria

All checkboxes above; manual curl smoke against a dev gateway: start → real claude.ai authorize URL opens → login → paste → `authenticated:true` → provider row `provider_type=claude_oauth` exists with settings expiry; a chat completion against a `claude-*` model works.

## Risk Assessment

- **Anthropic rotates client_id/endpoints or blocks the public client.** Signal: 400/401 from authorize/token on a previously working setup. Response: users set `GOCLAW_CLAUDE_OAUTH_CLIENT_ID` / `*_TOKEN_URL` overrides — verify override plumbing in tests, document in `docs/`. The `platform.claude.com/v1/oauth/token` URL (vs the older `console.anthropic.com/v1/oauth/token`) is config-overridable; curl-smoke both at implementation time before hardcoding.
- **Paste format drift** (console page changes how the code is displayed). Signal: callback parse failures in the wild. Response: parser already accepts 3 shapes; extend behind the same function.
- **`service_tier` regression on API-key providers** when touching middleware. Mitigation: existing middleware tests must stay green; OAuth-only gating asserted by the new transport test.
