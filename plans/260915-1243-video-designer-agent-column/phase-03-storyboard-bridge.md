---
phase: 3
title: "Storyboard bridge — parse ```storyboard block, StoryboardCard, Apply vào timeline"
status: done
priority: P1
effort: "1.5d"
dependencies: ["2"]
---

# Phase 3: Storyboard bridge

## Overview
Phần tử trung tâm của tính năng: agent kết thúc reply bằng fenced block ` ```storyboard {json} ` — column parse, validate, render **StoryboardCard** (thẻ xem trước có số cảnh/thời lượng/tỉ lệ + nút "Áp dụng vào timeline"), Apply gọi đúng path `loadJson()` hiện có. Kèm nút "đính kèm storyboard hiện tại" để user nhờ agent cải thiện bản đang sửa.

## Requirements
- Functional:
  - Parse block ` ```storyboard ` từ (a) assistant message hoàn tất, (b) stream text đang chạy — card hiện ngay khi block đóng trong stream
  - Validate JSON theo contract client: JSON.parse + `hasValidationErrors()` (ràng buộc source ảnh/video) + khớp `Scene[]` shape của `use-timeline.ts` (transition/transform/filter)
  - Apply: `timeline.replaceScenes(scenes)` + `setMeta` từ `{version, canvas, output}` — storyboard thành single source of truth, undo/redo tiếp tục hoạt động
  - "Đính kèm storyboard hiện tại": button cạnh composer chèn JSON storyboard hiện hành vào message (prefix context rõ ràng)
  - Nhiều storyboard trong 1 session: card nào cũng áp được (mỗi reply 1 card, không overwrite lẫn)
- Non-functional:
  - Parse an toàn: JSON sai → card lỗi cụ thể (không crash column), stream parse không block render (throttle)
  - Apply không đổi selectedIndex ngoài 0 (chọn cảnh 1 sau replace)

## Architecture
- **Parser** `ui/web/src/pages/tools/video/lib/parse-storyboard-blocks.ts`:
  - `extractStoryboardBlocks(text): { json: string; start: number; end: number }[]` — regex fenced block `storyboard`, trim, trả raw JSON string
  - `parseStoryboard(json): { ok: true; sb: Storyboard } | { ok: false; error: string }` — JSON.parse + shape check (version 1, scenes mảng, mỗi scene type hợp lệ), dùng chung type `Storyboard` từ video-tool-page export
- **StoryboardCard** `ui/web/src/pages/tools/video/components/storyboard-card.tsx` (memorable element theo frontend-design):
  - Header: chip tỉ lệ (`9:16`, từ canvas), `"{n} cảnh"`, `"{sec}s"` — `tabular-nums`
  - Mini scene strip: mỗi cảnh 1 ô nhỏ với icon loại (image/video/color) + duration; viền accent với cảnh có transition/transform
  - Footer: nút primary "Áp dụng vào timeline" + secondary "Xem JSON" (popover)
  - Trạng thái: valid (nút Apply enable) / invalid (chip lỗi đỏ + dòng lỗi ngắn) / stale (storyboard khác đã áp sau đó — mờ nút)
  - Được render TRONG message bubble: `MessageBubble` hỗ trợ children custom? → nếu không, render card ngay dưới bubble trong thread list của column (column tự render thread qua ChatThread — ChatThread nhận props messages thuần). **Quyết định**: column KHÔNG dùng ChatThread cho phần assistant-card; thay vào đó map messages tự render `MessageBubble` + chèn StoryboardCard sau assistant message có block. Giữ ChatThread chỉ khi không cần card — đơn giản hóa: column dùng MessageBubble + StreamingText + ActiveRunZone tự lắp (đúng kiểu session-detail-page.tsx precedent dùng MessageBubble ngoài chat page).
