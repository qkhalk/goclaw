---
title: "Light video creation pipeline"
description: "Pipeline tạo video MP4 từ storyboard (ảnh/text/caption + TTS) qua worker ffmpeg riêng, tách khỏi gateway, chạy được trên VPS 1 vCPU / 1GB RAM"
status: pending
priority: P1
effort: "~3-4 tuần (6 phase)"
tags: [video, ffmpeg, tts, sidecar, worker]
created: 2026-09-13
---

# Light video creation pipeline

## Overview

anh cần pipeline tạo video: user (hoặc agent) soạn **storyboard** (ảnh/video + text + caption + lời thoại TTS), GoClaw xuất ra **MP4**. Yêu cầu cứng: **nhanh + nhẹ**, chạy được trên server 1 vCPU / 1GB (hoặc 2GB) RAM. anh **không muốn nhét hoàn toàn vào gateway** — render phải nằm ở **worker/sidecar riêng** chỉ nói chuyện với GoClaw qua HTTP. Giọng đọc dùng **Edge TTS** (đã tích hợp sẵn, miễn phí), tùy chọn ghép thêm TTS offline nhẹ.

Kiến trúc đã chọn:

- **GoClaw core** (thay đổi nhỏ): bảng `video_render_jobs` (migration dual-DB), agent tool `render_video`, HTTP API nhỏ cho job, dispatcher đẩy job sang worker, sự kiện WS `video.job.updated`, upload deliverable lên cloud (tùy chọn).
- **Tier-1 worker (mặc định, bật)**: binary Go tĩnh `videoworker` chạy **ffmpeg thuần** — không browser, không Node.js, ~200–350MB RAM peak. Storyboard JSON → ffmpeg filtergraph (scene ảnh/video + Ken Burns + caption drawtext + mix narration/BGM).
- **Voice layer**: Edge TTS là default (synth trên cloud của Microsoft, ~0 RAM/CPU local — đã có sẵn tại `internal/audio/edge/tts.go`); Piper là provider offline tùy chọn (ONNX, ~100–200MB RAM), hợp đồng `NarrationProvider` pluggable.
- **Tier-2 worker (tùy chọn, TẮT mặc định)**: Remotion sidecar (headless Chrome + Node) — chỉ khuyến nghị cho máy ≥2GB, concurrency=1, video ≤1–2 phút. Không bao giờ bật trên máy ≤1GB.

Lý do **từ chối** các phương án khác (theo phân tích orchestrator): Remotion trên máy ≤1GB không khả thi (Chrome ~300MB–1GB+/instance, chính thức khuyến nghị ~2GB/render thread); OmniVoice bị loại vì backend inference chỉ chạy GPU (CUDA/MPS/XPU), không có chế độ CPU-only — ghi nhận làm backlog khi có máy GPU.


> **⚠️ COOKING NOTE (2026-09-13):** Nhánh `feat/clouds-drive-redesign` (Clouds redesign, Phase 1+2) đã dùng PG migration `000121` + SQLite `SchemaVersion 84` cho `cloud_account_bindings`. Khi cook plan này, các số migration ghi trong phase files phải dịch tiếp: PG bắt đầu từ `000122`, SQLite từ `85` (kiểm tra lại HEAD tại thời điểm cook).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Agent/user tạo video từ storyboard JSON qua tool `render_video`, nhận MP4 trong workspace | P1 |
| 2 | Worker tách rời gateway (sidecar cùng máy hoặc máy khác, cùng hợp đồng HTTP) | P1 |
| 3 | Chạy được trên 1 vCPU / 1GB RAM: ffmpeg concurrency=1, Edge TTS default, cap 720p | P1 |
| 4 | Voice: Edge TTS (default, miễn phí) + Piper (offline, tùy chọn), contract pluggable | P1 |
| 5 | Remotion Tier-2 tùy chọn, feature-gated OFF mặc định, chỉ khuyến nghị ≥2GB | P2 |
| 6 | Web UI: trang/section danh sách job + storyboard composer tối giản | P2 |

## Phases

| # | Phase | Status | Priority |
|---|-------|--------|----------|
| 1 | [Phase 1: Contracts, job store, config, dual-DB migrations](./phase-01-contracts-store.md) | Pending | P1 |
| 2 | [Phase 2: Tier-1 ffmpeg worker (binary `videoworker`)](./phase-02-ffmpeg-worker.md) | Pending | P1 |
| 3 | [Phase 3: GoClaw integration — tool `render_video`, HTTP API, dispatcher, WS event](./phase-03-gateway-integration.md) | Pending | P1 |
| 4 | [Phase 4: Voice layer — Edge TTS default + Piper offline tùy chọn](./phase-04-voice-layer.md) | Pending | P1 |
| 5 | [Phase 5: Remotion Tier-2 sidecar (tùy chọn, off mặc định)](./phase-05-remotion-tier2.md) | Pending | P2 |
| 6 | [Phase 6: Web UI — jobs page + storyboard composer tối giản](./phase-06-web-ui.md) | Pending | P2 |

