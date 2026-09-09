---
title: "Phase 1: Telegram text coalescer"
status: todo
---

# Phase 1: Telegram text coalescer

## Overview

Thêm bộ gộp tin text channel-level trong Telegram channel: các tin nhắn text thuần liên tiếp (không media, không album, không lệnh) từ cùng người gửi trong cùng chat/topic trong một cửa sổ im lặng (mặc định 1000ms, cấu hình được) được gộp thành MỘT `bus.InboundMessage` trước khi publish xuống bus — agent nhận đủ nội dung tin dài bị client cắt.

## Requirements

- Functional:
  - Gộp theo key `chatID|senderID|threadID` (DM + group + forum topic).
  - Nội dung join bằng `\n`, đúng thứ tự tới; metadata giữ của tin ĐẦU (reply context, is_group, local_key, topic…); gom toàn bộ `message_id` vào `metadata["merged_message_ids"]` (dùng lại key sẵn có — consumer `seedDedupFromMerged` ở `cmd/gateway_consumer_dedup.go:29-46` tự seed dedup cho các id anh em).
  - Cửa sổ im lặng reset mỗi khi có tin mới (pattern `internal/bus/inbound_debounce.go:94-107`).
  - Config: `TelegramConfig.TextCoalesceMs *int json:"text_coalesce_ms,omitempty"` — nil = 1000 (bật), 0 = tắt hoàn toàn; map qua `telegramInstanceConfig` cho DB instance (factory.go).
  - Tin có media/caption hoặc `MediaGroupID` KHÔNG qua coalescer (đường album aggregator + media floor giữ nguyên).
  - Cap an toàn: tối đa ~10 tin / ~32K ký tự trong buffer; vượt → flush ngay (tránh kẹt buffer khi spam liên tục).
  - `Stop()` của channel phải flush buffer còn đọng (pattern albumAggregator flush-before-cancel, `channel.go:229-238`).
- Non-functional: không thêm lock contention mới ngoài mutex của coalescer; zero-impact khi tắt (`TextCoalesceMs: 0` → publish path như cũ).

## Architecture

Chèn tại `processResolvedMessage` (`internal/channels/telegram/handlers.go:467`) — điểm duy nhất mọi tin text đã qua đủ gates (pairing, mention, topic) trước khi `PublishInbound` (`handlers.go:739-753`):

```
processResolvedMessage(ctx, rctx, msgs[])
  └─ msg text-thuần: c.textCoalescer.push(rctx, msg)   // giữ lại trong buffer
     msg media/album: đường cũ (album agg / publish ngay)
  └─ flush callback (timer hết im lặng): build 1 InboundMessage từ buffer → c.Bus().PublishInbound
```

- File mới `internal/channels/telegram/text_coalescer.go`: struct `textCoalescer{mu, buffers, window, publish func(resolvedMessageContext, []*telego.Message)}` — flush callback gọi chung hàm publish; refactor thuần tách phần thân `processResolvedMessage` thành hàm publish dùng lại được (không đổi hành vi đường thường).
- Timer per-key; buffer giữ `*telego.Message` gốc theo thứ tự để join `Text`.
- Lệnh `/xxx` không tới đây (handleBotCommand chặn trước ở `handlers.go:252`); lệnh lạ return false đi tiếp — chấp nhận gộp (desirable).
- Voice/media đã lọc trước khi tới path này — coalescer chỉ nhận `msg.Text != "" && msg.Media == nil && msg.MediaGroupID == ""`.

## Related Code Files

- Create: `internal/channels/telegram/text_coalescer.go`
- Create: `internal/channels/telegram/text_coalescer_test.go`
- Modify: `internal/channels/telegram/handlers.go` (tách hàm publish + cắm coalescer)
- Modify: `internal/channels/telegram/channel.go` (khởi tạo trong New, flush trong Stop)
- Modify: `internal/config/config_channels.go` (`TextCoalesceMs`)
- Modify: `internal/channels/telegram/factory.go` (`telegramInstanceConfig.TextCoalesceMs` + mapping)
- Modify: `docs/05-channels-messaging.md`, `CHANGELOG.md`

## Implementation Steps

1. Config field + default resolve helper (`textCoalesceWindow(cfg) time.Duration`).
2. `text_coalescer.go` (push/flush/stop, cap, key).
3. Refactor `processResolvedMessage` tách hàm publish; cắm coalescer; wiring New/Stop.
4. Unit tests (merge thứ tự, key isolation, timer reset, media bypass, off switch, cap, nil-config default).
5. Build 2 mode + vet + test trong golang:1.26 → commit.

## Todo

- [x] Config + default
- [x] text_coalescer.go + tests
- [x] Handler refactor + wiring
- [x] Docs + CHANGELOG
- [x] Build/test xanh + commit

## Success Criteria

- [x] Test: 3 tin text cách nhau < window → 1 publish, content đủ, `merged_message_ids` 3 id, metadata tin đầu.
- [x] Test: `window=0` → publish ngay từng tin (passthrough).
- [x] Test: media/album không vào buffer.
- [x] `go test ./internal/channels/telegram/ ./internal/bus/ ./internal/config/` xanh.

## Risk Assessment

- **Độ trễ +1s cho mọi tin text Telegram** (chờ cửa sổ): chấp nhận được cho chat bot; config `text_coalesce_ms: 0` để tắt. Tín hiệu vỡ: anh phàn nàn trễ → hạ default còn 700ms.
- **Refactor processResolvedMessage chạm đường publish chính**: chỉ tách hàm thuần, đổi điểm gọi; existing telegram tests xanh là gate hồi quy.
- **Tin đến sau khi flush đã kích hoạt** (người dùng dán chậm hơn window): vẫn tách run — residual risk ghi ở plan.md, không thuộc scope này.
