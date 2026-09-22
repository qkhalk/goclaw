---
title: "Phase 12: Docs: VitePress site + GitHub Pages (repo riêng goclaw-docs)"
status: todo
priority: P3
effort: "1.5d"
dependencies: []
---

<!-- Updated: Validation Session 1 - docs chuyển sang repo riêng goclaw-docs + sync PR thủ công (quyết định anh) -->

# Phase 12: Docs: VitePress site + GitHub Pages (repo riêng goclaw-docs)

## Overview
Docs site công khai cho goclaw tại **repo riêng `qkhalk/goclaw-docs`** → `https://<user>.github.io/goclaw-docs/` — VitePress (đúng generator trang 9router tham khảo: VitePress + i18n routing `/en/`, GitHub Pages base path), locales en + vi, search local. Sync nội dung từ goclaw bằng **PR thủ công** (có dòng checklist nhắc trong PR template goclaw).

## Requirements
- Functional: repo `goclaw-docs` build VitePress; locale en (mặc định, `/en/`) + vi (`/vi/`); navbar + sidebar đủ mục; local search; dark/light; landing hero quick-start; workflow push main → build → deploy Pages.
- Non-functional: không dịch vụ ngoài (search = minisearch built-in); build < 2 phút CI; en là ngôn ngữ chính, vi phủ trang chủ + getting started (mở rộng dần).
- Sync policy (quyết định validate): KHÔNG auto-trigger từ repo goclaw — mỗi lần goclaw đổi feature có ảnh hưởng user-facing thì PR thủ công sang goclaw-docs; PR template goclaw thêm 1 dòng checkbox "docs updated? (goclaw-docs PR nếu cần)".

## Architecture
```
qkhalk/goclaw-docs/          (repo riêng — toàn bộ nội dung docs sống ở đây)
  .vitepress/config.ts       locales {en, vi}, base '/goclaw-docs/' (env BASE override),
                             themeConfig nav/sidebar per-locale, search local
  en/ index.md getting-started/install.md configuration.md providers.md
      agents.md chat.md subagents.md skills-market.md gotools.md
      channels/telegram.md api/http.md api/ws-protocol.md self-hosting.md
      troubleshooting.md (gồm "trả lời ngáo checklist" — phase 7)
  vi/ index.md getting-started.md ... (subset mở rộng dần)
  public/logo.svg
  .github/workflows/docs.yml on: push main → pnpm install → vitepress build →
                              actions/upload-pages-artifact + actions/deploy-pages
```
- Cấu trúc nav theo 9router (Getting Started / Features / Channels / API / Deployment), giọng văn docs chuẩn, landing tiết chế emoji.
- Nguồn nội dung (viết 1 lần khi migrate, sau đó sync thủ công): README.md goclaw, AGENTS.md (viết lại thành "Architecture" public), `deploy/README-video-worker.md`, kinh nghiệm vận hành 192.168.1.103 (systemd, migrate tay — note từ mcp-catalog-installer deployment), quyết định validate session này.
- Trang `gotools.md`: giới thiệu repo GoTools (3 tool + admin model) + hướng deploy — vì GoTools là repo riêng nên docs tổng nằm ở goclaw-docs.
- Cổng Pages repo goclaw-docs: Settings → Pages → Source: GitHub Actions (1 lần tay).

## Related Code Files
- Create (repo goclaw-docs): toàn bộ cây ở Architecture + package.json pin exact vitepress version + workflow docs.yml
- Modify (repo goclaw): `.github/PULL_REQUEST_TEMPLATE.md` thêm dòng checkbox docs (1 dòng); README goclaw thêm link `https://<user>.github.io/goclaw-docs/`
- Reference: https://vibecoder11200.github.io/9router/ (VitePress + `/en/` locale + base path pattern), README.md, AGENTS.md

## Implementation Steps
1. Tạo repo goclaw-docs, scaffold vitepress + config locales/base/search + CI workflow.
2. Viết content en phần lõi: index + getting-started + install + configuration + providers (dùng giá trị config thật từ repo goclaw — không bịa).
3. Trang tính năng: subagents (phase 6/7), skills-market (phase 10/11), gotools (repo mới), telegram (lệnh + nút archive).
4. API docs: HTTP /v1 bảng endpoint chính + WS protocol tóm tắt (sinh bảng từ `pkg/protocol/methods.go` bằng script nhỏ nếu tiện).
5. vi subset: index + getting-started + 2-3 trang chính.
6. PR template goclaw + link README; bật Pages; verify URL live + search.
7. Ghi quy trình sync docs vào README goclaw-docs (ai cập nhật gì thì PR trang nào).

## Success Criteria
- [ ] URL Pages live, load <3s, dark/light, search tìm "subagents" ra đúng trang
- [ ] Locale switch en↔vi hoạt động; trang vi thiếu → fallback en (VitePress default)
- [ ] CI goclaw-docs chỉ chạy trong repo đó; build xanh <2 phút
- [ ] Quick-start copy-paste chạy được (docker run theo README goclaw — verify lệnh thật)
- [ ] PR template goclaw có dòng nhắc docs

## Risk Assessment
- Docs lệch code vì sync thủ công (rủi ro đã chấp nhận khi chọn tách repo): giảm nhẹ bằng checklist PR + mục "Cập nhật docs" trong Success Criteria các phase goclaw (7, 10, 11 đã có bước docs note).
- Repo docs bị bỏ quên nếu abandon: mỗi workstream phase có "docs note" trỏ về goclaw-docs — thói quen giữ sống.
- VitePress version drift: pin exact version trong package.json.
