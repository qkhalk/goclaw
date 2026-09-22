---
title: "Phase 1: Scaffold repo GoTools"
status: todo
priority: P2
effort: "2d"
dependencies: []
---

# Phase 1: Scaffold repo GoTools

## Overview
Dựng repo mới `qkhalk/gotools` (Go server + Vite SPA + SQLite) với khung chạy được: serve SPA nhúng, đăng nhập token admin, CI build binary Linux. Đây là móng cho phase 2–4 port 3 tool studio sang.

## Requirements
- Functional: repo riêng trên GitHub của anh; `go build` ra 1 binary serve web app ở cổng riêng (mặc định 18890); login bằng admin token (env/config); health endpoint.
- Non-functional:không import bất kỳ package `goclaw/internal/...` nào (Go cấm cross-module internal) — mọi code dùng chung là copy-port có đánh dấu PROVENANCE; build tĩnh CGO_ENABLED=0.

## Architecture
```
GoTools/
  server/           Go module github.com/qkhalk/gotools
    main.go         flags: --addr, --config, --data-dir
    config.go       JSON5 + env overlay (port pattern goclaw config_load.go)
    auth.go         1 admin token, constant-time compare (port internal/http/auth.go:382-460)
    db.go           modernc.org/sqlite, migrations embed (bảng: llm_providers, video_jobs,
                    designer_sessions, settings, job_events)
    webui.go        go:embed dist (port pattern goclaw embedui)
  web/              Vite 6 + React 19 + TS + Tailwind 4 (đối chiếu ui/web/package.json)
    src/port/       MỌI file copy từ goclaw đặt đây, header PROVENANCE
    (kit slice: components/ui/*, components/shared/*, stores use-auth/use-toast/use-ui,
     api/http-client.ts — thêm getBaseUrl() configurable, lib/utils, lib/format,
     hooks use-media-query/use-virtual-keyboard, i18n framework + namespace
     toolbox/tools/common × en,vi,zh,ko,ru)
  deploy/           gotools.service (systemd), Dockerfile, Caddy/nginx snippet domain riêng
  .github/workflows/ci.yml   pnpm build web → go build linux-amd64 → artifact/release
```
Data dir mặc định `~/.gotools/`. Port 18890 (tránh đụng 18790 gateway / 18791 videoworker).

## Related Code Files
- Create: toàn bộ repo mới (danh sách ở Architecture)
- Reference (nguồn port, KHÔNG sửa): `ui/web/package.json`, `ui/web/vite.config.ts`, `ui/web/src/api/http-client.ts`, `internal/http/auth.go:382`, `cmd/gateway.go` (embedui pattern)

## Implementation Steps
1. Tạo repo GitHub `GoTools` (private), init Go module + Vite app, copy `package.json` deps cơ bản (react, react-dom, react-router, react-query, zustand, tailwind 4, lucide, framer-motion — bỏ radix chưa cần).
2. Copy kit slice vào `web/src/port/` (mỗi file gắn header `// Ported from goclaw@<sha> <path>`); cấu hình vite alias `@/port`.
3. Port i18n framework (i18next setup) + 4 namespace × 5 locale từ `ui/web/src/i18n/`.
4. Viết `server/`: config + auth + sqlite migrate + embed SPA + `/health` + serve login page.
5. Viết login page (port `pages/login/login-page.tsx` TokenForm pattern, store `gotools:auth`).
6. CI workflow + systemd unit + README deploy ngắn.
7. Build + chạy local: login OK, health 200.

## Success Criteria
- [ ] Repo riêng tồn tại, CI xanh (web build + go build linux)
- [ ] Binary chạy ở :18890, serve login + shell trống, token sai → 401
- [ ] Không có import `goclaw/internal` trong go.mod graph (verify `go list -m all`)
- [ ] Mọi file port có header PROVENANCE

## Risk Assessment
- Copy nhầm dependency sâu (radix/dialog chain): kit slice giữ tối thiểu — nếu component cần radix thì copy đúng component đó + thêm dep (accept, ghi lại trong port/MANIFEST.md).
- Đánh dấu PROVENANCE thiếu sót → sync sau này khó chịu: thêm script `scripts/check-provenance.(sh|ps1)` fail CI nếu file trong `port/` thiếu header.
