---
phase: 5
title: "Phase 5: Remotion Tier-2 sidecar (tùy chọn, TẮT mặc định)"
status: pending
priority: P2
effort: "3-4 ngày (nếu làm)"
dependencies: [3, 4]
---

# Phase 5: Remotion Tier-2 sidecar (tùy chọn, TẮT mặc định)

## Overview

Tier-2 render cho storyboard cần hiệu ứng phức tạp mà ffmpeg filtergraph không diễn đạt nổi (animation DOM, chart động, subtitle karaoke...): sidecar **Remotion** (Node.js + headless Chrome) chạy như một engine thứ hai cạnh engine `ffmpeg`, chọn qua trường `engine` trong storyboard. **Trạng thái: optional, feature-gated, OFF mặc định, chỉ khuyến nghị máy ≥2GB RAM.** Trên máy 1 vCPU/1GB: tuyệt đối không bật (Remotion render qua headless Chrome ~300MB–1GB+ RAM/instance, chính thức khuyến nghị ~2GB RAM per render thread).

Phase này có thể **hoãn vô thời hạn** mà không ảnh hưởng P1 — gateway đã coi engine là chuỗi opaque từ Phase 1; toàn bộ giá trị Phase 5 nằm ở worker-side.

## Requirements

- [ ] Gateway chấp nhận `engine: "remotion"` chỉ khi config bật (`video.remotion.enabled=true`, default false); ngược lại từ chối create job với lỗi i18n rõ ràng
- [ ] Remotion sidecar expose cùng worker job API contract (Phase 1) trên port riêng — gateway không có code path đặc biệt, chỉ khác `worker_url` theo engine
- [ ] Worker Tier-1 forward job cho sidecar khi `engine != "ffmpeg"` và sidecar reachable; sidecar chết → job quay về `failed` với error chỉ dẫn, không crash worker
- [ ] Cảnh báo tài nguyên khi bật: log warn + docs ghi rõ yêu cầu ≥2GB, concurrency=1, video ≤1–2 phút
- [ ] Không thay đổi gì cho Tier-1: bật/tắt remotion không đụng path ffmpeg

## Architecture

```
videoworker (Tier-1, Go)                        remotion-sidecar (Node, riêng process/máy)
├ engine == "ffmpeg" → tự render                ├ POST /v1/jobs (cùng contract Phase 1)
├ engine == "remotion" & enabled                ├ render: Chrome headless (gl=swiftshader,
│    → forward POST sang sidecar, poll          │   concurrency=1), ffmpeg bundle của Remotion
│    → mirror output về output-dir              └ RAM: 0.3–1GB+/job — CHỈ máy ≥2GB
└ engine khác → failed "unknown engine"
```

- **Routing decision ở worker Tier-1** (không phải gateway): giữ gateway đơn giản — gateway chỉ validate engine được phép theo config rồi đẩy như thường. Worker biết địa chỉ sidecar qua flag `--remotion-url` (trống = không có sidecar → engine remotion trả failed ngay).
- Sidecar implementation: project Remotion tối thiểu — 1 composition `Storyboard` đọc storyboard JSON + assets dir, render `--gl=swiftshader --concurrency=1`, output MP4; HTTP wrapper Node nhỏ (Express hoặc stdlib http của Node) đặt cùng contract Phase 1 (chỉ cần 3 endpoint).
- Storyboard → Remotion mapping: mỗi `scene` = một `<Sequence>`; ken_burns/caption map sang CSS animation; narration audio dùng cùngNarrator output (sidecar nhận audio files materialize sẵn từ worker — worker synth narration TRƯỚC khi forward để không nhân bản voice layer sang Node).

## Related Code Files

### Create

- `sidecar/remotion/package.json` + `remotion.config.ts` — project Remotion (pnpm)
- `sidecar/remotion/src/Storyboard.tsx` — composition map scenes → Sequences
- `sidecar/remotion/src/server.ts` — HTTP wrapper cùng contract Phase 1
- `sidecar/remotion/README.md` — install (node ≥20, pnpm i, npx remotion browser ensure), RAM guidance, ví dụ systemd unit riêng `MemoryMax=1500M`
- `cmd/videoworker/internal/vworker/forward.go` — forward job + mirror output (Modify: runner routing)
- `internal/config/config_video.go` — thêm `RemotionConfig{Enabled *bool, URL string}` (Modify)
- `internal/video/validate.go` — engine allowlist theo config (Modify)
- `internal/i18n/` — lỗi "engine remotion chưa bật" 5 catalogs (Modify)

### Modify

- `cmd/videoworker/internal/vworker/runner.go` — engine routing: ffmpeg local / forward remotion / failed
- `internal/tools/render_video.go` — chỉ thêm validate engine khi config bật
- `deploy/README-video-worker.md` — mục Tier-2

## Implementation Steps

1. Config + gateway validate `engine` (default chỉ "ffmpeg"); i18n lỗi.
2. Worker `forward.go`: forward + poll + mirror output; test httptest sidecar giả (đúng contract, gồm case sidecar 500/timeout).
3. Sidecar Remotion: composition tối giản (image+caption+ken_burns trước; các scene type khác sau), server.ts, test render 1 storyboard 10s local máy dev (không chạy trong CI — Chrome quá nặng).
4. Docs + systemd unit riêng + cảnh báo RAM.
5. Checklist: Go build 2 tags; sidecar `pnpm lint && pnpm build` (ci.yaml hiện chỉ build web — không thêm CI job cho sidecar trong phase này, ghi rõ).

## Success Criteria

- [ ] Config mặc định: storyboard `engine:"remotion"` bị chặn ở tool với lỗi i18n; không request nào tới worker
- [ ] Bật config + sidecar chạy: job engine remotion render xong MP4, gateway hiển thị như job thường (không phân biệt engine ngoài field)
- [ ] Sidecar bị kill giữa job → gateway thấy job failed sau poll timeout, worker Tier-1 vẫn render bình thường job ffmpeg kế tiếp
- [ ] Đo RAM trên máy test ≥2GB: ghi số liệu thật vào PR (kỳ vọng 0.3–1GB+); docs khẳng định "không bật trên ≤1GB"

## Risk Assessment

| Risk | Observable signal | Pre-decided response |
|------|-------------------|----------------------|
| OOM khi bật remotion trên máy 1GB (dù docs cấm) | systemd OOM kill sidecar; box treo | Config default OFF + gateway chặn; unit sidecar có `MemoryMax=1500M` để chết có kiểm soát thay vì kéo sập box |
| Remotion API đổi nhanh (phiên bản mới break composition) | `pnpm build` lỗi sau khi upgrade | Pin exact version trong package.json; composition giữ tối giản; sidecar là optional tier nên break không ảnh hưởng P1 |
| Forward loop/t thất bại âm thầm làm job kẹt `rendering` | Job stuck > 30 phút | Worker forward có timeout + poll timeout gateway 30 phút (Phase 3) là lưới cuối — test case sidecar timeout bắt buộc |
| effort lan man (composition phức tạp hóa) | Phase vượt effort 4 ngày | Scope freeze: composition chỉ hỗ trợ đúng 3 scene type của storyboard v1; hiệu ứng ngoài v1 → backlog, không mở rộng trong phase |

**RAM/CPU footprint:** gateway không đổi; worker +~2MB (forward logic); sidecar riêng 0.3–1GB+/job khi chạy, 0 khi tắt. Đây là lý do phase này là P2 optional và tắt mặc định.
