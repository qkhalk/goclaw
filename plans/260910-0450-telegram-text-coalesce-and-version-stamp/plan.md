---
title: "Telegram text coalesce and version stamp"
description: "Sửa 2 lỗi: (1) tin nhắn Telegram dài bị client cắt thành nhiều phần nhưng agent chỉ nhận phần đầu — gộp các phần liên tiếp ở channel-level trước khi publish; (2) UI hiển thị version 0.1.0 (binary cũ stamp 0.1.0-4phases.2 đang chạy production) — build lại từ dev với version stamp đúng và redeploy."
status: completed
priority: P1
effort: "0.5d"
tags: [telegram, debounce, version, bugfix]
created: 2026-09-10
---

# Telegram text coalesce and version stamp

## Overview

Hai bug thực nghiệm anh báo:

1. **Gửi văn bản dài qua Telegram, agent chỉ nhận phần đầu.** Telegram client tự cắt tin >4096 ký tự thành 2–3 tin liên tiếp (<100ms). GoClaw publish mỗi tin là một `InboundMessage` riêng; cơ chế gộp duy nhất (bus debouncer) **mặc định tắt cho text** (`gateway.inbound_debounce_ms` = 0 → pass-through ngay, chỉ media mới có floor 1s). Kết quả: phần 1 chạy một run riêng; phần 2/3 thành run khác hoặc bị intent-classifier phân nhầm (new_task/steer/status) khi session đang bận.
2. **UI hiện "Đã kết nối · 0.1.0"** trong khi bản release hiện tại là 3.17.5. UI đọc `server.version` từ WS `connect` response = `cmd.Version` đóng dấu lúc build. Binary đang chạy trên server (`/opt/goclaw/goclaw`, start 20:56) tự báo `goclaw 0.1.0-4phases.2` — stamp của một bản build đầu fork. Code hiện tại mặc định `"dev"` (`cmd/root.go:13`), không gì sinh 0.1.0 → chỉ cần build lại từ dev với stamp đúng rồi thay binary.

## Root Causes (đã verify file:line)

- `internal/bus/inbound_debounce.go:66-78` — `debounceMs <= 0` → pass-through ngay (merge-into-buffer chỉ khi buffer đã tồn tại từ tin media).
- `cmd/gateway_consumer_debounce.go:29-36,67-72` — `resolveInboundDebounceDelay` lấy `Cfg.Gateway.InboundDebounceMs` (0 nếu unset, không có default nào khác), `applyMediaFloor` chỉ nâng lên 1000ms khi `len(msg.Media) > 0`.
- `internal/channels/telegram/handlers.go:451-456,739-753` — text không có MediaGroupID → bỏ qua album aggregator, publish ngay 1 InboundMessage/tin, mỗi tin có `message_id` riêng (dedup key gồm messageID nên không bị nuốt — `cmd/gateway_consumer_dedup.go:14-16`).
- `internal/scheduler/queue.go:57-65` — debounce 800ms của scheduler chỉ TRÌ HOÃN start, không gộp nội dung; mỗi item vẫn là 1 run riêng.
- Version: `cmd/root.go:12-13` (`var Version = "dev"` + ldflags), `cmd/gateway.go:647` (`server.SetVersion(Version)`), `internal/gateway/router.go:406-409` (connect response `server.version`), `ui/web/src/components/layout/connection-status.tsx:45-46` (UI render `· cleanVersion(serverVersion)`). Verify trực tiếp trên server: `/opt/goclaw/goclaw version` → `goclaw 0.1.0-4phases.2 (protocol 3)`.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Text coalescer channel-level cho Telegram: các tin text liên tiếp cùng người gửi cùng chat trong cửa sổ mặc định 1000ms gộp thành 1 InboundMessage (join `\n`), tôn trọng cấu hình, media/album đi đường cũ | P1 |
| 2 | Binary production thay bằng build mới từ dev (gồm cả coalescer) với `cmd.Version` stamp đúng (git describe / 3.17.5), UI hiển thị version đúng sau restart | P1 |

## Scope Challenge (đã cân nhắc)

- **Không** đổi default `gateway.inbound_debounce_ms` (quyết định 0=off là chủ ý, commit `2c699f31`; đổi sẽ cộng độ trễ cho mọi kênh + web chat). Coalescer đặt ở Telegram channel — nơi duy nhất có hành vi client-cắt-tin — không đụng semantics chung.
- **Không** sửa intent-classifier (phân nhầm fragment khi session bận): chỉ xảy ra khi phần sau tới trễ hơn cửa sổ coalesce (người dùng dán rất chậm). Ghi nhận là residual risk, không mở rộng scope.
- **Không** thêm endpoint version HTTP mới: UI đã đọc từ WS connect; không cần parity thêm.

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Telegram text coalescer](./phase-01-start.md) | Pending |
| 2 | [Phase 2: Version stamp and production redeploy](./phase-02-version-stamp-and-production-redeploy.md) | Pending |

## Success Criteria

- [ ] Gửi tin dài bị cắt 3 phần qua Telegram DM → agent nhận **1** message ghép đủ 3 phần (verify qua log/run timeline).
- [ ] `text_coalesce_ms: 0` → hành vi như cũ (không gộp); media/album không bị ảnh hưởng; `/lệnh` không qua coalescer.
- [ ] `go build ./...`, `go build -tags sqliteonly ./...`, `go vet`, test mới + test cũ (telegram, bus, config) xanh trong golang:1.26.
- [ ] Trên server: `/opt/goclaw/goclaw version` in đúng version stamp mới; web UI sidebar hiện version đó; bot Telegram trả lời đủ nội dung tin dài.

## Cross-Plan Relationships

Không có plan đang mở nào chồng lấn (các plan 2608xx/260910-0216 đã hoàn tất & merge).

## Surface Parity (kỳ vọng)

- **Gateway server:** telegram channel + config (thay đổi chính); consumer/bus không đổi.
- **API contract:** N/A — không thêm WS/HTTP method.
- **Web UI:** N/A — version hiển thị đọc từ nguồn có sẵn (WS connect), hết hiện 0.1.0 sau khi binary đúng stamp chạy lại.
- **CLI/runtime package:** N/A.
- **Desktop lite:** build-tag sqliteonly phải vẫn build xanh (channel code được compile chung).
