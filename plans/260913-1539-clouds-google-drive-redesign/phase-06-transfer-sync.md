---
phase: 6
title: "Cross-account transfer + sync"
status: pending
priority: P2
effort: "2d"
dependencies: [4, 5]
---

# Phase 6: Cross-account transfer + sync

## Overview

Hai tính năng "chuyển dữ liệu giữa các tài khoản": (1) **Transfer** — copy/move file hoặc thư mục từ account A sang account B ngay trong UI (dialog chọn đích, chạy bằng endpoint `POST /v1/cloud/transfer` Phase 4; folder async qua job poll); (2) **Sync** — cấu hình cặp đồng bộ A→B (one-way mirror) chạy thủ công hoặc định kỳ. Sync dùng bảng mới `cloud_sync_pairs` + **background worker riêng trong `internal/cloud`** — chọn phương án này thay vì cron (so sánh trade-off ở Architecture) vì sync cần per-pair trạng thái (last_run/status/error), tự quản remotes trong supervisor rcd, và không bắt người dùng bật `cron.command_enabled`.

## Surface parity

- **Backend:** migration `cloud_sync_pairs` (PG 121→122, SQLite 84→85) + store 3 lớp + `SyncService` worker + 5 endpoint sync-pairs + 1 endpoint transfer status.
- **API contract:** endpoint sync-pairs (CRUD + run) + `GET /v1/cloud/transfers/{jobID}` — document.
- **Web UI:** transfer dialog trong file area + section "Đồng bộ dữ liệu" trong settings sheet (Phase 2).
- **CLI/runtime:** N/A. **Desktop UI:** N/A — nhưng **SQLite store twin BẮT BUỘC** vì lite build chạy worker + store này (cloud được gate bởi `edition.Current().CloudAccountsEnabled` tại `cmd/gateway_cloud.go:63-65` — lite có gate tắt nên worker chỉ start khi gate bật; compile vẫn phải xanh `-tags sqliteonly`).

## Requirements

