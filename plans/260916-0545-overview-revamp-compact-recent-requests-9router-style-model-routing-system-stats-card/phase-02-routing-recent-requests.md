---
phase: 2
title: "Overview UI: 9router-style routing + compact recent requests"
status: pending
priority: P2
effort: "~5-6h"
dependencies: []
---

# Phase 2: Overview UI — 9router-style routing + compact recent requests

## Overview

Làm lại thẻ **Model routing** theo đúng topology ellipse của 9router (`ProviderTopology.js` trong `src/shared/components/`) và **thu nhỏ Recent Requests** thành bảng cuộn gọn như `UsageStats.js` (fixed height, sticky thead). Đổi tỉ lệ lưới từ 2:3 thành **3:2** nghiêng về routing.

## Requirements

- Functional: giữ nguyên 2 nguồn dữ liệu hiện có (`/v1/usage/routing`, thêm `/v1/usage/recent-requests?limit=20` cho trạng thái active); fallback edge-list <620px giữ nguyên hành vi.
- Non-functional: vẽ SVG thuần (không thêm @xyflow/react — chỉ 5-10 node, YAGNI); ping animation tắt được bằng `motion-reduce:animate-none`; bảng bọc `overflow-x-auto` theo mobile rules.

## Architecture

### 2a. RoutingGraphCard v2 (`routing-graph-card.tsx` viết lại phần graph, giữ fallback list)

**Layout ellipse (công thức từ ProviderTopology.js):**

```
n = providers.length (clamp ≤ 8, dư → moreLabel)
rx = max(190, ((130+16) * n) / (2π))   // nén lại cho card ~560px (9router dùng 180+24 cho trang full)
ry = max(120, rx * 0.55)
container height = 2*ry + NODE_H (~280-340px desktop)
center = (width/2, height/2)
provider_i: angle = -π/2 + 2πi/n; x = cx + rx*cos(angle) - NODE_W/2; y = cy + ry*sin(angle) - NODE_H/2
```

- **Node trung tâm GoClaw:** pill border-2 primary, icon Bot, tên, badge số tổng calls trong window (9router hiển thị active count; mình dùng tổng 24h vì không có SSE).
- **Provider node** (~150×48): rounded-lg border-2, màu viền = trạng thái; trong node: ô vuông màu 8×8 chứa chữ cái đầu provider (không có PNG logo như 9router — dùng letter avatar màu hash, tái dùng `dotColor()`), tên, **ping dot** (span animate-ping, 9router `ProviderTopology.js` styling) khi provider active.
- **Trạng thái & màu edge** (map từ 9router: green active / amber last-used / red error / gray idle — mình gộp amber vào active vì không track last-used riêng):
  - `errors > 0` (edge hoặc provider-level) → **red** (`text-red-500`), viền đỏ nhạt.
  - active (provider có mặt trong `recent-requests` fetch gần nhất) → **emerald** + ping dot.
  - còn lại → **gray** (`text-muted-foreground/35`, giữ như hiện tại).
- **Edge:** bezier từ node provider vào tâm (path `C` như `curve()` hiện tại :196-199), stroke = màu trạng thái, width 1.5.
- **Bảng Usage by model** bên dưới topology (mới, kiểu UsageTable.js thu gọn): top 8 model sort theo calls desc — cột: Model (mono + chấm màu provider) | Requests | In/Out (arrowUp rose / arrowDown emerald như RecentRequestsCard) | **% bar** (share calls, thanh `<div>` 6px rounded bg-primary/emerald, tabular-nums %). Bọc `overflow-x-auto` + `min-w-[480px]` bảng.
- Poll giữ `refetchInterval: 30_000`; thêm query `["usage","recent-requests","active"]` limit 20 → `Set<string>` provider active (dùng chung response shape `RecentLLMRequest` — export type từ `recent-requests-card.tsx` hoặc move vào `types.ts`).

### 2b. RecentRequestsCard v2 (compact)

- Giữ nguyên query + cấu trúc bảng; thay đổi:
  - Card bọc `flex flex-col h-full` + `overflow-hidden`; `CardContent` → `flex-1 overflow-y-auto` với `max-h` tự co theo cột (khớp chiều cao routing card qua grid stretch).
  - `<thead className="sticky top-0 bg-card z-10">` (sticky như 9router).
  - Row gọn: `py-2` (từ 2.5), ẩn dòng cost phụ — chuyển cost vào `title` của model; limit 8→10 (vì scroll được).
  - Wrap bảng trong `overflow-x-auto` + `min-w-[320px]`.

### 2c. overview-page.tsx

- Đổi grid `lg:grid-cols-5`: routing `lg:col-span-3`, recent `lg:col-span-2` (từ :278-285).

## Related Code Files

- Modify: `ui/web/src/pages/overview/routing-graph-card.tsx` (viết lại phần desktop graph + thêm bảng model)
- Modify: `ui/web/src/pages/overview/recent-requests-card.tsx` (compact + sticky)
- Modify: `ui/web/src/pages/overview/overview-page.tsx` (:278-285 tỉ lệ lưới)
- Modify: `ui/web/src/pages/overview/types.ts` (export `RecentLLMRequest`, thêm `RoutingEdge` dùng chung)
- Modify i18n **trước khi code**: `ui/web/src/i18n/locales/{en,vi,zh}/overview.json` — keys mới: `routing.byModel` ("Usage by model"), `routing.active`, `routing.idle`, `routing.errors` (nếu thiếu), `recentRequests.ofTotal` (không cần nếu bỏ). Kiểm tra key cũ trước khi xóa key nào.

## Implementation Steps

1. Thêm i18n keys (en/vi/zh) — bước đầu tiên theo quy tắc AGENTS.md #15.
2. Move `RoutingEdge` + `RecentLLMRequest` vào `types.ts`, import lại ở 2 card.
3. Viết lại `RoutingGraphCard` desktop branch: ellipse layout + trạng thái màu + ping + bảng per-model. Giữ nguyên narrow fallback (`width < GRAPH_MIN_WIDTH`).
4. Compact `RecentRequestsCard` (sticky thead, scroll, row gọn).
5. Đổi tỉ lệ lưới trong `overview-page.tsx`.
6. `pnpm build` + `pnpm dev` kiểm tra thủ công: desktop (routing 3 cột, recent 2 cột), thu hẹp <620px (fallback list), mobile 375px (stack 1 cột, không tràn ngang).

## Success Criteria

- [x] Topology ellipse hiển thị đúng với ≥3 provider; màu edge đúng trạng thái; ping chạy trên provider active; reduced-motion tắt ping.
- [x] Recent requests cao fixed ≈ chiều cao routing card, scroll mượt, thead không trôi.
- [x] Bảng per-model: % bar + số liệu đúng tổng của edges.
- [x] Mobile 375px không tràn ngang; fallback list hoạt động.
- [x] i18n 3 ngôn ngữ đủ key mới (không warning missing key trong console).

## Risk Assessment

- **Ellipse vỡ với 7-8 provider trên card hẹp:** clamp rx theo `width` thực (rx = min(rx, width/2 - NODE_W)); nếu vẫn chật, giảm NODE_W xuống 130. Signal: node đè lên nhau trong screenshot 1280px → giảm n tối đa xuống 6.
- **recent-requests fetch thêm 1 nguồn:** nhẹ (limit 20, đã có endpoint), poll cùng chu kỳ 30s — không thêm tải đáng kể.
