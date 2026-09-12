---
title: "Telegram interactive UX: inline pickers, ask-options, locale i18n, dev mode v2"
description: "Nâng cấp tương tác Telegram 2.0: /thinking và /reasoning dùng inline keyboard chọn mức (lọc theo capability của model), /skills thành danh sách nút bấm phân trang 10/page + reply-to-run, /dev nút bật tắt, tool ask-options để agent hỏi làm rõ bằng 3 lựa chọn + Other (bấm hoặc reply), ngôn ngữ lệnh Telegram theo locale Web UI (i18n đầy đủ 5 catalog), dev mode v2 sâu hơn."
status: pending
priority: P1
effort: "2.5d"
tags: [telegram, ux, inline-keyboard, callback-query, i18n, ask-options, dev-mode]
created: 2026-09-12
---

# Telegram interactive UX: inline pickers, ask-options, locale i18n, dev mode v2

## Overview

Bản 3.18.0 vừa ship cho lệnh dạng text (`/thinking high`). Anh muốn lên cấp tương tác: **bấm thay vì gõ** — mọi lệnh cấu hình hiện inline keyboard; agent khi chưa chắc thì **hỏi bằng nút chọn** (3 lựa chọn + Other) như CLI agent; ngôn ngữ phản hồi theo **locale Web UI** thay vì Telegram client; dev mode nội dung hơn.

Mọi mobile-UX rule của AGENTS.md áp dụng cho web không chặn gì ở đây (Telegram surface riêng), nhưng giữ nguyên tắc: callback luôn `AnswerCallbackQuery` ngay để không treo spinner (pattern sẵn `commands_tasks.go:202-204`).

## Scope Challenge (Step 0)

- **Existing code tái dùng (đã verify session này):**
  - Callback dispatch **đã chạy production**: long polling cho phép `callback_query` (`channel.go:251`), update loop rẽ nhánh `update.CallbackQuery != nil` spawn handler với `handlerSem`/`handlerWg` (`channel.go:345-356`); `handleCallbackQuery` (`commands_tasks.go:196-260`) đã có pattern: tenant inject, AnswerCallbackQuery im lặng, prefix-dispatch `"sa:"`/`"td:"`. Không xây router mới — mở rộng prefix-dispatch.
  - Inline keyboard **đã dùng**: `/tasks` (`commands_tasks.go:117-129`) và `/subagents` (`commands_subagents.go:105-117`) build `telego.InlineKeyboardMarkup` thủ công. `tu.InlineKeyboard` + `EditMessageText`/`EditMessageReplyMarkup` có sẵn trong telego v1.6.0 (methods.go:4227-4276, telegoutil methods.go:53-61) — chưa dùng, giờ dùng.
  - Reply-to detection **đã có**: `extractReplyInfo` (`context.go:122-157`) capture `ReplyToMessage` → `[Replying to X | reply_to_message_id=N]` vào content (`context.go:72-75`). Phase 3 móc vào đây.
  - Capability theo model: `providers.LookupReasoningCapability(model)` là hàm thuần (`reasoning_capability.go:47-60`, table GPT-5/Codex `:27-45`) — channel gọi trực tiếp với `AgentData.Model` (lấy qua `agentStore.GetByKey`, pattern `resolveAgentUUID` `commands.go:19-37` cần trả về full AgentData). Model lạ (mimo) → nil → hiện full list.
  - i18n: `i18n.T(locale, key)` (`i18n.go:35`) + **5 catalog** en/ko/ru/vi/zh (verify ls) — mọi string lệnh mới phải đủ cả 5. Channel WhatsApp đã có precedent `i18n.T` (`whatsapp/stt.go:49`).
  - Session prefs nền (metadata + Set-then-Save + chatSessionKey) từ 3.18.0 (`session_prefs.go`) — picker chỉ thay lớp UI, không đụng persistence.
