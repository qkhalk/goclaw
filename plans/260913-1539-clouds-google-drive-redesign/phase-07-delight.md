---
phase: 7
title: "Delight layer (starred / recent / preview / share / shortcuts)"
status: pending
priority: P3
effort: "1.5d"
dependencies: [3, 4, 5]
---

# Phase 7: Delight layer

## Overview

Lớp hoàn thiện trải nghiệm kiểu Drive: **Đã gắn dấu sao** (starred — backend lưu DB vì star là metadata per-user, provider không expose qua rc), **Gần đây** (recent — thuần client localStorage), **preview panel** (Sheet bên phải cho ảnh/text/pdf qua endpoint download Phase 4), **share link dialog** (backend `publiclink` đã có từ Phase 4), **keyboard shortcuts** (điều hướng + selection cơ bản + help dialog). Phase này chia rõ IN-SCOPE (làm) và BACKLOG (hoãn — listed ở plan.md) vì nhiều ý tưởng đề xuất phụ thuộc API ngoài tầm rclone rc.

## Surface parity

- **Backend:** migration `cloud_starred` (PG 122→123, SQLite 85→86) + store 3 lớp + 3 endpoint starred.
- **API contract:** starred CRUD endpoints — document.
- **Web UI:** toàn bộ phần còn lại.
- **CLI/runtime:** N/A. **Desktop UI:** N/A; sqliteonly build xanh (SQLite twin starred store bắt buộc).

## Requirements

### In-scope (làm trong phase này)

- **Starred:** nút sao trên item menu + icon sao trên card/row; rail mục "Đã gắn dấu sao" liệt kê items đã sao (mọi account của user), click mở đúng account+path; bỏ sao ở cùng menu hoặc trong view starred. Lưu DB `cloud_starred` (tenant, user, account, path, name, is_dir) — scope: user-level, tenant-scoped.
- **Recent:** localStorage per user (key `cloud.recent.<userID>`): mỗi lần MỞ file (preview/download) hoặc vào thư mục ghi entry `{accountId, provider, email, path, name, isDir, at}` cap 50 dedupe; rail mục "Gần đây"; clear-all button. Không backend.
- **Preview:** click file (không phải folder) → Sheet bên phải (reuse `ui/sheet.tsx` Phase 2): ảnh (fetchBlob → objectURL), text/markdown/json (giới hạn 1MB render `<pre>`), pdf (embed objectURL); footer: tên + size (`formatFileSize`) + nút Tải xuống + nút Đóng; nút prev/next di chuyển trong danh sách hiện tại; lỗi "không xem trước được" → fallback nút download. Preview KHÔNG auto-load >25MB.
- **Share link:** menu item "Sao chép liên kết" → POST `files/publiclink` (Phase 4) → dialog hiển thị URL + nút copy (`useClipboard` — mẫu `cloud-page.tsx:92`) + cảnh báo "liền kết công khai, ai có link đều xem được".
- **Keyboard shortcuts:** khi focus trong file area: `↑/↓/j/k` di chuyển con trỏ, `Enter` mở (folder navigate / file preview), `Esc/Backspace` lên thư mục cha, `Del` xóa (confirm), `Ctrl/Cmd+A` chọn tất cả, `/` focus search, `?` mở dialog liệt kê shortcuts. Không bắt khi đang gõ trong input/textarea (check `e.target`).

### Backlog (hoãn — không làm, listed plan.md)

