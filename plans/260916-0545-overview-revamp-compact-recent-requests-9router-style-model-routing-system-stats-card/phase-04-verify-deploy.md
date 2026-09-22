---
phase: 4
title: "Verify + deploy"
status: pending
priority: P1
effort: "~2h"
dependencies: [1, 2, 3]
---

# Phase 4: Verify + deploy

## Overview

Kiểm tra toàn diện theo Post-Implementation Checklist (AGENTS.md) rồi deploy lên server 192.168.1.103 theo flow đã dùng ở các session trước.

## Requirements

- Toàn bộ checklist build/test xanh trước khi scp.
- Deploy xong phải xác nhận bằng dữ liệu live, không tự khai "done".

## Implementation Steps

1. Go checks:
   ```bash
   go fix ./...
   go build ./...
   go build -tags sqliteonly ./...
   go vet ./...
   go test ./internal/sysstats/
   ```
2. Web build: `cd ui/web && pnpm build` → copy `ui/web/dist/* internal/webui/dist/`.
3. Cross-compile + deploy (flow chuẩn của branch):
   ```bash
   GOOS=linux GOARCH=amd64 go build -tags embedui -o goclaw-v412-linux.bin .
   ssh root@192.168.1.103 'systemctl stop goclaw'
   scp goclaw-v412-linux.bin root@192.168.1.103:/opt/goclaw/goclaw
   ssh root@192.168.1.103 'systemctl start goclaw && systemctl status goclaw --no-pager'
   ```
   (videoworker không đổi — không restart.)
4. Smoke test live:
   - `curl` unauth `/v1/system/stats` → 401; authed → 200 JSON, CPU% > 0 ở lần 2.
   - `/overview`: routing ellipse render, recent requests scroll gọn, System card hiện số khớp `top`/`free -h` trên server.
   - Mobile 375px (DevTools): không tràn ngang.
5. Screenshot lưu vào `plans/<plan-dir>/reports/` (verify-*.png) để anh xem.

## Success Criteria

- [x] Mọi command ở bước 1-2 exit 0.
- [x] Service `goclaw` active sau restart, health check OK.
- [x] Live smoke checklist bước 4 đạt hết.
- [x] Báo cáo cho anh kèm 1-2 screenshot trước/sau.

## Risk Assessment

- **Restart làm rớt session đang chạy:** restart lúc không có render job nào (`/v1/video/jobs` không có status rendering — check trước khi stop).
- **gopsutil hỏng cross-compile:** phát hiện ở bước 1 (build linux local được trước khi scp) — fallback theo phase 1 Risk.
