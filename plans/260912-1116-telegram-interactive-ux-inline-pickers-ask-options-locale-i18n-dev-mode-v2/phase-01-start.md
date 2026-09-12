---
phase: 1
title: "Interactive pickers: /thinking + /reasoning"
status: completed
priority: P1
effort: "5h"
dependencies: []
---

# Phase 1: Interactive pickers — /thinking + /reasoning

## Overview

Nâng `/thinking` từ nhập text sang inline keyboard (mức lọc theo capability model + nút Default), thêm `/reasoning` bật/tắt nhanh bằng 2 nút. Mở rộng callback router prefix-dispatch sẵn có và bổ panic-recover cho nhánh callback.

## Requirements

- Functional:
  - Gõ `/thinking` → gửi message kèm inline keyboard: các mức hợp lệ + `Default`. Nguồn mức: `providers.LookupReasoningCapability(model)` khi model khớp table (GPT-5/Codex family → levels của nó, bỏ `none`); model lạ → fallback `thinkingLevelList` đầy đủ. Nút hiện tại đang chọn được đánh dấu ✅ (load từ metadata + agent default).
  - Bấm nút mức → áp dụng (cùng logic `normalizeThinkingLevel`/`setChatPrefs` của 3.18.0) → **EditMessageText** chính message đó thành xác nhận (kèm mức mới), KHÔNG gửi tin mới. Bấm `Default` → xóa override, edit xác nhận.
  - `/reasoning` → 2 nút: `ON` (xóa override — về config agent) và `OFF` (ép off thật). Edit message xác nhận. Hiện trạng hiện tại ghi trong message gốc.
  - Group: lệnh gọi picker vẫn writer-only (`requireChatWriter` sẵn); nút trong group ai cũng bấm được — chấp nhận (R2) vì等效 với việc writer đổi giúp.
  - Callback từ message quá hạn (>10 phút hoặc đã chọn) → edit thành "expired, gõ lại /thinking" (không áp dụng).
- Non-functional: callback answered <1s; callback data ≤64 bytes (`th:<idx>` — map idx→mức lưu trong pendingPicker state, không nhét payload dài); mọi string qua i18n key (mục key làm ở Phase 4 — phase này tạm key + fallback en, KHÔNG hardcode tiếng Việt).

## Architecture

**Callback router mở rộng** (`commands_tasks.go:196-260`): thêm prefix `"th:"` → `handleThinkingCallback(ctx, query)` (file mới `pickers.go`). Payload = chỉ số nút; map idx→(command, level) lưu `pendingPickers sync.Map` keyed `messageID` khi gửi picker — chứa: levels list, expire time, sessionKey. Vì mức giới hạn cố định, có thể encode thẳng `th:<level>` (≤ `th:xhigh` = 8 bytes) — **chọn cách này**, khỏi cần pendingPickers cho level, chỉ cần expire check qua message date. Nhưng để biết "mức nào đang ✅" và sessionKey khi callback (callback không có text lệnh), lưu `pendingPickers[messageID] = pickerCtx{sessionKey, levels, chatIDStr, isGroup…}` — callback query.Message.GetChat() cho chatID (pattern `commands_tasks.go:218-221`), sessionKey tính lại bằng `chatSessionKey` với forum/dm-thread từ message gốc (lưu trong pickerCtx khi gửi).

**Panic-recover**: bọc goroutine callback như nhánh message (`channel.go:334-339` mirror sang `:345-356`).

**Capability filter**: helper `thinkingLevelsForModel(agent *store.AgentData) []string` — `LookupReasoningCapability(agent.Model)`; nil → full list; có → capability.Levels lọc bỏ `none`, append `adaptive` nếu DefaultEffort hợp lệ? KHÔNG — chỉ levels thuần + để `Default` nút riêng. Cần `resolveAgentData(ctx)` variant của `resolveAgentUUID` trả `*store.AgentData` (dùng `agentStore.GetByKey`, có sẵn channel.go:34).

**Edit flow**: bấm → AnswerCallbackQuery (text ngắn xác nhận) → EditMessageText(message đó, text xác nhận + không markup). Lỗi edit ("message is not modified") bỏ qua.

## Related Code Files

- Create: `internal/channels/telegram/pickers.go` (buildThinkingKeyboard, handleThinkingCallback, handleReasoningCallback, thinkingLevelsForModel, picker confirm render)
- Create: `internal/channels/telegram/pickers_test.go`
- Modify: `internal/channels/telegram/commands.go` (`/thinking` case → gửi picker thay vì text reply khi không tham số; có tham số text vẫn hoạt động như 3.18.0; case `/reasoning` mới; `resolveAgentData` helper)
- Modify: `internal/channels/telegram/commands_tasks.go` (dispatch thêm `"th:"`)
- Modify: `internal/channels/telegram/commands_pairing.go` (menu + `reasoning`)
- Modify: `internal/channels/telegram/channel.go` (panic-recover nhánh callback; struct field `pendingPickers sync.Map`)
- Modify: `internal/channels/telegram/commands.go` `/help` (+ `/reasoning`)

## Implementation Steps

1. `resolveAgentData` + `thinkingLevelsForModel` + unit test (model nil/có capability, lọc none).
2. `pickers.go`: build keyboard (đánh dấu ✅ mức hiện tại), gửi picker từ `/thinking`-no-arg và `/reasoning`; store pickerCtx.
3. Callback `"th:"` dispatch + handler: expire check → áp dụng qua `setChatPrefs` → edit message → answer callback. Payload scheme: `th:<level>` (level ≤6 chars) và `th:default`, `th:on`, `th:off` cho /reasoning — một prefix chung, phân biệt qua pickerCtx.kind.
4. Panic-recover wrapper.
5. Tests: fake bot caller assert EditMessageText gọi với text đúng + markup gỡ; callback áp mức → metadata đúng (fake store); expire không áp; group không writer → không keyboard.
6. Build/vet/test chuẩn.

## Success Criteria

- [ ] `/thinking` DM hiện keyboard với mức model hỗ trợ (test với model lạ = full list); bấm `high` → message edit thành "high ✓", metadata `thinking_level=high` persist (Save gọi).
- [ ] `/reasoning` OFF → request LLM kế tiếp không param thinking (unit: override "off" như 3.18.0).
- [ ] Callback data mọi nút ≤64 bytes (unit assert).
- [ ] Callback quá hạn không đổi gì ngoài edit "expired".
- [ ] Build/vet/test sạch 2 mode.

## Risk Assessment

- R1 (64 bytes) — scheme `th:` ngắn, test bound.
- R2 (group ai cũng bấm) — chấp nhận + ghi docs; nếu anh muốn chặt: thêm userID vào pickerCtx và so `query.From.ID` — 1 dòng, làm luôn nếu rẻ.
- **EditMessageText trên message có thể đã bị user xóa** → lỗi edit: catch + gửi tin thường thay thế (send fallback). Tín hiệu: log "message to edit not found" → đã có fallback.
- **pickerCtx leak** (gõ /thinking nhiều lần không bấm): TTL sweep — cleanup goroutine 15 phút quét pendingPickers xóa quá hạn (pattern defer; hoặc lazy check + cap map size 100, xóa cũ nhất).
