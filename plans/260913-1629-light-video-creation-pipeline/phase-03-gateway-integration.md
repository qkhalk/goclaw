---
phase: 3
title: "Phase 3: GoClaw integration — tool render_video, HTTP API, dispatcher, WS event"
status: pending
priority: P1
effort: "4-5 ngày"
dependencies: [1, 2]
---

# Phase 3: GoClaw integration — tool render_video, HTTP API, dispatcher, WS event

## Overview

Ghép pipeline vào GoClaw: agent tool **`render_video`** (nhận storyboard JSON → tạo job → trả path MP4 khi xong), HTTP API `/v1/video/*` cho Web UI, **dispatcher** đẩy job sang worker (push + poll fallback), sự kiện WS `video.job.updated`, deliverable vào workspace + tùy chọn upload cloud qua `StorageService`. Điểm neo wiring theo đúng pattern cloud stack hiện có.

**Quyết định tên tool:** `render_video` — KHÔNG dùng `create_video` vì tên này đã bị tool AI video generation (Gemini Veo/MiniMax) chiếm tại `internal/tools/create_video.go:44` (`Name() string { return "create_video" }`, đăng ký ở `cmd/gateway_managed.go:106`). Hai tool cộng tồn: `create_video` = sinh video bằng AI model (tốn credit API), `render_video` = compose storyboard cục bộ (miễn phí, nhẹ).

## Requirements

- [ ] Tool `render_video` hiện diện trong agent (Standard + Lite), interface đầy đủ `Name/Description/Parameters/Execute` theo `internal/tools/types.go:14`
- [ ] Storyboard asset path tương đối workspace được resolve + kiểm tra path traversal trước khi đẩy worker
- [ ] Dispatcher push job sang worker, poll fallback; worker chết → job `failed` sau timeout, không treo
- [ ] HTTP endpoints: `GET /v1/video/jobs`, `GET /v1/video/jobs/{id}`, `DELETE /v1/video/jobs/{id}` (cancel) — auth theo pattern `internal/http/cloud.go`
- [ ] WS event `video.job.updated` bắn khi status đổi (pattern `pkg/protocol/events.go`)
- [ ] Deliverable: MP4 vào workspace (`videos/<job-id>.mp4`), option `upload_cloud` đẩy lên account rclone qua `StorageService`
- [ ] Kill-switch `video.enabled=false` tắt sạch tool + endpoints + dispatcher
- [ ] Async completion: tool trả ngay job id (không block turn agent); khi done, kết quả đẩy về chat session như async tool completion hiện có

## Architecture

### Wiring (theo pattern cloud, đã verify)

```
cmd/gateway.go (~line 638, ngay cạnh cloud stack):
  videoStack := newVideoStack(cfg, pgStores, workspace)     // nil khi !VideoEnabled()
  defer wireVideoTools(videoStack, toolsReg, msgBus)()      // pattern wireCloudTools cmd/gateway_cloud.go:80
```

- `newVideoStack` giữ: `VideoJobs` store, `WorkerClient`, dispatcher goroutine. Gate mở đầu: `if !cfg.VideoEnabled() { return nil }` — kill-switch tắt từ gốc, không đăng ký gì cả.
- `WorkerClient` — HTTP client gọi worker, pattern `RCClient.do()` tại `internal/cloud/storage/rc_client.go:35`: một hàm `do(ctx, method, path, body, out)` dùng chung, Bearer token, timeout ngắn (2s cho poll, 10s cho submit).
- **Asset mirror:** trước khi submit, dispatcher chép asset workspace vào `assetsDir` job (nếu worker cùng máy dùng chung volume thì config `worker.assets_mode: "shared"` bỏ qua bước chép — mặc định `copy` cho máy khác).

### Dispatcher lifecycle

