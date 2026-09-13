---
title: "Clouds Google Drive redesign"
description: "Thiết kế lại trang Clouds thành file manager kiểu Google Drive: nâng scope OAuth lên ghi (upload/mkdir/rename/move/delete), nút cài đặt bánh răng gom BYO + scope bindings (hỗ trợ vô số rule tùy chọn), shell Drive (left rail + top bar + grid/list + URL deep-link), backend file ops qua rclone rc, kéo-thả upload có progress, chuyển file giữa các tài khoản, đồng bộ dữ liệu định kỳ, và lớp delight (starred/recent/preview/share link/shortcut)."
status: pending
priority: P1
effort: "11d"
tags: [cloud, web-ui, oauth, rclone, file-manager, sync]
created: 2026-09-13
---

# Clouds Google Drive redesign

## Overview

Thiết kế lại trang Clouds (`ui/web/src/pages/cloud/`) từ dạng "picker → thẻ tài khoản → bảng file read-only" thành một file manager kiểu Google Drive hoàn chỉnh: left rail (tài khoản/drive roots + quota), top bar (search, sort, grid/list), file area dạng grid card / bảng với context menu và multi-select, URL deep-linkable cho mọi thư mục. Mọi tính năng ghi (upload, mkdir, rename, move, delete, copy, share link) đều bị chặn bởi hard blocker OAuth: scope hiện tại là read-only (`drive.readonly`, `Files.Read.All` — scout GAPS #15), nên Phase 1 là nền tảng bắt buộc (nâng scope + flow re-consent cho tài khoản cũ). Backend file ops đi qua wrapper rclone rc (mỗi wrapper 3–6 dòng qua `RCClient.do()` — scout §2.3) và HTTP endpoints theo tiền giác `storage.go` upload/move (scout §3.5). Riêng upload chọn phương án temp-file + `operations/copyfile` (không multipart trực tiếp vào rc — lý do trong phase-04). Sync dữ liệu dùng bảng `cloud_sync_pairs` + background worker riêng (so sánh trade-off với cron trong phase-06). Toàn bộ dữ kiện code đã đối chiếu với `reports/scout.md` và được spot-check trực tiếp (scopes, routes.ts, version.go, schema.go, http-client.ts, ui/ primitives, edition gating).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Nâng OAuth scope Google (`drive`) + OneDrive (`Files.ReadWrite.All`) và flow re-consent cho tài khoản cũ, giữ nguyên đường read-only | P1 |
| 2 | Nút bánh răng Settings bên cạnh "Làm mới": gom ProviderClientSetup + ScopeBindingsPanel; scope rules thành bảng hỗ trợ nhiều rule tùy ý (thêm/sửa/enable/xóa, cột `enabled` + `priority`) | P1 |
| 3 | Shell kiểu Google Drive: left rail + top bar + grid/list + breadcrumb + URL params là source of truth (`/cloud/:provider?/:accountId??path=`) | P1 |
| 4 | Backend file ops: rc wrappers (mkdir/deletefile/rmdir/movefile/copy/copyurl/publiclink/sync) + HTTP endpoints có guard auth + tenant + write-scope | P1 |
| 5 | UI file ops: context menu, rename/new folder/delete, multi-select bulk, drag-drop upload có progress, download | P1 |
| 6 | Chuyển file giữa tài khoản (transfer) + đồng bộ dữ liệu (sync pairs, one-way mirror, chạy tay hoặc định kỳ) | P2 |
| 7 | Lớp delight: starred, recent, preview panel, share-link dialog, keyboard shortcuts (trash/restore, thumbnails, server-side search... vào backlog) | P3 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: OAuth write scopes + re-consent](./phase-01-oauth-write-scopes.md) | Pending |
| 2 | [Phase 2: Settings gear + scope rules](./phase-02-settings-gear.md) | Pending |
| 3 | [Phase 3: Drive-style shell (rail + topbar + routes)](./phase-03-drive-shell.md) | Pending |
| 4 | [Phase 4: File operations backend (rc + HTTP)](./phase-04-file-ops-backend.md) | Pending |
| 5 | [Phase 5: File operations UI](./phase-05-file-ops-ui.md) | Pending |
| 6 | [Phase 6: Cross-account transfer + sync](./phase-06-transfer-sync.md) | Pending |
| 7 | [Phase 7: Delight layer (starred/recent/preview/share/shortcuts)](./phase-07-delight.md) | Pending |

