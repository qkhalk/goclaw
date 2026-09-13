---
phase: 6
title: "Phase 6: Web UI — trang video jobs + storyboard composer tối giản"
status: pending
priority: P2
effort: "4-5 ngày (có thể tách nhỏ, xem Deferred)"
dependencies: [3, 4]
---

# Phase 6: Web UI — trang video jobs + storyboard composer tối giản

## Overview

Web UI (`ui/web/`) bổ sung: (1) **trang/section Video Jobs** — danh sách job, trạng thái, progress realtime qua WS event `video.job.updated`, xem/xóa deliverable; (2) **storyboard composer tối giản** — form tạo storyboard v1 (thêm scene ảnh/màu, caption, narration text + voice, sắp xếp thứ tự), nút "Render" gọi job. Composer P1 là **JSON editor có validate**; form kéo-thả là P3.

Mức ưu tiên: **P2 cho jobs list** (vận hành cần thấy job), **P3 cho composer form** (agent làm được qua tool rồi). Ghi rõ ở Deferred: nếu cần cắt scope, jobs list giữ, composer form hoãn — không chặn gì pipeline.

## Requirements

- [ ] Route `/video` (danh sách) + `/video/:jobId` (chi tiết) theo pattern routes hiện có (`ui/web/src/routes.tsx`); menu entry trong app shell
- [ ] Jobs list: bảng có `overflow-x-auto` + `min-w-[600px]` (Mobile UI rules), cột: id ngắn, status badge, progress, engine, created, actions (cancel/xóa)
- [ ] Realtime: cập nhật hàng qua WS event `video.job.updated` (subcribe pattern event hiện có của web UI); fallback refetch 10s
- [ ] Chi tiết job: storyboard JSON viewer (read-only, có copy), output video player + download, error hiển thị khi failed
- [ ] Composer JSON editor: textarea + validate client-side theo schema v1 (reuse validation rules Phase 1 dịch sang TS) + submit → POST tạo job; cảnh báo khối lượng render (tổng duration)
- [ ] Composer form (P3): danh sách scene dạng card, add/remove/reorder (nút lên/xuống, không DnD để giữ nhẹ), trường caption + narration voice (dropdown từ `GET /v1/video/voices`), preview giá ước tính thời gian
- [ ] i18n đủ 5 locale `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/` (thực tế repo có 5 locale dirs — đã verify; lưu ý AGENTS.md ghi 3 nhưng bằng chứng trong tree là 5, dùng 5)
- [ ] Mobile rules AGENTS.md: `h-dvh`, input `text-base md:text-sm`, dialog full-screen mobile, safe-area, touch target ≥44px

## Architecture

```
ui/web/src/
├ pages/video/           # VideoJobsPage, VideoJobDetailPage
├ components/video/      # StoryboardEditor (JSON), SceneCardList (P3), StatusBadge
├ api/video.ts           # GET /v1/video/jobs, GET {id}, DELETE, POST render, GET voices
└ i18n/locales/*/video.json   # namespace riêng "video" theo pattern namespace-split
```

- Data fetching theo pattern api/hooks hiện có của web UI (xem `src/api/` + `src/hooks/` cùng nhóm pages/cloud làm mẫu); WS: dùng client WS sẵn có, register handler cho `video.job.updated`, update Zustand store cục bộ.
- Không thêm dependency mới (không video editor lib, không DnD lib) — giữ bundle nhẹ đúng tinh thần dự án.
- RBAC: page yêu cầu auth; action cancel/xóa theo role hiện có (viewer read-only) — dùng helper permission của web UI hiện có.

## Related Code Files

### Create

