---
title: "Phase 3: Studio: port video studio + videoworker sidecar"
status: todo
priority: P2
effort: "3d"
dependencies: [1, 2]
---

# Phase 3: Studio: port video studio + videoworker sidecar

## Overview
Port Video Studio (trang nặng nhất) sang Studio: copy videoworker (đã standalone sẵn) làm sidecar render, port API `/v1/video/jobs` + dispatcher, đổi event WS → SSE. Designer video dùng lại slim endpoint phase 2 với skill `video-storyboard-design`.

## Requirements
- Functional: submit storyboard JSON → job render MP4 (ffmpeg worker) → poll status → download file; cancel job; designer column sinh storyboard blocks; client-side export (MediaRecorder) vẫn chạy.
- Non-functional: job events realtime qua SSE thay WS `video.job.updated`; file download qua signed token tạm (port `SignFileToken` — `<a href>` không mang bearer được, `internal/http/video.go:55-66`).

## Architecture
- **Videoworker copy**: nguyên `cmd/videoworker/` + `internal/vworker/` (server.go /v1/jobs, runner, ffmpeg, narrator — vốn 0 phụ thuộc gateway, bearer token riêng, port 18791, flags `--work-dir --output-dir --ffmpeg-path`). Build thành binary `studio-videoworker` trong repo mới, deploy cùng máy hoặc máy khác (config `worker_url/worker_token` — port `internal/config/config_video.go:12-17`).
- **Jobs API** port slice `internal/http/video.go`: `POST /v1/video/jobs` (validate storyboard → dispatch), `GET /v1/video/jobs`, `GET /v1/video/jobs/{id}`, `DELETE` (cancel proxy worker `video.go:252-256`), download sign `video.go:59-66`.
- **Dispatcher** port slice `internal/video/dispatcher.go` (poll worker, ghi output, phát event) + `worker_client.go` (SubmitJob/GetJobStatus/CancelJob, bearer :91) — thay broadcast WS `dispatcher.go:308` bằng publish kênh SSE.
- **SSE** `GET /v1/video/jobs/events`: 1 kênh per-user, event `video.job.updated` cùng payload cũ. Hook `use-video.ts:59` đổi từ `use-ws-event` → `EventSource`; giữ query key `["video","jobs"]` (:40).
- **Designer video**: `POST /v1/designer/chat` thêm `kind: "video"` — prompt từ `internal/video/designer_agent.go` + skill `bundled-skills/video-storyboard-design/SKILL.md` (+ video-color-motion nếu cần) inline; output parse bằng `parse-storyboard-blocks.ts` có sẵn.
- Studio cần ffmpeg + font trên host deploy — ghi rõ deploy/README (tham khảo `deploy/README-video-worker.md` goclaw).

## Related Code Files
- Create (repo mới): `worker/` (copy cmd/videoworker + internal/vworker), `server/video/` (jobs API + dispatcher + worker client + sse hub), `web/src/port/pages/tools/video/**` (page, canvas-player, designer-column, render-panel, scene-card, storyboard-card, template-gallery, timeline, hooks use-canvas-player/use-designer-chat/use-timeline/use-video-export/use-video, lib icon-library/parse-storyboard-blocks/templates)
- Reference: `internal/http/video.go:48-66,104-131,252-256`, `internal/video/dispatcher.go:53-121,308`, `internal/video/worker_client.go:42-91`, `cmd/videoworker/main.go:24-41`, `ui/web/src/pages/tools/video/hooks/use-video.ts:40-108`

## Implementation Steps
1. Copy worker (build + smoke test render 1 storyboard mẫu bằng curl trực tiếp worker).
2. Port jobs API + store `video_jobs` (SQLite) + signed download token (port crypto AES slice hoặc HMAC token đơn giản — chọn HMAC-SHA256 short-lived, tự chứa).
3. Port dispatcher poll loop + SSE hub; wire hook `use-video.ts` sang EventSource.
4. Copy trang video + components + hooks; mở StudioGate bỏ (không cần builtin-tool gate trong Studio — nav tĩnh 3 tool).
5. Designer video: thêm kind video vào slim endpoint (prompt + skill inline); wire designer-column video.
6. Capability probe: port check "video khả dụng" (worker health) hiển thị trạng thái worker trên render panel.
7. E2E tay trên server anh: submit → render xong → download MP4 play được; cancel job giữa chừng.
8. systemd thứ hai cho worker trong deploy/ (pattern unit videoworker goclaw).

## Success Criteria
- [ ] Render E2E storyboard 3 cảnh → MP4 (vi-VN narration nếu có) tải về chơi được
- [ ] Cancel job dừng thật (worker Hủy + job status cancelled)
- [ ] SSE client nhận update <2s sau khi worker đổi trạng thái
- [ ] Worker tắt → UI hiện trạng thái worker down rõ ràng, không treo submit

## Risk Assessment
- ffmpeg/font khác biệt giữa môi trường: README deploy liệt kê gói cần (`ffmpeg fonts-noto`...); worker có `--font-file` flag sẵn.
- SSE hub leak subscription: hub có unregister khi ctx done + test race (`-race`).
- Diverge storyboard validator giữa goclaw và Studio: copy nguyên validator kèm PROVENANCE; nếu goclaw sửa format sau này → sync theo header.
