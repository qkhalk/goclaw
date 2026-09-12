---
phase: 1
title: "Foundation — cloud_accounts store + OAuth flow provider-agnostic"
status: completed
priority: P1
effort: "2d"
dependencies: []
---

# Phase 1: Foundation — cloud_accounts store + OAuth flow provider-agnostic

## Overview

Bảng `cloud_accounts` (dual-DB) + package `internal/cloud` (provider registry, OAuth web flow với state HMAC-signed, token source tự refresh) + HTTP endpoints `/v1/cloud/*` (start authed, callback unauthed theo pattern MCP). Chưa có UI.

## Requirements

- Functional:
  - Migration PG `000118_cloud_accounts.up.sql` + bump `RequiredSchemaVersion` 117→118 (`internal/upgrade/version.go:5`; migration hiện mới nhất là `000117_routing_rules`). SQLite: thêm bảng vào `internal/store/sqlitestore/schema.sql` + entry `migrations` map (pattern 80→81 tại `schema.go:1511-1528`) + bump `SchemaVersion` 81→82 (`schema.go:19`). **Cả hai theo luật dual-DB.**
  - Schema bảng (mô phỏng `mcp_oauth_tokens` migration 000084 — per-user + tenant, cột token mã hóa riêng lẻ):
    ```sql
    CREATE TABLE cloud_accounts (
      id                UUID PRIMARY KEY,
      tenant_id         UUID NOT NULL,
      user_id           UUID NOT NULL,
      provider          TEXT NOT NULL,              -- 'google'
      email             TEXT NOT NULL,
      display_name      TEXT NOT NULL DEFAULT '',
      scopes            TEXT NOT NULL DEFAULT '[]', -- JSON array
      access_token      TEXT NOT NULL,              -- aes-gcm encrypted
      refresh_token     TEXT NOT NULL,              -- aes-gcm encrypted
      token_expires_at  TIMESTAMPTZ,
      status            TEXT NOT NULL DEFAULT 'active', -- active|expired|revoked|error
      status_message    TEXT NOT NULL DEFAULT '',
      settings          JSONB NOT NULL DEFAULT '{}',
      created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
      updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
      UNIQUE (tenant_id, user_id, provider, email)
    );
    ```
  - `CloudAccountStore` interface (`internal/store/cloud_account_store.go`): `Create / Get / ListByUser(ctx) / UpdateTokens / UpdateStatus / Delete` — mọi query lọc `tenant_id + user_id` từ ctx (`store.WithTenantID/WithUserID` pattern sẵn `internal/store/context.go:110`). PG impl encrypt khi write/decrypt khi read (pattern `internal/store/pg/providers.go:19,37-38,212-213`); SQLite impl tương tự (`mcp_oauth_tokens.go:126-140` là precedent). Thêm field `CloudAccounts` vào `Stores` (`internal/store/stores.go:6-131`) + 2 factory.
  - Package `internal/cloud`: `provider.go` (interface `Provider`: Name/AuthURL/TokenURL/Scopes/BuildAuthRequest/Exchange — thêm provider (microsoft) sau chỉ = thêm 1 file), `google.go` (scopes: `openid email profile` + `gmail.readonly` + `gmail.labels` + `gmail.modify` + `drive.readonly`; PKCE S256; `access_type=offline&prompt=consent`), `flow.go` (FlowManager + **state codec HMAC-signed** mô phỏng `internal/channels/bitrix24/oauth_state_codec.go:20-28`: payload `{provider, tenant_id, user_id, nonce, exp}` + HMAC-SHA256 bằng key dẫn xuất từ `GOCLAW_ENCRYPTION_KEY`, TTL 10 phút, **stateless** — restart gateway không mất flow đang chạy), `token.go` (TokenSource: `singleflight` refresh per account — chống race nhiều tool refresh cùng lúc; persist access token mới qua `UpdateTokens`; gặp 401 → force refresh đúng 1 lần rồi retry).
  - HTTP `internal/http/cloud.go` (routeRegistrar pattern `internal/gateway/server.go:38-40`, đăng ký qua `SetCloudHandler` + BuildMux loop `server.go:244-249`):
    - `GET /v1/cloud/accounts` — `requireAuth` Viewer; list account của ctx user (không trả token).
    - `POST /v1/cloud/oauth/{provider}/start` — `requireAuth` Operator; trả `{auth_url, redirect_uri}`; redirect_uri suy từ config `cloud.redirect_base_url` (fallback: scheme+host của request — bắt buộc khớp chuỗi URI đăng ký trong GCP console).
    - `GET /v1/cloud/oauth/callback` — **unauthenticated** GET browser callback (pattern nguyên bản `internal/http/mcp_oauth.go:85`): sai/thiếu state → 400; hết hạn state → 400; đúng → exchange code → `userinfo.email` → upsert `cloud_accounts` (encrypt) → `302 /cloud?connected=<email>` (lỗi → `302 /cloud?error=<code>`).
    - `DELETE /v1/cloud/accounts/{id}` — Operator + ownership check (row phải thuộc tenant+user ctx).
    - `GET /v1/cloud/status` — `{enabled, providers: {google: {configured: bool}}}` (client_id đã cấu hình chưa, edition bật chưa).
  - Config: section `cloud` mới (`internal/config/config_cloud.go` pattern các section channel): `enabled` (default true khi có credentials), `redirect_base_url`, `google.client_id`, `rclone_path` (phase 4 dùng). **Client secret qua env** `GOCLAW_CLOUD_GOOGLE_CLIENT_SECRET` — **audit-verify: env overlay hiện là per-key tường minh** (`applyEnvOverrides()` + `envStr()` tại `internal/config/config_load.go:171-180`), nên phải thêm dòng overlay tường minh cho key này trong loader mới — theo luật "secrets never in config.json".
  - Edition: thêm `CloudAccountsEnabled bool` vào `Edition` (`internal/edition/edition.go:9-23`); `Standard = true`, `Lite = false`. Wiring `cmd/gateway.go` chỉ khởi tạo cloud manager + handler khi enabled.
  - OpenAPI: thêm endpoints vào spec trong `internal/http/openapi.go` (`openapi_spec.json`).
