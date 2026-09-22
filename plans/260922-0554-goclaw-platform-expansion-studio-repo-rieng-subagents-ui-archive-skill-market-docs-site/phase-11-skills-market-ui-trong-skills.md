---
title: "Phase 11: Skills: market UI trong /skills"
status: todo
priority: P1
effort: "1.5d"
dependencies: [10]
---

# Phase 11: Skills: market UI trong /skills

## Overview
Tab "Market" trong trang /skills hiện có: grid skill theo category, tìm kiếm, cài/gỡ/cập nhật, multi-select, progress job, và chuyển sang gán agent ngay sau khi cài. Người dùng tự chọn skill muốn thêm vào goclaw — hết thời add cứng.

## Requirements
- Functional: tab Market cạnh list skill hiện có; card: icon theo category, tên, mô tả (truncate), badge "Đã cài vX" / "Có bản mới", nút Cài / Gỡ / Cập nhật; multi-select + "Cài (N)"; hàng tiến trình job (progress % + log cuối) theo poll; sau cài xong → dialog gán agent (reuse `skill-agent-grants-dialog.tsx` có sẵn) hoặc "Về skills của tôi".
- Non-functional: admin-only actions (cài/gỡ) — viewer chỉ xem; mobile grid `grid-cols-1 sm:grid-cols-2 lg:grid-cols-3`, touch ≥44px; i18n ×5 namespace `skills.json` (hiện en 270 dòng — thêm group `market.*`); search client-side đủ nhanh (≤200 mục) + khớp BM25 server có sẵn nếu muốn (chọn client-side đơn giản).

## Architecture
- Route: tab trong `pages/skills/skills-page.tsx` (state tab, không route mới — theo pattern tab các trang khác); tabs: "Đã cài" (list hiện tại) | "Market".
- Hook `use-skill-market.ts`: react-query `["skills","market"]` → GET /v1/skills/market; mutation install/uninstall → trả job_id → poll `["skills","market","job",id]` 1s tới terminal (pattern job MCP store card).
- Components (`pages/skills/market/`): `market-tab.tsx` (toolbar search + category chips + grid), `market-card.tsx` (card + actions + confirm gỡ — ConfirmDialog có sẵn), `install-progress-row.tsx` (progress bar + log cuối), `post-install-grants.tsx` (mở `skill-agent-grants-dialog.tsx` với skill vừa cài).
- Batch install: multi-select → install N slugs 1 job (backend nhận mảng) — 1 progress row tổng.
- Kit card đặc biệt: "goclaw-kit — cả bộ 115 skill" nút "Cài cả bộ" (install all slugs) cho ai muốn bản full như cũ.

## Related Code Files
- Create: `ui/web/src/pages/skills/market/market-tab.tsx`, `market-card.tsx`, `install-progress-row.tsx`, `ui/web/src/pages/skills/hooks/use-skill-market.ts`
- Modify: `ui/web/src/pages/skills/skills-page.tsx` (tabs), i18n `skills.json` ×5 (group `market.*`), có thể `ui/web/src/lib/query-keys.ts`
- Reference: `ui/web/src/pages/skills/skill-agent-grants-dialog.tsx` (reuse — tên file có tiền tố `skill-`, audit), Store MCP cards (pattern job UI), `ui/web/src/pages/store/` (card grid pattern)

## Implementation Steps
1. i18n keys `market.*` ×5 trước (quy tắc #15).
2. Hook + types theo response phase 10.
3. Market-tab + cards + toolbar; state select + batch.
4. Job progress row + poll; error state + retry.
5. Post-install grants flow + kit card install-all.
6. Mobile pass + dark mode + E2E tay trên server anh: cài 2 skill → gán agent → agent dùng skill → gỡ 1.
7. `pnpm build` + typecheck xanh.

## Success Criteria
- [ ] E2E tay: cài skill qua UI → dùng được bởi agent → gỡ sạch (file + DB + khỏi list "Đã cài")
- [ ] Job progress hiển thị thật (log dòng cuối), lỗi hiện rõ + retry được
- [ ] Viewer không thấy nút Cài/Gỡ (chỉ admin)
- [ ] Mobile 375px: grid 1 cột, không tràn ngang, touch đủ
- [ ] i18n ×5 không chuỗi hardcode

## Risk Assessment
- Cài cả bộ 115 skill tốn thời gian (job dài): job batch hiển thị "x/N", cho cancel job giữa chừng (đánh dấu các slug còn lại skipped); khuyến cáo trong UI dùng core + chọn lẻ.
- Catalog lớn render chậm: grid 200 card — dùng virtualization chỉ nếu đo >200ms (không sớm tối ưu).
