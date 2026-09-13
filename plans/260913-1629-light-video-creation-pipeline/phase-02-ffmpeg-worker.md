---
phase: 2
title: "Phase 2: Tier-1 ffmpeg worker (binary videoworker)"
status: pending
priority: P1
effort: "5-7 ngày"
dependencies: [1]
---

# Phase 2: Tier-1 ffmpeg worker (binary videoworker)

## Overview

Xây **worker render Tier-1**: binary Go tĩnh `videoworker` (entry `cmd/videoworker`) nhận job qua HTTP API mini đã chốt ở Phase 1 và render MP4 bằng **ffmpeg thuần** — không browser, không Node.js, không Python. Đây là trái tim "fast + light": ~200–350MB RAM peak, concurrency=1, chịu được MemoryMax của systemd. Worker chạy độc lập hoàn toàn (cùng máy gateway hoặc máy khác, cùng hợp đồng).

## Requirements

- [ ] Render storyboard v1 đầy đủ: scene image/video/color + Ken Burns + caption drawtext + narration mix + BGM mix → MP4 h264/aac
- [ ] HTTP server mini đúng contract Phase 1 (POST /v1/jobs, GET /v1/jobs/{id}, cancel, Bearer token)
- [ ] Concurrency = 1 cứng (semaphore), queue tối đa 5, trả 409 khi đầy
- [ ] Temp dir per-job + cleanup chắc chắn kể cả khi crash (quét orphan theo startup + TTL)
- [ ] Materialize assets: đọc file từ assetsDir (worker-local mirror do gateway đẩy, xem Phase 3) hoặc tải URL với size cap + SSRF guard
- [ ] Progress reporting: 0–100 theo scene hoàn thành
- [ ] Chạy được dưới systemd với `MemoryMax=600MB` `CPUQuota=100%` không bị kill giữa job thường

## Architecture

**Lựa chọn Go binary thay vì bash+ffmpeg script** (quyết định có lý do): storyboard là JSON lồng 3 cấp — bash không parse JSON an toàn; cần test unit cho filtergraph builder (đây là phần dễ sai nhất); anh phát triển trên Windows + deploy Linux — `go build` cho cả hai; binary tĩnh ~10–15MB, không cần cài runtime nào ngoài ffmpeg binary. Script bash bị loại vì cả 4 điểm trên.

**Pipeline render một job** (trong temp dir per-job):

1. **Materialize**: copy/link asset từ assetsDir hoặc tải URL (http.Client timeout, size cap 100MB/asset, SSRF guard — chặn private IP, theo tinh thần SSRF protection hiện có của repo).
2. **Narration synth** (Phase 4 cung cấp interface `NarrationProvider`; Phase 2 ship Edge provider + stub): mỗi scene có narration → audio file; đo duration bằng `ffprobe`.
3. **Per-scene render**: mỗi scene 1 lệnh ffmpeg → `scene_N.mp4` (image→video với zoompan Ken Burns + drawtext caption; video scene = trim/scale; color scene = lavfi color source). Chia nhỏ per-scene để (a) progress chính xác, (b) lỗi 1 scene không hỏng cả job, (c) RAM giới hạn ở 1 scene.
4. **Concat + mix**: concat demuxer nối các scene (re-encode chỉ khi cần), sau đó 1 lệnh ffmpeg mix narration track + BGM (`amix`, side-chain không cần — volume cố định), map sang aac 128k.
5. **Finalize**: chuyển file ra output dir, báo `done` + size; xóa temp.

ffmpeg flags "light" bắt buộc: `-preset veryfast -crf 28 -threads 1 -y`, scale về `output.height` cap (720p mặc định), `-r` cố định theo canvas fps. drawtext cần font: worker nhận `--font-file` flag (deploy hướng dẫn copy 1 font hỗ trợ tiếng Việt, ví dụ Noto Sans); nếu không có font → caption bị bỏ nhưng vẫn render, log warn.

**Trạng thái job** giữ in-memory (map + mutex) — worker restart = mất job đang chạy; gateway phát hiện qua poll timeout và đánh dấu `failed` (Phase 3). Không cần DB ở worker — giữ nhẹ.

### Résolve conflict với gateway hiện có

Không có code video render nào trong repo hiện tại (đã grep `video_render|createVideo` — chỉ có tool AI-gen `create_video` tại `internal/tools/create_video.go` và `read_video`; worker này là namespace riêng `cmd/videoworker` + `internal/vworker` để tránh đụng `internal/tools`).

## Related Code Files

### Create

