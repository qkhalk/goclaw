---
phase: 4
title: "Phase 4: Voice layer — Edge TTS mặc định + Piper offline tùy chọn"
status: pending
priority: P1
effort: "3-4 ngày"
dependencies: [2, 3]
---

# Phase 4: Voice layer — Edge TTS mặc định + Piper offline tùy chọn

## Overview

Lớp giọng đọc cho narration: **Edge TTS là mặc định** (miễn phí, không API key, synth chạy trên dịch vụ Microsoft nên ~0 RAM/CPU local — đúng mục tiêu light) và **Piper là provider offline tùy chọn** (ONNX, ~100–200MB RAM khi synth, có voice tiếng Việt `vi_VN-vais1000-medium`). Hợp đồng `NarrationProvider` pluggable để sau này gắn thêm (OmniVoice khi có GPU).

Hai mặt bài: (a) **worker-side** là nơi synth thực sự chạy trong pipeline render — Phase 2 đã để sẵn interface `Narrator`; (b) **gateway-side** cần validate config voice + cho agent biết voices khả dụng (tool meta/description), tránh job fail vì thiếu binary. Phase này hoàn thiện cả hai + hợp nhất với manager audio hiện có của gateway để không sinh ra hệ thống TTS song song.

## Requirements

- [ ] Edge TTS default: scene `narration.provider` trống hoặc `"edge"` → dùng edge; voice mặc định tiếng Việt `vi-VN-HoaiMyNeural` khi locale vi, `en-US-MichelleNeural` otherwise
- [ ] Piper provider: `provider: "piper"` hoạt động khi binary + model có mặt; config khai báo `piper_bin`, `piper_voices_dir`; thiếu → lỗi config rõ ràng lúc enqueue (không để fail giữa render)
- [ ] Gateway-side voice registry nhỏ: danh sách voices gợi ý (Edge vi/en + Piper models thấy trong voices_dir) phục vụ tool description + Web UI Phase 6
- [ ] Tái sử dụng, không trùng lặp: gateway đã có `internal/audio` (Manager + `edge.Provider` tại `internal/audio/edge/tts.go`, alias legacy `internal/tts/alias.go`) — phần gateway-side của narration đi qua manager hiện có; worker-side có `edgeNarrator` subprocess riêng (không import gateway code)
- [ ] Duration-accurate: narration synth xong đo `ffprobe` duration → scene kéo dài theo audio (đã thiết kế Phase 1/2)
- [ ] Không thêm API key/chi phí nào: cả hai provider đều miễn phí, chạy local/cloud Miễn phí

## Architecture

### Phân lớp provider

```
Gateway (internal/audio — đã có)              Worker (cmd/videoworker/internal/narrate)
┌────────────────────────────┐                ┌───────────────────────────────────────┐
│ Manager + edge.Provider     │   contract:    │ Narrator interface (Phase 2 để sẵn)    │
│ dùng cho: validate config,  │  storyboard    │  ├ edgeNarrator  (subprocess edge-tts) │
│ preview TTS ở Web UI,       │ ────────────►  │  └ piperNarrator (subprocess piper)    │
│ voices list cho tool/UI     │  JSON fields   │ Trả file .mp3/.wav + ffprobe duration  │
└────────────────────────────┘                └───────────────────────────────────────┘
```

- **Không** đưa synth stream qua gateway → worker (tốn RAM gateway, phá "light"); gateway chỉ **validate**: voice id có trong registry, provider được phép (config allowlist), text length cap (2000 ký tự/scene).
- Worker `edgeNarrator`: gọi `edge-tts --voice X --text ... --write-media out.mp3` (giống cách `internal/audio/edge/tts.go:59` shells out, output MP3 24kHz/48kbit mono). Retry 1 lần khi mạng lỗi; lỗi mạng liên tục → job failed với error chỉ dẫn (hoặc fallback Piper nếu config `narration_fallback: "piper"`).
- Worker `piperNarrator`: `piper --model <vi_VN-vais1000-medium.onnx> --output_file out.wav`; wav → ffmpeg chuyển mp3/mix. RAM Piper ~100–200MB — chỉ khuyến nghị máy ≥1GB dedicated cho worker.
- Duration scene theo audio: nếu `audio_sec > duration_sec` scene → kéo dài scene (Ken Burns tốc độ co lại tương ứng); ghi quyết định vào job progress log để debug.

### Voice registry (gateway)

- `internal/video/voices.go`: static Edge voices list (vi: HoaiMyNeural, NamMinhNeural; en: MichelleNeural + vài voice phổ biến) + dynamic scan `piper_voices_dir` nếu config trỏ tới.
- Tool `render_video` description + `get_tool_info` sẽ liệt kê; Web UI Phase 6 dùng endpoint `GET /v1/video/voices` (thêm 1 handler ở Phase 3 file `internal/http/video.go`).

