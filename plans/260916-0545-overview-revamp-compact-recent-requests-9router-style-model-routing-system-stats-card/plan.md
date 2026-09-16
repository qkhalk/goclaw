---
title: "Overview revamp: compact recent requests, 9router-style model routing, system stats card"
description: "Gọn gàng trang Overview: thu nhỏ Recent Requests, làm lại Model routing theo phong cách Usage dashboard của 9router (topology ellipse + bảng theo model), thêm thẻ System (CPU/RAM/Disk) ở cuối trang với endpoint /v1/system/stats mới."
status: pending
priority: P2
effort: "~1.5-2 ngày (4 phase)"
tags: [web-ui, overview, dashboard, system-stats, gopsutil, i18n]
created: 2026-09-16
---

# Overview revamp: compact recent requests, 9router-style model routing, system stats card

## Overview

Ba thay đổi trên tab **Tổng quan** (`/overview`, `ui/web/src/pages/overview/overview-page.tsx`) theo yêu cầu anh:

1. **Thu nhỏ Recent Requests** — hiện chiếm 3/5 độ rộng (`overview-page.tsx:278-285`) và cao dần theo 8 dòng. Chuyển thành thẻ nhỏ 2/5, chiều cao cố định khớp thẻ routing, scroll nội bộ với sticky header (đúng kiểu bảng Recent Requests của 9router trong `UsageStats.js`: height 480, thead sticky, divide-y).
2. **Model routing giống 9router dashboard/usage** — thay graph 3 cột hiện tại (`routing-graph-card.tsx`, providers → GoClaw → models bezier) bằng topology **ellipse** như `ProviderTopology.js` của 9router: các provider node xếp quanh node trung tâm GoClaw trên đường ellipse, **edge màu theo trạng thái** (xanh = đang hoạt động, đỏ = có lỗi, xám = idle), **ping dot** trên provider có traffic, badge số request ở node trung tâm. Bổ sung bên dưới **bảng Usage by model** kiểu `UsageTable.js` (model | provider | requests | in/out | % bar).
3. **Thẻ System ở cuối overview** — CPU %, RAM used/total, Disk (data dir), host uptime, load average, goroutines/heap. Backend chưa có gì (grep `gopsutil|runtime.NumCPU` = 0 kết quả trong `internal/`, `cmd/`) → cần endpoint mới `GET /v1/system/stats` dùng `github.com/shirou/gopsutil/v4`.

## Contract (brainstorm)

- **Outcome:** Operator mở `/overview` thấy routing topology trực quan như 9router, recent requests gọn không chiếm diện tích, và thẻ SystemHealth-type hiển thị sức khỏe phần cứng host.
- **Constraints:** Giữ nguyên design system shadcn/Tailwind hiện có (preserve-mode, dashboard preset variance 3 / motion 2 / density 6); mobile rules theo AGENTS.md (overflow-x-auto + min-w cho bảng, stack 1 cột 375px); i18n đủ en/vi/zh; không thêm lib chart mới (recharts 3.8.1 đã có trong `ui/web/package.json:52`, topology vẽ SVG thuần như hiện tại — không thêm @xyflow/react vì chỉ 5-10 node); auth `requireAuth("")` (GET → viewer floor); phải build được cả 2 tags (`go build ./...` và `-tags sqliteonly`).
- **Non-goals:** Không làm trang Combos/routing-rules CRUD (GoClaw không có khái niệm combo); không SSE/realtime stream (giữ polling 30s hiện có; "active" suy ra từ recent-requests); không sửa trang Usage; không đổi WS protocol (`pkg/protocol` giữ nguyên — endpoint mới là HTTP thuần).
- **Acceptance criteria:**
  - [ ] Recent requests: 2/5 width, max-height khớp cột routing, ~6 dòng nhìn thấy, scroll mượt, thead sticky.
  - [ ] Routing: providers xếp ellipse quanh GoClaw, edge emerald (active) / red (errors>0) / gray (idle), ping dot, badge tổng calls ở node giữa; dưới có bảng per-model với % bar; <620px fallback về edge list (giữ behavior hiện tại `routing-graph-card.tsx:149-182`).
  - [ ] `/v1/system/stats`: 401 khi chưa auth, viewer đọc được; JSON shape như phase 1; CPU% có giá trị thật từ request thứ 2.
  - [ ] Thẻ System cuối trang: 3 thanh CPU/RAM/Disk (amber >80%, đỏ >95%) + hàng meta; i18n 3 ngôn ngữ; mobile không tràn ngang.
  - [ ] `go build ./...`, `go build -tags sqliteonly ./...`, `go vet`, `pnpm build` pass; deploy lên 192.168.1.103 thành công.

## Kiến trúc đã chọn (scout + researcher verified)