1. Tool tạo row `video_render_jobs` (status `queued`) → bắn WS event → trả `{jobId, status:"queued"}` cho agent ngay lập tức.
2. Dispatcher (goroutine đơn, channel signal) nhận job → materialize assets → `POST /v1/jobs` → update `rendering`.
3. Poll worker mỗi 3s (chỉ khi có job `rendering`); status đổi → update DB + bắn WS event.
4. `done` → move output vào workspace `videos/<jobId>.mp4` (+ optional cloud upload) → update row + event; `failed`/timeout (mặc định 30 phút) → update error.
5. Cleanup: job TTL 7 ngày (config) — dọn file workspace + row theo `expires_at` (store method từ Phase 1).

### Async completion về chat

Tool implement `AsyncTool` (`SetCallback`, `internal/tools/types.go:42` — `AsyncCallback` tại :39) — repo đã có pattern async completion delivery (`internal/tools/async_completion_delivery.go`, `async_completion_media.go` dùng cho `create_video` AI-gen vốn cũng long-running). Kết quả done → callback → tin nhắn hệ thống vào session kèm `MEDIA:` path (đúng convention tool media hiện có).

### HTTP surface (pattern `internal/http/cloud.go:61-72`)

```go
mux.HandleFunc("GET /v1/video/jobs", requireAuth("", h.handleListVideoJobs))
mux.HandleFunc("GET /v1/video/jobs/{id}", requireAuth("", h.handleGetVideoJob))
mux.HandleFunc("DELETE /v1/video/jobs/{id}", requireAuth("", h.handleCancelVideoJob))
```

List/query theo tenant scope từ ctx (`store.WithTenantID`); master-scope param `tenant_id=` cho admin (predicate `store.IsMasterScope(ctx)` theo CONTRIBUTING tenant-scope guards).

### WS event

`pkg/protocol/events.go` thêm `EventVideoJobUpdated = "video.job.updated"` (payload `{jobId, status, progress, outputPath?, error?}`) — đặt cạnh group constants hiện có; bắn qua msgBus cùng cơ chế các event `agent_link.*` (events.go:76-78).

## Related Code Files

### Create

- `cmd/gateway_video.go` — `newVideoStack` + `wireVideoTools` (pattern `cmd/gateway_cloud.go:44-95`)
- `internal/video/dispatcher.go` — push/poll loop, timeout, deliverable move, TTL cleanup
- `internal/video/worker_client.go` — `do()` client (pattern rc_client)
- `internal/tools/render_video.go` + `render_video_test.go` — tool: params `storyboard` (object/string JSON), `upload_cloud` (bool), `session` resolve; validation gọi `internal/video.Validate()`; tạo job qua store
- `internal/http/video.go` — handlers list/get/cancel (pattern `internal/http/cloud.go`)

### Modify

- `cmd/gateway.go` — gọi `newVideoStack` + `wireVideoTools` cạnh cloud stack (~dòng 638-641)
- `internal/http/server.go` (hoặc file đăng ký route tương đương nơi cloud.go được mount) — đăng ký `internal/http/video.go`
- `pkg/protocol/events.go` — thêm `EventVideoJobUpdated`
- `internal/store/sqlitestore/…` + `internal/store/pg/…` — không đổi (Phase 1 xong store); chỉ wiring `Stores.VideoJobs` vào stack
- `internal/i18n/keys.go` + `catalog_{en,vi,zh,ko,ru}.go` — keys cho thông báo user-facing (job tạo thành công, thất bại, cancel) — **làm TRƯỚC khi handler dùng key** (AGENTS.md: missing key = runtime crash)
- `docs/` — 1 trang ngắn contract storyboard + API (nếu thư mục docs có index thì cập nhật)

### Edition/Lite gating

Worker nằm NGOÀI gateway nên không đụng giới hạn lite (5 agents/1 team...). Nhưng theo `TeamActionPolicy` (AGENTS.md, `internal/tools/team_action_policy.go` — lite chặn comment/review/approve...), `render_video` phải kiểm tra không rơi vào danh sách blocked; nếu policy chặn default thì đăng ký qua allow-list tương tự `cloud_storage.go`. Verify lúc code: grep cách `cloud_storage` tools được phép ở lite.