- **Requested scope:** 6 mục anh liệt kê (thinking/reasoning picker, skills phân trang, dev nút, ask-options 3+Other, locale Web UI, dev mode sâu hơn + gợi ý skill/feature). Deliver đủ.
- **Complexity:** ~20 file, 0 migration (ask pending state in-memory; locale lưu `tenant_users.metadata` — cột có sẵn migration 000027:31). 5 phase.
- **Mode: HOLD SCOPE** (hard: scout đã chạy; red-team sau khi viết).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | `/thinking` → inline keyboard: các mức hợp lệ của model (capability filter, fallback full list) + `Default`; `/reasoning` → 2 nút On/Off (On = về mức agent config, Off = tắt thật). Bấm = áp dụng + edit tin nhắn xác nhận | P1 |
| 2 | `/skills` → danh sách nút bấm **10 skill/page** (◀ ▶ phân trang), bấm skill → edit message hiện mô tả + hướng dẫn reply-to-run; reply tin nhắn đó với yêu cầu → chạy skill | P1 |
| 3 | Tool **ask_options** cho agent: khi chưa rõ gửi câu hỏi kèm ≤4 lựa chọn + nút `Other` — user bấm nút HOẶC reply tin nhắn hỏi; câu trả lời vào session như một lượt user thường | P1 |
| 4 | Locale lệnh Telegram theo **locale Web UI** (persist lúc WS connect vào `tenant_users.metadata.locale`), fallback Telegram client language, fallback en; toàn bộ string lệnh qua `i18n.T` + 5 catalog | P1 |
| 5 | Dev mode v2: prompt section mở rộng (dùng ask_options khi mơ hồ, verify trước khi kết luận), `/dev` nút ON/OFF, status line dev chế độ hiện chi tiết | P2 |
| 6 | Backlog gợi ý (không phase riêng, xem mục Suggestions) | P3 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Interactive pickers: /thinking + /reasoning](./phase-01-start.md) | Pending |
| 2 | [/skills phân trang + reply-to-run](./phase-02-skills-interactive-list.md) | Pending |
| 3 | [Ask-options tool (3 lựa chọn + Other)](./phase-03-ask-options-tool.md) | Pending |
| 4 | [Locale Web UI + i18n lệnh](./phase-04-locale-i18n.md) | Pending |
| 5 | [Dev mode v2 + polish](./phase-05-dev-mode-v2-polish.md) | Pending |

Thứ tự: 1 → 2 dùng chung callback router mở rộng (làm ở 1). 3 độc lập sau 1 (dùng reply-to). 4 độc lập. 5 cuối (ăn theo 3).

## Success Criteria

- [ ] Gõ `/thinking` thấy keyboard nút mức (lọc theo model khi biết, đủ `Default`); bấm nút → tin nhắn edit thành xác nhận mức mới, không spam tin mới; metadata persist đúng (tin kế tiếp nhận override).
- [ ] `/reasoning` → 2 nút; Off làm request LLM mất param thinking (span verify như 3.18.0); On xóa override về config.
- [ ] `/skills` → nút 10/page + ◀/▶; điều hướng edit chính message đó; bấm skill → mô tả; reply message đó "làm X đi" → tin nhập cảnh chứa tag chạy skill `<slug>` (trace verify skillFilter).
- [ ] Trong dev mode (hoặc model tự gọi), `ask_options` với 3 lựa chọn + Other: bấm nút → lượt user tiếp theo có nội dung `[Answer] <lựa chọn>`; gõ reply tự do cũng vậy; không bấm gì thì agent vẫn kết thúc lượt bình thường (không treo).
- [ ] Đổi language Web UI → connect WS → lệnh Telegram phản hồi bằng ngôn ngữ đó (verify vi + en); Telegram client language chỉ là fallback.
- [ ] Callback nào cũng AnswerCallbackQuery trong <1s (không spinner treo).
- [ ] `go build` 2 mode + `go vet` sạch; test mới pass; **0 string lệnh hardcode** — mọi string qua key i18n (grep check).
- [ ] Không migration schema.