- **Apply flow** (trong `use-designer-chat.ts`): `applyStoryboard(sb)` → gọi callback prop `onApply(sb)` từ page → page làm đúng `loadJson()` logic: `setMeta({...default, ...meta})` + `timeline.replaceScenes(scenes)`; toast thành công; đánh dấu card đã applied (state theo message id)
- **Attach current**: `onAttachCurrent()` → chèn vào text composer prefix `[Storyboad hiện tại]\n```storyboard {...}\n```\n\n` — dùng ref value của ChatInput? ChatInput là component đóng → đặt chuỗi vào state `pendingPrefix`, khi send thì nối vào message (use-designer-chat xử lý, không cần sửa ChatInput)

## Related Code Files
- Create: `ui/web/src/pages/tools/video/lib/parse-storyboard-blocks.ts`
- Create: `ui/web/src/pages/tools/video/components/storyboard-card.tsx`
- Modify: `ui/web/src/pages/tools/video/hooks/use-designer-chat.ts` (parser integration + applyStoryboard + attachCurrent)
- Modify: `ui/web/src/pages/tools/video/components/designer-column.tsx` (render MessageBubble + cards; attach button)
- Modify: `ui/web/src/pages/tools/video/video-tool-page.tsx` (onApply → loadJson path refactor: tách `applyStoryboardToEditor(sb)` dùng chung cho JSON mode + designer)
- Read-only reference: `ui/web/src/components/chat/message-bubble.tsx:15`, `ui/web/src/pages/sessions/session-detail-page.tsx:6` (precedent dùng MessageBubble ngoài chat page), `ui/web/src/pages/chat/hooks/use-chat-messages.ts:272-299` (chunk batching — parse chạy trong effect sau khi streamText đổi, throttle 300ms)

## Implementation Steps
1. Viết parser + unit test nhanh (vitest nếu có sẵn; nếu không, test bằng tsc + manual) — cases: block sạch, block cuối stream chưa đóng (bỏ qua), 2 blocks, JSON sai
2. StoryboardCard component (valid/invalid/applied states)
3. Refactor `video-tool-page.tsx`: tách `applyStoryboardToEditor(sb: Storyboard)` từ `loadJson()`; JSON mode + designer dùng chung
4. use-designer-chat: effect theo dõi messages + streamText → extract blocks → state `cards: Map<messageId, ParsedSB>`; applyStoryboard/attachCurrent
5. designer-column: render list = messages map → MessageBubble (+ StreamingText cho run đang chạy) + StoryboardCard sau assistant message; attach button cạnh composer
6. Verify end-to-end với agent thật (server up): hỏi design → card → Apply → timeline/canvas đúng

## Success Criteria
- [ ] Reply có block hợp lệ → card hiện (kể cả đang stream, khi fence đóng)
- [ ] Apply: timeline + canvas + tổng duration đúng; undo (Hoàn tác) quay lại storyboard cũ
- [ ] JSON rác trong block → card lỗi đỏ, không crash, column vẫn chat được
- [ ] Attach storyboard hiện tại → message chứa JSON context, agent trả bản sửa
- [ ] 2 cards khác nhau trong 1 session đều áp được độc lập

## Risk Assessment
- ChatThread không hỗ trợ chèn card giữa message → đã quyết định: column tự lắp MessageBubble/StreamingText/ActiveRunZone (precedent session-detail-page). Nếu tự lắp thiếu tool cards → dùng lại `MergedToolGroup` export từ chat-thread.tsx:177 hoặc chấp nhận designer agent ít dùng tool (allowlist chỉ 3 tool, hiếm khi hiện).
- Stream parse race (block đóng giữa 2 chunk) → effect re-run trên mỗi streamText change với throttle; fence chưa đóng = không match regex = an toàn.
- Storyboard type lệch giữa use-timeline Scene và wire (transition/transform/filter optional) → parser chỉ check shape lỏng (type + duration_sec number), để editor tự xử lý optional fields; sai kiểu cứng → invalid card.