- **Dữ liệu routing đã đủ**, không cần backend mới cho phase 2: `GET /v1/usage/routing` (`internal/http/usage.go:69-105`) trả `{window_hours, edges: [{provider, model, calls, input_tokens, output_tokens, errors}]}`. "Active" cho ping dot + edge màu: dùng thêm `GET /v1/usage/recent-requests?limit=20` (đã có `internal/http/usage.go:41-65`) — provider xuất hiện ở đó trong poll gần nhất = active (xanh). Edge có `errors > 0` = đỏ. Còn lại xám.
- **System stats cần backend mới**: handler `httpapi.NewSystemStatsHandler(dataDir)` + `GET /v1/system/stats`, register theo đúng pattern hiện có — `SetXxxHandler` append vào `s.handlers` (`internal/gateway/server.go:250-254, 682-691`), wire tại `cmd/gateway_http_wiring.go` (cạnh `SetUsageHandler`, dòng ~148). Dependency `gopsutil/v4` (MIT, cross-platform Linux/Windows/macOS — desktop Wails build vẫn chạy).
- **Layout mới của tab overview** (từ trên xuống): 5 StatCards → SystemHealthCard → ConnectedClients + CronJobs → **Routing (3/5) + Recent Requests (2/5)** → QuotaUsageCard → **SystemCard (mới, cuối trang)**.

## Design Read (ak:frontend-design Decision Procedure)

1. **Design Read:** Reading this as product/dashboard UI for the gateway operator, preserve-mode trên hệ shadcn hiện có, ngôn ngữ data-dense utilitarian, leaning restrained product (dashboard preset: variance 3, motion 2, density 6).
2. **Seeded variation:** Ảnh nguồn là hợp đồng — anh chỉ định rõ "làm giống Model routing ở dashboard/usage của 9router", nên layout topology ellipse + bảng model copy theo 9router (exception path "user explicitly asked"), chỉ re-skin bằng tokens của GoClaw (không mang màu indigo #6366f1 của 9router sang).
3. **Aesthetic thesis:** Utilization-first dashboard cho self-hosted gateway: nền card tinted neutral, accent trạng thái emerald/amber/red chỉ dùng cho trạng thái, số liệu mono tabular-nums, signature = ellipse topology với edge đổi màu theo trạng thái, điểm nhớ = ping dot nhấp nháy trên provider đang có traffic.
4. **Motion:** chỉ 150-250ms (ping animation + hover), `prefers-reduced-motion` tắt ping (Tailwind `motion-reduce:animate-none`).

## Mục tiêu Cross-Surface Parity

| Surface | Có đổi không | Ghi chú |
|---|---|---|
| Gateway server | CÓ | `internal/http/system_stats.go` mới + gopsutil dep + wiring |
| API contract | CÓ | Shape `/v1/system/stats` documented trong phase 1; không đổi `pkg/protocol` (HTTP-only) |
| Web UI | CÓ | `ui/web/src/pages/overview/*` + i18n en/vi/zh |
| CLI/runtime | N/A | Endpoint read-only dashboard, không có CLI command liên quan |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Backend /v1/system/stats](./phase-01-backend-system-stats.md) | Pending |
| 2 | [Phase 2: Overview UI — 9router-style routing + compact recent requests](./phase-02-routing-recent-requests.md) | Pending |
| 3 | [Phase 3: Overview UI — System card](./phase-03-system-card.md) | Pending |
| 4 | [Phase 4: Verify + deploy](./phase-04-verify-deploy.md) | Pending |

## Success Criteria

- [ ] Toàn bộ acceptance criteria ở phần Contract đạt.
- [ ] Phase 1-3 xong thì `pnpm build` local pass; Phase 4 deploy + xác nhận live trên 192.168.1.103.
- [ ] Không regression: các card khác của overview (StatCards, SystemHealth, Clients, Cron, Quota) nguyên vẹn.

## Risk Assessment

- **gopsutil trong Docker:** đọc /proc của host kernel — CPU% phản ánh host chứ không riêng container. Deployment hiện tại là binary trực tiếp trên host (không Docker) nên chính xác; ghi chú limitation trong comment.
- **CPU% lần đầu = 0:** `cpu.Percent(0, false)` trả delta từ lần gọi trước → sample ngay lúc khởi tạo handler + giữ state package-level có mutex; UI hiển thị "—" khi `cpu_percent == 0 && samples < 2`.
- **Nhiều provider làm vỡ ellipse:** clamp tối đa 8 provider node (như MAX_NODES=5 hiện tại), phần dư ghi label "+N more" (pattern `moreLabel` sẵn có).
- **Windows/desktop (sqliteonly):** `load.Avg()` lỗi trên Windows → trả pointer nil, UI ẩn chip; gopsutil v4 support windows nên build không vỡ.