Thứ tự thực thi: 1 → 2 → 3 → 4 → 5 → 6 → 7. Phase 2 có thể chạy song song Phase 1 (không phụ thuộc code, chỉ phụ thuộc thứ tự migration — chuỗi bump version: P2 = PG 121/SQLite 84, P6 = PG 122/SQLite 85, P7 = PG 123/SQLite 86; nếu đảo thứ tự phải renumber).

## Success Criteria

- [ ] Kết nối Google/OneDrive mới yêu cầu scope ghi; tài khoản cũ thấy nút "Cấp quyền lại" và mọi đường read-only vẫn hoạt động khi chưa re-grant
- [ ] Nút bánh răng cạnh "Làm mới" mở settings sheet chứa BYO client setup + bảng scope rules; add-row hỗ trợ cả `group` và `user`; mỗi rule bật/tắt (enabled) và xóa được; tạo được nhiều rule tùy ý cùng lúc
- [ ] Trang `/cloud` có left rail (tài khoản + quota), top bar (search/sort/grid-list), grid card + bảng list, breadcrumb deep-link được bằng URL (`?path=`), reload giữ nguyên vị trí
- [ ] Upload được file bằng kéo-thả từ ngoài browser, có progress; mkdir/rename/move/copy/delete hoạt động trên cả grid và list; multi-select + bulk delete/download hoạt động
- [ ] Copy/move file hoặc thư mục giữa 2 tài khoản cloud; tạo sync pair (A→B, manual hoặc định kỳ) và chạy ra dữ liệu bên đích
- [ ] Starred/recent/preview/share-link/keyboard shortcuts hoạt động
- [ ] `go build ./...` và `go build -tags sqliteonly ./...` xanh; `go vet` sạch; migration PG + SQLite đều áp dụng đúng (fresh + incremental)
- [ ] Mọi string UI mới có key ở đủ 5 locale (en, vi, zh, ko, ru) trong `cloud.json`
- [ ] Mobile: bảng `min-w-[600px]` trong `overflow-x-auto` (sửa lỗi 540px hiện tại), input `text-base md:text-sm`, target chạm ≥44px, rail mobile dùng sheet/drawer

## Out-of-scope / Backlog

Các ý tưởng đã đề xuất nhưng chủ động hoãn (chi tiết lý do trong phase-07):

- Thùng rác / restore (rclone `deletefile` xóa vĩnh viễn; Drive trash cần API ngoài bề mặt rc hiện có)
- Server-side recursive search (hiện chỉ client-side filter — scout GAPS #11)
- "Được chia sẻ với tôi" listing từ provider (hạn chế API qua rclone)
- Thumbnails / image preview pipeline (scout GAPS #8, #13)
- Virtualization cho thư mục 1000+ entries (`@tanstack/react-virtual` chưa có — GAPS #24)
- Two-way sync + conflict policy (v1 chỉ one-way additive mirror — GAPS #19)
- Activity/audit log đầy đủ cho file ops (GAPS #18)
- Drag & drop trực tiếp giữa 2 tài khoản (v1 dùng transfer dialog)
- Upload-by-URL UI (backend `copyurl` đã có từ Phase 4, UI hoãn)
- Consolidate `formatSize` ngoài phạm vi cloud: nằm ở Phase 3 (đã liệt kê đủ 7 file caller)

## Open Questions

1. **Quyền ghi trên tài khoản shared:** kế hoạch hiện chọn "chỉ owner của account hoặc tenant admin được ghi" (member chỉ đọc trên account được share). Anh có muốn member thường được ghi lên account shared không?
2. **Thay hay thêm scope Google:** kế hoạch thay `drive.readonly` bằng `drive` (superset, tránh trùng consent). Nếu anh muốn giữ cả hai (phân tích scope hiển thị), nói để điều chỉnh.
3. **Sync có xóa file đích không:** v1 chỉ one-way additive mirror (không bao giờ xóa ở đích). Exact mirror (`sync/sync`, xóa phần thừa ở đích) để backlog — anh có cần ngay không?

<!-- slug: clouds-google-drive-redesign -->
