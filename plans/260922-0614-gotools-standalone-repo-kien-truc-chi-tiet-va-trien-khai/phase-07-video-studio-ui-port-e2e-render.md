---
title: "Phase 7: Video studio UI port + E2E render"
status: todo
priority: P1
effort: "1.5d"
dependencies: [5, 6]
---

# Phase 7: Video studio UI port + E2E render

## Overview
Port trang Video Studio (trang lớn nhất) nối vào jobs API phase 6 + designer video (skill `video-storyboard-design` inline) phase 5. Client-side export MediaRecorder giữ nguyên. Chốt E2E render đầy đủ trên server thật.

## Requirements
- Functional: designer column chat sinh storyboard blocks → storyboard editor render; submit render server → panel tiến trình (SSE) → download MP4; cancel; template gallery + timeline + canvas player preview hoạt động; export client-side (MediaRecorder) vẫn chạy.
- Non-functional: lazy route chunk (trang nặng nhất); mobile: canvas player responsive 16:9/9:16; không poll vô hạn khi không có job active.

## Architecture
- Copy vào `web/src/port/pages/tools/video/`: page `video-tool-page.tsx`; components: canvas-player, designer-column, icon-picker, render-panel, render-shared, scene-card, scene-transition, storyboard-card, template-gallery, timeline; hooks: use-canvas-player, use-timeline, use-video-export (giữ nguyên), use-video (sửa transport), use-designer-chat (phase 5 đã port — dùng chung, kind: "video"); lib: icon-library, parse-storyboard-blocks, templates.
- **use-video.ts sửa**: `/v1/video/jobs*` cùng path (phase 6 khớp) — chỉ đổi event: `use-ws-event` → fetch-stream subscribe SSE hub (hook mới `use-sse-event.ts` wrapper reconnect + dispatch query invalidate); query key `["video","jobs"]` giữ.
- **Designer video**: settings `designer.video.*`; prompt assembly thêm `outputContract("video")` + skill `video-storyboard-design` (+`video-color-motion` nếu prompt gốc tham chiếu) — cùng endpoint phase 5, kind route.
- **Download link**: `/v1/files/videos/{name}?ft=` — backend trả job kèm signed URL sẵn (như goclaw `video.go:59-66`), UI chỉ gắn href.
- **Worker status**: render-panel hiện badge worker up/down (từ /health) — submit disable + tooltip khi down.

## Related Code Files
- Create: `web/src/port/pages/tools/video/**` (danh sách đầy đủ ở Architecture), `web/src/app/hooks/use-sse-event.ts`
- Reference: `ui/web/src/pages/tools/video/**` (nguồn), `ui/web/src/pages/tools/video/hooks/use-video.ts:40-108` (query + event), `ui/web/src/pages/tools/video/hooks/use-video-export.ts:105-224` (client export — giữ nguyên)

## Implementation Steps
1. Copy trang + components + libs + PROVENANCE; sửa imports; route /tools/video + nav.
2. use-sse-event wrapper (fetch-stream + reconnect backoff 2s + onEvent) + use-video đổi nguồn event.
3. Designer video: thêm kind "video" vào prompt assembly (skills + contract) — copy skills vào assets/skills/.
4. Wire download href + worker badge.
5. i18n toolbox namespace video (en/vi — copy từ goclaw bỏ phần dư).
6. E2E trên server anh (192.168.1.103, port 18890/18891): designer sinh storyboard 3 cảnh → submit → SSE tiến trình → MP4 tải về chơi được (check cả 9:16); cancel giữa chừng; export client-side 1 video ngắn.
7. Performance pass: lazy chunk OK, không block shell; prune console.log.

## Success Criteria
- [ ] E2E render 3 cảnh + 9:16 + 16:9 OK; MP4 play chuẩn (VLC + mobile)
- [ ] Cancel dừng thật (worker + DB status cancelled)
- [ ] SSE badge/progress phản ánh <2s; worker down hiển thị rõ
- [ ] MediaRecorder export vẫn hoạt động (không regression do port)
- [ ] Lighthouse: route chunk lazy, shell không chậm thêm

## Risk Assessment
- Trạng thái job lạc sau reconnect SSE: on reconnect → refetch list 1 lần (đồng bộ estado) — đơn giản, không cần replay buffer.
- Icon/template assets lớn: copy assets theo lib icon-library; nếu >2MB tổng → dynamic import theo nhóm (note, chỉ làm nếu đo chậm).
