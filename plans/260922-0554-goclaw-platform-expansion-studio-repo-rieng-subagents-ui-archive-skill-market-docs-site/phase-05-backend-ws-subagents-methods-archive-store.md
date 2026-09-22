---
title: "Phase 5: Backend: WS subagents methods + archive store"
status: todo
priority: P1
effort: "1d"
dependencies: []
---

# Phase 5: Backend: WS subagents methods + archive store

## Overview
Mở lộ `subagent_tasks` ra UI: bộ WS methods `subagents.*` (list/get/archive/cancel) + vá store (ListByParent lọc archived, thêm ArchiveByID per-ID, CancelByID). Đây là nền cho phase 6 (web UI) và phase 9 (Telegram).

## Requirements
- Functional: WS `subagents.list` (theo agentId hoặc sessionKey, filter status, includeArchived), `subagents.archive {taskId}`, `subagents.archive_completed {agentId}` (batch), `subagents.cancel {taskId}`; archived biến mất khỏi list mặc định.
- Non-functional: ownership check — user chỉ thấy task của agent mình (pattern event_filter.go:51-56 agent events by UserID); dual-DB (PG + SQLite) song song; i18n key backend nếu trả error message cho user (keys.go + 5 catalog).

## Architecture
- **Store interface** mở rộng `internal/store/subagent_store.go`:
  - `ListByParent(ctx, rootAgentID, statusFilter string, includeArchived bool)` — thay signature (hiện :79-81 khu vực; các caller hiện tại: Telegram `/subagents` `commands_subagents.go:66`, spawn tool roster `subagent_roster.go`).
  - `ArchiveByID(ctx, taskID uuid.UUID) error` — set archived_at=NOW() WHERE status IN terminal set (`subagent_store.go:24-31`); PG impl theo pattern batch `Archive` (`pg/subagent_tasks.go:207-242`); SQLite impl theo `sqlitestore/subagent-tasks.go:241-247`.
  - `CancelByID(ctx, taskID)` — delegate sang SubagentManager cancel path (terminal set `subagent_control.go:430-432`) hoặc trả lỗi nếu không tìm thấy runtime handle.
- **WS methods** `internal/gateway/methods/subagents.go` mới, register trong `cmd/gateway_methods.go` (pattern backup schedule methods :99):
  - `subagents.list` params `{agentId?, sessionKey?, status?, includeArchived?}` → resolve root agent UUID (resolver) → store list → rows {taskId, label, status, model, createdAt, completedAt, summary( truncate 500), error}.
  - `subagents.archive/archive_completed/cancel` → validate ownership + gọi store/manager.
  - Events: task đổi trạng thái đã có announce bus message (ingest như run announce); KHÔNG thêm event WS mới ở phase này (list là polling).
- **pkg/protocol**: thêm method constants (protocol/methods.go) + `ui/web/src/api/protocol.ts` mirror.
- **Sửa bug kèm theo**: `ListByParent` hiện KHÔNG lọc `archived_at IS NULL` (`pg/subagent_tasks.go:148-179` — verify ở spot-check) → sau fix, Telegram `/subagents` và mọi consumer mặc định chỉ thấy chưa archived.

## Related Code Files
- Modify: `internal/store/subagent_store.go` (interface), `internal/store/pg/subagent_tasks.go` (ListByParent + ArchiveByID), `internal/store/sqlitestore/subagent-tasks.go` (mirror), `pkg/protocol/methods.go` (constants), `cmd/gateway_methods.go` (register), `internal/channels/telegram/commands_subagents.go:66` (truyền includeArchived=false + nút archived sau), `internal/tools/subagent_roster.go` (caller signature)
- Create: `internal/gateway/methods/subagents.go`, test files
- UI contract: `ui/web/src/api/protocol.ts` (SUBAGENTS_* constants — phase 6 dùng)

## Implementation Steps
1. Interface + PG impl + SQLite impl + unit/integration tests (PG test container pattern tests/integration).
2. Cập nhật 3 caller hiện tại của ListByParent (Telegram, roster, persist repair nếu có) — default includeArchived=false giữ hành vi roster (roster cần active + terminal gần: truyền true nếu cần).
3. WS methods + register + ownership guard + rate limit (pattern chat.go:209-322).
4. i18n error keys ×5 catalogs (keys.go + catalog_*.go).
5. Test WS qua integration test client (pattern tests/integration WS connect frame).
6. `go fix ./... && go build ./... && go build -tags sqliteonly ./... && go vet ./...`.

## Success Criteria
- [ ] `subagents.list` trả task thật của agent (spawn thử qua chat → list thấy queued→running→completed)
- [ ] `subagents.archive` 1 task completed → list mặc định mất, includeArchived=true thấy với archived_at
- [ ] User B không list được task của user A (test ownership)
- [ ] Telegram `/subagents` không còn hiện task đã bị TTL-archive (bug fix verified)

## Risk Assessment
- Signature change ListByParent làm vỡ caller khuất: grep toàn repo `ListByParent` trước khi sửa (plan đã biết 3; verify lại lúc impl — quy tắc AGENTS.md #9).
- Cancel task chưa terminal nhưng manager không có handle (restart gateway): trả trạng thái "orphan — mark cancelled" qua store UpdateStatus, ghi log security.* nếu bất thường.
