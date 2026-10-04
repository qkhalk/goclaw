---
title: "fix(web): chat UI bugs from live E2E — deploy/HEAD divergence + 3 small fixes"
date: 2026-10-04
summary: "Reload-mat-session la deploy-only (HEAD da fix); fix aria-label URL server browser, status bar lech mode, drawer mobile inert"
---

# fix(web): chat UI bugs from live E2E — deploy/HEAD divergence + 3 small fixes

## What happened
E2E tren instance deploy v5.0.8 (192.168.1.103:18790) phat hien 4 bug UI chat.

1. [Nghiem trong] Reload/deep-link /chat/<session-key> -> redirect ve /chat, mat transcript + input.
   Chan doan: KHONG con trong repo HEAD. Verify bang vite dev (ui/web) proxy VITE_BACKEND_HOST sang gateway that + copy `goclaw:auth` localStorage giua 2 origin cung browser profile. HEAD giu nguyen URL, khoi phuc day du transcript + composer (ca desktop va mobile 375px). Nguyen nhan: build deploy cu hon HEAD. Khong sua code — chi can build/redeploy.
2. O URL che do server-browser thieu aria-label -> them key `browserPanel.remote.urlBar` (en/vi/zh) + aria-label (browser-remote-view.tsx).
3. Status bar ngoai van hien note "Trang tinh" khi dang o che do server -> an bar khi remoteMode vi RemoteBrowserView co footer rieng (browser-panel.tsx).
4. Drawer mobile dong van focusable/hien voi AT (translate off-canvas) -> aria-hidden + inert khi dong, giu animation (chat-page.tsx).

## Decision
- Bug 1 khong code thay doi trong repo; redeloy la du.
- Fix 2-4 la content/UI nho, giu nguyen pattern hien co (i18n 3 locale theo AGENTS.md).

## Next steps
- Build lai ui/web va redeploy instance 192.168.1.103 de an bug 1.
- Kiem tra upstream provider openai-compat (oc/mimo-v2.6-flash-free) dang HTTP 502 — ngoai pham vi code.
- Dao tao lai web_browse lan dau tien that bai ngay sau khi mo panel (agent tu retry thanh cong) — can log gateway.

## Gotcha quy trinh
Sidebar chat re-render lien tuc (token counter dem moi giay) lam Playwright locator click luon timeout (khong bao gio dat 2 frame stable). Workaround tin cay: element.click() tu page context, hoac cua click theo toa do da verify. Gap lai voi SPA co dong ho/polling thi dung thu lai locator.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
