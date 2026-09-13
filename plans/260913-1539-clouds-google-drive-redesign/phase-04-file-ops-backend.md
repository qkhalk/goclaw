---
phase: 4
title: "File operations backend (rc + HTTP)"
status: pending
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 4: File operations backend (rc + HTTP)

## Overview

Thêm bề mặt ghi cho cloud accounts: rc wrappers (mỗi cái 3–6 dòng qua `RCClient.do()` — `internal/cloud/storage/rc_client.go:35-65`, scout §2.3) cho mkdir/delete/rename/move/copy/copyurl/publiclink/sync, rồi các method trên `StorageService` và HTTP endpoints trong `internal/http/cloud.go` theo tiền giác upload/move của `internal/http/storage.go` (scout §3.5, GAPS #9/#10/#14). Mọi endpoint yêu cầu: `requireAuth` + account accessible qua `manager.AccountByID` (`internal/cloud/account_by_id.go:12-26`) + **write guard** (owner hoặc tenant admin) + **scope check** `can_write` từ Phase 1. Endpoint `POST /v1/cloud/transfer` ghép 2 remote qua 2 lần `ensureRemote` cho copy/move giữa tài khoản.

## Surface parity

- **Backend:** toàn bộ phase này.
- **API contract:** 9 endpoint mới liệt kê ở Architecture — cần document (request/response shape, error codes `cloud_write_scope_required` 403, `storageError` 502/503 mapping `cloud.go:536-543`).
- **Web UI:** phase 5/6/7 tiêu thụ.
- **CLI/runtime:** N/A bề mặt CLI; document endpoint trong docs.
- **Desktop UI:** N/A; `go build -tags sqliteonly ./...` bắt buộc xanh (không schema mới trong phase này).

## Requirements

### Functional
- Wrapper rc mới (rc_client.go): `OperationsMkdir`, `OperationsDeleteFile`, `OperationsRmdir`, `OperationsMoveFile` (đã có `OperationsCopyFile` 176-181 làm mẫu), `OperationsCopyDir` (`operations/copy`), `OperationsCopyURL`, `SyncCopy` (`sync/copy` — dùng Phase 6), `OperationsPublicLink`, `JobStatus` (`job/status` — dùng Phase 6 cho transfer async).
- HTTP endpoints (đủ danh sách ở Architecture): upload file, mkdir, rename/move, copy, copyurl, delete (file + dir), download stream, publiclink, transfer giữa accounts.
- Write guard: account owner (`acct.UserID == ctx userID`) HOẶC tenant admin (pattern `handleSetShared` `cloud.go:263-289` dùng `requireTenantAdmin`); member trên account shared chỉ đọc. (Open Question #1 cho anh — mặc định chọn owner-or-admin.)
- Scope guard: `cloud.AccountCanWrite(acct)` (Phase 1) = false → 403 `cloud_write_scope_required`.
- Download: stream file ra browser (tương analog `fetchBlob` client-side, `http-client.ts:71-79`) với size cap như `Fetch` (`storage_service.go:243-278` dùng `FetchSizeCapMB`, default 100 — `internal/config/config_cloud.go`).

### Non-functional
- Path safety: từ chối path chứa `..`, rỗng sau normalize, leading/trailing lạ — mirror guards của `storage.go:534-653` (path traversal prevention — security log `slog.Warn("security.*")` nếu reject).
- Upload size cap áp `FetchSizeCapMB` (config sẵn); temp file xóa bằng `defer` kể cả lỗi.
- Không viết load/stress test (AGENTS.md). Test = unit wrapper (httptest fake rcd) + handler test matrix auth/scope/tenant.

## Architecture

**Quyết định upload transport — chọn TEMP FILE + `operations/copyfile`, KHÔNG multipart trực tiếp vào rc:**

1. Tiền giác sẵn có: `POST /v1/storage/files` đã làm multipart → temp file → finalize (`storage.go:534-653`); `Fetch` đã dùng `OperationsCopyFile` (`storage_service.go:243-278`); wrapper `OperationsCopyFile` đã tồn tại (`rc_client.go:176-181`).
2. `RCClient.do()` là JSON-only với response cap 16MB (`rc_client.go:35-65`) — multipart cần plumbing HTTP mới toàn phần (encode boundary, stream body, error mapping), rủi ro cao hơn lợi ích.
3. Endpoint multipart `operations/uploadfile` phụ thuộc version rclone binary trên máy user (`RClonePath` config) — temp+copyfile chạy với mọi rclone.
4. Chi phí: ghi đĩa 1 lần tạm — giảm nhẹ bằng size cap + defer cleanup. `srcFs` cho copyfile là local path fs của rclone (backend `local`) trỏ vào thư mục temp.

**Endpoints mới (RegisterRoutes `cloud.go:57-76`):**

| Method + Path | Body/Query | Handler action (rc) |
|---|---|---|
| `POST /v1/cloud/accounts/{id}/files` | multipart `path` + `file` | temp → `OperationsCopyFile` |
| `POST /v1/cloud/accounts/{id}/folders` | `{path}` | `OperationsMkdir` |
| `PATCH /v1/cloud/accounts/{id}/files` | `{from, to}` | `OperationsMoveFile` (rename + move cùng remote) |
| `POST /v1/cloud/accounts/{id}/files/copy` | `{from, to}` | `OperationsCopyFile` |
| `POST /v1/cloud/accounts/{id}/files/copyurl` | `{url, path}` | `OperationsCopyURL` |
| `DELETE /v1/cloud/accounts/{id}/files` | `?path=&isDir=` | `OperationsDeleteFile` / `OperationsRmdir` |
| `GET /v1/cloud/accounts/{id}/files/download` | `?path=` | `OperationsCopyFile` → temp → `http.ServeContent` |
| `POST /v1/cloud/accounts/{id}/files/publiclink` | `{path}` | `OperationsPublicLink` → `{url}` |
| `POST /v1/cloud/transfer` | `{source_account_id, source_path, target_account_id, target_path, mode: "copy"\|"move"}` | 2×`ensureRemote` → file: `OperationsCopyFile`/`OperationsMoveFile`; folder: `SyncCopy` (`_async:true`) → `{job_id}` |

Tất cả: `requireAuth("")` + resolve account bằng `manager.AccountByID` (own + tenant-shared, không owner-scoped Get) + write guard + scope guard. `GET download` chỉ cần read (không write guard). Publiclink coi như write (tạo link công khai — cần guard như write).

**Layering:** `rc_client.go` (wrappers, thuần) → `storage_service.go` (methods `MkdirAccount/DeleteAccount/MoveAccount/CopyAccount/CopyURLAccount/PublicLinkAccount/DownloadAccount/TransferAccount` — pattern `ListAccount/AboutAccount` 223-238) → `cloud.go` handlers. Transfer cần `TransferService` nhỏ quản lý registry `jobID → {tenantID, userID, srcAcct, dstAcct}` in-memory (map + mutex) để poll `GET /v1/cloud/transfers/{jobID}` mà không leak giữa tenant.

## Related Code Files

**Modify:**
- `internal/cloud/storage/rc_client.go` — 9 wrappers + types (`PublicLinkInfo`)
- `internal/cloud/storage_service.go` — methods Account* mới + `Transfer`
- `internal/http/cloud.go` — 9 routes + handlers + helpers `requireAccountWrite(ctx, acct)` và `requireCanWriteScope(acct)`
- `internal/cloud/scopes.go` (Phase 1) — sentinel dùng tại đây

**Create:**
- `internal/cloud/transfer_service.go` — job registry + poll/stop
- `internal/cloud/storage/rc_client_test.go` (nếu chưa có) — httptest fake rcd
- `internal/http/cloud_files_test.go` — handler matrix

## Implementation Steps

1. Wrappers rc (`rc_client.go`), mỗi cái theo mẫu `OperationsCopyFile` (176-181); `SyncCopy` nhận param `_async bool`; `JobStatus(jobID)` → struct `{Finished, Success, Error, ...}`. Unit test với `httptest.NewServer` fake JSON endpoint + basic-auth check.
2. `storage_service.go`: methods Account* — mỗi method: `ensureRemote` → build `fs` qua `FS()` (166-172) + `remoteSpec` (111-117) → gọi wrapper. `DownloadAccount`: `Stat` (188-198) check size cap → copyfile vào `os.MkdirTemp` → trả path; handler phục vụ rồi xóa.
3. Transfer: `TransferService` registry; `TransferAccount` resolve 2 account bằng `AccountByID` + guard write trên CẢ HAI (đích ghi, nguồn đọc — nguồn chỉ cần accessible; đích cần write guard + scope); file mode đồng bộ; folder mode `SyncCopy(_async:true)` trả jobID; method `TransferStatus(ctx, jobID)` kiểm tra ownership từ registry rồi `JobStatus`.
4. HTTP handlers + routes: middleware guards theo Architecture; error mapping: sentinel `ErrCloudWriteScopeRequired` → 403 kèm `{code:"cloud_write_scope_required"}`; còn lại qua `storageError` (536-543). Path validation helper chung `cleanCloudPath(p string) (string, error)` (reject `..`/rỗng — log `slog.Warn("security.*")`).
5. Upload handler: `r.MultipartForm` → file field `file`, `path` (thư mục đích); temp file theo mẫu `storage.go:534-653`; cap size; gọi `CopyAccount`; xóa temp defer. Tên file đích sanitize (không `/`, không `..`).
6. Tests: handler matrix (auth fail 401, member-shared ghi 403, scope cũ 403 `cloud_write_scope_required`, path `../` 400, happy path với fake storage qua interface nếu có — nếu StorageService khó fake thì test đến mức guard + error mapping). Không benchmark.
7. Document API: 9 endpoint + error codes vào docs/OpenAPI nơi dự án đang giữ (grep docs hiện có cho `/v1/cloud/` để đặt cùng chỗ).
8. Checklist: `go fix ./...`, `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...`, `go test ./internal/cloud/... ./internal/http/...`.

## Success Criteria

- [ ] 9 wrapper rc có unit test qua fake rcd (JSON + basic auth + status != 200 → error có body truncate như `do()` 35-65)
- [ ] Upload file nhỏ (multipart) vào account Google test → file xuất hiện ở `GET files` (thử tay với account thật + embedded client đã re-grant)
- [ ] Mkdir → rename → copy → delete chuỗi thao tác thành công qua curl/HTTP
- [ ] Download stream ra đúng nội dung; vượt cap → lỗi 4xx rõ ràng
- [ ] Member thường ghi account shared → 403; account chưa re-grant scope → 403 `cloud_write_scope_required`; path `../etc` → 400 + log security
- [ ] Transfer file giữa 2 account (copy) ra đúng dữ liệu; folder transfer trả job_id và poll được status
- [ ] `go build -tags sqliteonly ./...` xanh; `go vet` sạch

## Risk Assessment

| Risk | Observable signal | Response (pre-decided) |
|---|---|---|
| rclone `deletefile` xóa VÍNH VIỄN (Drive không vào trash qua rc) | User mất file vĩnh viễn | UI Phase 5 confirm dialog ghi rõ "xóa vĩnh viễn"; trash/restore để backlog (plan.md) |
| `operations/movefile` khác hành vi giữa remote (Drive rename = move cùng thư mục) | Rename lỗi hoặc dữ liệu trôi | Test rename + move 2 thư mục riêng trên Google thật; nếu rc bắt buộc fs giống, chia rename (movefile cùng fs) — luôn cùng 1 remote trong endpoint PATCH nên an toàn |
| Transfer folder quá 60s timeout rc client (`rc_client.go:25-32`) | Request treo/timeout | Folder transfer BẮT BUỘC `_async:true`; chỉ file đơn mới đồng bộ; client Phase 6 poll |
| Publiclink tạo link công khai vô thời hạn ngoài ý muốn | Vấn đề bảo mật dữ liệu | Guard như write + UI cảnh báo link công khai; note trong docs |
| Temp file rò đĩa khi panic giữa copy | `df` đầy dần | `defer os.Remove` ngay sau tạo; download xóa sau ServeContent (goroutine cleanup sau write) |
| Quên guard write trên 1 trong 9 endpoint | Member/scope-cũ ghi được | Bước 6 matrix test cho TẤT CẢ endpoint ghi — mỗi test case chạy đủ 9 |
