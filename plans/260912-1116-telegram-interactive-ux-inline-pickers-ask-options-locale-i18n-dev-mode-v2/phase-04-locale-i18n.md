---
phase: 4
title: "Locale Web UI + i18n lệnh"
status: completed
priority: P1
effort: "6h"
dependencies: []
---

# Phase 4: Locale Web UI + i18n lệnh

## Overview

Lệnh Telegram phản hồi theo ngôn ngữ anh đặt ở Web UI: locale được persist khi WS connect (vào `tenant_users.metadata.locale`), Telegram channel đọc ra khi xử lý lệnh; mọi string lệnh chuyển qua `i18n.T` với **5 catalog** (en/ko/ru/vi/zh — verify ls đã có). Telegram client language chỉ còn là fallback.

## Requirements

- Functional:
  - WS connect với `locale` param (đã có `router.go:140,149`) → gateway **persist** vào `tenant_users.metadata.locale` cho user đó (cột có sẵn migration 000027:31; upsert qua store — verify tenant_users store interface có Update metadata? nếu chưa có method: thêm `SetUserLocale(ctx, userID, locale)` vào TenantStore/UserStore interface + PG + SQLite theo luật dual-DB — KHÔNG migration).
  - Telegram channel: khi xử lý lệnh (handleBotCommand + callbacks), resolve locale theo thứ tự: (1) `tenant_users.metadata.locale` của user liên kết với sender — **bước verify đầu tiên: xác định link sender TG ↔ user row** (channel_contacts.user_id migration 000014:8? pairing store? xem Audit Log dưới); (2) fallback `user.LanguageCode` (TG client, như hôm nay `handlers.go:681-683`); (3) "en".
  - Nếu bước verify (1) không tìm thấy link vững (id space khác nhau, không map): **fallback thiết kế** — locale per-chat lưu session metadata `locale` (lệnh `/language <en|vi|zh|ko|ru>` setter đơn giản), Web UI connect KHÔNG tự áp cho TG. Chọn đường này nếu cross-link không đáng tin — ghi rõ quyết định trong Verify Log.
  - i18n hóa TOÀN BỘ string lệnh hiện có + mới: /thinking, /reasoning, /dev, /status (labels card), /skills picker, ask_options (question prefix + nút Other + confirm), notices lỗi cũ (/help, /reset reply...). Key đặt `telegram.cmd.*` trong `internal/i18n/keys.go` + **đủ 5 catalog**.
  - `/status` label card theo locale (giữ số liệu/emoji như nhau, chỉ nhãn).
- Non-functional: resolve locale 1 query/hoặc cache per-sender 5 phút trong channel (map nhỏ); KHÔNG migration schema (metadata JSONB có sẵn).

## Architecture

- New helper `cmdLocale(ctx, senderID, tgLangCode) string` trong telegram package: check cache → store lookup → fallback chain. Cache `sync.Map` senderID→(locale, expiry) 5 phút.
- Connect persist: trong `sendConnectResponse` (router.go:365-442) sau khi có user_id + locale, fire-and-forget `SetUserLocale` (error → log warn, không chặn connect).
- Chuẩn hóa mọi reply: các handler đổi `send("...")` → `send(i18n.T(c.cmdLocale(...), i18n.MsgTGThinkingCurrent, args...))`. Status card: builder nhận locale + labels map từ i18n.

## Related Code Files

- Modify: `internal/gateway/router.go` (persist locale khi connect)
- Modify: store interface + `internal/store/pg/` + `internal/store/sqlitestore/` (SetUserLocale — hoặc thêm method metadata generic nếu đã có)
- Create: `internal/channels/telegram/locale.go` (cmdLocale + cache)
- Modify: `internal/channels/telegram/commands*.go`, `pickers.go`, `ask_options.go`, `session_prefs.go`, `commands_status.go` (replace hardcode)
- Modify: `internal/i18n/keys.go` + `catalog_en.go` + `catalog_vi.go` + `catalog_zh.go` + `catalog_ko.go` + `catalog_ru.go`
- Create: `internal/channels/telegram/locale_test.go`, i18n key coverage test (grep-style unit: mọi key gọi trong code có trong 5 catalog)

## Implementation Steps

1. **Verify Log (làm đầu tiên)**: truy vấn DB server dev: bảng channel_contacts (user_id của sender telegram:xxx?), tenant_users (row của anh), pairing tables — xác nhận map sender→user. Ghi kết quả + quyết định đường (cross-link hay /language fallback).
2. Store method `SetUserLocale` (PG + SQLite + interface) + test.
3. Router persist + test.
4. `locale.go` + cache + fallback chain + test.
5. Thay string: mỗi file lệnh một commit nhỏ; thêm key + 5 catalog theo từng cụm; test coverage key.
6. Manual: Web UI chuyển tiếng Việt → reconnect → `/status` hiện nhãn tiếng Việt.
7. Build/vet/test chuẩn.

## Success Criteria

- [ ] Web UI language = vi → connect → lệnh TG phản hồi tiếng Việt; chuyển en → tiếng Anh (verify 2 ngôn ngữ; ko/ru/zh key có mặt nhưng không bắt verify thủ công).
- [ ] Không còn string lệnh hardcode (grep `send("` trong telegram package chỉ còn key i18n hoặc số liệu thuần).
- [ ] 5 catalog đủ key (unit test coverage).
- [ ] Fallback hoạt động khi user không có row (TG client language → en).
- [ ] Build/vet/test sạch 2 mode.

## Risk Assessment

- **R4 cross-link user**: Verify Log quyết. Nếu fallback `/language` — vẫn đạt "không phụ thuộc TG client" nhưng không tự theo Web UI; anh biết rõ trade-off (ghi trong report cook).
- **Catalog dài** — key thêm theo cụm, PR tách commit cho review được; thiếu key chỉ rơi về tiếng Anh (T fallback `i18n.go:35`) không crash.
- **Cache locale làm chậm đổi ngôn ngữ nhận diện 5 phút** — chấp nhận.
- **i18n key conflict tên** — prefix `telegram.cmd.` + `telegram.ask.` toàn bộ.
