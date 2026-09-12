---
phase: 2
title: "De-noise top bar & console; i18n hết chuỗi hardcode"
status: pending
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: De-noise top bar & console; i18n hết chuỗi hardcode

## Overview

Top bar chat chỉ còn thứ liên quan hội thoại: 4 control workspace console (picker + files + jobs + terminal) thu vào 1 nút popover (quyết định D1); xóa label "Ready" và context badge UI chết (D4 — Phase 3 xây lại); mọi chuỗi hardcode tiếng Anh trong chat surface chuyển qua i18n đủ 5 locale.

## Requirements

- Functional:
  - Top bar chat mới: **agent emoji + tên | spinner (khi chạy) | 1 nút console (icon)**. Hết: text "Không có workspace" thường trực, 2 nút files/jobs, nút terminal disable thường trực.
  - Nút console mở popover chứa: WorkspacePicker (nguyên trạng dropdown + tạo workspace inline), 3 toggle Files / Jobs & Tasks / Terminal — enable/disable theo rule hiện tại (terminal cần workspace). Panel tương ứng vẫn mở ở cột phải như cũ khi bật toggle. Chỉ 1 panel mở 1 lúc (giữ mutual exclusion hiện có `chat-page.tsx:325-342`).
  - Bỏ label "Ready" (`chat-top-bar.tsx:174-176`). **GIỮ context badge** (red-team sửa: badge có data thật — `sessions.list` trả `SessionInfoRich` nguyên trạng; Phase 3 nâng cấp UX). Dọn đúng mức: không đụng badge, không đụng logic `metadata.last_compaction_at` (Phase 3 dùng).
  - i18n: mọi string user-facing trong `pages/chat/**` + `components/chat/**` qua `t()` namespace `chat`. Danh sách vị trí hardcode đã audit: phase labels `chat-top-bar.tsx:36-43`, "Team: N task(s)" `:171`, "Running Tasks"/"No active tasks" `task-panel.tsx:22,36` (nay trong pill popover Phase 1), "Team: N tasks active" `team-activity-panel.tsx:21` (đã xóa Phase 1 — chỉ cần key mới pill), "Drop files here" `drop-zone.tsx:41`, mô tả `GC_COMMANDS` `command-palette.tsx:38-53`, label nút console mới, tiêu đề 3 panel (Files / Jobs & Tasks / Terminal), empty-state strings.
- Non-functional: locale bắt buộc đủ 5: `en, ko, ru, vi, zh` (`ui/web/src/i18n/locales/` — namespace `chat.json` đã tồn tại, thêm key vào đủ 5 file). Không đổi behavior backend.

## Architecture

- Component mới `console-menu.tsx` trong `components/chat/`: 1 IconButton + Radix Popover; bên trong nhúng `WorkspacePicker` (dùng lại `use-workspaces.ts`) + 3 toggle row (truyền callback hiện có `filesPanelOpen/jobsPanelOpen/termOpen` từ `chat-page.tsx` xuống qua props, hoặc dựng context nhẹ nếu props drilling sâu — ước lượng 1 cấp thôi nên props đủ).
- `chat-top-bar.tsx` mất 4 control cũ, nhận thêm props console.
- i18n key thêm vào `chat.json` × 5 locale, prefix hợp lý: `chat.console.*`, `chat.phase.*`, `chat.dropzone.*`, `chat.command.*`.

## Related Code Files

- Modify: `ui/web/src/components/chat/chat-top-bar.tsx`, `ui/web/src/pages/chat/chat-page.tsx` (props console + giữ panels), `ui/web/src/components/chat/drop-zone.tsx`, `ui/web/src/components/chat/command-palette.tsx`, `ui/web/src/components/chat/file-explorer-panel.tsx` / `jobs-tasks-panel.tsx` / `terminal-panel.tsx` (title i18n), `ui/web/src/i18n/locales/{en,ko,ru,vi,zh}/chat.json`
- Create: `ui/web/src/components/chat/console-menu.tsx`
- Delete: không xóa file (panels còn dùng) — chỉ xóa node render trong top bar

## Implementation Steps

1. Thêm i18n key vào 5 locale TRƯỚC khi sửa component (rule AGENTS.md: thiếu key = crash runtime).
2. Tạo `console-menu.tsx` (popover: workspace picker + 3 toggle, rule enable/disable giữ nguyên).
3. Sửa `chat-top-bar.tsx`: thay 4 control bằng ConsoleMenu; xóa "Ready" + context badge + helper chết.
4. i18n hóa các string còn lại (drop-zone, command palette, panel titles, phase labels).
5. Grep xác nhận sạch: `grep -rn "\"Ready\"\|Running Tasks\|Drop files\|Team: " ui/web/src/pages/chat ui/web/src/components/chat` = 0 hit hardcode (chỉ còn key i18n).
6. Build `pnpm build`; kiểm tay: popover mở/đóng, toggle mở đúng panel, terminal disable khi chưa chọn workspace, tạo workspace inline vẫn hoạt động.

## Success Criteria

- [x] Top bar chat chỉ còn: tên agent, context badge (giữ nguyên, Phase 3 nâng cấp), spinner khi chạy, nút console — screenshot so sánh trước/sau
- [x] Không còn chuỗi "Không có workspace" thường trực trên top bar (chỉ trong popover khi mở)
- [x] Chuyển UI sang English/Korean/Russian/Chinese: mọi string chat hiển thị đúng ngôn ngữ (không sót tiếng Anh khi locale ≠ en)
- [x] Grep hardcode = 0; `pnpm build` pass; flow workspace tạo + mở files/terminal không hồi quy

## Risk Assessment

- **Popover lồng dropdown (WorkspacePicker là portal dropdown bên trong popover mới)**:portal trong portal có thể bị đóng theo focus. Giải pháp: ConsoleMenu dùng Radix Popover với `modal={false}`; nếu WorkspacePicker xung đột, thay bằng render thẳng list workspace trong popover ConsoleMenu (đơn giản hơn, vẫn tái dùng hook). Tín hiệu gãy: click workspace picker trong popover đóng cả popover → chuyển phương án render thẳng.
- **Người dùng đang quen 3 nút nhanh**: thay bằng 1 nút + 2 click. Chấp nhận được vì độ dùng thấp (panels phụ thuộc workspace — hiện "Không có workspace"). Không làm hotkey riêng (YAGNI).
- **Giữ context badge**: red-team đã bác kết luận "UI chết" ban đầu (đọc nhầm struct `handlePatch` params làm response list). Badge hoạt động trên PG lẫn SQLite; Phase 3 nâng cấp popover/mobile/live-update. Phase 2 KHÔNG đụng badge.