## Cross-Plan Relationships

- Xây trên nền plan `260912-0312-...` (đã completed, ship 3.18.0): tái dùng `session_prefs.go` (persistence), callback infra, matching normalization. Không chỉnh plan cũ.
- Không plan mở nào khác đụng `internal/channels/telegram/`.

## Surface Parity (kỳ vọng khi hoàn tất)

- **Gateway server:** telegram channel (callback router + 3 lệnh mới dạng picker + reply-to hook), consumer (ask-answer inbound), tool registry (+ask_options), gateway router (persist locale khi connect), tenant store (đọc/ghi metadata locale).
- **API contract:** tool mới `ask_options` = contract LLM (schema tool) — thêm vào docs tool nếu có danh sách; WS connect shape KHÔNG đổi (chỉ đọc param locale sẵn có). N/A còn lại.
- **Web UI:** N/A — không màn hình đổi; locale được UI gửi sẵn ở connect (`router.go:140,149`).
- **CLI/runtime package:** N/A.
- **Docs:** update `docs/25-telegram-runtime-commands.md` (pickers, ask-options, locale) + docs skills runtime nếu liệt kê tool.
- **Migrations:** KHÔNG — locale dùng `tenant_users.metadata` (000027:31 có sẵn), ask state in-memory.

## Key References (verify-passed 2026-09-12)

Callback & keyboard:
- `internal/channels/telegram/channel.go:251` (allowed updates có callback_query), `:345-356` (dispatch + sem/wg; **chưa có panic-recover ở nhánh này — thêm khi mở rộng**, mirror nhánh message `:334-339`).
- `internal/channels/telegram/commands_tasks.go:196-260` — `handleCallbackQuery`: tenant inject `:199-200`, AnswerCallbackQuery `:202-204`, prefix dispatch `"sa:"`/`"td:"`.
- `internal/channels/telegram/commands_tasks.go:117-129`, `commands_subagents.go:105-117` — pattern build InlineKeyboardMarkup thủ công.
- telego v1.6.0: `InlineKeyboardButton{Text, CallbackData}` (types.go:3103-3124, CallbackData 1-64 bytes), `EditMessageTextParams{ChatID, MessageID, Text, ParseMode, ReplyMarkup}` (methods.go:4227-4261) + `EditMessageText`, `EditMessageReplyMarkup` (methods.go:4511-4545); `tu.InlineKeyboard` (telegoutil/types.go:203-205).

Reply-to:
- `internal/channels/telegram/context.go:29-34` (ReplyInfo có MessageID), `:122-157` extractReplyInfo, `:55-84` enrich `[Replying to X | reply_to_message_id=N]`; call site `handlers.go:257-258` (SAU handleBotCommand — lệnh không thấy reply context; Phase 3 cần hook riêng trước đó cho pending asks).

Thinking & capability:
- `internal/providers/reasoning_capability.go:10-20` (ReasoningCapability + Supports), `:27-45` (table GPT-5/Codex), `:47-60` (LookupReasoningCapability thuần, normalize model `:62-71`).
- `internal/channels/telegram/session_prefs.go` (3.18.0): `chatSessionKey`, `setChatPrefs`, `normalizeThinkingLevel`, `thinkingLevelList :127`, `handleThinkingCommand`.
- `internal/store/agent_store.go:44-52` (AgentData.Model/Provider); `commands.go:19-37` resolveAgentUUID (cần variant trả AgentData).

Locale & i18n:
- TG hôm nay: `handlers.go:681-683` (`user.LanguageCode` → MetaUserLocale), key `tools/team_metadata_keys.go:48-52`, consumer `gateway_consumer_normal.go:804-809` (inboundLocale) + `:815-825` (applyInboundContext → WithLocale :821-823).
- WS: `gateway/router.go:107-108,140,149` (connect param locale → ctx per-connection; **không persist**).
- `tenant_users.metadata JSONB` (migrations/000027:25-35, cột :31).
- `internal/i18n/i18n.go:35` T(locale, key, args...); **5 catalog**: catalog_en/ko/ru/vi/zh.go; precedent channel dùng: `whatsapp/stt.go:49`.

