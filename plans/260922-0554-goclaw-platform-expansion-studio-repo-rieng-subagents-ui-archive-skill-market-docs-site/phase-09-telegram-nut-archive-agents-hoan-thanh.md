---
title: "Phase 9: Telegram: nút archive agents hoàn thành"
status: todo
priority: P1
effort: "0.5d"
dependencies: [5]
---

# Phase 9: Telegram: nút archive agents hoàn thành

## Overview
Nút archive inline trên Telegram cho subagent task đã hoàn thành: gắn vào danh sách `/subagents` (mỗi task terminal 1 nút + nút "archive tất cả đã xong") và optional keyboard trên tin nhắn announce hoàn thành. Dùng store `ArchiveByID` của phase 5.

## Requirements
- Functional: `/subagents` — task terminal (completed/failed/cancelled) có nút 🗄; bấm → tin nhắn edit xác nhận (i18n), task khỏi list (list đã lọc archived từ phase 5); keyboard có nút "Lưu trữ tất cả đã xong" khi ≥1 terminal; (stretch) tin announce hoàn thành mang inline keyboard archive.
- Non-functional: callback stateless (UUID trong callback data, re-query store lúc bấm — pattern `sa:` `commands_subagents.go:194-221`); callback data ≤64 bytes; nhóm cần quyền writer (`requireChatWriter` `session_prefs.go:71-94`); i18n key mới vào ĐỦ 5 catalog + list parity test.

## Architecture
- **Callback prefix mới `ar:`** — route trong `handleCallbackQuery` (`commands_tasks.go:196-221`, bảng prefix trung tâm): format `ar:<taskUUID>` = 39 bytes (an toàn so budget `ask_options.go:48`).
  - `ar:all:<agentUUID>` cho archive-all (47 bytes, vẫn an toàn).
- **Handler** `handleArchiveCallback` (file commands_subagents.go): AnswerCallbackQuery → check terminal status (chỉ archive được task terminal, đang chạy → toast "chưa xong") → `ArchiveByID` → EditMessageText bản list mới (re-render list `/subagents` như `commands_subagents.go:105-118` đang build) hoặc edit caption xác nhận.
- **List `/subagents`**: mỗi row terminal thêm button `🗄` cạnh nút `sa:` chi tiết hiện có; footer row "Archive completed (N)" khi N≥1.
- **Announce keyboard (stretch)**: `send.go` metadata convention — thêm `tools.MetaArchiveTask` song song `MetaAskOptions` (`send.go:256-258`); announce completion message gắn keyboard 1 nút; cần map taskID truyền qua announce payload (`subagent_exec.go:21-130` announceTask — thêm field).
- **i18n**: keys `MsgTGSubagentArchiveDone`, `MsgTGSubagentArchiveAllConfirm`, `MsgTGSubagentNotTerminal` vào `internal/i18n/keys.go:430-480` vùng MsgTG* + 5 catalog `catalog_{en,vi,zh,ko,ru}.go` + thêm vào `telegram_keys_parity_test.go:13-78` (test fail nếu thiếu).

## Related Code Files
- Modify: `internal/channels/telegram/commands_tasks.go:196-221` (prefix route), `internal/channels/telegram/commands_subagents.go` (buttons + handler mới), `internal/channels/telegram/send.go:251-258` (metadata stretch), `internal/tools/subagent_exec.go` (announce payload stretch), `internal/i18n/keys.go` + 5 catalogs + `internal/i18n/telegram_keys_parity_test.go`
- Reference: `ask_options.go:27-84` (keyboard + callback budget), `pickers.go:63-73` (edit-message UX)

## Implementation Steps
1. i18n keys + catalogs + parity test list (làm trước theo quy tắc AGENTS.md #15).
2. Prefix route + handler + nút trong list; test nhóm: non-writer bấm → bị chặn.
3. Archive-all button.
4. (Stretch) announce keyboard qua metadata.
5. E2E tay bot test: spawn 2 subagent qua chat → /subagents thấy nút 🗄 → archive 1 → list mất 1; archive-all → sạch.

## Success Criteria
- [ ] Nút archive hoạt động single + all, list refresh đúng (edit message)
- [ ] Bấm task đang chạy → bị từ chối với thông điệp rõ
- [ ] Group chat: chỉ writer archive được
- [ ] Parity test i18n ×5 xanh

## Risk Assessment
- EditMessageText fail (tin nhắn quá cũ >48h Telegram giới hạn): fallback gửi tin xác nhận mới; log warn.
- Announce stretch chạm format announce batch (`announce_queue.go:198-248`): nếu phức tạp quá → cắt khỏi phase (nút trong /subagents đã đủ yêu cầu anh), ghi lại decision.