Thứ tự phụ thuộc: 1 → 2 → 3 → 4 → (5, 6 song song, đều phụ thuộc 3+4). Phase 4 có thể chạy song song với 3 nếu thiếu nhân lực, nhưng 3 là điểm ghép chính.

## Kiến trúc tổng thể

```
┌─────────────────────────────┐        HTTP JSON + token         ┌──────────────────────────────┐
│ GoClaw gateway               │  POST /v1/jobs (storyboard)      │ videoworker (Tier-1, Go tĩnh)│
│  tool render_video           │ ───────────────────────────────► │  ffmpeg filtergraph render   │
│  dispatcher (poll+retry)     │  GET  /v1/jobs/{id} (poll)       │  Edge TTS (subprocess, 0 RAM)│
│  video_render_jobs (PG/SQLite)│ ◄─────────────────────────────── │  hoặc Piper (ONNX, offline)  │
│  WS event video.job.updated  │  callback (tùy chọn)             │  temp dir + cleanup + TTL    │
│  workspace + cloud upload    │                                  │  [Tier-2: remotion sidecar]  │
└─────────────────────────────┘                                  └──────────────────────────────┘
```

- Worker là process riêng, **không import** code gateway; chỉ chia sẻ schema storyboard JSON (định nghĩa trong Phase 1, worker có parser Go riêng trong `cmd/videoworker`).
- Worker cùng máy: gateway đẩy qua `http://127.0.0.1:<port>`; worker máy khác: đổi base URL + token trong config — **cùng một hợp đồng**, không có chế độ đặc biệt.
- Tài nguyên: ffmpeg là công việc nặng duy nhất; systemd `MemoryMax`/`CPUQuota` + concurrency=1 + cleanup temp là biên an toàn.

## Deployment matrix (nói thật về tài nguyên)

| Máy | Chạy gì | RAM dự kiến | Ghi chú |
|-----|---------|-------------|---------|
| 512MB / 1CPU | **KHÔNG hỗ trợ gateway + worker cùng máy.** Chỉ worker riêng (gateway ở máy khác), cap 480p/720p ngắn | gateway ~150–250MB; worker ffmpeg ~200–350MB peak | Prerequisite bắt buộc: **cài ffmpeg** (server hiện không có) |
| 1GB / 1CPU | Gateway + Tier-1 worker cùng máy. Edge TTS (0 RAM local). Piper KHÔNG khuyến nghị cùng máy | ~500–750MB peak khi render, concurrency=1 | Cap 720p, video ≤3–5 phút, systemd `MemoryMax=600MB` cho worker |
| 2GB / 1CPU | Như trên + Piper offline (~150–200MB khi synth) HOẶC Remotion Tier-2 (marginal: concurrency=1, video ≤1–2 phút, có rủi ro OOM — off mặc định) | ~0.8–1.4GB peak | Remotion cần Node.js + headless Chrome (~300MB–1GB+/instance) |

Hiệu năng trung thực: 1 vCPU render x264 veryfast 720p30 ≈ **2–8 phút render cho 1 phút video** (tùy nội dung ảnh/video). "Nhanh" đạt được ở mức video ngắn (30s–3 phút) cho social/TikTok — không phải render dài.

## Success Criteria

- [ ] Agent gọi `render_video` với storyboard JSON → nhận job id → MP4 nằm trong workspace, kèm path trả về chat
- [ ] Worker chạy được trên máy 1 vCPU/1GB (test: render video 60s ảnh + caption + narration Edge TTS, RAM peak ≤350MB, không OOM)
- [ ] Edge TTS là voice mặc định, không cần API key; Piper hoạt động khi `provider: "piper"` và binary/model có mặt
- [ ] `video.enabled=false` (kill-switch, default-on như `config_cloud.go`) tắt sạch tool + HTTP + dispatcher
- [ ] Migration dual-DB xanh: PG 000121 + `RequiredSchemaVersion=121`; SQLite schema.sql + patch 83→84 + `SchemaVersion=84` — desktop lite khởi động OK
- [ ] Worker chết giữa chừng → job được gateway đánh dấu failed/retry, không treo vĩnh viễn; temp files được dọn theo TTL
- [ ] Web UI xem được danh sách job + trạng thái (P2); i18n đủ 5 locale (en, vi, zh, ko, ru)

## Surface parity