Pending state & synthetic inbound:
- `channel.go:42-57` sync.Map pattern (pendingDraftID :57 keyed localKey); Phase 3 thêm `pendingAsks` keyed messageID.
- Publish inbound pattern: `/reset` `commands.go:133-148` (fields Channel/SenderID/ChatID/Content/PeerKind/AgentID/UserID/TenantID/Metadata command/local_key/is_forum/message_thread_id).

ask_user hiện tại (không options):
- `internal/tools/team_tasks_tool.go:26-103` (schema flat, không có options — NOT FOUND), `team_tasks_followup.go:12-61` (executeAskUser = reminder thuần). Phase 3 làm tool RIÊNG `ask_options`, không đụn team_tasks.

## Risk Assessment

- **R1 — Callback data 64 bytes:** mọi prefix scheme phải ngắn (`th:`/`sk:`/`ak:` + payload); test bound. Tín hiệu vỡ: Telegram reject callback. Phòng: hằng số + unit test độ dài.
- **R2 — Callback spam / user khác bấm trong group:** callback không có sender gate — ai cũng bấm được nút trong group. Phòng: Phase 1 thêm check `query.From.ID` với người gọi lệnh (lưu trong callback payload? không đủ chỗ → lưu pendingPicker map key messageID → userID). Accept trade-off đơn giản: DM-only pickers trong group thì yêu cầu writer (đã có requireChatWriter). Quyết: picker command chạy như lệnh (group = writer-only); nút bấm của bất kỳ ai trong DM là OK.
- **R3 — Pending asks mất khi restart:** in-memory sync.Map — chấp nhận (câu hỏi cũ vẫn reply được qua reply_to_message_id vì enrich thêm `[Answering question]` không cần state; chỉ mất auto-reminder). Ghi docs.
- **R4 — Locale không có cross-link user TG ↔ user Web:** Web user (auth) và TG sender là 2 id space; map qua `channel_contacts.user_id` (migration 000014:8) hoặc pairing. Phase 4 bước đầu là verify link thực tế trên DB anh; nếu không liên kết được → fallback: locale lưu theo chat (session metadata) + lệnh `/language` — vẫn đạt ý "không phụ thuộc Telegram client".
- **R5 — i18n 5 catalog:** thêm string mà thiếu 1 catalog → runtime tiếng Anh lẫn; grep gate trong success criteria.
- **R6 — Model yếu gọi ask_options tùm lum hoặc không bao giờ gọi:** prompt guidance trong tool description + dev mode section; không enforce. Ghi nhận rủi ro hành vi.

## Suggestions (backlog — không phải phase, anh chọn sau)

**Dev mode v2+ (sau phase 5):** preset "Deep" (thinking high + ask-first + verbose plan), auto-attach file workspace summary, tắt placeholder emoji khi dev; `/context` xem snapshot context hiện tại; `/mode` tổng hợp mọi toggle một chỗ.
**Skills đề xuất thêm:** `api-smoke` (curl health-suite API trước deploy), `changelog` (sinh CHANGELOG từ git log theo conventional commits), `commit-review` (review diff sắp commit), `webhook-test` (test endpoint webhook подписка/mocking), `portscan-lite` (nmap top-100 nhanh cho checklist pre-launch).
**Feature đề xuất:** reaction level per-chat qua /reactions picker; `/stats` ngày (token/cost theo ngày từ usage_snapshots); broadcast tin nhắn định kỳ từ agent (cron đã có — chỉ cần lệnh tạo); Web UI panel hiển thị + sửa per-chat prefs này.
