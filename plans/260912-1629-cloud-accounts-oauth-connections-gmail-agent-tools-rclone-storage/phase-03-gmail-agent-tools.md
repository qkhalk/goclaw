---
phase: 3
title: "Gmail agent tools (search/read/archive/unsubscribe)"
status: completed
priority: P1
effort: "2d"
dependencies: ["phase-1"]
---

# Phase 3: Gmail agent tools — search / read / archive / unsubscribe

## Overview

Agent thao tác mailbox các tài khoản đã nối (phase 1): tìm kiếm (Gmail `q` syntax), đọc, archive/trash/label (không có delete vĩnh viễn), phân tích + thực thi unsubscribe RFC 8058 có xác nhận user. Gmail API client mỏng tự viết — không kéo `google-api-go-client`.

## Requirements

- Functional:
  - `internal/cloud/mail/gmail.go` — thin client trên `net/http` + `RetryDo` (exported generic tại `internal/providers/retry.go:148` — reuse trực tiếp): `users.getProfile`, `messages.list` (q, labelIds, maxResults default 25 cap 100, pageToken), `messages.get` (format full/metadata), `messages.modify` (add/removeLabelIds, cap 100), `labels.list`. Token lấy từ `internal/cloud` TokenSource (phase 1).
  - Tools (file `internal/tools/cloud_mail.go`, pattern `ask_options.go:45-76` — struct implement `Name/Description/Parameters/Execute`):
    - `mail_accounts` — list account mail đã nối (model cần biết cái gì tồn tại).
    - `mail_search(query, account?, max_results?)` — Gmail search-box syntax; trả compact JSON `[{id, thread_id, from, subject, date, snippet}]`.
    - `mail_read(message_id, account?)` — body text (ưu tiên `text/plain`, fallback strip HTML), truncate 8KB (config), đính kèm chỉ liệt kê tên+size.
    - `mail_archive(message_ids[], action: archive|trash|untrash|mark_read|add_label|remove_label, label_name?, account?)` — batch ≤50; **không implement `messages.delete`/`batchDelete` ở bất kỳ đâu** — vắng mặt ở mức code là guardrail v1; mọi thao tác ghi log `slog.Info("cloud.mail_modify", ...)`.
    - `mail_unsubscribe(message_id, account?, execute=false)`:
      - Luân phân tích luôn: đọc headers `List-Unsubscribe` + `List-Unsubscribe-Post` (format=metadata) → trả về plan `{mechanism: rfc8058_post|mailto|https_get, url, sender}`.
      - `execute=true` mới thực hiện: RFC 8058 POST (`List-Unsubscribe=One-Click`, content-type form-urlencoded, không redirect theo), mailto fallback (gửi mail trống từ account — **defer gmail.send scope: v1 chỉ https mechanisms**; mailto-only mail → trả hướng dẫn cho agent tự báo user), https GET (fetch, KHÔNG auto theo redirect).
      - **Consent gate**: tool description + system prompt + dev-mode rule bắt buộc agent hỏi user xác nhận (ask_options đã có 3.19) trước khi gọi với `execute=true`; mỗi execute log `slog.Warn("security.mail_unsubscribe", "sender", ...)`.
  - Multi-account: param `account` (email) — resolve trong store theo tenant+user ctx; sai/sốt quyền → error rõ ràng (`CredentialUserIDFromContext` pattern `internal/store/context.go:136`).
  - Wiring: `cmd/gateway_tools_wiring.go` register (điều kiện `edition.Current().CloudAccountsEnabled` + cloud config có google); builtin seed entries trong `cmd/gateway_builtin_tools.go` `builtinToolSeedData()` (:13) — category "cloud"; system prompt: entries trong `coreToolSummaries` (`internal/agent/systemprompt.go:190-238`) + bổ sung guidance dev_mode_prompt v2 (quy tắc mail: không delete vĩnh viễn, thao tác xóa/archive phải xác nhận, unsubscribe phải được user đồng ý).
  - i18n backend: error messages user-facing qua `i18n.T` — key mới vào `internal/i18n/keys.go` + **5 catalog** (en/vi/zh/ko/ru — backend đã 5 catalog từ 3.19).
