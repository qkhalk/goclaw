---
title: "Phase 5: Designer engine: prompt assembly + SSE chat + PPTX studio"
status: todo
priority: P1
effort: "2d"
dependencies: [4]
---

# Phase 5: Designer engine: prompt assembly + SSE chat + PPTX studio

## Overview
"Bộ não" designer của GoTools: endpoint SSE `/v1/designer/chat` stream phản hồi LLM (system prompt + skill inline → 1 call, không agent loop), lưu history SQLite. Port toàn bộ PPTX Studio (trang + designer column + export pptxgenjs client-side).

## Requirements
- Functional: chat với "PPTX Designer" sinh deck theo markdown fence blocks mà `parse-deck-blocks.ts` parse sẵn; stream chữ từng delta; abort dừng sạch; history theo convId persist + list + delete.
- Non-functional: goroutine leak = 0 khi client ngắt (`go test -race` + ctx cancel); prompt tổng < 24KB (skill inline có chọn lọc); response format GIỮ contract parse hiện có.

## Architecture
- **Prompt assembly** `internal/designer/prompt.go`:
  ```
  system = basePrompt(kind)                          // port + tinh chỉnh từ internal/pptx/designer_agent.go
          + "\n\n" + skillFile("pptx-deck-design")   // assets/skills/pptx-deck-design/SKILL.md
          + "\n\n" + skillFile("pptx-visual-style")  // assets/skills/pptx-visual-style/SKILL.md
          + outputContract(kind)                      // ràng buộc fence blocks format (copy từ prompt gốc)
  messages = [system] + history(designer_sessions, 20 turn gần nhất) + [user message]
  ```
  Skills embed: `//go:embed assets/skills/*` (copy SKILL.md từ goclaw `bundled-skills/` — chúng là prompt thuần, allowlist designer goclaw vốn chỉ skill_search/use_skill).
- **SSE handler** `/v1/designer/chat`:
  - Request `{convId, kind: "pptx"|"video", message}`; response `text/event-stream`.
  - Events: `data: {"type":"delta","text":"..."}` / `{"type":"done","usage":{...}}` / `{"type":"error","message":"..."}`.
  - Stream qua fetch-stream client (Authorization header được — KHÔNG EventSource, KHÔNG ticket).
  - Client ngắt (ctx.Err) → cancel upstream LLM call, không ghi half-message (hoặc ghi kèm flag truncated — chọn: discard).
  - Xong: save user + assistant message vào designer_sessions.
- **Caller**: dùng `internal/providers.ChatStream` phase 4; model/provider resolve từ settings `designer.pptx.*`.
- **Session endpoints**: GET `/v1/designer/sessions?kind=pptx` (convId, firstMessage truncate 60 chars làm title, updated_at); DELETE theo convId.
- **PPTX Studio UI port** — copy vào `web/src/port/pages/tools/pptx/`:
  page `pptx-tool-page.tsx`, components: deck-card, designer-column, icon-picker, slide-editor, slide-view; lib: icon-library, parse-deck-blocks, pptx-export (pptxgenjs), sample-deck, slide-spec, types.
- **Hook** `use-designer-chat.ts` port: đổi transport WS (`Methods.CHAT_*`) → fetch-stream `/v1/designer/chat`; giữ interface (messages/send/abort/busy) để designer-column giữ nguyên logic; history load từ `/v1/designer/sessions`.
- Chat input port: `components/chat/chat-input.tsx` + `active-run-zone` + `message-bubble` (subset render markdown) — nhập prompt goclaw chat stack đã biết.

## Related Code Files
- Create: `internal/designer/` (prompt.go, service.go, sse.go), `assets/skills/{pptx-deck-design,pptx-visual-style}/SKILL.md` (copy), `web/src/port/pages/tools/pptx/**`, `web/src/port/components/chat/**` (subset), `web/src/app/hooks/use-designer-chat.ts`
- Reference: `internal/pptx/designer_agent.go:21-56` (prompt + skills gốc), `bundled-skills/pptx-deck-design/SKILL.md`, `ui/web/src/pages/tools/pptx/**` (toàn bộ trang), `ui/web/src/pages/tools/pptx/hooks/use-designer-chat.ts:21-66` (contract hook), `ui/web/src/pages/tools/pptx/lib/parse-deck-blocks.ts` (fence format)

## Implementation Steps
1. Copy skills vào assets + embed + prompt assembly + unit test (prompt chứa đủ section, < 24KB).
2. SSE handler + service (history load/save, abort) + race test với client disconnect mock.
3. use-designer-chat port fetch-stream + wire designer-column (tạm page skeleton test chat).
4. Copy PPTX page còn lại + export + sample; route /tools/pptx + nav.
5. i18n toolbox namespace pptx (en/vi).
6. E2E tay: "5 slide giới thiệu GoTools cho dev" → blocks render slide editor → sửa 1 slide → export .pptx mở PowerPoint/Google Slides; refresh → history còn; abort giữa dòng → không treo.

## Success Criteria
- [ ] Chat stream mượt (<300ms first token với provider gần), abort sạch, refresh giữ history
- [ ] Export .pptx hợp lệ (mở PowerPoint thật)
- [ ] Prompt đúng contract fence blocks ngay lần đầu (không cần retry prompt engineering > 2 vòng)
- [ ] `go test -race` xanh (SSE + provider mock)

## Risk Assessment
- Slim designer không biết "hỏi lại" như agent loop (ask_options): bổ sung 1 đoạn prompt "nếu thiếu thông tin then chốt, hỏi 1-3 câu ngắn trước khi sinh deck"; nếu vẫn kém → open "connect GoClaw mode" (ngoại phạm vi, note lại).
- Token history phình: cap 20 turn + mỗi message truncate 4KB (đếm bytes) — đủ cho iteration deck.
