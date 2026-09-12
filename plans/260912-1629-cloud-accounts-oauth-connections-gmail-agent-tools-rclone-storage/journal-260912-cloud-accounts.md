# Journal — Cloud Accounts (Google OAuth + Gmail/Drive agent tools)

Ngày: 2026-09-12. Plan: `260912-1629-cloud-accounts-oauth-connections-gmail-agent-tools-rclone-storage` (5/5 phases).

## Điều đã xây

- **`cloud_accounts` table** (PG 000118 + SQLite v82, dual-DB law): per-user OAuth connections, token columns AES-256-GCM encrypted (`aes-gcm:` prefix), UNIQUE(tenant,user,provider,email), store scoped bằng ctx tenant+user — scope violation không leak tồn tại.
- **`internal/cloud`**: OAuth web flow BYO client (x/oauth2 + PKCE S256), state HMAC-SHA256 stateless (bitrix24 pattern; key derive riêng context label), payload chứa cả code_verifier lẫn redirect_uri (RFC 6749 §4.1.3 — exchange phải lặp đúng redirect_uri). TokenSource: single-flight + persist token mới về DB.
- **HTTP `/v1/cloud/*`**: start (authed), callback (unauth, state = CSRF), status/list/delete; `enabled` flag gộp edition + kill-switch `cloud.enabled`; secret qua env `GOCLAW_CLOUD_GOOGLE_CLIENT_SECRET` (overlay per-key tường minh).
- **Web UI trang `/cloud`**: connect = full-page redirect thật (không paste URL), account cards + status chips, setup banner hiển thị exact redirect URI, i18n 5 locale, sidebar item gated theo `/v1/edition`.
- **Agent tools**: `cloud_accounts`, `mail_search` (metadata fetches), `mail_read` (truncated body + attachments), `mail_archive` (archive/trash/untrash/mark_read/label — KHÔNG có messages.delete ở bất kỳ đâu), `mail_unsubscribe` (RFC 8058 analyze-first, execute consent-gated, log `security.mail_unsubscribe`, không theo redirect, mailto bị chặn v1); Drive qua rclone rcd loopback (random basic-auth): `cloud_ls/read/fetch/about`.
- **Skill `mail-digest`** + docs `docs/30-cloud-accounts.md` + CHANGELOG + openapi 5 paths.

## Bài học / sự cố đáng nhớ

1. **Bug schema pre-existing**: schema.sql có dòng `=======` thiếu prefix `--` (commit routing_rules 000117 làm rơi) làm `tx.Exec(schemaSQL)` vỡ "near '=='" trên fresh DB — 7 sqlitestore upgrade tests fail từ TRƯỚC khi mình chạm code. Fix 1 dòng; verify pre-existing bằng stash + rerun trước khi kết luận.
2. **Code-review bắt 2 P0** mà build/test xanh không phát hiện: (a) PG UpdateTokens nối WHERE bằng dấu phẩy (SQLite twin hardcode `AND` nên test pass — classic dual-DB asymmetry; cần integration test cho PG store), (b) exchange thiếu redirect_uri (auth URL có, exchange không — Google sẽ từ chối). Thêm: singleflight của tôi có bug theo mẫu (leader clear flight trước wg.Done → follower thành leader mới, 8 calls) — chỉ phát hiện khi chạy `-race -count=2` với fake có delay; pattern đúng là publish kết quả TRƯỚC khi Done.
3. **rclone.conf là runtime token truth** (rclone tự refresh, ghi lại conf của nó): disconnect phải ConfigDelete remote để token không nằm trên disk; reconnect = Disconnect+Connect.

## Trạng thái

- Build PG + sqliteonly xanh; vet sạch; tests: cloud + mail + tools + store(sqlite) + agent + edition + config pass; race test cloud pass; PG migration smoke up→118/down→117/up→118 dirty=false.
- Chưa làm: E2E thật với Google account (cần GCP client của anh — docs có từng bước), rclone E2E trên server, deploy.
- Follow-ups (P2 từ review): SSRF guard cho unsubscribe URL, per-call rate limit, pageNoToken cho search, gitignore `ui/web/.pnpm-store/`.

## Vòng 2 (feedback anh): setup Web UI + rclone bundled

- **Setup OAuth client ngay trên Web UI** (public-project flow): `GET/PUT /v1/cloud/settings` (admin + requireMasterScope — instance-wide config), credentials lưu `config_secrets` (mã hóa sẵn), manager resolve động: web-UI-saved > env/config. Lưu xong form **ẩn hẳn** (còn nút pencil nhỏ để rotate); tools hoạt động ngay không cần restart (wireCloudTools bỏ gate creds, lỗi "no connected account" rõ ràng khi chưa nối account). Kill-switch `cloud.enabled` tách riêng khỏi credentials check (`KillSwitchOn`).
- **rclone bundled mọi Docker variants** (base/latest/full) — `apk add rclone` ở lớp chung; docs cập nhật.
- **Phân trang OAuth models vs Cloud**: trang Providers giữ nguyên toàn bộ provider OAuth (ChatGPT/Codex...); trang Cloud chỉ chứa cloud accounts (Gmail/Drive) — tách bạch sạch, không đụng nhau.
