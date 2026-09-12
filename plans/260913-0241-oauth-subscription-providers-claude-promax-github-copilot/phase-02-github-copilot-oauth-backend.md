---
phase: 2
title: "GitHub Copilot OAuth backend"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 2: GitHub Copilot OAuth backend

## Overview

Add the `copilot_oauth` provider type via GitHub device flow: the UI displays a `user_code`, the user authorizes at github.com/login/device, the gateway polls to completion, validates Copilot access through `copilot_internal/user`, and registers an OpenAI-compatible provider pointing at the resolved Copilot API base.

## Requirements

- Functional: start → `{user_code, verification_uri}`; gateway-side poller finishes the flow (`authorization_pending` / `slow_down` / expiry handled); provider registered per-tenant with validated API base; `status`; `logout`; agent chat via OpenAI-compat `/chat/completions` with the GitHub token as Bearer.
- Non-functional: SSRF allowlist on the resolved API base; no token refresh loop (long-lived GitHub token) but revalidation on auth failure; secrets encrypted; no schema migration.

## Architecture

- `internal/oauth/copilot.go`:
  - Constants: `CopilotClientID = "Iv1.b507a08c87ecfe98"`, `CopilotScope = "read:user"`, `https://github.com/login/device/code`, `https://github.com/login/oauth/access_token`, verification URI from response, `CopilotAPIBaseDefault = "https://api.individual.githubcopilot.com"`, `https://api.github.com/copilot_internal/user`.
  - `StartLoginCopilot(ctx) (*PendingCopilotLogin, error)`: POST device/code → `{UserCode, VerificationURI, DeviceCode, interval, expiresAt, tokenCh, errCh}`; spawns the poll goroutine (5s default interval, `slow_down` → interval += 5s, stop at `expiresAt`).
  - `ValidateCopilotToken(ctx, ghToken) (apiBase, login string, err error)`: GET `/copilot_internal/user`, extract `endpoints.api`, enforce `isTrustedCopilotAPIHost` (exact port of OpenClaw `runtime-auth.ts`): scheme https, no userinfo/query/fragment, host == `copilot-proxy.githubusercontent.com` OR suffix `.githubcopilot.com` (GHE: `copilot-api.<tenant>` — structure kept, v1 wires github.com only), else fall back to the default base. Never follow a disallowed host (SSRF guard, log `slog.Warn("security.*")`).
  - `CopilotDBTokenSource` (mirrors the Claude one but static): `Token()` returns the stored GitHub token (no TTL math); `SaveOAuthResult` stores `provider_type: "copilot_oauth"`, `api_base` = validated base, `settings{account_login, api_base, validated_at}`.
- `internal/providers/openai_config.go` (and the OpenAI provider request builder): add `WithOpenAITokenSource(ts TokenSource)` mirroring the Anthropic option; when set, the Authorization header comes from `ts.Token()` instead of the static API key. Constructor `NewCopilotProvider(name, ts, apiBase, model)` composing the option + `WithExtraHeaders{"Copilot-Integration-Id": "copilot-developer-cli", "Editor-Version": "vs-code/1.99.0", "User-Agent": "GitHubCopilot/1.0"}` (identity header verified via OpenClaw docs; `WithExtraHeaders` already exists — Kimi Coding UA precedent).
- Plumbing: `ProviderCopilotOAuth = "copilot_oauth"` + `ValidProviderTypes`; `cmd/gateway_providers.go` + `internal/http/providers.go` cases (token source; api base from `p.APIBase` or default; default model `gpt-5`, configurable per provider row like any provider).
- Routes — `internal/http/oauth_copilot.go`:
  - `POST /v1/auth/copilot/{provider}/start` → `{user_code, verification_uri, provider_name}` + background waiter on `tokenCh` → validate → `SaveOAuthResult` → register (mirror the `waitForCallback` pattern, `internal/http/oauth.go:258-286`). One in-flight flow per `tenant:user:provider` (a second start returns the existing flow's code instead of spawning a second poller); logout cancels the poller. Poller is bounded by device-code expiry (~15 min) — no leak.
  - `GET /v1/auth/copilot/{provider}/status` — same shape as chatgpt status (UI already polls this pattern).
  - `POST /v1/auth/copilot/{provider}/logout` — delete + unregister + audit.
  - No callback route (device flow has no redirect).

## Related Code Files

- Create: `internal/oauth/copilot.go`, `internal/oauth/copilot_test.go`, `internal/oauth/copilot_token.go`, `internal/http/oauth_copilot.go`, `internal/http/oauth_copilot_test.go`
- Modify: `internal/store/provider_store.go`, `internal/providers/openai_config.go` (+ option plumbing in the OpenAI provider request builder), `cmd/gateway_providers.go`, `internal/http/providers.go`, `internal/gateway/server.go`
- Delete: none

## Implementation Steps

1. `copilot.go` device-flow state machine + unit tests with httptest fake GitHub (pending → success, `slow_down` interval math, expiry, HTTP error paths).
2. `ValidateCopilotToken` + SSRF allowlist table tests (allow `api.individual.githubcopilot.com`, `*.githubcopilot.com`, `copilot-proxy.githubusercontent.com`; reject `http://`, `localhost`, `169.254.169.254`, userinfo, non-allowlisted tenant, garbage).
3. `CopilotDBTokenSource` (static token, save/delete, settings round-trip) + tests.
4. OpenAI provider TokenSource option + transport test asserting Bearer header + Copilot headers.
5. Plumbing cases in store/config/cmd/http + `NoEmbeddingTypes` += `copilot_oauth` (provider_store.go:351-357; Copilot has no embedding endpoint).
6. `oauth_copilot.go` routes + handler tests (start returns user_code shape; status transitions unauthorized → authenticated via fake upstream; logout).

## Todo

- [ ] Steps 1-6 above, in order
- [ ] `go build ./...` + vet + `go test ./internal/oauth/... ./internal/http/... -run "Copilot" -race` green
- [ ] `go build -tags sqliteonly ./...` green

## Success Criteria

Same bar as Phase 1 with the device-flow UX: start returns a `user_code` the UI can display; after GitHub authorize the status flips to authenticated and a chat completion on the copilot provider works.

## Risk Assessment

- **GitHub restricts non-VS-Code clients.** Signal: 401/403 from `/chat/completions` or `/copilot_internal/user` with a valid device token. Response: `Copilot-Integration-Id`/`Editor-Version` already overridable via the existing provider extra-headers mechanism (same as the Kimi Coding fixed-UA precedent).
- **`endpoints.api` returns an unexpected host.** Mitigation: allowlist rejects → fall back to the default base; security log.