Thùng rác/restore (rc `deletefile` vĩnh viễn, Drive trash cần Graph/Drive API ngoài rc), server-side recursive search, "shared with me" listing, thumbnails, virtualization 1000+ (`@tanstack/react-virtual` chưa có — GAPS #24), two-way sync + conflict policy, activity/audit log (GAPS #18), drag giữa 2 tài khoản, upload-by-URL UI (backend copyurl Phase 4 đã có — chỉ thiếu UI), gallery view.

## Architecture

**Migration `cloud_starred` (PG 123, SQLite 86):** `id UUID PK`, `tenant_id UUID NOT NULL`, `user_id UUID NOT NULL`, `account_id UUID NOT NULL`, `path TEXT NOT NULL` (đường dẫn file/thư mục), `name TEXT NOT NULL`, `is_dir BOOLEAN NOT NULL`, `starred_at TIMESTAMPTZ DEFAULT now()`, UNIQUE `(tenant_id, user_id, account_id, path)`.

- Store: `internal/store/cloud_starred_store.go` (interface List/Add/Remove) + `pg/cloud_starred.go` + `sqlitestore/cloud_starred.go` (twin).
- HTTP: `GET /v1/cloud/starred` (list của user), `PUT /v1/cloud/starred` `{account_id, path, name, is_dir}` (validate account accessible qua `AccountByID`), `DELETE /v1/cloud/starred/{id}` — `requireAuth` user-level (không admin).
- UI state: starred query `["cloud","starred"]`; recent thuần util `lib/cloud-recent.ts` (read/push/clear, SSR-safe guard `typeof window`).
- Preview: `preview-sheet.tsx` nhận `{entry, entries, index, onNavigate}` — fetch qua `http.fetchBlob` (`http-client.ts:71-79`) + `URL.createObjectURL` + revoke khi đóng/đổi.
- Shortcuts: hook `use-drive-shortcuts.ts` gắn ở `drive-shell.tsx` (keydown window-level, guard input focus); con trỏ selection hiện có từ `use-selection.ts` (Phase 5) mở rộng `cursorIndex`.

## Related Code Files

**Create:**
- `migrations/000123_cloud_starred.up.sql` + `.down.sql`
- `internal/store/cloud_starred_store.go`, `internal/store/pg/cloud_starred.go`, `internal/store/sqlitestore/cloud_starred.go`
- `ui/web/src/lib/cloud-recent.ts`
- `ui/web/src/pages/cloud/drive/preview-sheet.tsx`, `use-drive-shortcuts.ts`, `shortcuts-dialog.tsx`, `starred-view.tsx`, `recent-view.tsx`

**Modify:**
- `internal/upgrade/version.go` → 123; `internal/store/sqlitestore/schema.sql` + `schema.go` → 86
- `internal/http/cloud.go` — 3 route starred
- `ui/web/src/pages/cloud/hooks/use-cloud.ts` — `useCloudStarred`
- `ui/web/src/pages/cloud/drive/drive-rail.tsx` — 2 mục mới; `drive-item-menu.tsx` — Star + Share; `drive-grid/table` — sao icon; `drive-shell.tsx` — route `?view=starred|recent` (query param, không thêm path segment để tránh đụng `:accountId`) + shortcuts mount
- `ui/web/src/pages/cloud/drive/drive-item-menu.tsx` — mở placeholder Phase 5 thành action thật
- 5× `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json`

## Implementation Steps

1. **Migration:** PG 000123 + bump 123; SQLite schema.sql + patch 85→86 + bump 86; verify fresh + incremental; `go build -tags sqliteonly ./...`.
2. Store 3 lớp starred + test (unique constraint, list theo user).
3. HTTP starred 3 endpoint (guards + validate account accessible). Document.
4. **i18n:** nhóm `starred.*`, `recent.*`, `preview.*`, `share.*`, `shortcuts.*` ×5 locale: `starred.title/menu_item/empty/remove`, `recent.title/empty/clear`, `preview.close/download/cannot_preview/loading/too_large`, `share.menu_item/title/url/copied/public_warning`, `shortcuts.title/navigation/open/up/delete/select_all/search/help`. Đủ 5 file trước UI.
5. Starred UI: menu item + star icon toggle (optimistic update + rollback lỗi); `starred-view.tsx` bảng items (account column + mở đúng path) mount khi `?view=starred`; rail item có badge count.
6. Recent: `cloud-recent.ts` push khi mở file/preview + navigate folder (depth > 0); `recent-view.tsx` tương tự starred; clear-all.
7. Preview: `preview-sheet.tsx` — nhận diện loại theo extension (ảnh: png/jpg/jpeg/gif/webp/svg; text: txt/md/json/csv/log; pdf) — `operations/stat` trả `MimeType` (`rc_client.go:139`) nhưng list endpoint hiện không trả mime → dùng extension (không đổi backend); cap 25MB ảnh/pdf, 1MB text; prev/next; download button.
8. Share dialog: menu → POST publiclink → dialog copy URL + cảnh báo; lỗi provider không hỗ trợ (OneDrive) → toast lỗi gốc.
9. Shortcuts: hook + dialog help; guard input/textarea/contentEditable; test tay từng phím.
10. Checklist: build 2 tags + vet + test; `pnpm build`; thử tay full luồng starred/recent/preview/share/shortcuts desktop + mobile (preview sheet full-screen mobile, `safe-bottom`).

## Success Criteria

- [ ] Star/unstar hoạt động, persists qua reload, view "Đã gắn dấu sao" mở đúng file (đúng account + path); bỏ sao từ view được
- [ ] "Gần đây" ghi đúng các lần mở file/vào thư mục, cap 50, clear-all sạch; không gửi request nào lên backend
- [ ] Preview: ảnh + text + pdf render đúng; >25MB không auto-load; lỗi loại khác → fallback download; prev/next chạy trong thư mục
- [ ] Share link copy được URL; cảnh báo công khai hiển thị; OneDrive không hỗ trợ → toast lỗi rõ
- [ ] Shortcuts: j/k/Enter/Esc/Del/Ctrl+A`/`/`?` hoạt động; không kích hoạt khi đang gõ ở search box; dialog `?` liệt kê đúng
- [ ] Migration PG 123 + SQLite 86 fresh & incremental sạch; sqliteonly build xanh
- [ ] i18n đủ 5 locale; preview sheet full-screen mobile đúng chuẩn dialog rule

## Risk Assessment

| Risk | Observable signal | Response (pre-decided) |
|---|---|---|
| Star vào account bị disconnect sau đó | View starred click mở account không tồn tại | Render item nhưng click → empty-state "account không còn truy cập" (resolve qua `accounts` list, có rồi) |
| objectURL memory leak khi preview liên tục | Tab web RAM tăng | `URL.revokeObjectURL` trong cleanup effect mỗi lần đổi/đóng (bước 7) |
| Shortcut chặn input elsewhere (search, rename) | Không gõ được | Guard `e.target` tag + `isContentEditable` (bước 9) + test tay rename while shortcuts mounted |
| Starred UNIQUE conflict double-click | 500 duplicate key | `INSERT ... ON CONFLICT DO NOTHING` (PG + SQLite cùng cú pháp) |
| Publiclink không hỗ trợ trên onedrive rc | 500/501 từ rc | Map lỗi rõ `provider_not_supported` + toast; ghi docs endpoint |
| Migration number collide P7 vs phase khác | migrate version exists | Chuỗi 123/86 cấp trong plan.md; rebase renumber |
