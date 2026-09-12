---
title: "Cloud Accounts: OAuth connections, Gmail agent tools, rclone storage"
description: "Thêm section Cloud vào Web UI: user kết nối nhiều tài khoản Google qua OAuth (BYO client credentials), token lưu mã hóa trong bảng cloud_accounts mới; agent có tool đọc/tìm kiếm/xóa ngầm (archive/trash/unsubscribe) mailbox Gmail; lớp lưu trữ rclone (rcd loopback + rc API) cho Google Drive với tool cloud_ls/cloud_read/cloud_fetch. Kèm automation dọn mail hằng ngày."
status: completed
priority: P1
effort: "7d"
tags: [cloud, oauth, gmail, rclone, web-ui, agent-tools, security]
created: "2026-09-12"
---

# Cloud Accounts: OAuth connections, Gmail agent tools, rclone storage

## Overview

Anh muốn Web UI có section **Cloud** để login nhiều tài khoản (Google đầu tiên), và agent đọc được tài nguyên của các tài khoản đó — mail trước hết: đọc, tìm kiếm, "tắt các mail không dùng" (archive/trash/unsubscribe). Lưu trữ cloud (Drive) qua **rclone**.

**Kết quả research định hướng thiết kế (2 researcher, sources trong `reports/`):**

1. **OpenClaw KHÔNG có gì giống vậy** — channels chỉ là chat (Discord/Signal/Telegram/…), tool list không có mail/drive/calendar, token storage không tài liệu hóa, v2.0 còn bị chê "secrets at rest không mã hoá" (Wikipedia). Làm Cloud section là **khác biệt hóa**, không phải đuổi theo.
2. **rclone không có backend mail** (chỉ Drive/GCS/Photos cho Google). Mail phải dùng **Gmail API trực tiếp** (scopes `gmail.readonly` + `gmail.labels` + `gmail.modify`; search bằng `q`; archive/trash = `messages.modify` gỡ/Thêm label). IMAP generic bị loại v1: go-imap v2 còn WIP, XOAUTH2 với Gmail bắt buộc scope rộng nhất `https://mail.google.com/`.
3. **rclone nên chạy như binary + rc API** (`rclone rcd` loopback + basic-auth sinh ngẫu nhiên, inject remote qua `rc config/create`), KHÔNG import làm Go library (~40 backend dependencies + global state, không cam kết API ổn định). rc = shell-equivalent → chỉ loopback, không expose.
4. **OAuth phải là BYO client** (mỗi install tự đăng ký GCP project): Google testing mode làm refresh token **hết hạn 7 ngày** + cap 100 test users → hướng dẫn user publish unverified hoặc internal org app. PKCE + `access_type=offline` + state CSRF (HMAC sign, pattern có sẵn `bitrix24/oauth_state_codec.go`).
5. **Unsubscribe theo RFC 8058**: POST one-click **bắt buộc có sự đồng ý của user** ("MUST NOT perform a POST without user consent") → tool phải hỏi xác nhận trước (dùng ask_options đã có từ 3.19).

**Codebase scout — mọi precedent cần thiết đã có:**

| Cần | Precedent có sẵn |
|---|---|
| Bảng token per-user mã hóa | `mcp_oauth_tokens` (migration 000084, encrypted columns từng cái) |
| HTTP callback GET + state CSRF | `internal/http/mcp_oauth.go:85` (unauth callback, state→flow map) + signed-state `internal/channels/bitrix24/oauth_state_codec.go` |
| Mã hóa token | `internal/crypto` AES-256-GCM (`Encrypt/Decrypt`, key từ `GOCLAW_ENCRYPTION_KEY` qua `store.StoreConfig`) |
| Tool + wiring + seed + prompt | `tools/ask_options.go` / `cmd/gateway_tools_wiring.go` / `gateway_builtin_tools.go` / `systemprompt.go coreToolSummaries` |
| Trang web API-backed + i18n 5 locale | `ui/web/src/pages/providers/` pattern, `routes.tsx`, sidebar connectivity group |
| Migration kế tiếp | PG `000118` + `RequiredSchemaVersion` 117→118; SQLite `schema.sql` + map + `SchemaVersion` 81→82 |

Repo hiện **không có bất kỳ** code cloud/gmail/imap/rclone (grep sạch) — không conflict.

## Scope Challenge (Step 0)

**Trong scope (v1):**
- OAuth Google (Gmail + Drive scope), **multi-account** (nhiều Gmail cùng lúc, per user + tenant)
- Trang Cloud: list tài khoản, connect qua redirect flow thật (không paste URL), disconnect, status
- Agent tools mail: `mail_search`, `mail_read`, `mail_archive` (archive/trash/label — **không** delete vĩnh viễn), `mail_unsubscribe` (consent-gated)
- Lưu trữ: rclone rcd + `cloud_ls`, `cloud_stat`, `cloud_read`, `cloud_fetch` (tải về workspace), `cloud_about` (quota) — Google Drive đầu tiên
- Automation: cron digest mail hằng ngày (template, off mặc định)
- Edition gating: Standard-only v1 (flag `CloudAccountsEnabled` trong `internal/edition`)

**Ngoài scope (backlog, ghi rõ lý do trong Backlog dưới):**
- Microsoft Graph mail/Outlook, Google Calendar/Photos/Contacts, IMAP generic, gửi mail, mount FUSE, sync/backup 2 chiều, per-agent ACL tài khoản cloud.

