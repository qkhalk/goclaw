---
title: "Phase 6: Web: subagents UI kiểu Paseo"
status: todo
priority: P1
effort: "1.5d"
dependencies: [5]
---

# Phase 6: Web: subagents UI kiểu Paseo

## Overview
Surface subagents trong /chat theo triết lý Paseo (chat = bề mặt hội thoại, tracker = pill trên composer, chi tiết trong panel — đúng kế thừa plan 260912-1137): pill "Subagents" cạnh TeamTasksPill, panel quản lí (status/cancel/archive), timeline hiển thị card "subagent hoàn thành".

## Requirements
- Functional: pill đếm active/completed xuất hiện khi agent có subagent task; click mở panel liệt kê (label, status icon, model, thời gian, summary); nút Cancel (đang chạy) + Archive (đã xong) + "Archive tất cả đã xong"; announce message trong timeline render thành card có nút archive inline.
- Non-functional: mobile — panel là Sheet full-screen `max-sm:inset-0` (dialog pattern), pill ≥44px touch; polling 5s chỉ khi có task active (không poll vô hạn); i18n ×5 (en,vi,zh,ko,ru) namespace `chat.json`.

## Architecture
- **Hook** `use-subagents.ts` (pages/chat/hooks/): react-query key `["subagents", agentId]`, queryFn gọi WS `subagents.list` (phase 5); `refetchInterval` = 5s khi có task queued/running/waiting, false khi tất cả terminal; invalidate khi nhận announce run mới (use-chat-messages.ts:252-254 đã nhận announce — hook vào đó) và khi pill mount.
- **Pill** `subagents-pill.tsx` cạnh `<TeamTasksPill>` (`chat-page.tsx:381` pattern): icon + "N đang chạy" / "M xong" (chỉ hiện khi >0); click → mở SubagentsSheet.
- **Sheet** `subagents-sheet.tsx` (components/chat/): list rows — status icon mapping dùng đúng bộ vocabulary queued/running/waiting/completed/failed/cancelled (khớp Telegram `commands_subagents.go:19-34` để đồng bộ cảm nhận 2 bề mặt); row actions: Cancel (confirmation) → WS cancel; Archive → WS archive + optimistic remove; footer "Archive tất cả đã xong" → archive_completed.
- **Timeline card**: message announce (RunKind="announce") hiện đang render như message thường — augment message-bubble variant "subagent-result": header "Subagent hoàn thành · label", body summary, action Archive inline (dùng cùng mutation).
- **Empty state**: chưa có subagent nào → panel hiện hướng dẫn ngắn "Agent của bạn có thể tự spawn subagent bằng tool spawn — thử: 'chia việc này thành 3 subagent'" (giúp anh phát hiện feature).

## Related Code Files
- Create: `ui/web/src/pages/chat/hooks/use-subagents.ts`, `ui/web/src/components/chat/subagents-pill.tsx`, `ui/web/src/components/chat/subagents-sheet.tsx`, augment `ui/web/src/components/chat/message-bubble.tsx` (announce variant)
- Modify: `ui/web/src/pages/chat/chat-page.tsx` (render pill cạnh TeamTasksPill :381), `ui/web/src/pages/chat/hooks/use-chat-messages.ts` (invalidate query), `ui/web/src/api/protocol.ts` (constants phase 5)
- Reference: `ui/web/src/pages/chat/components/team-tasks-pill.tsx` (pattern pill), `internal/channels/telegram/commands_subagents.go:19-34` (status icon vocabulary), plan 260912-1137 (Paseo philosophy)

## Implementation Steps
1. protocol.ts constants + hook use-subagents (WS method caller theo pattern hook khác trong pages/chat/hooks/).
2. Pill + wire vào chat-page (điều kiện agent có subagent capability — hiển thị luôn, đơn giản).
3. Sheet: list + actions (cancel/archive/archive-completed) + i18n keys `chat.subagents.*` ×5.
4. Timeline announce variant + archive inline.
5. Mobile pass: sheet full-screen mobile, pill touch target, test 375px.
6. E2E tay trên server: chat "spawn 2 subagent tóm tắt A và B" → pill 2 running → xong → panel archive → pill biến mất.

## Success Criteria
- [ ] Pill + panel phản ánh đúng trạng thái spawn thật (E2E tay)
- [ ] Cancel dừng task (status → cancelled trong <2s), Archive làm row biến mất và list Telegram cũng mất
- [ ] Không poll khi không có task active (verify react-query devtools/log)
- [ ] i18n đủ 5 locale, không chuỗi hardcode

## Risk Assessment
- Agent không chủ động dùng spawn (LLM không biết): roster prompt đã 注入; thêm 1 dòng gợi ý trong system prompt template bootstrap (SOUL/IDENTITY editor-safe) — chỉ 1 câu, tránh phình prompt.
- Poll dồn dập khi nhiều user: query key per-agent + staleTime 3s + chỉ poll khi active có sẵn trong thiết kế; theo dõi qua tracing sau deploy.
