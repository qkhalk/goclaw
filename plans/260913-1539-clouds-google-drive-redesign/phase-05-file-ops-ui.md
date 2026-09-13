---
phase: 5
title: "File operations UI"
status: pending
priority: P1
effort: "2d"
dependencies: [3, 4]
---

# Phase 5: File operations UI

## Overview

Wire các thao tác file vào shell Drive ở Phase 3 với backend Phase 4: menu ⋮ mỗi item (DropdownMenu) + right-click (ContextMenu), rename/new folder/delete/move/copy, **multi-select** (checkbox + Shift-click + Ctrl+A) với bulk action bar, **kéo-thả file từ ngoài browser** để upload (tái dùng `DropZone`) có **progress** (mở rộng `HttpClient` với XHR onprogress — hiện `upload()` là fetch thuần không có progress, scout GAPS #26), và drag-move file vào folder trong cùng account (dnd-kit đã có sẵn trong deps). Account chưa có scope ghi (`can_write=false` từ Phase 1) → ẩn/vô hiệu thao tác ghi + gợi ý "Cấp quyền lại".

## Surface parity

- **Backend:** N/A (tiêu thụ endpoint Phase 4); bug found ở backend → fix trong phase này được.
- **API contract:** N/A.
- **Web UI:** toàn bộ phase này (kèm 4 ui primitives mới).
- **CLI/runtime:** N/A. **Desktop UI:** N/A.

## Requirements

### Functional
- Menu ⋮ trên mỗi item (grid card + list row) và ContextMenu right-click cùng một danh sách action: Mở (folder), Tải xuống, Đổi tên, Sao chép tới... (folder picker), Di chuyển tới... (folder picker), Xóa (confirm ghi rõ "xóa vĩnh viễn"). Share link + Star render phase 7 (placeholder ẩn).
- New folder button (top bar) → dialog tên; rename inline bằng `ui/inline-edit-text.tsx` (component có sẵn); delete dùng `ConfirmDialog` (`components/shared/confirm-dialog.tsx`).
- Multi-select: checkbox hiện khi hover hoặc khi đang có selection; Shift-click chọn range; Ctrl/Cmd+A chọn tất cả; bulk bar dưới cùng (số item đã chọn + Tải xuống / Di chuyển / Xóa / Bỏ chọn). Selection là state cục bộ, reset khi `path` đổi.
- Upload: kéo file từ OS vào file area (overlay `DropZone` — `components/chat/drop-zone.tsx`, tái dùng as-is) HOẶC nút "Tải lên" (top bar) mở file picker; nhiều file cùng lúc; mỗi file một hàng progress (aggregate bar trên cùng), upload xong invalidate query files. Folder đích = `?path=` hiện tại.
- Download: link dùng endpoint GET download (Phase 4) qua `fetchBlob` + object URL (auth header cần thiết — không dùng href trực tiếp).
- Move/copy dialog: folder tree picker đơn giản (list folder theo cấp, browse bằng query files với filter isDir — không cần tree phức tạp).
- Drag-move trong account: file row/card draggable (dnd-kit `useDraggable` — mẫu `shared/file-tree-dnd-wrappers.tsx`), folder card/row là `useDroppable`; thả → move; drop vào breadcrumb segment → move vào thư mục đó.

### Non-functional
- 4 primitives mới theo pattern shadcn/Radix hiện có (`ui/*.tsx`): `dropdown-menu.tsx`, `context-menu.tsx`, `checkbox.tsx`, `progress.tsx` (scout GAPS #22: thiếu toàn bộ).
- `http-client.ts` thêm `uploadWithProgress(path, formData, onProgress): Promise<T>` + hỗ trợ abort (XMLHttpRequest; `upload()` fetch cũ giữ nguyên cho các trang khác).
- Mobile: menu ⋮ chạm được ≥44px; bulk bar dùng `safe-bottom`; dialog full-screen mobile (chuẩn `ui/dialog.tsx`); ContextMenu long-press vẫn hoạt động (Radix touch).
- Account `can_write=false`: mọi action ghi ẩn + banner mảnh trong file area "Tài khoản chỉ đọc — cấp quyền lại để tải lên/thao tác" với nút re-grant (flow Phase 1).

## Architecture

```
DriveFileArea (Phase 3)
├─ DropZone wrapper (onDrop → uploadQueue)
├─ DriveGrid/DriveTable + DriveItem
│   ├─ Checkbox (selection)  ← useSelection hook (Set<path>, shift anchor)
│   ├─ ItemActionsMenu (⋮)   ← DropdownMenu; ContextMenu bọc item
│   └─ dnd draggable/droppable
├─ BulkActionBar (count + actions)
├─ UploadManager (hook use-cloud-uploads.ts)
│   ├─ queue: File[] → uploadWithProgress từng file (tuần tự, tránh parallel provider rate-limit)
│   └─ progress map name→{loaded,total,status}; Panel + ui/progress.tsx
└─ Dialogs: NewFolder, Rename (inline), MoveCopy (folder picker), Delete (ConfirmDialog)
```

- Data mutations: mở rộng `use-cloud.ts` — `useCloudFileOps(accountId)` trả `{mkdir, rename, move, copy, remove, upload(file,onProgress), downloadUrl(path), invalidate}`; invalidate `queryKeys.cloud.files(accountId, path)` + ancestors (invalidate pattern `["cloud"]` cục bộ hơn: invalidate đúng key files của path hiện tại + parent).
- Guards UI: `canWrite = account.can_write !== false && (account Owner hoặc admin)` — role check `useAuthStore` như `cloud-page.tsx:84-85`; field owner có sẵn trong `CloudAccount` (`use-cloud.ts:6-18` — `UserID`/`user_id` JSON; verify tên field thực tế khi implement).
- Upload tuần tự 1 file 1 lúc (queue) — provider rate-limit an toàn; hủy = abort XHR.

## Related Code Files

**Create:**
- `ui/web/src/components/ui/dropdown-menu.tsx`, `context-menu.tsx`, `checkbox.tsx`, `progress.tsx`
- `ui/web/src/pages/cloud/drive/drive-item-menu.tsx` (⋮ + ContextMenu nội dung chung)
- `ui/web/src/pages/cloud/drive/use-selection.ts`
- `ui/web/src/pages/cloud/drive/use-cloud-uploads.ts` + `upload-panel.tsx`
- `ui/web/src/pages/cloud/drive/folder-picker-dialog.tsx`, `new-folder-dialog.tsx`, `move-copy-dialog.tsx`

**Modify:**
- `ui/web/src/api/http-client.ts:78-94` — thêm `uploadWithProgress` (XHR, onProgress, AbortSignal)
- `ui/web/src/pages/cloud/hooks/use-cloud.ts` — `useCloudFileOps`
- `ui/web/src/pages/cloud/drive/drive-grid.tsx`, `drive-table.tsx`, `drive-topbar.tsx` (nút Tải lên + Thư mục mới), `drive-shell.tsx` (DropZone + bulk bar + dialogs mount)
- 5× `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json`

## Implementation Steps

1. **i18n trước:** nhóm `files.*` ×5 locale: `files.upload`, `files.upload_hint`, `files.new_folder`, `files.rename`, `files.delete`, `files.delete_confirm_title/description` (ghi rõ xóa vĩnh viễn), `files.move`, `files.copy`, `files.download`, `files.selected_count` (interpolation `{count}`), `files.bulk.move/delete/download/clear`, `files.readonly_banner`, `files.regrant`, `files.uploading`, `files.upload_done`, `files.upload_failed`, `files.cancel_upload`, `files.name_placeholder`, `files.folder_name_placeholder`, `files.pick_folder`, `files.op_failed`. Đủ 5 file mới sang bước sau.
2. Primitives: 4 file `ui/*.tsx` theo shadcn pattern (Radix `DropdownMenuPrimitive`, `ContextMenuPrimitive`, `CheckboxPrimitive`, `ProgressPrimitive` — deps `@radix-ui/react-*` cài thêm bằng pnpm nếu chưa có trong package.json; kiểm tra trước).
3. `http-client.ts`: `uploadWithProgress` — XMLHttpRequest, `upload.onprogress` → onProgress(loaded, total), `signal` abort, error parse giống `upload()` (78-94). Unit test nhẹ (jsdom) hoặc thử tay.
4. `use-cloud.ts`: `useCloudFileOps` — mỗi mutation gọi endpoint Phase 4, dùng `queryKeys.cloud.*` (Phase 3), invalidate files + about (sau upload/DELETE quota đổi).
5. Selection hook `use-selection.ts`: `Set<string>` keyed theo entry name (path-relative), toggle/range (anchor index)/all/clear; reset qua `useEffect` khi `path` đổi.
6. Item menu: `drive-item-menu.tsx` nhận `{entry, canWrite, handlers}` render DropdownMenuContent + cùng list item cho ContextMenu; gắn vào DriveItem cả grid + table. Mobile target ≥44px.
7. Dialogs: new-folder (input `text-base md:text-sm`), move-copy (folder picker browse theo query files lọc isDir + nút "chọn thư mục này"), delete (ConfirmDialog, description warning vĩnh viễn).
8. Uploads: `use-cloud-uploads.ts` queue tuần tự + `upload-panel.tsx` (aggregate progress + per-file status + cancel); DropZone bọc file area (`components/chat/drop-zone.tsx` import — nếu muốn không phụ thuộc chat, move sang `components/shared/drop-zone.tsx` và update import chat; grep `drop-zone` trước khi dời).
9. Drag-move: bọc DriveItem bằng `useDraggable`/`useDroppable` (mẫu `file-tree-dnd-wrappers.tsx`), drop → move; breadcrumb segment droppable; chỉ bật khi `canWrite`.
10. Bulk bar: hiện khi selection > 0 (`safe-bottom` trên mobile); actions gọi ops tuần tự với toast kết quả (`use-toast-store`).
11. Readonly state: `canWrite=false` → ẩn nút Tải lên/Thư mục mới/menu ghi, banner + nút re-grant (flow `startConnect` như Phase 1).
12. Checklist: `pnpm build`; thử tay đủ luồng: upload kéo-thả + progress + cancel; mkdir/rename/move/copy/delete; multi-select bulk delete; drag vào folder; mobile viewport kiểm tra menu/bulk bar.

## Success Criteria

- [ ] Kéo 2+ file từ desktop vào file area → upload tuần tự, progress từng file + tổng, xong hiện trong danh sách; cancel giữa chừng hoạt động
- [ ] ⋮ menu và right-click menu cùng action; rename inline lưu được; new folder xuất hiện ngay; delete có confirm "xóa vĩnh viễn"
- [ ] Multi-select: shift-click range, Ctrl+A, bulk delete/move/download chạy đúng số item; selection reset khi đổi thư mục
- [ ] Drag file vào folder card → move thành công; drop lên breadcrumb → move tới thư mục đó
- [ ] Account chưa re-grant: mọi action ghi ẩn, banner + nút "Cấp quyền lại" hoạt động (flow Phase 1)
- [ ] 4 primitives mới build xanh; i18n `files.*` đủ 5 locale; mobile: bulk bar `safe-bottom`, menu target ≥44px
- [ ] Lỗi op hiển thị toast có message (403 `cloud_write_scope_required` map sang gợi ý re-grant)

## Risk Assessment

| Risk | Observable signal | Response (pre-decided) |
|---|---|---|
| Radix primitives chưa có trong package.json | `pnpm build` lỗi import | Bước 2 kiểm tra + `pnpm add @radix-ui/react-dropdown-menu @radix-ui/react-context-menu @radix-ui/react-checkbox @radix-ui/react-progress` |
| Upload nhiều file song song bị Google 403 rate-limit | Upload fail loạt | Queue tuần tự mặc định (Architecture); không đổi |
| Progress không chạy (fetch không có upload progress) | Bar đứng 0% | Bắt buộc XHR `uploadWithProgress` — không fallback fetch cho upload cloud |
| DropZone dời chỗ phá chat | Kéo-thả chat hỏng | Nếu dời `drop-zone.tsx` thì grep import + update; nếu ngại để nguyên import từ chat (quyết định mặc định: DỜI sang shared vì chat không còn là chủ sở hữu hợp lý — verify với grep trước) |
| Selection key dùng path tuyệt đối trùng nhau giữa 2 entry | Xóa nhầm item | Key theo `entry.name` trong 1 thư mục (tên không trùng trong 1 folder) |
| 403 từ backend không map rõ (member-shared) | Toast lỗi chung chung | Bước 11 + bước 12: map code lỗi → i18n message riêng |
