---
title: "Phase 8: Deployment pack + domain + release v1.0.0"
status: todo
priority: P1
effort: "1d"
dependencies: [3, 7]
---

# Phase 8: Deployment pack + domain + release v1.0.0

## Overview
Đóng gói vận hành: systemd ×2, reverse proxy domain riêng (SSE-safe), Dockerfile optional, workflow release tag `v*`, runbook deploy lên server anh cạnh goclaw, release v1.0.0.

## Requirements
- Functional: `deploy/` đủ để lên server mới trong <30 phút: unit gotools.service + gotools-videoworker.service, nginx snippet (hoặc Caddyfile), .env.example, README runbook từng lệnh; CI release tag `v1.0.0` → 2 binary linux-amd64 + tarball (đủ migrations embed sẵn trong binary).
- Non-functional: 2 service cạnh goclaw không đụng port (18890/18891 vs 18790/18791); secret qua env file 600, không commit.

## Architecture
- **systemd units** (port pattern `deploy/README-video-worker.md` goclaw):
  ```ini
  # gotools.service: ExecStart=/opt/gotools/gotools --addr :18890 --data-dir /opt/gotools/data
  # EnvironmentFile=/opt/gotools/gotools.env (GOTOOLS_TOKEN, GOTOOLS_SECRET, GOTOOLS_WORKER_TOKEN)
  # gotools-videoworker.service: ExecStart=/opt/gotools/gotools-videoworker --addr 127.0.0.1:18891 \
  #   --work-dir /opt/gotools/worker/work --output-dir /opt/gotools/worker/out \
  #   --ffmpeg-path /usr/bin/ffmpeg --token $GOTOOLS_WORKER_TOKEN
  # After=network.target; Restart=on-failure; yêu cầu gói: ffmpeg, fonts-noto-cjk
  ```
- **Reverse proxy snippet**: domain placeholder `tools.example.com` (anh điền thật), TLS certbot, location / → proxy_pass 18890 + `proxy_buffering off` + `proxy_read_timeout 3600s` (SSE + render dài) + `X-Accel-Buffering no` đã có từ server.
- **Dockerfile optional** (multi-stage: node build web → go build 2 binary → runtime image debian slim + ffmpeg + fonts; EXPOSE 18890/18891, 1 container 2 process qua supervisord HOẶC compose 2 service — chọn compose 2 service sạch hơn).
- **Release workflow** `.github/workflows/release.yml`: tag `v[0-9]+.[0-9]+.[0-9]+` → build web → embed → `go build` 2 binary (CGO_ENABLED=0, `-ldflags "-s -w -X .../version.Version=${TAG}"`) → tarball `gotools-${TAG}-linux-amd64.tar.gz` (2 binary + deploy/ + README) → GitHub Release. Version hiển thị ở /health + UI footer.
- **Runbook** `deploy/README.md`: từ server trống → cài gói → tạo env → systemd enable → certbot → verify checklist (health, login, 3 tool E2E nhanh); nâng cấp: tải tarball mới → thay binary → restart (migrations tự chạy); backup: cron tar `data-dir` (sqlite + worker outputs) hàng ngày.
- **Deploy thật server anh**: cạnh goclaw trên 192.168.1.103 (đủ RAM — worker chỉ chiếm khi render), domain trỏ DNS, smoke checklist, sau đó tag v1.0.0.

## Related Code Files
- Create: `deploy/{gotools.service,gotools-videoworker.service,nginx.conf.example,Caddyfile.example,gotools.env.example,README.md}`, `deploy/docker/{Dockerfile,docker-compose.yml}`, `.github/workflows/release.yml`, README.md root (quickstart + screenshot)
- Reference: `deploy/README-video-worker.md` (goclaw — pattern unit + ffmpeg deps), `.github/workflows/release.yaml` (goclaw — pattern tag release)

## Implementation Steps
1. Viết 2 unit + env example + nginx/Caddy snippet + runbook.
2. Release workflow + test bằng tag v1.0.0-rc.1 trên repo (kiểm artifact chạy được trên server).
3. Docker compose 2 service (optional, sau binary).
4. Deploy lên 192.168.1.103 theo runbook: cài gói, DNS domain, TLS, smoke checklist đầy đủ (login, watermark, pptx E2E, video render E2E, worker down drill: stop worker → UI báo rõ → start lại).
5. Backup cron data-dir + note restore (thay file sqlite khi stopped).
6. Tag `v1.0.0` release chính thức; README root hoàn thiện (ảnh chụp 3 tool + admin).

## Success Criteria
- [ ] Domain riêng https lên xanh, SSE không bị buffer (test render progress realtime qua domain)
- [ ] goclaw vẫn chạy bình thường sau deploy (port/DB tách biệt — verify 18790 health + 1 chat)
- [ ] Upgrade thử: rc.1 → v1.0.0 bằng thay binary + restart, migrations idempotent
- [ ] Release artifact tải về chạy được ngay trên server trống (theo runbook <30 phút)
- [ ] Backup cron chạy + test restore 1 lần

## Risk Assessment
- Server 512MB thêm 2 process: gotools idle ~20-30MB + worker chỉ spawn khi render (ffmpeg tạm ~100-300MB/lúc render) — lịch render tránh giờ cao; docs ghi budget RAM.
- Domain/DNS anh chưa có sẵn: runbook có nhánh "chạy subpath/ip:port trước, domain sau" (Caddy on-demand TLS optional).
- Migration SQLite mới về sau: giữ nguyên runner version tăng dần, KHÔNG sửa 0001 đã release (lesson goclaw dual-DB).