## Related Code Files

### Create

- `cmd/videoworker/internal/narrate/edge.go` — subprocess `edge-tts`, retry 1, timeout per call (30s, khớp mặc định `internal/audio/edge/tts.go`)
- `cmd/videoworker/internal/narrate/piper.go` — subprocess piper + wav convert
- `cmd/videoworker/internal/narrate/narrate_test.go` — mock subprocess (script shell giả) + golden argv
- `internal/video/voices.go` — registry static + scan piper dir
- `internal/video/voices_test.go`

### Modify

- `cmd/videoworker/internal/vworker/runner.go` — nối Narrator vào step narration; fallback logic theo config worker (`--narration-fallback`)
- `cmd/videoworker/main.go` — flags `--piper-bin`, `--piper-voices-dir`, `--narration-fallback`
- `internal/config/config_video.go` — thêm `AllowedNarrationProviders []string` (default `["edge"]`), `PiperVoicesDir` (gateway-side để scan)
- `internal/video/validate.go` (Phase 1 types) — validate voice/provider theo registry
- `internal/tools/render_video.go` — description nêu voices gợi ý; validate provider trước khi tạo job
- `internal/http/video.go` — handler `GET /v1/video/voices`
- `internal/i18n/` — keys lỗi narration (thiếu edge-tts binary, voice không hợp lệ) vào 5 catalogs

## Implementation Steps

1. Worker `edgeNarrator` + test mock; nối runner; integration test 1 scene narration tiếng Việt thật (yêu cầu `pip install edge-tts`).
2. Worker `piperNarrator` + test (test dùng model dummy/truncated — không download model thật trong CI).
3. Gateway voice registry + validate trong `render_video` (chặn sớm provider/voice sai → trả lỗi i18n cho agent, không tạo job rác).
4. Endpoint `/v1/video/voices` + i18n keys.
5. Docs deploy README (Phase 2): bổ sung mục voice — edge-tts cài `pip install edge-tts`, Piper tải model `vi_VN-vais1000-medium` từ HuggingFace rhasspy/piper-voices.
6. Checklist: `go build ./...` (cả `-tags sqliteonly`), `go vet`, tests.

## Success Criteria

- [ ] Job storyboard tiếng Việt không chỉ định provider → narration Edge giọng `vi-VN-HoaiMyNeural`, audio khớp nội dung (nghe được, duration ffprobe > 0)
- [ ] `provider: "piper"` trên máy có piper → render OK offline (tắt mạng test vẫn xong); thiếu piper → job failed với error hướng dẫn, hoặc fallback Edge theo config
- [ ] Gateway chặn sớm: voice không tồn tại → tool trả lỗi ngay, KHÔNG tạo row job
- [ ] `GET /v1/video/voices` trả danh sách Edge + Piper (khi config dir hợp lệ)
- [ ] RAM đo: Edge path không tăng RAM worker đáng kể (subprocess Python ~60–120MB tạm thời trong lúc synth — ghi số đo); Piper path peak ≤ 350MB tổng worker

## Risk Assessment

| Risk | Observable signal | Pre-decided response |
|------|-------------------|----------------------|
| `edge-tts` đổi giao thức/bị chặn bởi Microsoft (đã từng xảy ra với project edge-tts upstream) | Synth 403/401 hàng loạt; job failed voice | Fallback Piper khi config; deploy README ghi cách `pip install -U edge-tts`; vì synth ở worker, gateway không phải release lại |
| edge-tts subprocess (Python) tốn RAM hơn dự kiến trên máy 1GB | Peak worker > 450MB khi synth | Synth tuần tự từng scene (đã là tuần tự vì concurrency=1), trả file ngay rồi giải phóng; nếu vẫn căng → khuyến nghị Piper-only cho box 1GB |
| Piper model tiếng Việt chất lượng/dialect không đạt | anh nghe mẫu không hài lòng | Registry thêm voices dễ (chỉ thả .onnx vào dir); quality là chủ quan — cung cấp 2-3 voice vi trong docs để anh chọn |
| Thêm hệ thống TTS song song với internal/audio → duplicated code về sau | Review thấy 2 chỗ định nghĩa voices | Gateway-side đi qua `internal/audio`/registry ở `internal/video/voices.go` duy nhất; worker-side cố ý tách (độc lập binary) — ghi lý do vào file header cả 2 |

**RAM/CPU footprint:** gateway +~1MB (registry); worker: Edge synth subprocess tạm thời ~60–120MB (giải phóng sau mỗi scene), Piper ~150–200MB trong lúc synth. Synth trên Microsoft cloud nên CPU local ~0 với Edge.
