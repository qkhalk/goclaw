---
title: "Phase 2: Web shell: port kit + login + nav + i18n"
status: todo
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 2: Web shell: port kit + login + nav + i18n

## Overview
Port khung UI từ goclaw vào `web/src/port/` (PROVENANCE từng file), dựng shell: login token, layout sidebar 3 tool + admin, i18n en/vi, http-client trỏ đúng API GoTools. SPA thật thay placeholder phase 1.

## Requirements
- Functional: mở `/` chưa login → redirect /login; nhập token đúng → vào shell; sidebar: Watermark / PPTX / Video / Quản lí model (placeholder page); logout; toast; dark mode.
- Non-functional: MỌI file copy vào `web/src/port/` có header `// Ported from goclaw@<sha>: <src path>` — `scripts/check-provenance.sh` chặn CI nếu thiếu; mobile rules AGENTS.md (dvh, text-base md:text-sm, touch 44px); chỉ locale en + vi.

## Architecture
```
web/  (Vite 6 + React 19 + TS + Tailwind 4 — mirror deps ui/web/package.json goclaw:
      react, react-dom, react-router, @tanstack/react-query, zustand,
      lucide-react, framer-motion, clsx, tailwind-merge; KHÔNG radix ở shell)
  src/
    port/                    ← MỌI thứ copy từ goclaw
      components/ui/         badge, button, input, label, progress, select,
                             sheet, switch, tabs, textarea
      components/shared/     confirm-dialog, drop-zone, empty-state, page-header
      stores/                use-auth-store (key localStorage "gotools:auth"),
                             use-toast-store, use-ui-store
      api/http-client.ts     sửa: baseUrl = import.meta.env.VITE_API_BASE ?? "" (same-origin)
      lib/utils.ts, lib/format.ts
      hooks/                 use-media-query, use-virtual-keyboard
      pages/login/           login-page + token-form (đổi store key)
    app/
      routes.tsx             /login, / (layout guard), /tools/* (placeholder),
                             /admin/providers (placeholder)
      components/app-shell.tsx  sidebar + topbar + <Outlet/>
    i18n/
      index.ts (react-i18next), locales/{en,vi}/{common,toolbox,admin}.json
```
- Dev proxy: vite.config.ts `server.proxy['/v1'] = 'http://127.0.0.1:18890'` + `/health` (giữ dev trải nghiệm giống goclaw `ui/web/vite.config.ts:20-26`).
- Auth guard: zustand persist + route wrapper redirect (port tinh thần goclaw RequireAuth).
- i18n: copy framework setup goclaw; locale files mới viết (en đầy đủ, vi đầy đủ; KHÔNG copy ko/ru/zh).

## Related Code Files
- Create: cây ở Architecture + `port/MANIFEST.md` (bảng map nguồn → đích + sha goclaw gốc)
- Reference: `ui/web/package.json`, `ui/web/vite.config.ts:20-26`, `ui/web/src/api/http-client.ts`, `ui/web/src/stores/use-auth-store.ts:101-105`, `ui/web/src/pages/login/**`, `ui/web/src/i18n/` (framework setup)

## Implementation Steps
1. Scaffold Vite + TS + Tailwind 4 theo goclaw ui/web (copy vite.config/tsconfig sửa alias), cài deps tối giản.
2. Copy port kit theo manifest (mỗi file thêm PROVENANCE + sửa import path `@/port/...`).
3. http-client: baseUrl env + bearer từ gotools auth store + error normalize (401 → logout redirect).
4. Login page + auth store + guard + logout.
5. App shell + routes placeholder 4 mục nav + active state + mobile (sheet sidebar).
6. i18n setup + 3 namespace en/vi; kiểm không chuỗi hardcode (grep text trong JSX).
7. check-provenance script + thêm vào CI.
8. `pnpm build` xanh; server serve dist thật (phase 1 webui embed); E2E tay login→shell→logout.

## Success Criteria
- [ ] Login token sai → thông báo; đúng → shell; refresh giữ phiên; logout về /login
- [ ] 401 từ API bất kỳ → tự redirect login (interceptor)
- [ ] CI check-provenance pass (100% file port có header)
- [ ] `pnpm build` + `tsc` xanh; dark mode toggle hoạt động; mobile 375px không vỡ

## Risk Assessment
- Chain dependency component (ui/select dùng radix?) — nếu cần radix thì thêm dep + ghi MANIFEST (chấp nhận, quan trọng là đúng bản bản goclaw dùng).
- Tailwind 4 config khác 3: copy nguyên `@theme`/css setup từ goclaw ui/web/src/index.css (PROVENANCE) thay vì viết mới.
