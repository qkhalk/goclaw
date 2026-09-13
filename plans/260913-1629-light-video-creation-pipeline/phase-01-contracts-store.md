---
phase: 1
title: "Phase 1: Contracts, job store, config, dual-DB migrations"
status: pending
priority: P1
effort: "3-4 ngày"
dependencies: []
---

# Phase 1: Contracts, job store, config, dual-DB migrations

## Overview

Đặt nền móng cho cả pipeline: (1) **storyboard JSON schema v1** — hợp đồng dữ liệu duy nhất giữa gateway, worker và (sau này) Web UI; (2) **worker job API contract** — hợp đồng HTTP giữa gateway và sidecar worker; (3) bảng `video_render_jobs` với **migration dual-DB** (PG + SQLite); (4) config block `video` với kill-switch theo pattern cloud. Phase này chưa render gì — chỉ hợp đồng + lưu trữ, để Phase 2 (worker) và Phase 3 (gateway) có thể làm song song trên cùng một bản hợp đồng.

## Requirements

- [ ] Storyboard JSON v1 được định nghĩa đầy đủ (scene image/video/color, caption, narration, audio mix, output) và có validation shared
- [ ] Worker job API contract (POST /v1/jobs, GET /v1/jobs/{id}, cancel) chốt dạng request/response Go structs có JSON tags camelCase
- [ ] Bảng `video_render_jobs` tạo ở cả PG (migration `000121`) và SQLite (schema.sql + patch 83→84)
- [ ] Interface `store.VideoRenderJobStore` + 2 implementation `pg` và `sqlitestore`
- [ ] Config `video.*` với `Enabled *bool` (default-on) theo đúng pattern `config_cloud.go`
- [ ] Không có code render/tool nào trong phase này — chỉ hợp đồng + store (Phase 2/3 ghép vào)

## Architecture

### Storyboard JSON v1 (hợp đồng trung tâm)

```json
{
  "version": 1,
  "canvas": { "width": 1080, "height": 1920, "fps": 30 },
  "engine": "ffmpeg",
  "scenes": [
    {
      "type": "image",
      "source": "media/banner.png",
      "duration_sec": 5,
      "fit": "cover",
      "ken_burns": { "zoom_from": 1.0, "zoom_to": 1.15, "pan": "none" },
      "caption": { "text": "Xin chào", "position": "bottom", "font_size": 48 },
      "narration": { "text": "Chào mừng các bạn", "provider": "edge", "voice": "vi-VN-HoaiMyNeural" }
    },
    { "type": "video", "source": "media/clip.mp4", "duration_sec": 8, "mute": true },
    { "type": "color", "color": "#101820", "duration_sec": 2,
      "caption": { "text": "Kết thúc" } }
  ],
  "audio": { "bgm_path": "", "bgm_volume": 0.2, "narration_volume": 1.0 },
  "output": { "format": "mp4", "height": 1280, "video_bitrate": "1500k" }
}
```

Quy ước quan trọng (viết thẳng vào struct comment):

- `source` là **đường dẫn tương đối workspace** hoặc URL http(s); worker nhận asset qua bước "materialize" riêng (Phase 2) — schema không chứa nội dung file.
- `narration.text` do agent/user cung cấp; `provider` mặc định `"edge"` (Phase 4 thêm `"piper"`). Worker tự synth và biết duration audio thật.
- Duration scene là **trần tối đa**; nếu narration dài hơn thì scene tự kéo dài theo audio (worker quyết định, ghi vào `progress` log). Đây là quyết định "fast + light": tránh ffprobe hai lượt gateway-side.
- Không có nested layout phức tạp (no absolute positioning, no templates) — v1 cố tình tối giản để ffmpeg filtergraph giữ được đơn giản.

### Worker job API contract (Go structs dùng chung, JSON camelCase)

- `POST /v1/jobs` body `{jobId, storyboard, callbackUrl?, assetsDir?}` → `202 {jobId, status:"queued"}` | `409` khi đang render (concurrency=1, queue depth cap 5)
- `GET /v1/jobs/{id}` → `200 {jobId, status: queued|rendering|done|failed|cancelled, progress 0-100, error?, outputPath?, outputSizeBytes?, durationMs?}`
- `POST /v1/jobs/{id}/cancel` → `200` hoặc `409` nếu đang không cancel được
- Auth: header `Authorization: Bearer <token>` — shared secret trong config, bắt buộc khi worker không loopback
- Structs định nghĩa trong package mới `internal/video` (types only) để gateway và docs dùng chung; **worker không import** package này — worker có parser copy-shape riêng trong `cmd/videoworker/internal/contract` (ghi chú lý do: giữ worker độc lập binary, không kéo store/config của gateway vào). Có test GoldenFiles đảm bảo 2 parser nhận cùng JSON.

### Bảng `video_render_jobs` (theo pattern bảng gần nhất `cloud_account_bindings` ở migration 000120: TEXT id, tenant FK)

