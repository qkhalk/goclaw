---
phase: 1
title: "De-duplicate — one picker, one task surface, one fetch"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: De-duplicate — one picker, one task surface, one fetch

## Overview

Mỗi mối quan tâm trên `/chat` hiển thị đúng 1 nơi: bỏ AgentPickerPrompt grid (giữ AgentSelector sidebar), thay 3 surface team-task bằng 1 pill trên composer, run-phase chỉ còn ActivityIndicator dưới thread, `/v1/agents` fetch 1 lần qua shared react-query.

## Requirements

- Functional:
  - Chọn agent chỉ qua AgentSelector sidebar. Khi chưa có agent (`!agentConfirmed`), vùng trung tâm hiển thị empty-state gọn có 1 CTA mở dropdown AgentSelector (không còn grid thẻ agent).
  - Team task hiện qua **1 pill ngay trên composer**: `N tác vụ team đang chạy` + popover danh sách task (tái dùng render hàng task của TaskPanel cũ). Popover tự cập nhật từ `teamTasks`. KHÔNG còn TaskPanel cột phải, KHÔNG auto-open panel, KHÔNG chip "Team: N tasks" trên top bar, KHÔNG TeamActivityPanel inline trong thread.
  - SystemNotification trong timeline (Task #n completed...) GIỮ NGUYÊN — đó là marker hội thoại.
  - Top bar chỉ còn spinner mờ khi `isRunning` (bỏ text phase "Thinking…/Running tool…/Responding…" —(ActivityIndicator dưới thread đã có).
  - `GET /v1/agents` gọi 1 lần/màn hình qua 1 hook `useAgents()` (react-query, queryKey chuẩn hóa) dùng chung cho AgentSelector + ChatTopBar.
- Non-functional: không thêm dependency mới; mobile không xuất hiện cột/panel mới (pill responsive, `text-base md:text-sm`).

## Architecture

- `chat-page.tsx`: bỏ state `taskPanelOpen` + effect auto-open (`:167-174`), bỏ render TaskPanel (`:324-327`); bỏ nhánh AgentPickerPrompt (`:296-304`), thay bằng empty-state CTA — cần cơ chế mở dropdown từ ngoài: thêm prop/callback `requestOpenSelector` xuống `ChatSidebar` → `AgentSelector` (controlled open 1 lần).
- Component mới `team-tasks-pill.tsx` trong `components/chat/`: nhận `teamTasks`, render pill + Popover (Radix Popover sẵn dùng trong dự án — kiểm tra pattern ở workspace-picker), nội dung popover là hàng task tái sử dụng từ `task-panel.tsx` (đặt trên composer trong `chat-page.tsx`, cùng khối với ChatInput).
- Run-phase: sửa `chat-top-bar.tsx:159-163` bỏ label text, giữ spinner thu nhỏ.
- Hook `use-agents.ts` trong `ui/web/src/hooks/`: react-query `["agents"]`, staleTime ngắn (30s); thay 3 chỗ fetch cũ (`agent-selector.tsx:33`, `chat-top-bar.tsx:56`; `agent-picker-prompt.tsx` bị xóa).

## Related Code Files

- Modify: `ui/web/src/pages/chat/chat-page.tsx`, `ui/web/src/pages/chat/chat-thread.tsx` (bỏ render TeamActivityPanel `:165`), `ui/web/src/components/chat/chat-top-bar.tsx`, `ui/web/src/components/chat/chat-sidebar.tsx`, `ui/web/src/components/chat/agent-selector.tsx`, `ui/web/src/hooks/use-chat-messages.ts` (giữ nguyên logic teamTasks, chỉ đổi consumer)
- Create: `ui/web/src/components/chat/team-tasks-pill.tsx`, `ui/web/src/hooks/use-agents.ts`
- Delete: `ui/web/src/components/chat/agent-picker-prompt.tsx`, `ui/web/src/components/chat/team-activity-panel.tsx`, `ui/web/src/components/chat/task-panel.tsx` (sau khi đã chuyển render hàng task sang pill popover — grep `task-panel\|TeamActivityPanel\|AgentPickerPrompt` toàn `ui/web/src` trước khi xóa để bắt reference còn sót)

## Implementation Steps

1. Tạo `use-agents.ts` (react-query shared) — thay fetch trong `agent-selector.tsx` + `chat-top-bar.tsx`.
2. Tạo `team-tasks-pill.tsx` (pill + popover, tái dùng hàng task); i18n key ngay bước này (xem Phase 2 cho danh sách key — pill thuộc nhóm key phase này dùng: `chat.teamTasksPill.*` vào 5 locale).
3. Sửa `chat-page.tsx`: mount pill trên composer; bỏ TaskPanel + auto-open + AgentPickerPrompt; thêm empty-state CTA → mở AgentSelector qua prop mới.
4. Sửa `chat-thread.tsx` bỏ TeamActivityPanel; `chat-top-bar.tsx` bỏ phase text + chip team.
5. Xóa 3 component file sau khi grep sạch reference.
6. Build + test UI: `pnpm build` trong `ui/web`; kiểm tay flow: chưa chọn agent → CTA mở selector; team task mô phỏng → pill hiện, popover mở được, timeline vẫn có notification.

## Success Criteria

- [x] Grep `AgentPickerPrompt|TeamActivityPanel|TaskPanel` trong `ui/web/src` = 0 kết quả render (chỉ còn tên file mới nếu tái dùng type)
- [x] `/chat` không còn cột phải TaskPanel; pill trên composer hiện đúng số task + popover hoạt động
- [x] DevTools Network: vào `/chat` chỉ 1 request `/v1/agents`
- [x] `pnpm build` pass; mobile viewport (375px) không layout vỡ

## Risk Assessment

- **Radix Popover trong portal + Radix Dialog (nếu popover mở trong dialog)**: AGENTS.md đã cảnh báo `pointer-events-auto` cho custom portal dropdown — dùng Radix Popover/Popover.Content chuẩn (Radix tự xử lý) như workspace-picker đang dùng; nếu tự viết portal thì phải thêm class.
- **Empty-state CTA → mở dropdown từ code**: AgentSelector hiện là uncontrolled portal dropdown; cần 1 controlled "open" nhẹ. Nếu phức tạp, fallback: CTA focus sidebar + highlight selector (không auto-open) — vẫn đạt mục tiêu 1 picker.
- **Giả định có thể gãy**: `teamTasks` dữ liệu hình dạng không đổi khi đổi consumer. Tín hiệu: type error khi build pill; xử lý: đọc lại `use-chat-team-tasks.ts` và map tại pill, không sửa producer.
