---
title: "Journal 2026-09-15: QA browser-panel + plans + lint unblock"
date: 2026-09-15
---

# 2026-09-15 — QA live browser panel, ak:plan updates, fork-dev lint unblock

## Deploy
- Integration branch (origin/dev 8d110616b + browser-panel 074b8dfa3 + tools-redesign a81b65993) → build `v4.5.0-integration.20260915` (embedui, linux/amd64, UI pnpm build 45.8s) → scp /opt/goclaw → restart → HTTP 200, active.

## QA browser panel (item 1) — bằng chứng gốc
- **Bug A (trang SPA trắng) — root cause CHỨNG MINH:** live iframe `sandbox="allow-scripts allow-forms"` thiếu `allow-same-origin` (browser-panel.tsx:46) → opaque origin → `localStorage` ném SecurityError → SPA crash lúc boot. Harness A/B: A(current)=SecurityError, B(+allow-same-origin)=OK. Site thật: neo.doralove.io.vn kẹt splash ở A, render đầy đủ UI đăng nhập ở B (vision xác nhận). neo bundle có 9 chỗ localStorage; ai.1k.io.vn static text ~51 chars < ngưỡng 80 → auto-live; /dashboard 307→/login.
- **Bug B (kéo mở rộng vỡ layout) — repro + đo:** pane 320→700px nhưng cột chat bị nghiền 288px (< floor 360), sidebar ép 220px; text wrap vỡ dòng (vision). Nguyên nhân: dynamicMax dùng store sidebar width, flex shrink các cột thật; thiếu min-width cứng cột chat. Shield iframe trong drag ĐÃ có và hoạt động (chat-side-pane.tsx:71-77). Double-click reset OK (700→384).
- **Bug C (mới):** toggle "Chế độ tĩnh" bị override ngầm về live vì relay vẫn thin → setMode("live") (use-browser-panel.ts:446-448).
- Cập nhật plans/browser-panel-improvements.md: mục QA evidence + 1.9 (sandbox fix có guard origin) + 1.10 (modeSource user/auto) + 2.8 root cause confirm + 3.1 thêm option A0.

## ak:plan
- Plan mới `plans/260915-1800-goclaw-design-studio-skill-set` (ak plan create + add-phase ×3, validate OK): 12 skill design nguyên bản (không port skill cá nhân), kit.yaml, Go test BM25, deploy + pin 3 skill lõi cho video-designer, E2E bài báo công nghệ. 4 phase.
- Plan `260915-1243-video-designer-agent-column` phase-02: thêm section Design direction theo ak:frontend-design (preserve-mode shadcn, empty-state chips, min-width canvas 480 guard từ bài học Bug B).

## Fork dev CI unblock (item 4 follow-up)
- 3 lỗi ESLint trên origin/dev (= qkhalk/goclaw dev): drive-file-area.tsx:133 (no-useless-assignment), canvas-player.tsx:97 (no-unused-expressions), video-tool-page.tsx:467 (_scenes — thiếu varsIgnorePattern/ignoreRestSiblings trong eslint config).
- Fix trên worktree riêng `.goclaw-lintfix` (branch fix/dev-eslint): eslint.config.js thêm `varsIgnorePattern:"^_"` + `ignoreRestSiblings:true`; if/else thay ternary-statement; `let cmp: number;` bỏ init chết. Full `eslint .` 0 errors, `tsc --noEmit` sạch. Push `0f0dd3f03` → fork dev. PR #78/#80 web check sẽ xanh sau re-run.

## E2E video từ bài báo công nghệ (item 5) — THÀNH CÔNG
- Nguồn: bài VNExpress 15/9 UBTech (nhà máy Liễu Châu, 10.000+ robot hình người/năm). Storyboard 7 cảnh 29s 1080x1920@30, output 720p, 2 ảnh CDN có chữ ký `?w=&s=`.
- 4 lỗi phát hiện dọc đường, 3 đã fix + deploy (commit `6256d6a9b`, `de1996554` trên feat/client-browser-panel — CHƯA push):
  1. videoworker chết + systemd không tự restart → start thủ công.
  2. Ảnh vnecdn 401 (URL trần) → dùng URL có chữ ký + thêm User-Agent/Accept browser vào `downloadAsset` (assets.go).
  3. Worker queue deadlock: Submit đếm `len(r.jobs)` (bao gồm job done không dọn) + dispatcher bỏ qua `resp.Status` từ chối lúc submit → đếm đúng theo status + `failJob` khi bị từ chối (runner.go, dispatcher.go).
  4. drawtext vỡ filtergraph với dấu `:`/`,` trong caption tiếng Việt → `captionFilterValue()` ghi `caption_%03d.txt` + `textfile=` (ffmpeg.go).
- Kết quả: job `cc13b9ad` Hoàn tất, 712.145 bytes, h264 1080x1920, duration 29.000s chính xác, render ~38s. Vision xác nhận caption tiếng Việt đúng dấu (scene màu "ROBOT SẢN XUẤT ROBOT", scene ảnh UBTech + caption dưới, viền đen).
- Job `798710b9` kẹt "Đang render" từ trước fix deadlock (worker đã restart) — kẹt vĩnh viễn, nút Hủy trong UI click timeout; để lại làm evidence, anh có thể Hủy tay.
- Evidence: `.tmp-browse/e2e-joblist.png` (UI Hoàn tất + Tải MP4), `.tmp-browse/e2e-video-cc13b9ad.mp4`, `.tmp-browse/frame_color2.jpg`, `.tmp-browse/frame_img.jpg`.
- Ghi chú: goclaw-videoworker.service thiếu `Restart=always` (worker idle-exit 08:23 làm chết pipeline lúc 18:07) — nên thêm.

## Next
- Báo cáo cuối tổng hợp cho anh (xong trong session này).
