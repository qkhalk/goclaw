---
phase: 3
title: "Ask-options tool (3 lựa chọn + Other)"
status: pending
priority: P1
effort: "1d"
dependencies: ["phase-1"]
---

# Phase 3: Ask-options tool — agent hỏi khi chưa rõ

## Overview

Tool mới `ask_options` cho agent: khi chưa chắc ý user, gửi câu hỏi kèm tối đa 4 lựa chọn + nút `Other`. User bấm nút HOẶC reply tin hỏi bằng text tự do — cả hai đều thành một lượt user bình thường trong session (agent nhận được và chạy tiếp). Đây là hành vi "3 mục + Other" như CLI agent mà anh muốn.

## Requirements

- Functional:
  - Tool `ask_options` params: `question string` (bắt buộc), `options []string` (1–4 mục, mỗi mục ≤64 chars hiển thị). Tool gửi câu hỏi ra channel kèm keyboard `[opt1][opt2][opt3][Other]`, lưu pendingAsk, và **trả về tool-result** "Question sent to user. End your turn now and wait for their reply." — agent kết thúc lượt (không pause/resume machinery, đúng honesty của codebase).
  - Bấm nút lựa chọn → publish inbound `Content = "[Answering your question] <question ngắn> → <option>"` (AgentID/PeerKind/metadata đầy đủ theo pattern `/reset` commands.go:133-148) — đi vào session như tin user, agent chạy lượt mới.
  - Bấm `Other` → edit tin hỏi thêm dòng "Hãy reply tin nhắn này với ý kiến của bạn" — chờ user reply.
  - **Reply tin hỏi** (bất kỳ lúc nào trong 24h) → hook ở `handleMessage` (chung vị trí Phase 2): content = `[Answering your question] <reply text>` + enrich reply context sẵn có. Không cần state cho path này (đọc question từ chính tin được reply — text có sẵn trong ReplyInfo).
  - PendingAsk TTL 24h, sweep chung; nút đã trả lời → edit "✓ Đã trả lời" và gỡ keyboard (EditMessageReplyMarkup rỗng).
  - Dev mode (Phase 5) khuyến khích tool này; prompt mặc định của tool description đã dạy: "Use when the request is ambiguous and 2-4 distinct interpretations exist".
- Non-functional: tool hoạt động mọi channel? **KHÔNG** — v1 Telegram-only: schema tool mô tả "Telegram channel only"; channel khác → tool-result lỗi rõ ràng (giống reject internal channel của ask_user `team_tasks_followup.go:47-49`). Không đụng team_tasks ask_user (giữ nguyên reminder semantics).

## Architecture

Đường gửi: tool `ask_options` (file `internal/tools/ask_options.go`) → cần gửi message kèm keyboard: **qua bus OutboundMessage với metadata convention** `ask_options` (JSON: options list) — channel `Send()` (send.go:166) nhận thấy metadata này → gắn keyboard và **capture MessageID ngay trong send path** (mirror `sendPlaceholder` send.go:456-475 vốn trả `(*telego.Message, error)` và `PostToTopic` send.go:858-876 vốn trả sentID — Send() hiện trả error-only nên phải capture ở closure retrySend; quyết định thiết kế: keyboard gắn vào **chunk cuối** nếu câu hỏi dài phải chia). Sau gửi → store `pendingAsks[messageID]`. Metadata convention precedent: `placeholder_update` (send.go:223-225). Tool lấy chatID/channel từ context (`ToolChannelFromCtx`/`ToolChatIDFromCtx` — `tools/context_keys.go:136-139,168-171`, pattern `team_tasks_followup.go:42-45`), reject internal.

Đường trả lời:
- Callback `ak:<idx>` → map idx qua `pendingAsks[query.Message.MessageID]` → publish inbound (pattern /reset) + edit tin (✓ + gỡ markup) + AnswerCallbackQuery.
- Reply-to: hook `handleMessage` (chung Phase 2 file): nếu `reply` là tin bot có trong `pendingAsks` → prefix `[Answering your question]`; nếu reply tin bot thường mà ĐÚNG là câu hỏi ( heuristic: pendingAsks chỉ map tin hỏi thật) — không prefix với tin bot khác (tránh nhiễu).