Cột: `id TEXT PK`, `tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE`, `user_id TEXT NOT NULL DEFAULT ''`, `agent_id TEXT NOT NULL DEFAULT ''`, `session_key TEXT NOT NULL DEFAULT ''`, `status TEXT NOT NULL DEFAULT 'queued'`, `engine TEXT NOT NULL DEFAULT 'ffmpeg'`, `storyboard_json TEXT NOT NULL`, `output_path TEXT NOT NULL DEFAULT ''`, `output_size_bytes BIGINT NOT NULL DEFAULT 0`, `error TEXT NOT NULL DEFAULT ''`, `created_at/updated_at TIMESTAMPTZ`, `started_at/finished_at TIMESTAMPTZ NULL`, `expires_at TIMESTAMPTZ NULL` (job TTL/cleanup). Index: `(tenant_id, created_at DESC)`, `(status)`, `(expires_at) WHERE status IN ('done','failed','cancelled')` — chỉ PG hỗ trợ partial index, SQLite dùng index thường trên `expires_at`.

## Related Code Files

### Create

- `internal/video/types.go` — Storyboard v1 structs + `Validate()` (số scene 1..60, duration 1..30s/scene, tổng ≤ 600s, canvas cap 1920)
- `internal/video/job_types.go` — Job request/response structs cho worker API (camelCase JSON tags)
- `internal/video/validate_test.go` — golden fixture: storyboard hợp lệ/từ chối
- `internal/store/video_job_store.go` — interface `VideoRenderJobStore` (Create, Get, UpdateStatus, ListByTenant, ClaimNextQueued, DeleteExpired) — pattern theo `internal/store/cloud_account_store.go`
- `internal/store/pg/video_jobs.go` — PG impl (database/sql + `$1..$n`, pattern `internal/store/pg/cloud_accounts.go`)
- `internal/store/sqlitestore/video_jobs.go` — SQLite impl (`?` params)
- `migrations/000121_video_render_jobs.up.sql` + `.down.sql`
- `internal/config/config_video.go` — `VideoConfig{Enabled *bool, WorkerURL string, WorkerToken string, ...}`

### Modify

- `internal/upgrade/version.go` — `RequiredSchemaVersion` 120 → **121** (hiện tại dòng 5)
- `internal/store/sqlitestore/schema.go` — thêm patch `83:` vào map `migrations` (comment liên kết "PG 000121", đúng convention patch `82:` hiện có) + bump `SchemaVersion` 83 → **84** (dòng 19)
- `internal/store/sqlitestore/schema.sql` — thêm DDL `video_render_jobs` cho fresh DB
- `internal/store/stores.go` — thêm field `VideoJobs VideoRenderJobStore` vào struct `Stores`
- `internal/config/config.go` — nhúng `Video VideoConfig` vào config gốc + helper `VideoEnabled() bool` (default-on: `c.Video.Enabled == nil || *c.Video.Enabled`, y hệt pattern config_cloud.go:53)

## Implementation Steps

1. Viết `internal/video/types.go` + validation + golden tests (không phụ thuộc gì — làm đầu tiên để chốt hợp đồng).
2. Viết migration PG `000121` (up/down) theo shape bảng ở trên; bump `RequiredSchemaVersion = 121`.
3. SQLite: thêm bảng vào `schema.sql`, thêm patch map `83:` liên kết PG 000121, bump `SchemaVersion = 84`.
4. Định nghĩa `VideoRenderJobStore` interface + `Stores.VideoJobs`; implement `pg/` trước rồi `sqlitestore/` (cùng một test table-driven, chạy trên cả 2 dialect).
5. `internal/config/config_video.go` + nhúng vào config gốc + default-on helper; thêm test round-trip JSON5.
6. Chạy checklist: `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...`, `go test ./internal/video/... ./internal/store/...`.

## Success Criteria

- [ ] `go test ./internal/video/...` xanh: storyboard v1 parse + validate đúng các case biên
- [ ] `go test -tags integration ./tests/integration/ -run Video` xanh trên PG (bảng tồn tại, CRUD store OK, tenant-scoped)
- [ ] SQLite fresh-DB test (desktop path) tạo bảng OK, patch 83→84 chạy từ DB cũ 83 OK
- [ ] `RequiredSchemaVersion = 121` và `SchemaVersion = 84` khớp thực tế sau khi viết (grep lại trước khi merge)
- [ ] Config `video.enabled: false` parse được và `VideoEnabled()` trả false; bỏ field → true

## Risk Assessment

| Risk | Observable signal | Pre-decided response |
|------|-------------------|----------------------|
| Storyboard v1 thiếu trường thực cần, phải breaking-change khi gặp video thật | Phase 2 render scene demo bị chặn vì schema không diễn đạt được | V1 thêm field **optional** (backward-compat) — validation chỉ từ chối trường lạ khi `version > 1`; chưa từng chặn add-optional |
| SQLite patch 83 quên hoặc sai cú pháp → desktop lite crash khi mở DB cũ | `go build -tags sqliteonly` + test fresh DB fail; smoke `SchemaVersion` mismatch | AGENTS.md liệt kê đây là lỗi crash đã biết — step 3 là step riêng có test; nếu lỡ skip, characterization test store chặn trước merge |
| Bảng thiếu index đúng → List jobs chậm khi nhiều tenant | `EXPLAIN` full-scan trên `(tenant_id, created_at)` | Index tạo ngay trong 000121; SQL review checklist (AGENTS.md) bắt check index trước merge |
| Config default-on gây риск bất ngờ cho deployment hiện có | Tool xuất hiện ở agent sau upgrade dù không dùng | Đây là chủ đích (kill-switch pattern cloud default-on); nếu anh muốn default-off, đổi 1 dòng helper — ghi trong PR description |

**RAM/CPU footprint của phase:** gần như 0 runtime (chỉ store + validation, chạy trong process gateway đã có).