- Non-functional: token/refresh token **không bao giờ** log (review bằng grep trong CI); callback không cần auth nhưng exchange + state HMAC ngăn CSRF/forgery; mọi store query parameterized.

## Architecture

Web UI (phase 2) → `POST /v1/cloud/oauth/google/start` → gateway build auth URL (state HMAC) → browser sang Google → Google redirect `GET /v1/cloud/oauth/callback?code&state` → verify state → POST token endpoint (PKCE verifier nhúng trong payload state — tránh map in-memory) → userinfo → upsert encrypted row → 302 về SPA. Tool/agent (phase 3) → TokenSource → decrypt + auto-refresh → Gmail API.

**Đã verify: `golang.org/x/oauth2 v0.34.0` đã trong go.mod** — flow dùng x/oauth2 (`oauth2.Config.AuthCodeURL` + `Exchange` với `AuthCodeOptions` PKCE S256) thay vì tự build HTTP tay; HMAC state codec vẫn tự viết theo bitrix24 pattern.

Điểm khác `internal/oauth` hiện tại: package đó chỉ cho OpenAI-Codex với client ID hardcode + callback port cố định 1455, 1 flow/lần (`internal/http/oauth.go:23-26`) — **không tái dùng**, giữ nguyên cho provider ChatGPT.

## Related Code Files

- Create: `migrations/000118_cloud_accounts.up.sql` + `.down.sql`
- Modify: `internal/upgrade/version.go` (117→118)
- Modify: `internal/store/sqlitestore/schema.sql`, `internal/store/sqlitestore/schema.go` (map entry + `SchemaVersion` 82)
- Create: `internal/store/cloud_account_store.go`, `internal/store/pg/cloud_accounts.go`, `internal/store/sqlitestore/cloud_accounts.go`
- Modify: `internal/store/stores.go`, `internal/store/pg/factory.go` (+ sqlitestore factory) — nhận `cfg.EncryptionKey` sẵn
- Create: `internal/cloud/{provider,google,flow,token}.go` + tests
- Create: `internal/http/cloud.go` (+ test), modify `internal/gateway/server.go` (setter + register)
- Create: `internal/config/config_cloud.go`
- Modify: `internal/edition/edition.go`, `cmd/gateway.go` (wiring)
- Modify: `internal/http/openapi.go` (spec entries)

## Implementation Steps

1. Migration PG + SQLite + bump 2 version constants (verify `migrate up/down` trên Docker pgvector test).
2. Store interface + PG + SQLite impls + test encrypted-at-rest (query raw thấy prefix `aes-gcm:` — pattern test `mcp_oauth_tokens`).
3. `internal/cloud`: state codec (roundtrip/tamper/expiry tests) → google provider → flow manager (httptest token server) → token source (singleflight, 401-refresh-retry test).
4. HTTP handler + auth tests (viewer list OK, callback unauth OK, sai state 400, delete account người khác → 403/404).
5. Config + edition + wiring + openapi entries.
6. `go fix ./...` + build 2 mode + vet + integration tests.

## Success Criteria

- [x] `POST start` (authed) trả auth URL chứa `state` HMAC + PKCE; đi qua URL thật → DB có row `cloud_accounts` với `access_token`/`refresh_token` prefix `aes-gcm:`; browser về `/cloud?connected=`.
- [x] Sai state / state quá 10 phút → HTTP 400, không có row.
- [x] Restart gateway: account đã lưu vẫn đọc/decrypt được (flow pending mất là chấp nhận được — user bấm lại).
- [x] Store test: query raw thấy token mã hóa; list/delete scoped đúng tenant+user.
- [x] Lite build (`-tags sqliteonly`): `/v1/cloud/status` trả `enabled:false`, endpoints khác 404, không crash.
- [x] Build/vet/test sạch 2 mode.

## Risk Assessment

- **Restricted scopes Google (gmail.readonly/modify)**: app unverified → warning screen click-through được; testing mode refresh token 7 ngày. BYO client per install + docs (phase 5); anh dùng internal org app (Google Workspace) thì không bị. Signal vỡ: connect xong 7 ngày token chết → docs chỉ dẫn publish-to-production unverified.
- **redirect_uri mismatch** (NAT/reverse proxy): bắt buộc config `cloud.redirect_base_url`; UI (phase 2) hiển thị exact redirect URI để paste vào GCP console. Signal: lỗi `redirect_uri_mismatch` từ Google → kiểm tra config trước khi debug code.
- **HMAC key = encryption key**: derive key riêng cho state (context label "cloud-oauth-state") — không dùng AES key trực tiếp; nonce 16 byte. Signal vỡ (state giả mạo xuất hiện trong log audit) → quay lại state lưu DB + revoke.
- **PKCE verifier lộ trong state** (state base64-readable, nằm trong URL/history): chấp nhận được vì client là confidential (client_secret chỉ nằm server) — kẻ thấy URL có `code` + verifier vẫn không exchange được. Nếu sau này thêm provider public-client → chuyển verifier sang map in-memory TTL ngắn.
- **Abuse callback endpoint** (unauth): rate limit sẵn của gateway áp cho route; state phải verify trước mọi DB write. Exchange thất bại chỉ log code ngắn (không log token).