- `cmd/videoworker/main.go` — flags: `--addr` (default `127.0.0.1:18791`), `--token`, `--work-dir`, `--output-dir`, `--ffmpeg-path`, `--font-file`, `--max-scene-sec`
- `internal/vworker/server.go` — HTTP mini theo contract Phase 1 (stdlib `net/http`, Go 1.22+ pattern `mux.HandleFunc("POST /v1/jobs", ...)`)
- `internal/vworker/runner.go` — job lifecycle: materialize → narration → per-scene → concat/mix → finalize; semaphore concurrency=1
- `internal/vworker/ffmpeg.go` — build argv per scene type + zoompan/drawtext expressions; `Exec` với context timeout + stderr tail vào error
- `internal/vworker/probe.go` — ffprobe duration/streams (JSON output)
- `internal/vworker/assets.go` — materialize assetsDir/URL + SSRF guard + size cap
- `internal/vworker/cleanup.go` — temp dir TTL sweep + startup orphan scan
- `internal/vworker/contract/` — parser copy-shape của storyboard/job types (Phase 1 contract) + golden-file test khớp với `internal/video`
- `internal/vworker/ffmpeg_test.go` — golden tests: argv builder cho từng scene type (không cần ffmpeg thật)
- `internal/vworker/integration_test.go` — integration (build tag `integration`, skip nếu không có ffmpeg): render storyboard 3-scene 10s → assert MP4 + duration ±0.5s
- `deploy/videoworker.service` — systemd unit với `MemoryMax=600MB`, `CPUQuota=100%`, `Restart=on-failure`
- `deploy/README-video-worker.md` — cài ffmpeg (apt), font tiếng Việt, token, reverse same-box/other-box

## Implementation Steps

1. Scaffold `cmd/videoworker` + `internal/vworker/server.go` với contract structs (copy-shape + golden test với `internal/video` fixtures từ Phase 1).
2. `ffmpeg.go` argv builder + golden tests cho: image+KenBurns+caption, video trim, color+caption, concat, audio mix. Đây là phần core — review kỹ biểu thức `zoompan`/`drawtext` escape (dấu `'` `:` trong caption phải escape).
3. `probe.go` + `assets.go` (SSRF guard + size cap + content-type check).
4. `runner.go` lifecycle + progress + cancel (context);
5. Narration: định nghĩa interface `Narrator { Synthesize(ctx, text, voice string) (path string, err error) }`; ship `edgeNarrator` shells ra `edge-tts` CLI (giống cách `internal/audio/edge/tts.go` đã làm — gọi subprocess, không thêm Go dep) — Phase 4 sẽ nâng cấp phía gateway; worker chỉ cần interface này.
6. Cleanup + flags + systemd unit + deploy README.
7. Integration test local (yêu cầu ffmpeg trên máy dev; CI skip nếu thiếu) + chạy được trong systemd với MemoryMax.
8. Checklist: `go build ./...`, `go vet ./...`, `go test -race ./internal/vworker/...`; binary worker KHÔNG nằm trong `go build -tags sqliteonly` pathway chính nhưng phải compile sạch cả hai tag (không import store).

## Success Criteria

- [ ] Golden argv tests xanh cho 4 loại scene + concat + mix (escape caption đúng)
- [ ] Integration test: storyboard 3 scenes (image+KenBurns / video / color+caption) + narration stub → MP4 chơi được (ffprobe đọc ra h264/aac, duration ±0.5s)
- [ ] Edge narration thật: 1 scene tiếng Việt `vi-VN-HoaiMyNeural` → audio đúng dài, mix vào MP4 (cần `pip install edge-tts` trên máy test — ghi rõ trong README)
- [ ] Concurrency: POST 3 jobs khi đang render → 1 rendering + 2 queued; POST thứ 6 → 409
- [ ] Chạy với `systemd-run --scope -p MemoryMax=600M` render 60s video: RAM peak ≤ 350MB, không OOM-kill
- [ ] Kill -9 giữa job → restart worker → orphan temp được quét dọn, gateway poll thấy job mất (test thủ công ghi trong README)
- [ ] Băng thông: render 60s ảnh+caption trên 1 vCPU VM ≤ 8 phút (ghi số đo thật vào PR)

## Risk Assessment

| Risk | Observable signal | Pre-decided response |
|------|-------------------|----------------------|
| ffmpeg drawtext escape sai với tiếng Việt/có dấu nháy → render fail | Integration test caption "Xin chào!" + quote fail; stderr tail trong error | Golden test escape ngay step 2; fallback: caption render qua subtitle file ASS (`subtitles=` filter) — đã là phương án B ghi sẵn trong `ffmpeg.go` |
| `edge-tts` CLI không có mặt trên server → narration fail toàn job | `Synthesize` trả "executable not found" | Worker trả job `failed` với error rõ ràng chỉ dẫn `pip install edge-tts`; Phase 4 thêm Piper offline làm phương án dự phòng; gateway validate config khi enqueue (Phase 3) |
| zoompan frame jitter (hiệu ứng Ken Burns giật) trên ảnh tĩnh | Demo video nhìn giật — feedback anh | Dùng công thức `zoom` trên `fps*duration` frames + `s=WxH` cố định trong zoompan (đã là cách chuẩn); nếu vẫn giật, fallback scale-before-zoompan 2x rồi xuống hạng quality |
| RAM vượt trên máy yếu khi scene video gốc 4K | MemoryMax kill worker giữa job | `assets.go` đọc ffprobe resolution trước; nếu > 2x canvas → pre-scale 1 lượt `veryfast` trước khi vào pipeline; cap canvas 720p mặc định từ Phase 1 |
| Worker chết mất job đang chạy, gateway treo chờ | Gateway poll timeout (Phase 3 thiết kế sẵn poll timeout 1h) | Worker stateless có chủ đích — Phase 3 đánh dấu `failed` khi poll 404/timeout; cleanup orphan ở startup |

**RAM/CPU footprint:** idle ~15–30MB (HTTP + map); render ~200–350MB peak (1 scene + ffmpeg); CPU = 1 core full trong lúc render (CPUQuota=100% là thiết kế, không phải sự cố).