- Non-functional:
  - Rate limit per account: token bucket đơn giản (default 10 req/phút, config `cloud.mail.rate_per_minute`) — tránh 429 Gmail; RetryDo lo backoff.
  - Mọi HTTP response lớn truncate trước khi vào ForLLM/observation (mailbox hàng nghìn thread không phá context window).
  - KHÔNG bao giờ log nội dung mail đầy đủ ở info level (chỉ message_id + metadata).

## Architecture

Tool → resolve account (store, tenant+user scoped) → TokenSource (decrypt, refresh nếu hết hạn, singleflight) → gmail client → ForLLM compact JSON. Guardrail phân lớp: (1) tool không có tác vụ delete vĩnh viễn; (2) description + system prompt yêu cầu xác nhận; (3) log mọi thao tác ghi; (4) RBAC role của user vẫn áp qua middleware như mọi tool call.

## Related Code Files

- Create: `internal/cloud/mail/gmail.go` (+ test với httptest fake Gmail)
- Create: `internal/tools/cloud_mail.go` (+ test)
- Modify: `cmd/gateway_tools_wiring.go`, `cmd/gateway_builtin_tools.go`
- Modify: `internal/agent/systemprompt.go` (coreToolSummaries + ToolNames), `internal/agent/dev_mode_prompt.go`
- Modify: `internal/i18n/keys.go` + 5 catalog files
- Modify: `internal/config/config_cloud.go` (mail rate, read truncate)

## Implementation Steps

1. Gmail client + httptest fake (list q passthrough, get parsing multipart body text/plain, modify body shape, header parsing).
2. `mail_accounts` + `mail_search` + `mail_read` + tests (format compact, truncate, attachment listing).
3. `mail_archive` + tests (batch cap 50, label resolve theo tên → id, không có delete path — assert API fake không nhận delete).
4. `mail_unsubscribe` + tests: parse 3 mechanism; execute=false KHÔNG phát sinh HTTP (assert fake transport zero calls); rfc8058 POST body đúng; https GET không theo redirect.
5. Wiring + seeds + systemprompt + dev-mode text + i18n 5 catalog.
6. Manual E2E trên dev bot: "kiểm tra mail mới hôm nay từ X" → agent search+read; "archive mấy mail quảng cáo của Y" → agent liệt kê + hỏi + archive; "unsubscribe Z" → agent hỏi xác nhận → execute.

## Success Criteria

- [x] Agent trả lời được "mail mới nhất từ <sender> nói gì" dùng mail_search + mail_read, đúng tài khoản khi có 2 account.
- [x] mail_archive: archive (gỡ INBOX) + trash hoạt động; KHÔNG tồn tại code path delete vĩnh viễn (grep `messages.delete|batchDelete` = 0 trong repo).
- [x] mail_unsubscribe execute=false thuần phân tích (0 HTTP); execute=true POST RFC 8058 đúng format; mailto-only → agent nhận hướng dẫn báo user.
- [x] Token hết hạn giữa chừng → tool tự refresh (test: fake server trả 401 một lần) và thành công, không lặp vô hạn.
- [x] Lite: tools không đăng ký (grep registry sau wiring), system prompt không nhắc.
- [x] Build/vet/test sạch 2 mode; integration test chạy qua fake Gmail end-to-end.

## Risk Assessment

- **Model bỏ qua consent gate** (gọi execute=true không hỏi): giảm thiểu bằng description + dev-mode rule + ask_options pattern, nhưng về bản chất là soft gate. Nếu lộ (log audit thấy unsubscribe không có confirm): thêm hard gate — flag per-account `allow_unsubscribe` user bật trong UI Cloud (phase 5 polish có thể nâng cấp). Ghi rõ đây là chấp nhận có kiểm soát của v1.
- **Gmail 429/quota**: token bucket + RetryDo backoff; max_results cap. Signal vỡ: user complaint chậm → giảm default rate.
- **Multipart parsing mail phức tạp** (nested multipart, encoding base64/quoted-printable, charset lạ): dùng thư viện chuẩn `mime/multipart` + `mime/quotedprintable`; case lạ → trả metadata + snippet thay vì crash (fail-open về nội dung, không fail toàn tool).
- **`gmail.labels` scope sensitivity**: labels là non-sensitive scope — an toàn; `gmail.modify` là restricted — đã tính trong docs phase 5.
- **Context blowup**: truncate mọi output (search list cap 100, read cap 8KB, labels cap mặc định bỏ system labels trừ INBOX/STARRED quan trọng).
