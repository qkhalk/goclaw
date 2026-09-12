---
phase: 2
title: "Web UI Cloud page + accounts API"
status: completed
priority: P1
effort: "1.5d"
dependencies: ["phase-1"]
---

# Phase 2: Web UI Cloud page + accounts API

## Overview

Trang `/cloud` trong Web UI: danh sách tài khoản đã kết nối (Google), nút Connect chạy OAuth redirect flow thật (không paste URL), disconnect, hiển thị redirect URI để cấu hình GCP, i18n 5 locale.

## Requirements

- Functional:
  - Route `ROUTES.CLOUD = "/cloud"` trong `ui/web/src/lib/routes.ts` (re-export qua `lib/constants.ts:3`), lazy route trong `ui/web/src/routes.tsx` dưới `<AppLayout>` (pattern ProvidersPage `routes.tsx:63-65,191-192`) — **không** RequireAdmin (mỗi user quản lý account của mình).
  - Sidebar: item "Cloud" trong connectivity group (`ui/web/src/components/layout/sidebar.tsx:103-112`), label qua `useTranslation("sidebar")`; ẩn khi edition Lite (đã có pattern `isAdmin` conditional `:105-111` — dùng edition từ store).
  - Trang `ui/web/src/pages/cloud/`:
    - Header + status banner: chưa cấu hình client_id → hướng dẫn setup (link docs) + hiển thị **exact redirect URI** cần đăng ký trong GCP console (lấy từ `POST /v1/cloud/oauth/google/start` response).
    - Account card: icon provider, email, display_name, status chip (active/expired/revoked/error + status_message), token expiry, scopes (collapse), nút Disconnect (confirm dialog), nút Test (phase 5 wired — để UI sẵn, gọi `/v1/cloud/accounts/{id}/test` nếu có).
    - Connect flow: bấm "Connect Google" → `POST start` → `window.location.href = auth_url`; về `/cloud?connected=<email>` → toast thành công + refresh list; `?error=` → toast lỗi.
    - Empty state + loading + error states đầy đủ (pattern providers page).
  - API client `ui/web/src/api/cloud.ts` trên `HttpClient` (`ui/web/src/api/http-client.ts` — get/post/delete; headers auth sẵn `:114-126`).
  - i18n: namespace `cloud.json` + key sidebar trong **5 locale dir** `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/`.
  - Mobile UI rules theo AGENTS.md: grid `grid-cols-1 sm:grid-cols-2`, dialog full-screen mobile (`max-sm:inset-0` — `ui/dialog.tsx` sẵn), touch target ≥44px.
- Non-functional: không render token/scopes nhạy cảm ngoài nhu cầu hiển thị; Disconnect cần confirm; mọi string qua i18n key.

## Architecture

SPA không có route callback riêng — gateway callback 302 về `/cloud?connected=` nên trang chỉ cần đọc `useSearchParams` một lần rồi cleanup URL (replace). Tương tự pattern read-once param đã dùng ở các trang detail. Data fetching: hook `useCloudAccounts` + `useCloudStatus` (pattern `pages/providers/hooks/use-chatgpt-oauth-provider-statuses.ts`).

**Phối hợp cross-plan:** plan in-progress `260912-1137-web-chat-declutter` cũng sửa sidebar — item Cloud nằm ở connectivity group (plan kia tập trung /chat page) nên conflict thấp; nếu conflict merge thì rebase phần sidebar.tsx.

## Related Code Files

- Create: `ui/web/src/pages/cloud/{cloud-page.tsx, account-card.tsx, connect-button.tsx, hooks/use-cloud-accounts.ts}`
- Create: `ui/web/src/api/cloud.ts`
- Modify: `ui/web/src/lib/routes.ts`, `ui/web/src/lib/constants.ts`, `ui/web/src/routes.tsx`
- Modify: `ui/web/src/components/layout/sidebar.tsx`
- Create: `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json` + sidebar key trong 5 `sidebar.json`

## Implementation Steps

1. API client + hooks + types (match response shapes của phase 1 — lấy từ Go struct json tags).
2. Page + card + connect flow + query-param handling; i18n skeleton en trước.
3. Dịch 4 locale còn lại (vi/zh/ko/ru) — dùng chính catalog translations sẵn làm reference văn phong.
4. Sidebar + routes + edition gating.
5. Manual: connect 2 tài khoản thật, disconnect 1, reload trang, mobile viewport 375px check.

## Success Criteria

- [x] Connect 2 Gmail → 2 card hiện đúng email/status; disconnect → biến mất sau confirm; reload giữ nguyên.
- [x] Redirect URI hiển thị đúng `https://<host>/v1/cloud/oauth/callback` cho anh paste vào GCP.
- [x] Chưa cấu hình client_id → banner setup hướng dẫn (không crash, không nút Connect chết).
- [x] Edition Lite: không thấy nav Cloud; chỉnh sửa edition trong dev → hiện.
- [x] i18n: chuyển 5 locale không còn chuỗi hardcode; mobile 375px không tràn ngang.
- [x] `pnpm build` sạch (CI=true trong Docker).

## Risk Assessment

- **OAuth popup vs redirect**: dùng full-page redirect (đơn giản, khỏi popup blocker); nếu anh muốn giữ ngữ cảnh trang thì nâng cấp sau thành window.open + postMessage — không block v1.
- **connected param replay khi refresh**: cleanup URL ngay sau khi đọc (history.replaceState) — tránh toast lặp.
- **Đồng bộ i18n với phase 5** (backend strings): namespace UI riêng `cloud` nên không đụng backend catalog — chỉ cần giữ key naming nhất quán (`cloud.connect`, `cloud.status.active`...).