## Implementation Steps

1. i18n keys trước (5 catalogs) — keys: `msg.video.job_created`, `msg.video.job_failed`, `msg.video.job_cancelled`, `msg.video.job_done`.
2. `internal/video/worker_client.go` + test httptest server mô phỏng worker contract.
3. `internal/tools/render_video.go`: validate storyboard, resolve asset paths (chặn `..`, symlink ra ngoài workspace — dùng helper path-safe hiện có của tools filesystem), tạo row, register AsyncTool callback.
4. `internal/video/dispatcher.go`: channel-driven, submit + poll + timeout + deliverable + cloud upload option (`StorageService.List/Stat` pattern — dùng FS API như `internal/cloud/storage_service.go:221` `FS()` cho thao tác ghi; cần rclone account trong params `cloud_account`).
5. `cmd/gateway_video.go` wiring + `cmd/gateway.go` gọi; WS event vào `pkg/protocol/events.go` + bắn từ dispatcher.
6. `internal/http/video.go` endpoints + đăng ký route.
7. Tests: tool unit (mock store + mock worker), dispatcher integration với httptest worker, HTTP handler test (auth + tenant scope).
8. Checklist AGENTS.md: `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...`.

## Success Criteria

- [ ] Agent chat: "render video từ storyboard này" → tool gọi → reply chứa job id; vài phút sau tin nhắn async kèm `MEDIA:` path MP4 trong workspace
- [ ] `GET /v1/video/jobs` trả jobs của tenant hiện tại; tenant A không thấy jobs tenant B (test isolation — P0 invariant tier)
- [ ] Kill worker giữa chừng → dispatcher timeout đánh dấu `failed` + WS event + thông báo chat, không goroutine leak (test: đóng httptest server)
- [ ] `video.enabled=false` → tool không có trong registry, endpoint 404, dispatcher không chạy (test config)
- [ ] `upload_cloud: true` với account hợp lệ → MP4 xuất hiện trên remote, row ghi remote path
- [ ] i18n: 5 locale đủ keys, `git_keys_parity_test` xanh
- [ ] Lite build (`-tags sqliteonly`) compile + tool hoạt động với SQLite store

## Risk Assessment

| Risk | Observable signal | Pre-decided response |
|------|-------------------|----------------------|
| Dispatcher goroutine leak khi worker treo (không trả, không đóng) | Số goroutine tăng dần; poll timeout không fire | Mọi HTTP call có `context.WithTimeout`; test đóng server giữa poll; timeout 30 phút là vũ khí cuối đánh dấu failed |
| Agent gửi asset path ngoài workspace (path traversal) | Tool nhận `../../etc/passwd` | Resolve tuyệt đối rồi `filepath.Rel` check prefix workspace — tái dùng helper filesystem tools hiện có; test case traversal |
| Cloud upload làm job chậm/treo ở bước cuối | Job done nhưng callback muộn nhiều phút | Upload chạy sau khi đã báo `done` (deliverable local đã xong); upload fail chỉ log + ghi remote_path rỗng, không ảnh hưởng trạng thái |
| Trùng hợp đồng worker ↔ gateway do 2 parser (internal/video vs vworker/contract) | Submit 400 từ worker với storyboard hợp lệ gateway-side | Golden-file test dùng chung fixtures (Phase 1 bắt từ đầu); CI chạy cả 2 test package trên cùng JSON |
| Tool đốt tài nguyên: agent spam tạo jobs | Nhiều row queued, workspace đầy | Queue cap worker 5 + tool-level rate: max 2 job đang chạy/user (check trong tool bằng store count); TTL cleanup từ Phase 1 dọn file |

**RAM/CPU footprint:** trong gateway thêm ~10–20MB (dispatcher + mirror buffer); phần nặng (render) nằm ở worker Phase 2 — đúng yêu cầu "không nhét vào hệ thống".
