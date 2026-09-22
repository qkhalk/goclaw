---
title: "Phase 6: Videoworker port + video jobs API + SSE events"
status: todo
priority: P1
effort: "2d"
dependencies: [1]
---

# Phase 6: Videoworker port + video jobs API + SSE events

## Overview
Port videoworker (đã self-contained — audit xác nhận chỉ import stdlib + vworker/contract) thành `gotools-videoworker` port mặc định **18891** (tách khỏi 18791 của goclaw để cùng tồn tại trên 1 máy). Xây jobs API + dispatcher poll + SSE events + download sign.

## Requirements
- Functional: submit storyboard JSON → validate → dispatch worker → poll status → SSE `video.job.updated` broadcast → download MP4 qua link signed tạm thời; cancel proxy worker; worker down → job lỗi rõ ràng.
- Non-functional: download token HMAC TTL 10 phút (port pattern SignFileToken); SSE hub.unregister sạch (race test); storyboard validator port NGUYÊN VĂN từ goclaw để format tương thích.

## Architecture
- **Worker port**: copy `cmd/videoworker/` + `internal/vworker/` (server/runner/ffmpeg/narrator + contract) vào `worker/` của repo gotools; đổi module path + default `--addr 127.0.0.1:18891`; flags giữ nguyên (`--work-dir --output-dir --ffmpeg-path --font-file --token` — nguồn `cmd/videoworker/main.go:24-41`). Binary `cmd/gotools-videoworker/`.
- **Storyboard validator** `internal/video/validate.go`: port logic validate từ goclaw `internal/http/video.go:104-131` (đứng trước submit) + contract types copy từ `internal/vworker/contract` (đã port cùng worker).
- **Jobs service** `internal/video/jobs.go`: submit (validate → insert video_jobs queued → POST worker /v1/jobs lấy worker_job_id), poll loop 2s/ job active (GET worker status → update DB + publish event), cancel (POST worker cancel + status cancelled), retention dọn file output > 7 ngày (cron nội bộ đơn giản).
- **SSE hub** `internal/httpapi/ssehub.go`: subscribe per-connection (fetch-stream + Authorization); broadcast `video.job.updated {id, status, progress, output_name, error}`; heartbeats 20s (comment `: ping` giữ kết nối qua proxy).
- **Dispatcher client** `internal/video/worker_client.go`: port SubmitJob/GetJobStatus/CancelJob/HealthCheck (nguồn goclaw `internal/video/worker_client.go:42-91`, bearer header) + health check 30s → flag `worker_up` expose `/health` của server.
- **Download** `/v1/files/videos/{name}?ft=`: HMAC-SHA256(secret, name+exp) so sánh constant-time + check file nằm trong output dir (filepath.Rel không được `..`) → ServeFile.

## Related Code Files
- Create: `worker/` (cmd/gotools-videoworker + internal/vworker port), `internal/video/{validate,jobs,worker_client}.go`, `internal/httpapi/{video.go,ssehub.go,files.go}`
- Reference: `cmd/videoworker/main.go:24-41`, `internal/vworker/server.go:38-41,146-156` (HTTP surface + bearer), `internal/http/video.go:48-66,104-131,252-256` (API + validate + sign pattern), `internal/video/worker_client.go:42-91`, `internal/video/dispatcher.go:53-121` (poll pattern), `deploy/README-video-worker.md` (goclaw)

## Implementation Steps
1. Copy worker + build + smoke: curl thẳng worker submit storyboard mẫu → MP4 ra (trước khi đụng server).
2. Validator + types port + unit test (storyboard hợp lệ/lỗi từ fixture goclaw nếu có test sẵn — copy kèm).
3. Jobs service + store + poll loop + retention; unit test với worker fake (httptest).
4. SSE hub + heartbeats + broadcast; race test subscribe/unsubscribe đồng thời.
5. HTTP handlers wire + download sign + path traversal test (`../` bị chặn).
6. Worker health → /health server hiển thị `worker: up/down`.
7. E2E tay local: submit → SSE nhận queued→running→completed <2s mỗi chuyển → download play.

## Success Criteria
- [ ] Worker port render MP4 từ storyboard chuẩn goclaw (byte-format tương thích — mở video thật)
- [ ] SSE events tới <2s sau chuyển trạng thái; reconnect client nhận được event kế tiếp (không replay bắt buộc)
- [ ] Download token hết hạn sau 10 phút (test), path traversal chặn (test)
- [ ] Worker tắt → submit trả lỗi rõ "worker down", không treo request

## Risk Assessment
- Poll dồn khi nhiều job: loop 2s/job tuần tự đủ cho single-user; nếu cần → batch status endpoint worker (note, không làm trước).
- Narration TTS bên worker (narrator.go) cần network/provider: mặc định tắt narration nếu chưa cấu hình — flag worker giữ nguyên, docs ghi.