Tool registration: `cmd/gateway_setup.go` cạnh skill_search (verify chỗ đăng ký builtin tools); NOT gated by lite? Lite: tool Telegram-only — lite có telegram channel? Lite "No channels" → tool vô nghĩa nhưng vô hại; quyết: đăng ký everywhere, runtime reject khi channel không phải telegram (đơn giản).

## Related Code Files

- Create: `internal/tools/ask_options.go` (+ test)
- Create: `internal/channels/telegram/ask_options.go` (keyboard render trong Send path, handleAskCallback, pendingAsks map, reply hook helper `transformAskReply`)
- Create: `internal/channels/telegram/ask_options_test.go`
- Modify: `internal/channels/telegram/send.go` (Send nhận metadata ask_options → gắn keyboard + store pendingAsk sau gửi)
- Modify: `internal/channels/telegram/commands_tasks.go` (dispatch `"ak:"`)
- Modify: `internal/channels/telegram/handlers.go` (reply hook, gộp với Phase 2)
- Modify: `internal/channels/telegram/channel.go` (`pendingAsks sync.Map`)
- Modify: `cmd/gateway_setup.go` + `cmd/gateway_builtin_tools.go` (builtin metadata entry, pattern read_video :79-81) + `cmd/gateway_managed.go` nếu cần đăng ký có điều kiện (:103-105)
- Modify: `internal/agent/systemprompt.go:221` area (thêm mô tả tool vào bảng tool guidance nếu có danh sách — verify; and `resolver_helpers.go` guidance nhắc ask_options)

## Implementation Steps

1. Tool + schema + test đơn vị (validate options 1-4, reject internal channel, reject non-telegram).
2. Channel Send metadata convention + pendingAsks store + keyboard (unit với recording caller: markup có N+1 nút).
3. Callback `ak:` + publish inbound + edit/answer (unit: publish content đúng format, metadata đủ fields kiểu /reset).
4. Reply-to hook + transform test.
5. Manual E2E trên dev bot: yêu cầu mơ hồ trong dev mode → agent hỏi 3 lựa chọn → bấm → agent làm tiếp đúng hướng lựa chọn.
6. Build/vet/test chuẩn.

## Success Criteria

- [ ] Agent gọi ask_options với 3 lựa chọn → user thấy keyboard 3 + Other; bấm lựa chọn 2 → lượt mới trong session có `[Answering your question] ... → lựa chọn 2` (trace/spans verify), agent phản hồi theo lựa chọn.
- [ ] Reply text tự do vào tin hỏi cũng tạo lượt `[Answering your question] <text>`.
- [ ] Tool trả về nhắc agent kết thúc lượt; session không treo, không chạy tiếp trong khi chờ.
- [ ] Restart gateway giữa chừng: reply vẫn hoạt động (prefix qua reply-info, không phụ thuộc state); nút bấm cũ → edit "expired".
- [ ] Build/vet/test sạch 2 mode.

## Risk Assessment

- **Send() trả error-only (audit đã verify)** — không lấy messageID từ ngoài; capture bên trong send path như `sendPlaceholder` (:456-475) / `PostToTopic` (:858-876); keyboard gắn chunk cuối nếu chia tin. Nếu capture vẫn trục trặc (chunk/placeholder interaction) → **fallback self-contained đã audit-verify khả thi**: callback đọc label trực tiếp từ chính tin được bấm — chú ý `query.Message` là **interface** `MaybeInaccessibleMessage` (types.go:3243-3251): đúng chuỗi truy cập là `if m := query.Message.Message(); m != nil && m.ReplyMarkup != nil { m.ReplyMarkup.InlineKeyboard... }` (accessor `Message()` types.go:1049-1066, trả nil với InaccessibleMessage; `query.Message.ReplyMarkup` trực tiếp KHÔNG compile). Message accessible (date≠0, types.go:3283-3289) thì markup đầy đủ — label theo idx recoverable stateless.
- **Builtin tool registry**: ngoài `gateway_setup.go`, tool builtin cần entry metadata ở `cmd/gateway_builtin_tools.go` (pattern read_video :79-81: Enabled/Requires gating) + đăng ký có điều kiện kiểu `gateway_managed.go:103-105` — thiếu thì UI/config gating không thấy tool.
- **Spam ask trong group** — mention gate vẫn chặn lượt chạy tiếp; nút ai cũng bấm được (R2 đã chấp nhận).
- **Model yếu lạm dụng tool** — description chặt + dev guidance; đo bằng usage sau deploy.