- `ui/web/src/pages/video/VideoJobsPage.tsx`
- `ui/web/src/pages/video/VideoJobDetailPage.tsx`
- `ui/web/src/components/video/JobStatusBadge.tsx`
- `ui/web/src/components/video/StoryboardJsonEditor.tsx` (P2) / `SceneCardList.tsx` (P3)
- `ui/web/src/api/video.ts`
- `ui/web/src/types/video.ts` — mirror storyboard v1 + job types (camelCase khớp `pkg/protocol`/HTTP shapes)
- `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/video.json`

### Modify

- `ui/web/src/routes.tsx` — 2 route mới
- App shell nav (file menu tương đương nơi khai báo items cho pages/cloud) — entry "Video"
- `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/` index/namespace imports
- `ui/web/src/api/client.ts` (hoặc nơi export api modules) — export video api

## Implementation Steps

1. Types + api client + hooks (list với polling, WS handler) — không UI trước.
2. i18n namespace `video.json` đủ 5 locale (keys: status_* , action.cancel, action.delete, composer.*...) — **trước khi render UI dùng keys** (missing key crash quy ước i18next config hiện có).
3. VideoJobsPage + detail (P2 hoàn chỉnh): list, badge, progress, WS update, player, download.
4. StoryboardJsonEditor + POST render (P2).
5. SceneCardList form (P3): scene CRUD + reorder + voices dropdown + ước tính duration; dựng storyboard JSON từ form (không song song với editor — 1 nguồn truth là JSON, form là view).
6. Mobile pass theo checklist AGENTS.md (viewport, touch targets, dialog, table scroll).
7. `pnpm lint && pnpm build` + smoke `pnpm dev` với gateway local.

## Success Criteria

- [ ] `/video` hiển thị jobs, tự cập nhật khi agent tạo job mới (không F5)
- [ ] Chi tiết job done: video play được ngay trong page (stream từ workspace file serving hiện có), download OK
- [ ] Composer JSON: dán storyboard hợp lệ → render → job xuất hiện trong list; storyboard lỗi → thông báo validate cụ thể (scene nào, lỗi gì)
- [ ] i18n: switch 5 ngôn ngữ không thấy key trần; parity test locale xanh
- [ ] Mobile 375px: bảng cuộn ngang, nút chạm đủ, dialog composer full-screen; không layout vỡ
- [ ] `pnpm build` không tăng bundle đáng kể (không thêm lib mới — so sánh size trước/sau ghi trong PR)

## Risk Assessment

| Risk | Observable signal | Pre-decided response |
|------|-------------------|----------------------|
| WS event không tới client (reconnect, filter session) | Hàng job không cập nhật tới khi F5 | Fallback refetch 10s thiết kế sẵn; WS handler là enhancement, không phải dependency chức năng |
| File serving MP4 chưa phủ path `videos/` trong workspace | Video 404 ở detail page | Check `internal/http/files.go` 2-layer path isolation (AGENTS.md) — nếu workspace-scope cho phép mọi file trong workspace thì không cần đổi; nếu whitelist, thêm `videos/` vào scope cho phép (thay đổi nhỏ backend, ghi vào phase này thay vì Phase 3) |
| Composer form phình scope (ken_burns UI, timeline...) | PR vượt effort | Freeze: form chỉ 3 scene type + caption + narration; mọi trường nâng cao chỉnh qua JSON editor; phần còn lại là backlog |
| Locale files lệch keys giữa 5 ngôn ngữ | UI hiện key trần khi đổi ngôn ngữ | Thêm keys 1 PR cho cả 5 file cùng lúc + parity test hiện có làm lưới |

**RAM/CPU footprint:** 0 backend mới (chỉ gọi API Phase 3); client bundle +~15–25KB gzip (pages + types, không lib mới).

### Deferred (nói rõ cái gì có thể hoãn)

- P3: SceneCardList form (giữ JSON editor là đủ dùng cho P2 release)
- P3: preview thumbnail scene (cần thêm endpoint sinh thumbnail — hoãn)
- Backlog: kéo-thả timeline, preview render nháp 360p (cần worker endpoint preview — backlog chung với templates marketplace)
