---
phase: 3
title: "Context meter UX: popover, mobile, live-update"
status: pending
priority: P2
effort: "1d"
dependencies: [2]
---

# Phase 3: Context meter UX: popover, mobile, live-update

## Overview

Red-team đã sửa kết luận audit: **context badge KHÔNG chết** — WS `sessions.list` trả `SessionInfoRich` nguyên trạng (handler `internal/gateway/methods/sessions.go:72-76` gọi `ListPagedRich`, store struct `session_store.go:95-104` có sẵn `estimatedTokens/contextWindow/compactionCount`; PG SQL `sessions_list.go:175-181`; SQLite có `ListPagedRich` + test `sessions_display_tokens_integration_test.go:68`). Vấn đề còn lại là UX, theo pattern Paseo "context meter được khen":

1. Badge chỉ có `title` tooltip — không click/popover được.
2. Badge `hidden sm:flex` — mất hoàn toàn trên mobile (Paseo: context usage luôn nhìn thấy được).
3. Live-update: `session.updated` là protocol event có thật (`ui/web/src/api/protocol.ts:322`, hook chat đã nghe) nhưng backend broadcast payload/dịp phát CHƯA verify — có thể chỉ phát khi patch metadata chứ không phát sau mỗi run.

## Requirements

- Functional:
  - Badge click → popover: used/max (mono), % với color ramp giữ nguyên, số lần compact, lần compact gần nhất (`metadata.last_compaction_at` — đã được ghi bởi `loop_pipeline_callbacks.go:916` `SessionMetaKeyLastCompactionAt`), nút "Xem chi tiết session" → `/sessions/:key`.
  - Badge + popover hiển thị cả mobile (bỏ `hidden sm:flex`, đổi sang responsive size).
  - Sau mỗi run của session, badge tự cập nhật số token (không cần refetch list): verify backend đã broadcast `session.updated` với đủ field sau run; nếu chưa — thêm broadcast additive (map từ `SessionInfoRich` của session đó, phát khi pipeline kết thúc run).
- Non-functional:
  - Nếu phải sửa backend: **additive only** (field optional), không đổi field cũ; cả PG + SQLite paths; `go build -tags sqliteonly ./...` phải pass.
  - Không N+1: popover đọc từ session object đã có trong state; KHÔNG gọi thêm WS per-render.

## Architecture

- `chat-top-bar.tsx`: tách badge block (`:111-127`) thành component `context-meter.tsx` nhận `session`; popover dùng pattern portal dropdown sẵn có của dự án (KHÔNG có Radix Popover wrapper — dự án dùng `radix-ui` unified package nhưng chat dropdowns đều là `createPortal` thủ công, xem `workspace-picker.tsx:93`, `agent-selector.tsx:81`; AGENTS.md: custom portal cần `pointer-events-auto` nếu nằm trong Radix Dialog — ở đây không trong dialog nên an toàn).
- Live-update: listener `session.updated` đã có ở hook chat — verify payload; nếu backend chỉ broadcast khi `handlePatch` thì thêm 1 phát ở cuối pipeline run (điểm: nơi mà `last_prompt_tokens` được ghi vào metadata — `loop_history_sanitize.go:433` gọn nhất, emit event bus/WS broadcast sau khi save thành công).

## Related Code Files

- Modify: `ui/web/src/components/chat/chat-top-bar.tsx`, `ui/web/src/pages/chat/hooks/use-chat-sessions.ts` (chỉ nếu cần map thêm field vào listener)
- Create: `ui/web/src/components/chat/context-meter.tsx`, `ui/web/src/i18n/locales/{en,ko,ru,vi,zh}/chat.json` thêm key `chat.context.*`
- Modify (chỉ nếu backend thiếu broadcast): nơi pipeline lưu `last_prompt_tokens` (`internal/agent/loop_history_sanitize.go` quanh `:433`) hoặc nơi đã có WS broadcast helper — scout tại chỗ, phát `session.updated` payload map đủ field

## Implementation Steps

1. i18n key `chat.context.*` × 5 locale TRƯỚC.
2. Tách `context-meter.tsx` (badge + popover + link); mount lại vào top bar; bỏ `hidden sm:flex`.
3. Verify backend: grep nơi broadcast `session.updated` (UI protocol.ts:322 có event name; backend broadcast path cần tìm — `internal/gateway/` hoặc events bus). Chạy 1 run thật trên server test → xem client có nhận event sau run không.
4a. Nếu event đã phát đủ field → không đụng backend, xong.
4b. Nếu thiếu → thêm broadcast additive ở backend (1 chỗ, map SessionInfoRich), test Go package liên quan.
5. Checks: `pnpm build`; nếu đụng backend: `go build ./... && go vet ./... && go build -tags sqliteonly ./...` + `go test` package.
6. Live verify server test: mở session có hội thoại → badge hiện số thật; bấm → popover đủ 4 mục + link; chạy 1 run → số tăng không reload; thu nhỏ viewport 375px → badge vẫn hiện.

## Success Criteria

- [x] Popover mở/đóng, đủ used/max/%, compaction count, last compaction, link session
- [x] Badge hiện trên mobile viewport 375px (không vỡ layout top bar)
- [x] Sau 1 run, badge tự cập nhật (hoặc ghi nhận backend không hỗ trợ và đã thêm broadcast additive)
- [x] `pnpm build` pass; nếu đụng backend: build + vet + sqliteonly + test pass

## Risk Assessment

- **`session.updated` emission chưa rõ** (rủi ro chính của phase): UI nghe event này (protocol.ts:322) nên nó tồn tại trong contract; nếu backend phát từ nơi không có access đến Rich data → phải map tay 3 field từ session store (1 query `Get`-style, không N+1 vì chỉ phát 1 lần cuối run). Tín hiệu gãy: không tìm thấy broadcast nào tên `session.updated` trong `internal/` → khi đó badge vẫn hoạt động khi vào lại session (list refetch), và live-update thành mục "không làm nếu tốn quá 1 điểm sửa backend" — báo lại anh thay vì lồng ghép phức tạp.
- **Popover custom portal**: dự án không có Radix Popover wrapper; tự viết như workspace-picker. Tín hiệu gãy: click trong popover đóng ngay (outside-click handler sai) → copy pattern portal của `workspace-picker.tsx:93-222` (đã xử lý outside click).
- **Mobile space**: top bar mobile chật — badge thu thành chỉ % (dạng `42%`); popover giữ full info.