- **Gateway server:** Phase 1, 3, 4 (store, dispatcher, tool, WS event).
- **API contract:** Phase 1 (storyboard JSON v1, job API shapes), Phase 3 (HTTP `/v1/video/*`), documented trong phase tương ứng.
- **Web UI:** Phase 6.
- **CLI/runtime package:** N/A — worker là binary `cmd/videoworker` riêng trong repo này; không có command CLI mới cho `cmd/goclaw` ngoài docs.
- **Desktop/Lite:** Phase 1 migrations chạy cả SQLite; tool gating theo edition được ghi chú trong Phase 3 (lite vẫn dùng được render_video vì worker ở ngoài gateway — không vi phạm giới hạn lite).

## Out of scope / Backlog

| Mục | Lý do hoãn |
|-----|-----------|
| OmniVoice (zero-shot TTS + voice cloning, 600+ ngôn ngữ) | Backend inference GPU-only (CUDA/MPS/XPU), không có CPU mode. Apache-2.0 — reconsider khi có máy GPU |
| GPU transcoding (NVENC/QSV/VAAPI) | Máy đích 1 vCPU không có GPU; thêm flag ffmpeg sau nếu nâng cấp |
| Timeline editor hai chiều (edit MP4 ngược về storyboard) | scope lớn, cần parse ngược; backlog |
| Templates marketplace (storyboard mẫu, watermark brand) | cần Phase 6 ổn định trước |
| Multi-worker queue / render farm | concurrency=1 là đủ cho 1 vCPU; chỉ mở khi có nhiều box |
| Subtitle auto-sync từ ASR | Phase 4 chỉ dùng text do user/agent cung cấp |

## Open questions cho anh

1. Server đích render: chạy **cùng máy gateway** (1GB) hay **máy riêng**? Plan mặc định hỗ trợ cả hai, chỉ khác config.
2. Độ phân giải/mục đích dùng: 720p dọc 9:16 (TikTok/Reels) làm default có đúng không? Plan default 1080x1920@30fps.
3. BGM: có cần sẵn thư viện nhạc nền trong repo, hay chỉ hỗ trợ user upload/đường dẫn? Plan mặc định chỉ hỗ trợ đường dẫn file.
4. Anh có muốn agent tự động sinh storyboard từ một topic (prompt → storyboard JSON bởi LLM) trong Phase 3, hay chỉ nhận storyboard có sẵn? Plan để tool nhận storyboard trực tiếp; prompt→storyboard là 1 step agent-side không cần code.

<!-- slug: light-video-creation-pipeline -->


## UPDATE 2026-09-13 — Làm rõ của anh (trả lời Open Questions)

1. **"Không nhét vào hệ thống" = không vendor các project Remotion/OmniVoice/Piper vào binary GoClaw** — giữ nguyên kiến trúc worker tách biệt của plan (contract + sidecar). ✅ plan đã đúng hướng, nhấn mạnh: không import code gateway vào worker và ngược lại.
2. **Chất lượng & kích thước tùy chỉnh được** (trả lời OQ-2): mỗi job được phép override `resolution` (480p/720p/1080p), `bitrate`, `fps`, `aspect` (9:16 | 16:9 | 1:1); default 720p 9:16. Worker nhận đầy đủ qua storyboard JSON, gateway chỉ validate phạm vi hợp lệ. → cập nhật phase-01 (schema) + phase-02 (ffmpeg args).
3. **Nguồn nhạc nền (trả lời OQ-3): KHÔNG commit nhạc vào git** — dùng **music pack qua manifest** (JSON: id, title, duration, license, url, credit) sync/caching khi cần:
   - **Wikimedia Commons** (API ổn định, lọc CC0/CC-BY, direct file URL) — nguồn chính.
   - **OpenGameArt.org** (filter CC0, nhạc pack game, direct download) — nguồn phụ.
   - **Incompetech (Kevin MacLeod)** CC-BY 4.0 — cho phép, nhưng phải **auto-credit** dòng attribution vào final video/description.
   - Worker cache track theo id (LRU, cap dung lượng); job chỉ tham chiếu `music_id`. Pack builder script = việc nhỏ trong phase-04/06.
4. OQ-4 (agent tự sinh storyboard) giữ mở — kiến trúc không chặn: tool `render_video` nhận storyboard JSON, agent có thể được thêm prompt→storyboard sau.
5. **OQ-4 ĐÃ CHỐT (anh chọn "tự sinh HOẶC nhận chủ đề")**: agent được hỗ trợ sinh storyboard từ chủ đề qua LLM (agent-side, không cần code mới ở tool — nhưng Phase 3 phải: (a) tool schema có mô tả + ví dụ storyboard đầy đủ để agent sinh đúng shape, (b) system prompt guidance "khi user đưa chủ đề → tự viết storyboard 4–12 scene, mỗi scene có narration + visual", (c) Phase 6 composer cho user chỉnh tay rồi render lại. Editing = sửa storyboard (chat hoặc composer) → re-render; TTS/ảnh cache theo scene để re-render nhanh.
