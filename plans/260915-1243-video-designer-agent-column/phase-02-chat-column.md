---
phase: 2
title: "Chat column trong Video Editor — layout, session, reuse chat blocks"
status: done
priority: P1
effort: "1.5d"
dependencies: ["1"]
---

# Phase 2: Chat column trong Video Editor

## Overview
Cột hội thoại designer bên phải trang /tools/video (desktop), bottom-sheet full-screen (mobile): resize + collapse + persist width; hội thoại chạy reuse hooks + components chat hiện có, session key cố định theo page mount.

## Requirements
- Functional:
  - Chat 2 chiều với `video-designer`: gửi, stream (chunk/thinking), abort, load history khi reload page (cùng 1 session key ổn định)
  - Toggle mở/đóng cột; resize kéo được; width + trạng thái mở persist qua reload (useUiStore)
  - Nút "cuộc trò chuyện mới" (sinh convId mới, reset state)
- Non-functional:
  - Không phá chat page chính: 2 surface song song độc lập (session key khác nhau)
  - Không thêm navigation/URL — cột là DOM thuần trong VideoToolPage
  - Desktop ≥1280px mới hiện cột dạng rail; <lg → bottom-sheet

## Architecture
- **Layout**: `video-tool-page.tsx` bọc nội dung hiện tại (`max-w-5xl` → `flex-1 min-w-0`) trong hàng flex `w-full max-w-[1600px]`; cột designer bên phải clone shell `chat-side-pane.tsx` (`ResizeHandle` + width từ `useUiStore`, pattern chat-page.tsx:191-204,364-380):
  - Key mới `VIDEO_DESIGNER_WIDTH` (min 320 / max 560 / default 384) + `VIDEO_DESIGNER_OPEN` (boolean) trong `ui/web/src/stores/use-ui-store.ts`
  - Header cột: tên "Video Designer" + status dot (idle/running) + nút new-chat + nút close (icon ≥44px touch)
- **Chat wiring** (tất cả reuse, không copy):
  - `useChatMessages(sessionKey, "video-designer")` — lấy messages/stream/history/status (`ui/web/src/pages/chat/hooks/use-chat-messages.ts:93`)
  - `useChatSend({ agentId: "video-designer", onMessageAdded, onExpectRun })` (`use-chat-send.ts:24`)
  - `AskOptionsProvider` wrap thread với value tự wired (template chat-page.tsx:160-171)
  - Render: `ChatThread` (props thuần) + `ActiveRunZone` + `ChatInput`
- **Session key**: `useState(() => buildSessionKey("video-designer", uniqueId()))` — dùng `lib/session-key.ts:18`; KHÔNG để empty→key transition (tránh race `skipNextHistoryRef` use-chat-messages.ts:157-192). Persist convId vào `localStorage["goclaw.video-designer-conv"]` để reload giữ history; nút new-chat xoá key đó.
- **Composer fix**: thêm prop `storageKey?: string` cho `ChatInput` (default `"goclaw.composer-override"` giữ hành vi cũ — `chat-input.tsx:26,59-65`); designer column truyền `"goclaw.composer-override:video-designer"`.
- **Mobile**: `<lg` render `Sheet` (ui/sheet.tsx) side="bottom" `max-sm:inset-0` slide-up, composer `pb-[calc(env(safe-area-inset-bottom)+12px)]`, `text-base md:text-sm`.

