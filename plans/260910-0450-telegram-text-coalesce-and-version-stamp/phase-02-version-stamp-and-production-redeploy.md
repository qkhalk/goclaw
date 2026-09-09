---
title: "Phase 2: Version stamp and production redeploy"
status: todo
dependencies: [1]
---

# Phase 2: Version stamp and production redeploy

## Overview

Thay binary production `/opt/goclaw/goclaw` trên server 192.168.1.103 (đang là build stamp `0.1.0-4phases.2`) bằng build mới từ dev sau khi merge Phase 1 — đóng dấu `cmd.Version` đúng (git describe, kỳ vọng dạng `v3.17.4-NN-gxxxx` hoặc 3.17.5 nếu anh muốn số tròn), restart gateway, xác nhận UI hiển thị version đúng.

## Requirements

- [ ] Build Linux amd64 từ dev mới nhất (sau merge PR của Phase 1) với:
      `CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X github.com/nextlevelbuilder/goclaw/cmd.Version=<VERSION>" .`
      VERSION lấy từ `git describe --tags --abbrev=0 --match "v[0-9]*"` + số commit lệch (khớp convention Makefile:1-2); nếu fork có tag mới hơn dùng tag đó.
- [ ] Backup binary cũ trên server (`/opt/goclaw/goclaw.bak-0.1.0`) trước khi thay — rollback 1 lệnh.
- [ ] Xác định cách gateway đang được chạy (systemd unit / supervisor / nohup — `systemctl list-units | grep -i goclaw`, `ps aux`) và restart đúng cơ chế đó; KHÔNG đụng QLTC/9router/zyrox.
- [ ] Sau restart: `/opt/goclaw/goclaw version` in version mới; `GET /health` trả ok; web UI sidebar hiện version mới (WS connect `server.version`); bot Telegram still connected.
- [ ] Binary test cũ `/root/goclaw-linux-test` dọn hoặc ghi đè bằng bản mới (tránh nhầm bản `dev-local`).

## Architecture

Không đổi code (trừ Phase 1). Chuỗi version đã verify: `cmd.Version` (ldflags) → `server.SetVersion` (`cmd/gateway.go:647`) → WS `connect` response `server.version` (`internal/gateway/router.go:406-409`) → `useAuthStore.serverInfo.version` (`ui/web/src/api/ws-client.ts:319`) → sidebar `connection-status.tsx:45-46`. Thay binary là đủ — không cần đụng UI.

## Related Code Files

- Modify (ops): `/opt/goclaw/goclaw` trên 192.168.1.103 (backup → thay → restart).
- Create (local, tạm): `goclaw-linux-test` build artifact — không commit.

## Implementation Steps

1. Merge PR Phase 1 vào dev; pull dev mới nhất.
2. Xác định VERSION (git describe / tag fork).
3. Build binary Linux trong container golang:1.26 local (đúng quy trình anh đặt: build local, không build trên server).
4. Scp lên server vào `/opt/goclaw/goclaw.new`; `goclaw version` thử binary trước khi thay.
5. Backup cũ → swap → restart theo đúng service manager đang dùng → health check (HTTP /health + log telegram connected + WS connect version).
6. Rollback plan: `mv goclaw.bak-0.1.0 goclaw` + restart nếu có vấn đề.

## Todo

- [ ] PR Phase 1 merged
- [ ] Binary build + stamp đúng
- [ ] Backup + swap + restart
- [ ] Health check + UI version xác nhận

## Success Criteria

- [ ] `/opt/goclaw/goclaw version` → `goclaw <VERSION> (protocol 3)`.
- [ ] Web UI sidebar: "Đã kết nối · <VERSION>" (không còn 0.1.0).
- [ ] Bot Telegram online, tin dài test thấy agent nhận đủ (cùng lúc verify Phase 1 live).

## Risk Assessment

- **Restart gián đoạn session đang chạy** (~vài giây): anh vừa tự deploy 20:56 hôm nay nên đang chủ động quản lý box này; restart là điều kiện cần của fix. Thông báo trước khi restart trong luc làm; rollback binary giữ sẵn.
- **Cơ chế chạy gateway không rõ (systemd/nohup/screen)**: xác định TRƯỚC khi stop (`systemctl`, `ps ppid`) — stop sai cách có thể không tự quay lại. Nếu không xác định được → dừng, báo anh cách đang chạy.
- **Version stamp lệch kỳ vọng (3.17.5 vs git describe)**: mặc định dùng git describe (truth của repo); nếu anh muốn đúng chữ "3.17.5" thì stamp thủ công — ghi rõ trong báo cáo.
