---
phase: 3
title: "Drive-style shell (rail + topbar + routes)"
status: pending
priority: P1
effort: "2d"
dependencies: [2]
---

# Phase 3: Drive-style shell (rail + topbar + routes)

## Overview

Cấu trúc lại trang Clouds theo bố cục Google Drive (theo screenshot của anh): **left rail** (danh sách tài khoản/drive roots có icon, prefix 🏢 cho account shared, card Bộ nhớ/quota), **top bar** (breadcrumb, ô search, sort, toggle grid/list, refresh), **file area** dạng grid card hoặc bảng. Toàn bộ navigation state đưa lên URL — route `/cloud/:provider?/:accountId?` + `?path=` query — theo rule "URL params as source of truth" (AGENTS.md:279), sửa đúng điểm hiện `selectedProvider`/`detailTab`/`path` đang là local state (scout GAPS #25, §Constraints). Phase này chưa có thao tác ghi — chỉ browse + navigation (read-only giữ nguyên như hiện tại). Kèm dọn dẹp: consolidate 3 bản duplicate byte formatter về `formatFileSize`, sửa bảng `min-w-[540px]` → `min-w-[600px]`.

## Surface parity

- **Backend:** không đổi (dùng nguyên GET accounts/about/files có sẵn — `internal/http/cloud.go:64,67,68`).
- **API contract:** N/A — không endpoint mới.
- **Web UI:** toàn bộ phase này.
- **CLI/runtime:** N/A.
- **Desktop UI:** N/A (verify Phase 1). Build sqliteonly backend vẫn phải xanh (không đổi Go code trong phase này).

## Requirements

### Functional
- Route mới: `/cloud` (home: rail + khu "Drive của tôi" liệt kê accounts dưới dạng card + khu kết nối provider + coming-soon), `/cloud/:provider` (provider view), `/cloud/:provider/:accountId` (file browser của account). `?path=` điều khiển thư mục hiện tại (encode URL). `ROUTES` thêm pattern (`ui/web/src/lib/routes.ts:25` hiện chỉ có `CLOUD: "/cloud"`); thay `<Route path={ROUTES.CLOUD}>` tại `ui/web/src/routes.tsx:200`.
- Provider/account/path INVALID → redirect/empty-state, không crash: provider không hợp lệ → về `/cloud`; accountId không trong danh sách accessible → empty-state "không tìm thấy".
- Left rail: mục "Tài khoản" nhóm theo provider (google/onedrive), mỗi account một item (icon provider + email, 🏢 prefix nếu `shared` — dùng lại quy ước `scope-bindings-panel.tsx:102-105`); card quota của account đang mở (GET `about` — query `["cloud","about",accountId]` tại `account-detail.tsx:70-74`, thanh màu đỏ>90%/vàng>75%/xanh giữ nguyên logic 110-127); nút "Mail" mở dialog `MailboxPreview` (261-309) cho account google có Gmail.
- Top bar: breadcrumb từ `?path=` (giữ logic split/decode của `account-detail.tsx:156-163` nhưng navigate bằng `setSearchParams`), ô search lọc client-side theo tên (GAP #11 server search để backlog), Select sort (tên/size/mod-time, tăng/giảm — sort dirs-first client-side hiện có 165-169 mở rộng), toggle grid/list (icon `LayoutGrid`/`List`), nút Refresh.
- File area **grid mode** (mặc định): card folder/file với `FileIcon` (`ui/web/src/components/shared/file-tree-file-icon.tsx` — thay icon lucide thô `account-detail.tsx:227,238`); **list mode**: bảng tên/size/mod-time trong `overflow-x-auto` + `min-w-[600px]`. View mode lưu localStorage ( preference theo thiết bị, không cần deep-link).
- Dashboard stats 3-card (298-328) + provider picker (332-388) giữ ở trang `/cloud` home; paste-back panel (400-419) và connect flow giữ nguyên hoạt động (dời vị trí nếu cần).
- Settings gear (Phase 2) mount trong shell mới; ScopeBindingsPanel nhận provider từ settings sheet (không còn phụ thuộc `selectedProvider`).

### Non-functional
- KHÔNG duplicate route param vào useState (AGENTS.md:279 — tránh race setState/navigate). `path` chỉ tồn tại trong `useSearchParams`.
- ErrorBoundary key: `AppLayout` đã dùng `stableErrorBoundaryKey(pathname)` strip dynamic segment (`app-layout.tsx`) — `/cloud/google/acc1` → key `/cloud`, không remount khi đổi param. Không thêm key theo pathname raw.
- Mobile: rail ẩn mặc định (`useIsMobile()` — hook có sẵn `shared/file-browser.tsx:14-23`), mở bằng nút hamburger trong top bar dưới dạng Sheet (tái dùng `ui/sheet.tsx` Phase 2); shell dùng `flex h-dvh` (chuẩn `app-layout.tsx:43`), không `h-screen`.
- Query keys đưa vào `ui/web/src/lib/query-keys.ts` (factory `queryKeys.cloud.*`) — scout §Constraints ghi chú cloud đang dùng ad-hoc arrays.

## Architecture

```
AppLayout (h-dvh, sidebar toàn cục)
└─ CloudPage (/cloud/:provider?/:accountId?)
   ├─ DriveRail        ← accounts (useCloudAccounts), quota (about), Mail dialog
   │                    mobile: Sheet + hamburger
   ├─ DriveTopBar      ← breadcrumbs (?path=), search, sort, grid|list, refresh,
   │                    [P5] upload/new-folder
   └─ DriveFileArea    ← GET files?path= (read-only phase này)
      ├─ DriveGrid     ← card grid (grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6)
      └─ DriveTable    ← bảng min-w-[600px]
```

- Dữ liệu: `useCloudAccounts()` + query files per `(accountId, path)` — reuse `use-cloud.ts:84-133` + query pattern `account-detail.tsx:147-154`. Components đọc URL qua `useParams()` (provider, accountId) + `useSearchParams()` (path).
- `FilesBrowser` cũ (`account-detail.tsx:142-257`) bị THAY THẾ bởi DriveFileArea — grep delete scope: `FilesBrowser` chỉ dùng trong `account-detail.tsx`; `AccountDetail` (54-139) giữ lại phần quota? Không — quota chuyển vào rail; `AccountDetail` + `MailboxPreview` xử lý: MailboxPreview dời sang `mailbox-preview.tsx` dùng riêng; `AccountDetail` xóa khi không còn refer (grep `AccountDetail|DetailTab`).
- Consolidate formatter (scout GAPS #28): xóa `formatBytes` (`account-detail.tsx:42-48`) và `formatSize` (`lib/file-helpers.ts:151`) — callers của `formatSize` đã verify bằng grep: `components/chat/media-gallery.tsx`, `components/shared/file-browser.tsx`, `components/shared/file-tree.tsx`, `components/shared/file-viewer-panels.tsx`, `components/shared/image-lightbox.tsx`, `pages/skills/skill-file-helpers.ts`, `pages/storage/storage-page.tsx` — tất cả chuyển sang `formatFileSize` (`lib/format.ts:97`).

## Related Code Files

**Create (thư mục `ui/web/src/pages/cloud/drive/`):**
- `drive-shell.tsx` — layout rail + topbar + area, URL state owner
- `drive-rail.tsx`, `drive-topbar.tsx`, `drive-breadcrumbs.tsx`
- `drive-grid.tsx`, `drive-table.tsx` (item card/row component chung `drive-item.tsx`)
- `mailbox-preview.tsx` (dời từ account-detail.tsx:261-309)

**Modify:**
- `ui/web/src/lib/routes.ts:25` — thêm `CLOUD_PATTERN: "/cloud/:provider?/:accountId?"`
- `ui/web/src/routes.tsx:200` — route mới
- `ui/web/src/pages/cloud/cloud-page.tsx` — rewrite lớn: home + host shell; giữ connect/paste/refresh/stats
- `ui/web/src/pages/cloud/account-detail.tsx` — xóa FilesBrowser + AccountDetail + formatBytes (giữ gì cần dời)
- `ui/web/src/pages/cloud/hooks/use-cloud.ts` — (optional) trả thêm helper select theo provider
- `ui/web/src/lib/file-helpers.ts:151` — xóa `formatSize` sau khi dời callers
- 7 file callers `formatSize` (liệt kê ở Architecture)
- `ui/web/src/lib/query-keys.ts` — thêm `queryKeys.cloud.{status,accounts,about,files,bindings,settings}`
- 5× `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json`

## Implementation Steps

1. **i18n trước:** thêm nhóm `drive.*` ×5 locale: `drive.my_drives`, `drive.accounts`, `drive.shared_tag`, `drive.storage`, `drive.storage_unavailable`, `drive.mail`, `drive.search_placeholder`, `drive.sort.name/size/modified/asc/desc`, `drive.view.grid/list`, `drive.breadcrumb.root`, `drive.empty_folder`, `drive.not_found`, `drive.files_error`, `drive.readonly_note` (reuse key `detail.*` cũ khi được; xóa key không còn dùng sau khi rà).
2. Routes: `routes.ts` + `routes.tsx` — pattern optional segments; render `CloudPage` cho cả 3 depth.
3. `query-keys.ts`: factory cloud keys; đổi `use-cloud.ts` + `account-detail.tsx` queries sang factory (behavior giữ nguyên).
4. `drive-shell.tsx`: đọc `useParams`/`useSearchParams`; điều hướng = `navigate`/`setSearchParams` (path encode từng segment như `account-detail.tsx:221-223` nhưng qua URL). Validate provider/account → redirect/empty-state.
5. `drive-rail.tsx`: accounts list (click → `/cloud/:provider/:accountId`), quota card (reuse markup 110-127), Mail button → Dialog `mailbox-preview.tsx`. Mobile: hidden + Sheet qua hamburger.
6. `drive-topbar.tsx` + `drive-breadcrumbs.tsx`: breadcrumbdecode từ `?path=`; search input `text-base md:text-sm` (rule 267) lọc client-side; sort Select + toggle grid/list (localStorage key `cloud.viewMode`); Refresh giữ.
7. `drive-grid.tsx`/`drive-table.tsx` + `drive-item.tsx`: card/bảng read-only; folder click → navigate `?path=`; file row KHÔNG clickable (ghi chú read-only footer 253-255 giữ); `FileIcon` cho icon; size dùng `formatFileSize`; bảng `min-w-[600px]` + `overflow-x-auto`.
8. Rewrite `cloud-page.tsx`: `/cloud` home (stats + provider cards + coming-soon + connect/paste flow giữ nguyên logic 212-247, 400-419) + mount DriveShell khi có provider/account. Gear (Phase 2) vào top bar shell + giữ ở home header.
9. Dọn dẹp: xóa `FilesBrowser`/`AccountDetail`/`formatBytes`; dời `MailboxPreview`; xóa `formatSize` + dời 7 callers sang `formatFileSize`; grep `formatBytes|formatSize|FilesBrowser|AccountDetail|DetailTab` = 0 refer chết.
10. Checklist: `pnpm build` + `pnpm dev` thử các path: `/cloud`, `/cloud/google`, `/cloud/google/<id>?path=/a/b`, reload giữ vị trí; DevTools mobile 390px: rail drawer, bảng scroll ngang, không zoom input (font 16px).

## Success Criteria

- [ ] `/cloud/:provider?/:accountId??path=` deep-link được: paste URL vào tab mới ra đúng thư mục; Back/Forward browser hoạt động
- [ ] Rail hiển thị accounts (own + 🏢 shared), quota card đúng số liệu, Mail dialog hoạt động với account Gmail
- [ ] Grid/list toggle hoạt động và nhớ lựa chọn sau reload; search lọc client-side tức thì; sort name/size/modified 2 chiều
- [ ] Bảng list `min-w-[600px]` trong `overflow-x-auto` (không còn 540px); mobile rail dạng drawer; không có `h-screen`
- [ ] `grep -rn "formatBytes\|formatSize\b" ui/web/src` chỉ còn `formatFileSize` (+ skill-file-helpers nếu đổi xong = 0 bản duplicate)
- [ ] Provider/account không hợp lệ → redirect/empty-state, không trắng màn; ErrorBoundary không remount khi đổi folder (thử: state trong topbar giữ nguyên khi navigate)
- [ ] i18n `drive.*` có đủ 5 locale; `pnpm build` xanh

## Risk Assessment

| Risk | Observable signal | Response (pre-decided) |
|---|---|---|
| Duplicate path state (useState + URL) → UI flash khi navigate | Breadcrumb nhảy A→B→A | Rule cứng bước 4: path CHỈ ở `useSearchParams`; code review grep `useState` trong drive-shell |
| Route optional param `:provider?` không khớp depth giữa | `/cloud/google` render home thay vì provider view | Test tay cả 3 depth bước 10; nếu Vite/RR7 optional segment hành xử khác.expect — tách 2 route tường minh |
| Xóa `formatSize` phá trang khác (storage/skills/chat) | `pnpm build` lỗi TS | Build ngay sau bước 9; callers đã liệt kê đủ 7 file (đã verify grep) |
| Mail preview mất sau restructure | User báo mất tab Mail | MailboxPreview dời nguyên trạng + rail button; success criterion có mục Mail |
| Grid quá rộng trên mobile | Card tràn/vió touch target | Grid `grid-cols-2` mobile-first + breakpoints sm/lg/xl; card ≥44px hit area (CSS coarse-pointer có sẵn) |