## Design direction (ak:frontend-design — product UI, preserve-mode trên hệ shadcn/Tailwind sẵn có)
Cột là **tool-surface**, không phải trang marketing — mọi quyết định theo craft rules: hierarchy rõ, densiti đúng ngữ cảnh công cụ, zero trang trí thừa.
- **Vai trò thị giác**: cột designer là "đối tác sáng tạo" cạnh canvas kỹ thuật. Phân biệt bằng 1 dấu hiệu duy nhất: header có icon `Palette`/`Wand2` (lucide) + accent dot trạng thái (muted khi idle / primary pulse khi running). KHÔNG đổi màu nền, KHÔNG thêm gradient/glows — giữ token `bg-background`, `border-l`, `text-muted-foreground` của chat-side-pane.
- **Density + spacing**: khớp chat pane hiện có (px-3 header, py-1.5 toolbar, gap-2). Message bubble reuse `MessageBubble` nguyên bản — designer column không style lại bubble; chỉ khác badge agent nhỏ "Designer" đầu thread.
- **Empty state** (chưa có tin nhắn): icon palette 24px muted + 1 dòng giá trị ("Mô tả video bạn muốn — agent sẽ vẽ storyboard") + 3 suggestion chips (pill, `border rounded-full px-3 py-1.5 text-sm hover:bg-accent`): "Intro 9s gradient tím 3 cảnh", "Tin công nghệ → video 30s", "Làm lại storyboard theo style tối giản". Chips là shortcut duy nhất có màu chủ đạo (text-primary).
- **Storyboard hint**: khi agent đang trả lời, status bar dưới cột hiện 1 dòng mono-xs muted "đang chốt storyboard..." (từ Phase 3 bridge) — feedback liên tục, không spinner to.
- **Resize + collapse**: đúng pattern chat pane; thêm guard **min-width canvas 480px** cho editor (bài học Bug B QA browser-panel: cột trái phải có floor cứng, không để flex nghiền nát).
- **Mobile bottom-sheet**: full-safe-area, grab-handle 32×4 rounded ở giữa top, tap ngoài đóng; composer sticky bottom với biến keyboard-height như chat.
- **Tabular-nums** cho mọi con số thời lượng/duration trong cột; dark mode auto theo token, không hardcode màu.

## Related Code Files
- Modify: `ui/web/src/pages/tools/video/video-tool-page.tsx` (layout flex + mount column)
- Create: `ui/web/src/pages/tools/video/components/designer-column.tsx` (shell + wiring)
- Create: `ui/web/src/pages/tools/video/hooks/use-designer-chat.ts` (session key mgmt + glue, đóng gói useChatMessages/useChatSend)
- Modify: `ui/web/src/components/chat/chat-input.tsx` (thêm storageKey prop — non-breaking)
- Modify: `ui/web/src/stores/use-ui-store.ts` (2 key mới)
- Read-only reference: `ui/web/src/components/chat/chat-side-pane.tsx:44`, `ui/web/src/components/shared/resize-handle.tsx`, `ui/web/src/pages/chat/chat-page.tsx:160-171,191-204`

## Implementation Steps
1. useUiStore: thêm `videoDesignerWidth`, `videoDesignerOpen` + setters
2. ChatInput: thêm `storageKey` prop (mặc định key cũ)
3. `use-designer-chat.ts`: session key từ localStorage hoặc sinh mới; expose messages/isRunning/handleSend/abort/newChat
4. `designer-column.tsx`: shell resize + header + ChatThread/ActiveRunZone/ChatInput trong AskOptionsProvider; Sheet variant cho mobile
5. `video-tool-page.tsx`: flex layout + mount column + nút toggle ở PageHeader actions
6. Verify thủ: reload giữ width + history; chat page + designer song song stream 2 session không lẫn event

## Success Criteria
- [ ] Gửi message → stream chunk hiện trong cột; abort dừng run
- [ ] Reload page: history load lại (cùng conv), width + open state giữ nguyên
- [ ] Chat page mở song song: 2 luồng stream không cản/hiện lẫn message
- [ ] Composer designer không còn chia sẻ localStorage với chat page
- [ ] <lg: cột là bottom-sheet,-safe-area OK; ≥lg: rail resize 320-560

## Risk Assessment
- `run.started` không mang sessionKey sẽ kích capture ở mọi instance (scout risk #3): trước khi ship Phase 5 verify payload; nếu thật thiếu → thêm guard `event.agentId !== "video-designer"` bên trong use-designer-chat (KHÔNG sửa use-chat-messages chung). Tín hiệu: designer column hiện "đang chạy" khi chat page chạy agent khác.
- `useChatMessages` fetch teams.tasks mỗi session (noise nhẹ): chấp nhận — designer agent không thuộc team nên response rỗng.
- ChatInput props surface lớn: chỉ thêm 1 prop optional, tsc守住 backward-compat.
