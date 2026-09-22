---
title: "Phase 8: Web: archive hoàn thành trong /chat (sessions)"
status: todo
priority: P2
effort: "1d"
dependencies: []
---

# Phase 8: Web: archive hoàn thành trong /chat (sessions)

## Overview
Giải thích thứ hai của "archive agents đã dùng rồi": lưu trữ **hội thoại** đã dùng xong khỏi list chính (hiện chỉ có Delete — `use-chat-sessions.ts:61-71`). Thêm cột `archived_at` cho sessions + WS archive/restore + sidebar section "Đã lưu trữ". Lưu trữ ≠ xoá: mở lại được.

## Requirements
- Functional: row menu session trong chat sidebar có "Lưu trữ"; session biến mất khỏi list chính, vào section "Đã lưu trữ (N)" (collapse/expand); trong section: Khôi phục / Xoá; archive không xoá data (messages còn nguyên khi khôi phục).
- Non-functional: dual-DB migration (PG + SQLite checklist AGENTS.md); sessions.list mặc định exclude archived (back-compat mọi caller); i18n ×5.

## Architecture
- Migration PG mới (số tiếp theo sau 000127 — verify lúc impl): `ALTER TABLE sessions ADD COLUMN archived_at TIMESTAMPTZ`; index partial `WHERE archived_at IS NULL` nếu query chậm (session list theo agent hiện có index — không thêm nếu không cần).
- SQLite: `internal/store/sqlitestore/schema.sql` + patch trong `migrations` map + bump `SchemaVersion`.
- Store: `SessionStore.ArchiveSession(ctx, sessionKey) / RestoreSession` + `ListPagedRich` thêm điều kiện mặc định `archived_at IS NULL` + param `IncludeArchived` (caller "section archived" truyền true rồi lọc client-side hoặc server-side filter ngược).
- WS methods `sessions.archive` / `sessions.restore` (methods/sessions.go cạnh SESSIONS_DELETE pattern) + protocol.ts constants.
- UI: `chat-sidebar.tsx` — section archived dưới list chính (chỉ render khi N>0), toggle chevron; row action menu thêm Archive (icon Box); archived rows menu: Unarchive / Delete. Không đổi ErrorBoundary key / route params (nguyên tắc AGENTS.md).

## Related Code Files
- Modify: `migrations/000128_sessions_archived.up.sql` (mới — verify số), `internal/store/session_store.go` (interface), `internal/store/pg/sessions_*.go` (Archive/Restore + list filter), `internal/store/sqlitestore/schema.sql` + `schema.go`, `internal/gateway/methods/sessions.go` (2 methods), `pkg/protocol/methods.go`, `ui/web/src/api/protocol.ts`
- Modify UI: `ui/web/src/pages/chat/chat-sidebar.tsx`, `ui/web/src/pages/chat/hooks/use-chat-sessions.ts`, i18n ×5 `chat.json`
- Reference: `ui/web/src/pages/chat/hooks/use-chat-sessions.ts:24-71` (list + delete pattern), `migrations/000034_subagent_tasks.up.sql:23` (archived_at precedent)

## Implementation Steps
1. Migration PG + SQLite (checklist dual-DB) + store methods + tests.
2. sessions.list filter mặc định — rà mọi caller của list (chat sidebar, designer columns dùng GET /v1/sessions: `video/components/designer-column.tsx:75`, `pptx/.../designer-column.tsx:70` — hành vi không đổi vì mặc định exclude).
3. WS methods + ownership (session thuộc user — pattern chat.go:285-290).
4. UI sidebar section + menus + i18n.
5. E2E tay: archive → list chính mất → section hiện → restore → về lại, messages nguyên vẹn.

## Success Criteria
- [ ] Archive/restore E2E ok, messages không mất
- [ ] Caller cũ (designer columns) không thấy khác hành vi
- [ ] Bản desktop (sqliteonly) build + migrate sạch trên DB fresh + DB cũ
- [ ] i18n ×5

## Risk Assessment
- Session key format nhiều loại (ws/cron/team/subagent — `internal/sessions/key.go`: WS direct :170-175, subagent :79-81, team :88-90, cron :105-110): archive chỉ áp menu cho session chat WS trực tiếp; sessions hệ thống (cron/team) không hiện nút — lọc theo prefix.
- Nếu validate chốt "chỉ cần subagent archive" (open question #1) → phase này hạ P3/hoãn, không vức phí (migration thiết kế không đụng gì phase 6/9).