### Functional
- Transfer UI: chọn nhiều item (hoặc thư mục) → "Chuyển tới tài khoản khác..." → dialog chọn target account (Select, loại account nguồn) + browse folder đích (reuse `folder-picker-dialog` Phase 5) + copy|move; chạy xong toast kết quả; folder lớn hiện "đang chạy nền" với job poll.
- Sync pair config: source account+path → target account+path, lịch: manual / mỗi giờ / mỗi ngày / custom phút; enable switch; nút "Chạy ngay"; bảng pairs hiển thị last run + status (ok/error/running) + error message rút gọn.
- Worker chạy pair: `sync/copy` (additive mirror — copy những gì thiếu ở đích, **không bao giờ xóa** ở đích; exact mirror `sync/sync` để backlog theo Open Question #3). Pair chạy tuần tự, không song song giữa các pair của cùng tenant (tránh rate-limit + loop nếu A↔B).
- Manual "Chạy ngay" đặt pair vào hàng đợi worker (không chạy trong request handler — tránh timeout HTTP).

### Non-functional
- Guard: sync-pairs là cấu hình tenant-level → `requireTenantAdmin` như bindings (`cloud.go:70-72`); transfer/status guard theo Phase 4.
- Migration dual-DB: PG `migrations/000122_cloud_sync_pairs.up/down.sql` + bump `RequiredSchemaVersion` 121→122 (`internal/upgrade/version.go:5`); SQLite `schema.sql` + patch map 84→85 + bump `SchemaVersion` (`internal/store/sqlitestore/schema.go:19,97`).
- Worker shutdown sạch (context cancel) khi gateway tắt; status "running" không kẹt vĩnh viễn (đánh dấu error nếu process chết — check stale `running` quá 30 phút khi start).

## Architecture

**Trade-off phương án sync (lý do chọn):**

| Phương án | Ưu | Nhược |
|---|---|---|
| `CronCommandSpec` (argv shell, DB cron có sẵn — `internal/store/cron_store.go:64-69`) | Không code worker | Bị gate `cron.command_enabled` (security flag hay tắt); cần rclone CLI riêng đọc đúng config remotes (remotes sống trong config dir của supervisor rcd — dễ lệch); không có per-pair state/progress; UI phải tự агрегate từ cron runs |
| Payload kind mới `cloud_sync` trong cron | Tận dụng scheduler | Chạm shared subsystem (cron store + WS methods + gateway) cho nhu cầu domain-specific; vẫn thiếu state table |
| **Worker riêng + bảng pairs (CHỌN)** | Self-contained trong `internal/cloud`; state per-pair (last_run/status/error) hiển thị trực tiếp; tái dùng `SyncCopy` + `JobStatus` Phase 4; SQLite twin đơn giản | Tự viết ticker + due-check (~150 dòng) |

**Schema `cloud_sync_pairs`:** `id UUID PK`, `tenant_id UUID NOT NULL`, `source_account_id UUID NOT NULL`, `source_path TEXT NOT NULL DEFAULT '/'`, `target_account_id UUID NOT NULL`, `target_path TEXT NOT NULL DEFAULT '/'`, `interval_minutes INT NOT NULL DEFAULT 0` (0 = chỉ manual), `enabled BOOLEAN NOT NULL DEFAULT TRUE`, `last_run_at TIMESTAMPTZ`, `last_status TEXT` (ok|error|running), `last_error TEXT`, `created_by UUID`, `created_at`, `updated_at`. Index `(tenant_id)`.

**Worker `internal/cloud/sync_service.go`:** `SyncService{store, manager, storage, logger}`; `Start(ctx)` goroutine ticker 60s: SELECT pairs `enabled AND interval_minutes > 0 AND (last_run_at IS NULL OR now - last_run_at >= interval)` → với mỗi pair: mark running → `ensureRemote` cả 2 → `SyncCopy(srcFs, srcRemote, dstFs, dstRemote, _async:false)` (blocking trong worker, không chịu 60s HTTP timeout — nhưng rc client timeout 60s áp cho call; folder lớn cần `_async:true` + poll `JobStatus` trong worker loop tới Finished) → mark ok/error. `RunNow(ctx, pairID)` chỉ set flag due (hoặc push channel) để worker nhặt. Start trong `cmd/gateway_cloud.go` cùng chỗ wire `StorageService` (48-55), `Shutdown` khi server stop.

**Async quyết định:** trong worker, folder sync dùng `_async:true` + poll `JobStatus` (Phase 4) để không bị rc client 60s timeout chặt; file/dir nhỏ vẫn qua cùng đường (sync/copy luôn dùng cho folder; file đơn có thể copyfile đồng bộ).

**HTTP:** `GET/POST /v1/cloud/sync-pairs`, `PUT/DELETE /v1/cloud/sync-pairs/{id}`, `POST /v1/cloud/sync-pairs/{id}/run` — admin tenant-scoped; validate account bằng `AccountByID` + provider match (mẫu `handleUpsertBinding` `cloud.go:335-384`), cấm source == target account+path trùng. `GET /v1/cloud/transfers/{jobID}` — registry Phase 4.

## Related Code Files

**Create:**
- `migrations/000122_cloud_sync_pairs.up.sql` + `.down.sql`
- `internal/store/cloud_sync_store.go` — interface `CloudSyncPairStore` (List/Create/Update/Delete/Get/MarkRunning/MarkResult) + struct
- `internal/store/pg/cloud_sync.go` — PG impl
- `internal/store/sqlitestore/cloud_sync.go` — SQLite twin
- `internal/cloud/sync_service.go` + `sync_service_test.go` — worker + due logic (clock injectable cho test)
- `ui/web/src/pages/cloud/drive/transfer-dialog.tsx`
- `ui/web/src/pages/cloud/settings/sync-section.tsx` (pairs table + create dialog)

**Modify:**
- `internal/upgrade/version.go` → 122; `internal/store/sqlitestore/schema.sql` + `schema.go` → 85
- `internal/http/cloud.go` — 6 route mới + handlers
- `cmd/gateway_cloud.go` — start/stop SyncService
- `ui/web/src/pages/cloud/hooks/use-cloud.ts` — `useCloudSyncPairs` + transfer status poll hook
- `ui/web/src/pages/cloud/drive/drive-item-menu.tsx` + `drive-shell.tsx` — action "Chuyển tới tài khoản khác..."
- `ui/web/src/pages/cloud/settings-sheet.tsx` — mount SyncSection
- 5× `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json`

## Implementation Steps

1. **Migration:** PG 000122 + bump 122; SQLite schema.sql + patch 84→85 + bump 85. Verify fresh + incremental cả 2 DB; `go build -tags sqliteonly ./...`.
2. Store 3 lớp: interface + struct `CloudSyncPair` + PG + SQLite (pattern `cloud_accounts.go`/twin). `DB query reuse` rule: không re-query account trong loop — worker resolve account 1 lần cho batch.
3. `SyncService`: ticker + due query + mark running/result; async poll với `JobStatus`; stale-running cleanup khi Start. Unit test due logic với fake clock + fake store (không cần rclone thật).
4. HTTP handlers 6 endpoint (guards + validation như Architecture); transfer status endpoint nếu chưa có từ Phase 4 (đã có thì bỏ qua).
5. `cmd/gateway_cloud.go`: start SyncService sau `newCloudStack` (48-55), wiring theo `wireCloud` (61-73) — tôn trọng edition gate; shutdown hook.
6. **i18n:** nhóm `transfer.*` + `sync.*` ×5 locale: `transfer.menu_item`, `transfer.title`, `transfer.target_account`, `transfer.target_folder`, `transfer.mode.copy/move`, `transfer.started`, `transfer.done`, `transfer.background_running`, `sync.section_title`, `sync.add_pair`, `sync.source`, `sync.target`, `sync.schedule.manual/hourly/daily/custom_minutes`, `sync.interval_minutes`, `sync.last_run`, `sync.status.ok/error/running/never`, `sync.run_now`, `sync.enabled`, `sync.delete_confirm`, `sync.same_pair_error`, `sync.empty`.
7. Transfer UI: menu item → `transfer-dialog.tsx` (target Select + folder picker reuse + mode radio); chạy → toast; folder mode poll `GET /v1/cloud/transfers/{jobID}` hiện "đang chạy nền".
8. Sync UI: `sync-section.tsx` trong settings sheet — bảng pairs (mobile `overflow-x-auto` + `min-w-[600px]`), create dialog (2 account Select sentinel `NONE` + folder picker + schedule Select), switch enable, run-now, delete confirm, badge last status, error tooltip.
9. Checklist: `go fix/build 2 tags/vet/test`; `pnpm build`; thử tay: tạo pair manual A→B chạy ngay thấy file ở đích; pair hourly để chạy 1 chu kỳ (hoặc chỉnh interval 1 phút test rồi tắt).

## Success Criteria

- [ ] Migration PG 122 + SQLite 85 fresh & incremental sạch; `sqliteonly` build xanh
- [ ] Transfer 1 file + 1 thư mục A→B (2 account Google test) đúng nội dung; move = copy + xóa nguồn
- [ ] Folder transfer lớn trả job_id, UI hiện "đang chạy nền" và poll xong
- [ ] Sync pair manual "Chạy ngay" mirror A→B (file mới ở nguồn xuất hiện ở đích); KHÔNG xóa file đích khi bỏ nguồn
- [ ] Pair hourly chạy đúng 1 lần/giờ (test với interval nhỏ); last_status/error hiển thị đúng khi nguồn sai mật khẩu/permission
- [ ] Member thường → 403 mọi endpoint sync-pairs; source == target → 400
- [ ] i18n đủ 5 locale; bảng pairs mobile scroll ngang đúng chuẩn

## Risk Assessment

| Risk | Observable signal | Response (pre-decided) |
|---|---|---|
| Loop sync khi user tạo A→B và B→A cùng paths | CPU/network tăng, file duplicate ping-pong | Additive mirror không xóa nên chỉ copy thêm một lần rồi idempotent (sync/copy bỏ qua file đã giống) — chấp nhận; cảnh báo trong UI khi phát hiện pair ngược chiều cùng paths (validate có thể thêm sau) |
| Worker kẹt status `running` sau crash | Pair mãi "running" | Cleanup stale running >30 phút khi Start (Requirements) |
| rclone `sync/copy` xóa nhầm? | File biến mất ở đích | `sync/copy` KHÔNG xóa (chỉ `sync/sync` mới xóa) — plan chỉ dùng `sync/copy`; test criterion có case "không xóa đích" |
| rc client 60s timeout với folder lớn | Sync lỗi timeout | Bước 3: `_async:true` + poll trong worker (Architecture) |
| SQLite twin thiếu → desktop crash | Lite build lỗi migrate | Bước 1 dual-DB trong cùng commit + build 2 tags (AGENTS.md checklist) |
| Quên edition gate → lite chạy worker tốn tài nguyên | Lite desktop chạy sync nền | Bước 5: start chỉ khi `wireCloud` được gọi (đã gate sẵn `cmd/gateway_cloud.go:63-65`) |
