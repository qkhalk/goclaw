---
phase: 3
title: "Overview UI: System card (CPU/RAM/Disk)"
status: pending
priority: P2
effort: "~3h"
dependencies: [1]
---

# Phase 3: Overview UI — System card (CPU/RAM/Disk)

## Overview

Thẻ **System** mới ở **cuối** tab overview (sau QuotaUsageCard) hiển thị sức khỏe phần cứng host: 3 thanh CPU / RAM / Disk + hàng meta (host uptime, platform, load average, goroutines, heap, Go version). Nguồn dữ liệu: `GET /v1/system/stats` từ phase 1.

## Requirements

- Functional: poll 15s (nhẹ hơn chu kỳ 30s của usage vì số liệu hardware đổi liên tục); hiển thị "—" cho CPU khi `samples < 2`; ẩn chip khi field absent (load trên Windows, disk khi dataDir trống).
- Non-functional: màu thanh theo ngưỡng (normal = primary, >80% amber, >95% red); không thêm lib — dùng `Progress` sẵn có (`ui/web/src/components/ui/progress.tsx`); số dùng `tabular-nums`.

## Architecture

```
overview-page.tsx
  └─ <SystemCard />  (lazy không cần — endpoint nhẹ; component thường)
       └─ useQuery(["system","stats"], http.get("/v1/system/stats"), refetchInterval 15s)
            └─ response shape = phase 1 contract
```

- Component mới `system-card.tsx` trong `ui/web/src/pages/overview/`.
- Layout trong card: `grid gap-3 sm:grid-cols-3` — 3 ô (CPU / RAM / Disk), mỗi ô: icon (Cpu, MemoryStick, HardDrive — lucide) + label + % lớn `text-lg font-semibold tabular-nums` + dòng phụ used/total (`formatBytes` — có sẵn trong `lib/format`? kiểm tra; nếu chưa có thêm helper) + `<Progress value={pct} className kondisi>`.
  - Progress indicator màu: ép class trên indicator (`[&>div]:bg-amber-500` khi >80, `[&>div]:bg-red-500` khi >95) — Progress dùng var(--primary) mặc định.
- Hàng meta dưới: flex wrap chips (pattern chip của `system-health-card.tsx:167-181`): host uptime (`formatUptime` có sẵn ở `hooks/use-live-uptime.ts`), platform, `load 1/5/15` (ẩn khi nil), `goroutines`, `heap (formatBytes)`, `go_version`.
- CPU "—": backend trả kèm `samples` int (số lần sample kể từ start); `samples < 2` → hiển thị "—" thay vì 0% gây hiểu nhầm.

## Related Code Files

- Create: `ui/web/src/pages/overview/system-card.tsx`
- Modify: `ui/web/src/pages/overview/overview-page.tsx` (thêm `<SystemCard />` sau QuotaUsageCard :288-290)
- Modify: `ui/web/src/lib/format.ts` (chỉ khi chưa có `formatBytes`)
- Modify i18n **trước khi code**: `ui/web/src/i18n/locales/{en,vi,zh}/overview.json` — namespace `system.*`: `title`, `cpu`, `memory`, `disk`, `swap`, `hostUptime`, `platform`, `loadAvg`, `goroutines`, `heap`, `goVersion`, `unavailable`.

## Implementation Steps

1. Thêm i18n keys en/vi/zh (bước đầu tiên).
2. Kiểm tra/.addObject `formatBytes` trong `lib/format.ts`.
3. Viết `system-card.tsx` theo Architecture; handle loading (skeleton 3 ô) + error (ẩn card hoặc dòng "unavailable" — chọn: ẩn hẳn card khi fetch lỗi để không gây nhiễu khi backend cũ chưa có endpoint).
4. Gắn vào `overview-page.tsx` cuối tab overview.
5. `pnpm build` + kiểm tra thủ công 375px (3 ô stack dọc, chips wrap).

## Success Criteria

- [x] Card hiển thị CPU/RAM/Disk với % đúng thực tế server (đối chiếu `top`/`free -h` trên 192.168.1.103 khi deploy).
- [x] >80% amber, >95% red; reduced-motion an toàn (không animation).
- [x] Ẩn card khi endpoint 404/error (backward-compat với backend chưa deploy phase 1).
- [x] i18n 3 ngôn ngữ đủ; mobile không tràn ngang.

## Risk Assessment

- **Backend cũ chưa có endpoint** (deploy web trước backend): card tự ẩn — đã xử lý ở bước 3.
- **Progress component khác variant:** nếu `progress.tsx` không cho override màu con, thay bằng div bar thuần 6px (giống % bar phase 2) — không thêm dep.
