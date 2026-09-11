---
phase: 1
title: "Nền per-chat prefs + /thinking"
status: pending
priority: P1
effort: "6h"
dependencies: []
---

# Phase 1: Nền per-chat prefs + /thinking

## Overview

Xây nền lưu preference theo chat (session metadata: store inject + key builder chung + pattern Set-then-Save) và lệnh `/thinking` xem/bật/tắt mức suy luận. Consumer đọc metadata ở điểm build `RunRequest` và áp vào `ThinkingLevelOverride` — cơ chế override đã tồn tại hoàn chỉnh, chỉ cần nguồn điều khiển từ Telegram.

## Requirements

- Functional:
  - `/thinking` (không tham số) → hiện: mức override của chat (nếu có) + mức default của agent (từ `agents.thinking_level` / `reasoning_config`).
  - `/thinking <level>` với level ∈ {`off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `auto`, `adaptive`} (chuẩn hóa qua `providers.NormalizeReasoningEffort` `reasoning_resolution.go:131-139`; `adaptive` sentinel như `thinkingOverrideFor` `chat.go:163-168`) → persist `thinking_level` vào `sessions.metadata`, reply xác nhận "hiệu lực từ tin nhắn kế tiếp".
  - `/thinking off` → FORCE DISABLE (semantics audit-verified: override "off" → `RequestEffort()` rỗng → option `OptThinkingLevel` vắng mặt → mọi provider tắt; Ollama ép `think=false`). KHÔNG phải "về default".
  - `/thinking default` (keyword riêng của lệnh TG, không phải provider level) → xóa override (set rỗng) → agent dùng cấu hình mặc định.
  - `/thinking none` → TỪ CHỐI kèm giải thích 1 dòng (audit: "none" pass-through và BẬT thinking 10k budget trên Claude — `anthropic_request.go:278-289` default branch).
  - `/thinking <giá trị sai>` → reply danh sách mức hợp lệ + mô tả ngắn. Caveat ghi trong reply khi model là Gemini-compat: "off" không tắt được thinking trên route này (mặc định high — `openai_request.go:273-284`) — chỉ hiện khi phát hiện được (provider type), không chặn.
  - Group: chỉ writer được đổi (reuse `CheckPermission` pattern `/reset` `commands.go:107-124`, copy cả fail-open khi DB lỗi `:115-117`); DM: tự do.
  - Hiệu lực: tin nhắn kế tiếp trong cùng chat có `ThinkingLevelOverride` đúng mức (verify: span/request param).
- Non-functional: handler trả lời tức thì, 0 LLM token; metadata persist xuống DB ngay (Set + Save); không migration schema.

## Architecture

Luồng đặt: Telegram update → `handleBotCommand` → case `/thinking` → `handleThinkingCommand` → build sessionKey (builder chung, xem dưới) → `sessionPrefs.SetSessionMetadata(ctx, key, {"thinking_level": lvl})` + `Save(ctx, key)` → reply HTML.

Luồng hiệu lực: inbound message → `processNormalMessage` → (trước literal `agent.RunRequest{` tại `cmd/gateway_consumer_normal.go:482`) đọc `GetSessionMetadata(ctx, sessionKey)["thinking_level"]` → nếu khác rỗng set `ThinkingLevelOverride` → loop áp tại `loop_pipeline_callbacks.go:572-574,595-596` (đã có, không sửa).

**Session key (Risk R1 — đã audit, làm đúng hướng dẫn sau):**
- Consumer build key tại `cmd/gateway_consumer_normal.go:83-110`: mặc định `sessions.BuildScopedSessionKey(agentID, msg.Channel, PeerKind(peerKind), msg.ChatID)` (:83, delegate `BuildSessionKey` `key.go:127`); nếu local_key chứa `":thread:"` → `BuildScopedThreadSessionKey` (:86-91); forum (`is_forum=="true"` + topicID>0) → `BuildGroupTopicSessionKey(agentID, msg.Channel, msg.ChatID, topicID)` (:95-101); DM thread (`dm_thread_id!=""` + threadID>0) → `BuildDMThreadSessionKey` (:104-110). Metadata `is_forum`/`dm_thread_id`/`message_thread_id` do channel gắn tại `handlers.go:699-706`; `msg.Channel` = `c.Name()` (handlers.go:758).
- Channel-side helper `chatSessionKey(agentKey, chatType, chatID, threadID)` gọi đúng builder + inputs đó (audit xác nhận tái lập được — trừ 2 residual đã ghi trong plan.md R1: voice → VoiceAgentID, routing-rules đổi agent khi msg.AgentID rỗng).
- Test characterization: 4 case (DM, group, forum topic, DM thread) — key channel tính phải bằng key consumer tính.

**Injection (Risk R8 — HAI đường, không được thiếu một):** Option mới `WithSessionPrefs(store SessionPrefsStore)`; interface hẹp trong package telegram (fake dễ trong test):
```go
type SessionPrefsStore interface {
    GetSessionMetadata(ctx context.Context, key string) map[string]string
    SetSessionMetadata(ctx context.Context, key string, metadata map[string]string)
    Save(ctx context.Context, key string) error
}
```
`*PGSessionStore` và SQLite store đều thoả (PG `pg/sessions.go:259-279` + Save; SQLite `sqlitestore/sessions.go:188-208` — verify signature Save trả error). Wire Ở CẢ HAI: (a) `FactoryWithStoresAndAudio` (`factory.go:62-77`) + caller của nó `cmd/gateway.go:1049`; (b) direct `telegram.New(...)` tại `cmd/gateway_channels_setup.go:52` (config branch KHÔNG đi qua factory — audit finding).

## Related Code Files

- Create: `internal/channels/telegram/commands_thinking.go` (interface SessionPrefsStore, WithSessionPrefs, chatSessionKey helper, handleThinkingCommand, validThinkingLevels, buildThinkingReply)
- Create: `internal/channels/telegram/commands_thinking_test.go`
- Modify: `internal/channels/telegram/commands.go` (case `/thinking` cạnh `/skills` `:211`; `/help` text `:81-100`)
- Modify: `internal/channels/telegram/commands_pairing.go` (`DefaultMenuCommands` + `{thinking, "View or set thinking level"}`)
- Modify: `internal/channels/telegram/factory.go` (param + Option), `cmd/gateway.go` (caller factory :1049), `cmd/gateway_channels_setup.go` (direct `telegram.New` :52 — Option)
- Modify: `cmd/gateway_consumer_normal.go` (đọc metadata → ThinkingLevelOverride, trước literal :482; dùng `deps.SessStore` — field có sẵn `gateway_consumer.go:59`, đã là `store.SessionStore` đầy đủ, KHÔNG type-assert)
- Modify (chỉ nếu R3 vỡ): `internal/store/sqlitestore/schema.sql` + `schema.go` migrations map + `SchemaVersion`
- Verify-only: `internal/providers/reasoning_resolution.go` (characterization test đặt ở `internal/providers/reasoning_off_test.go` nếu chưa cover "off")

## Implementation Steps

1. Unit test khóa semantics "off" (audit kết luận, chống regression): override "off" → `ResolveReasoningDecision` cho `RequestEffort()==""` (option absent). Viết 1 regression test tại `internal/providers/` nếu chưa có.
2. `commands_thinking.go`: interface + Option + helper key (theo bản đồ key ở Architecture) + handler + validator (reuse `NormalizeReasoningEffort`; bộ chấp nhận: off/minimal/low/medium/high/xhigh/auto/adaptive + keyword `default`; từ chối `none`).
3. Case switch + `/help` + menu entry + wiring CẢ HAI đường (factory + direct New — R8).
4. Consumer: đọc metadata tại điểm build RunRequest (~:482, dùng `deps.SessStore`) → set override khi != "".
5. Tests: (a) key characterization 4 case; (b) handler: set hợp lệ → store nhận đúng map + Save gọi; `off`/`default` đúng loại thao tác; `none` bị từ chối; sai giá trị → reply list; group không có perm → từ chối; hiện trạng khi có/không override; (c) consumer: metadata có `thinking_level` → RunRequest nhận override (fake store); rỗng → không đụng field.
6. `go build ./... && go build -tags sqliteonly ./... && go vet ./... && go test ./internal/channels/telegram/ ./internal/providers/ ./cmd/`.

## Success Criteria

- [ ] 4 key case khớp consumer (test xanh).
- [ ] `/thinking high` → tin kế tiếp trace/span cho thấy reasoning effort high (thử nghiệm thủ công trên dev bot hoặc span assert trong test integration nếu có harness).
- [ ] `/thinking off` → request LLM KHÔNG có param thinking/reasoning (regression test + span verify); `/thinking default` xóa override (agent default áp lại).
- [ ] Restart gateway → toggle còn nguyên (integration/unit với store thật).
- [ ] Build + vet + test sạch cả 2 mode.

## Risk Assessment

- R1 (key mismatch) — bản đồ key consumer đã audit (`gateway_consumer_normal.go:83-110`); test characterization 4 case là gate. Residual voice/routing-rules đã ghi plan.md — không chặn, ghi docs.
- R2 (mất metadata) — test bắt buộc assert Save gọi sau Set; quên Save là bug persist không phải lỗi biên dịch → gate bằng test `TestThinkingToggle_PersistsThroughSave`.
- R3 (SQLite thiếu cột) — ĐÃ RESOLVE bởi audit: `sqlitestore/schema.sql:281` có cột; chỉ cần merge key (maps.Copy sẵn), không thay cả map (SQLite tự inject `last_prompt_tokens` vào cùng map — `sessions_ops.go:39-42`).
- R4 (off semantics) — ĐÃ RESOLVE bởi audit: "off" = disable thật (option absent), "none" bị từ chối; giữ regression test. Caveat Gemini-compat ghi docs page (Phase 3).
- R8 (wiring 2 đường) — checklist impl bước 3 nêu rõ 2 site; grep verify cả hai sau khi xong.