**Không trim gì trong yêu cầu gốc:** anh yêu cầu "login nhiều tài khoản", "agent đọc/tìm kiếm mail", "tắt mail không dùng" — cả 3 đều trong scope; rclone có mặt (Drive) dù mail đi đường API riêng vì rclone không làm mail được.

## Mode

Hard (security surface: OAuth tokens, mail access). Red-team adversarial pass chạy sau khi viết plan (bắt buộc theo AGENTS.md "Verify pass MANDATORY").

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Kết nối 2+ tài khoản Google từ trang Cloud, token mã hóa AES-256-GCM trong `cloud_accounts` | P1 |
| 2 | Agent tìm/đọc mail trên các tài khoản đã nối: `mail_search`/`mail_read` | P1 |
| 3 | "Tắt mail không dùng": `mail_archive` (archive/trash/label) + `mail_unsubscribe` RFC 8058 có xác nhận | P1 |
| 4 | Lưu trữ Drive qua rclone: `cloud_ls`/`cloud_read`/`cloud_fetch`/`cloud_about` | P2 |
| 5 | Digest mail hằng ngày bằng cron lane (template) | P2 |
| 6 | Surface parity: gateway + HTTP contract + Web UI + docs; Lite không vỡ | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Foundation — cloud_accounts store + OAuth flow provider-agnostic](./phase-01-foundation-store-oauth.md) | Completed |
| 2 | [Web UI Cloud page + accounts API](./phase-02-webui-cloud-page.md) | Completed |
| 3 | [Gmail agent tools (search/read/archive/unsubscribe)](./phase-03-gmail-agent-tools.md) | Completed |
| 4 | [rclone storage layer + cloud tools](./phase-04-rclone-storage-tools.md) | Completed |
| 5 | [Mail digest automation + polish + docs](./phase-05-digest-polish-docs.md) | Completed |

## Success Criteria

- [ ] 2 tài khoản Gmail nối qua trang Cloud; disconnect/reconnect ổn; token trong DB có prefix `aes-gcm:`, không bao giờ xuất hiện trong log.
- [ ] `mail_search "from:netflix"` trả list; `mail_read` trả nội dung thu gọn; `mail_archive` archive/trash đúng label; KHÔNG có đường delete vĩnh viễn.
- [ ] `mail_unsubscribe` luôn hỏi xác nhận (ask_options hoặc AskUser) trước khi POST RFC 8058 / gửi mailto.
- [ ] `cloud_ls`/`cloud_fetch` hoạt động với Drive qua rclone rcd loopback; thiếu binary rclone → lỗi rõ ràng, mail tools không ảnh hưởng.
- [ ] Callback OAuth: sai/expired state → 400; đúng state → 302 về `/cloud?connected=<email>`; restart gateway giữa flow không mất tài khoản đã lưu.
- [ ] `go build` + `go build -tags sqliteonly` + `go vet` sạch; migration up/down PG 117→118 OK; SQLite fresh DB + upgrade path OK.
- [ ] Surface parity statement đầy đủ trong PR (gateway / API contract / Web UI / CLI-runtime).

## Risk Assessment

- **Google OAuth verification**: restricted scopes (gmail.readonly/modify) — app unverified hiện warning screen; testing mode refresh token 7 ngày. **Giải pháp:** BYO client per install + docs hướng dẫn publish-to-production-unverified hoặc internal org app. Đây là lý do KHÔNG ship shared GoClaw client ID.
- **rc API = shell-equivalent** — rcd phải loopback + basic-auth ngẫu nhiên + không bao giờ expose qua gateway HTTP; không dùng `core/command`.
- **Token refresh race** (nhiều tool cùng refresh 1 account) — singleflight/mutex per account trong token source.
- **Lite edition** — flag `CloudAccountsEnabled=false` trong Lite preset; tool không đăng ký; UI ẩn item sidebar.
- **rclone binary không có trên server** — config `cloud.rclone_path`, detect lúc start, degrade có thông báo; Docker full variant cài sẵn rclone.
- **Cross-plan overlap:** plan in-progress `260912-1137-web-chat-declutter` cũng sửa sidebar — phối hợp khi thêm nav item Cloud (khác group, conflict thấp, ghi chú trong phase 2).

## Backlog (deferred, không phải phase)

| Feature | Vì sao defer |
|---|---|
| Microsoft Graph mail (`Mail.Read`/`Mail.ReadWrite`, offline_access) | API surface thứ hai; flow module đã provider-agnostic nên thêm sau rẻ |
| Google Calendar read/create | OAuth surface mới, giá trị trung bình |
| Google Photos, OneDrive, Dropbox, S3 | Miễn phí sau khi rclone layer xong (thêm backend type vào remote) |
| IMAP generic | go-imap v2 WIP; Gmail-qua-IMAP phải scope `mail.google.com/` rộng nhất |
| Gửi mail (`gmail.send`) | Rủi ro lạm dụng; cần guardrail riêng kỹ hơn |
| Sync/backup workspace↔cloud 2 chiều (`sync/bisync`) | Cron + tool đơn giản trước |
| Per-agent cloud account ACL | Chưa có demand; v1 toàn bộ tools thấy mọi account của user |

## Research Reports

- `reports/research-external-260912.md` — openclaw, rclone rc/authorize/config, Gmail API vs IMAP vs Graph, RFC 8058, OAuth verification (researcher agent, URLs in-file).
- Codebase scout report — inline trong các phase (mọi claim kèm `file:line`).
