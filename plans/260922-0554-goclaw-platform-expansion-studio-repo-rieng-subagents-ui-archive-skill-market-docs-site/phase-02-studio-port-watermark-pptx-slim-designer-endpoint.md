---
title: "Phase 2: Studio: port watermark + PPTX + slim designer endpoint"
status: todo
priority: P2
effort: "2d"
dependencies: [1]
---

# Phase 2: Studio: port watermark + PPTX + slim designer endpoint

## Overview
Port 2 tool không cần backend nặng: Watermark (100% client) và PPTX Studio (export client-side pptxgenjs). Designer chat của PPTX thay WS agent-loop bằng endpoint **slim completion** mới: system prompt + skill inline → 1 LLM call stream SSE.

## Requirements
- Functional: trang Watermark chạy xoá watermark trong browser; trang PPTX tạo deck + export .pptx; designer column chat sinh deck blocks (parse-deck-blocks.ts ăn được).
- Non-functional: designer response format GIỮ NGUYÊN markdown fence blocks mà UI hiện parse (`parse-deck-blocks.ts`) — không đổi contract parse; mobile rules AGENTS.md (grid 1→2→3, input text-base).

## Architecture
- **Watermark**: copy nguyên `pages/tools/watermark/` (page + `removal-reveal.tsx`), thêm dep `@pilio/gemini-watermark-remover` (ui/web/package.json:21). Zero backend.
- **PPTX**: copy `pages/tools/pptx/` (page, deck-card, designer-column, icon-picker, slide-editor, slide-view, lib parse-deck-blocks/pptx-export/sample-deck/slide-spec, types) + dep `pptxgenjs` (^4.0.1).
- **Slim designer** `POST /v1/designer/chat` (SSE stream):
  ```
  body: {convId, message, kind: "pptx"}
  server: load history từ designer_sessions (SQLite, giữ 20 turn gần nhất)
        + system prompt = base prompt (port tinh thần internal/pptx/designer_agent.go)
          + TOÀN BỘ nội dung bundled skills pptx-deck-design + pptx-visual-style inline
        → LLM call (provider/model từ settings, phase 4 admin) → stream chunk SSE
  save: assistant message vào designer_sessions
  ```
  KHÔNG tool loop — skills designer vốn là prompt thuần (scout: allowlist chỉ skill_search/use_skill/read_file/session_status/ask_options, output là markdown blocks).
- Session list: `GET /v1/designer/sessions?kind=pptx` (port slice `GET /v1/sessions` mà pptx/components/designer-column.tsx:70 đang gọi).
- Hook: port `use-designer-chat.ts` đổi transport WS `Methods.CHAT_*` → SSE endpoint; giữ interface (messages, send, abort, busy) để designer-column ít phải sửa.

## Related Code Files
- Create (repo mới): `web/src/port/pages/watermark/*`, `web/src/port/pages/pptx/*`, `server/designer/` (handler + prompt + sse), `server/store/designer_sessions.go`
- Reference: `ui/web/src/pages/tools/pptx/**`, `ui/web/src/pages/tools/watermark/**`, `internal/pptx/designer_agent.go` (prompt + seeded skills `pptx-deck-design`, `pptx-visual-style` tại :27-30), `bundled-skills/pptx-deck-design/SKILL.md`, `bundled-skills/pptx-visual-style/SKILL.md`

## Implementation Steps
1. Copy watermark page + dep, test chạy trong shell Studio.
2. Copy pptx page + libs + dep pptxgenjs; tạm disable designer column (chưa có endpoint).
3. Viết `server/designer`: prompt assembly (base + 2 skill file inline), SSE handler (port SSE writer pattern từ `internal/providers/sse_reader.go` phía đọc — phía ghi dùng `http.Flusher`), designer_sessions store.
4. Port `use-designer-chat.ts` sang SSE (EventSource/fetch-stream), mở lại designer column; verify parse-deck-blocks nhận blocks như cũ.
5. Session list endpoint + wire vào designer-column (port use của `GET /v1/sessions`).
6. i18n: namespace `toolbox` đã copy phase 1 — thêm key mới nếu có (×5).
7. E2E tay: tạo deck từ prompt "5 slide giới thiệu goclaw" → editor render → export .pptx mở được.

## Success Criteria
- [ ] Watermark xoá được watermark ảnh test trong browser, không gọi server
- [ ] PPTX designer chat trả blocks render thành slide; export .pptx hợp lệ
- [ ] History/persistent theo convId (refresh không mất)
- [ ] Abort dừng stream sạch (server huỷ ctx, không leak goroutine — test `go test -race`)

## Risk Assessment
- Slim designer kém chất lượng hơn agent loop (không ask_options, không search skill): skills inline đầy đủ phần bù; nếu vẫn kém → fallback "connect GoClaw mode" (out of scope, ghi open question).
- SSE qua reverse proxy bị buffer: hướng dẫn deploy thêm `X-Accel-Buffering: no` header từ server + cấu hình nginx snippet trong deploy/.
