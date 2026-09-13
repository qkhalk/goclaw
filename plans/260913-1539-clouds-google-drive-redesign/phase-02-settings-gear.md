---
phase: 2
title: "Settings gear + scope rules"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 2: Settings gear + scope rules

## Overview

Thêm nút bánh răng (Settings) cạnh nút "Làm mới" trên header trang Clouds, mở một settings sheet gom hết những gì đang làm rối trang: card "Dùng OAuth client riêng (nâng cao)" (`ProviderClientSetup`, hiện chôn trong collapsible cuối trang — `cloud-page.tsx:504-518`) và panel "Phạm vi sử dụng" (`ScopeBindingsPanel`). Đồng thời redesign ScopeBindingsPanel thành bảng rules hỗ trợ **vô số rule tùy ý**: add-row chọn được cả `group` lẫn `user` (backend đã hỗ trợ đủ 3 loại — `internal/http/cloud.go:293-302`, scout §1.3), liệt kê rules trong bảng với enable/disable + xóa + đổi account inline. Cột `enabled` + `priority` là migration mới (dual-DB). Giữ nguyên ngữ nghĩa ưu tiên resolve: explicit name > group > user > tenant default > own (`internal/cloud/access.go:54-138`).

## Surface parity

- **Backend:** migration cột `enabled`/`priority` trên `cloud_account_bindings`; `PUT /v1/cloud/bindings` nhận thêm 2 field; resolve lọc enabled + sort priority.
- **API contract:** request/response bindings thêm `enabled` (bool, default true) + `priority` (int, default 100).
- **Web UI:** settings sheet + bảng rules.
- **CLI/runtime:** N/A — không có bề mặt CLI cho bindings.
- **Desktop UI:** N/A (không có trang cloud ở desktop — đã verify Phase 1).

## Requirements

### Functional
- Nút gear (icon `Settings` của lucide) trong PageHeader actions, cạnh nút Refresh (`cloud-page.tsx:272-281`), luôn hiển thị trên trang Clouds (kể cả màn picker — sheet có provider Select riêng ở đầu, default theo `selectedProvider` nếu có).
- Settings sheet chứa: (1) provider switch, (2) `ProviderClientSetup` (admin/owner only — giữ check role `useAuthStore` tại `cloud-page.tsx:84-85`), (3) bảng scope rules của provider đang chọn (admin only — mount điều kiện như `cloud-page.tsx:422`).
- Bảng rules: mỗi hàng có Switch enable/disable, badge loại (tenant/group/user), scope_key, account (Select đổi trực tiếp), nút xóa. Add-row: Select loại có `group` + `user` (bỏ giới hạn hiện tại `scope-bindings-panel.tsx:204-207`), key input giữ datalist gợi ý, account Select dùng sentinel `NONE` (`scope-bindings-panel.tsx:22`).
- Rule tenant default giữ hàng riêng phía trên (Select với `NONE` = xóa binding — `scope-bindings-panel.tsx:116-127`).
- Tạo được nhiều rule liên tiếp (sau add, giữ nguyên loại + xóa key input, focus lại key).

### Non-functional
- Không đổi semantic upsert: 1 binding per `(tenant, scope_type, scope_key, provider)` (`pg/cloud_accounts.go:293-301`).
- Resolve với binding `enabled=false` phải bỏ qua hoàn toàn; nhiều rule cùng tier khớp thì `priority` nhỏ thắng, hòa thì giữ tier-rank cũ (group > user > tenant) rồi `created_at ASC`.
- Migration dual-DB bắt buộc (AGENTS.md): PG + SQLite cùng lúc.

## Architecture

**Migration (PG 120 → 121, SQLite 83 → 84):**

```sql
ALTER TABLE cloud_account_bindings ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE cloud_account_bindings ADD COLUMN priority INTEGER NOT NULL DEFAULT 100;
```

- PG: file mới `migrations/000121_cloud_binding_enable_priority.up.sql` + `.down.sql` (đặt tên theo tiền giác `000120_cloud_bindings.up.sql`); bump `RequiredSchemaVersion` 120 → 121 (`internal/upgrade/version.go:5`).
- SQLite: sửa `internal/store/sqlitestore/schema.sql` (cột trong CREATE TABLE + index nếu cần) + thêm patch vào map `migrations` từ 83 (`internal/store/sqlitestore/schema.go:97`, bump `SchemaVersion` 83 → 84 tại schema.go:19).

**Store:** `CloudBinding` thêm `Enabled bool` + `Priority int` (`internal/store/cloud_account_store.go:54-64`); PG impl (`internal/store/pg/cloud_accounts.go:265-301`) thêm cột vào SELECT/INSERT/upsert-SET; SQLite twin `internal/store/sqlitestore/cloud_accounts.go` đồng bộ.

**Resolve:** `internal/cloud/access.go:54-138` — thêm điều kiện `enabled` khi fetch bindings và sort `ORDER BY priority ASC` trong từng tier; tier-rank group→user→tenant giữ nguyên cho priority bằng nhau (đảm bảo dữ liệu cũ, priority=100, hành vi không đổi).

**HTTP:** `handleUpsertBinding` (`internal/http/cloud.go:335-384`) nhận optional `enabled`, `priority` (validate priority 0–1000, default 100 khi thiếu); `handleListBindings` (306-324) trả đủ 2 field. Guard giữ nguyên `requireTenantAdmin` (cloud.go:70-72).

**UI:** component mới `ui/web/src/components/ui/sheet.tsx` (Radix Dialog pattern như shadcn: trượt từ phải trên desktop, full-screen `max-sm:inset-0` trên mobile — theo mobile rule dialog AGENTS.md). `SettingsSheet` tách provider Select (Radix `Select`, sentinel `NONE`-style hoặc value điều khiển) + sections. `ProviderClientSetup` tách khỏi `cloud-page.tsx:82-181` thành file riêng `provider-client-setup.tsx` (giữ nguyên logic, chỉ dời chỗ).

## Related Code Files

**Create:**
- `migrations/000121_cloud_binding_enable_priority.up.sql` + `.down.sql`
- `ui/web/src/components/ui/sheet.tsx`
- `ui/web/src/pages/cloud/settings-sheet.tsx`
- `ui/web/src/pages/cloud/provider-client-setup.tsx` (dời từ cloud-page.tsx:82-181)

**Modify:**
- `internal/upgrade/version.go:5` — 120 → 121
- `internal/store/sqlitestore/schema.sql` + `internal/store/sqlitestore/schema.go` (migrations map + `SchemaVersion` 83 → 84)
- `internal/store/cloud_account_store.go:54-64` — struct `CloudBinding`
- `internal/store/pg/cloud_accounts.go:265-301` — SELECT/INSERT/upsert cột mới
- `internal/store/sqlitestore/cloud_accounts.go` — twin đồng bộ
- `internal/cloud/access.go:54-138` — filter enabled + sort priority
- `internal/http/cloud.go:306-384` — list/upsert bindings thêm field
- `ui/web/src/pages/cloud/scope-bindings-panel.tsx` — redesign thành bảng rules
- `ui/web/src/pages/cloud/hooks/use-cloud.ts:23-30,135-167` — `CloudBinding` thêm `enabled`, `priority`; `upsertBinding` input thêm optional fields
- `ui/web/src/pages/cloud/cloud-page.tsx:272-281,422,504-518` — nút gear + mount SettingsSheet; xóa BYO collapsible cũ (grep `showByoSetup`, `ProviderClientSetup` để dọn hết refer)
- 5× `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json`

## Implementation Steps

1. **Migration trước (rule #15 — schema trước code):** tạo PG migration 000121 up/down; bump `RequiredSchemaVersion`; sửa SQLite `schema.sql` + patch map 83→84 + bump `SchemaVersion`. Verify: fresh SQLite DB tạo bảng có cột mới; PG migrate up/down sạch.
2. Store layer: mở rộng `CloudBinding` + PG impl + SQLite twin (SELECT list, INSERT, upsert SET `enabled`, `priority`). `go build ./...` + `go build -tags sqliteonly ./...`.
3. Resolve: `access.go` lọc `enabled=false` + sort `priority ASC` trong tier; giữ tier-rank. Thêm/adjust unit test: binding disabled bị bỏ qua; 2 group binding khác priority → priority nhỏ thắng.
4. HTTP: `handleUpsertBinding` parse optional `enabled`/`priority` (validate 0–1000); response list thêm field. Document request/response shape mới.
5. **i18n (bắt buộc trước UI):** thêm keys vào `cloud.json` ×5 locale, nhóm `settings.*` + `scope.*` mới: `settings.title`, `settings.provider`, `settings.scope_rules`, `scope.enabled`, `scope.disabled`, `scope.priority`, `scope.col_type`, `scope.col_target`, `scope.col_account`, `scope.col_actions`, `scope.scope_type` (label cho select), `scope.rule_added`, `scope.table_empty`. Kiểm tra đủ 5 file trước khi sang bước 6.
6. Tạo `ui/web/src/components/ui/sheet.tsx` (Radix Dialog, side-right desktop, `max-sm:inset-0` mobile slide-up; portal có `pointer-events-auto` nếu custom — Radix-native thì tự xử lý).
7. Tách `ProviderClientSetup` sang `provider-client-setup.tsx` (props: `provider`), giữ role-check + logic save (`useCloudSettings`, `cloud-page.tsx:86-121`).
8. Tạo `settings-sheet.tsx`: props `{open, onOpenChange, provider, onProviderChange}`; chứa provider Select + `ProviderClientSetup` + `ScopeBindingsPanel` (mount theo admin như cũ).
9. Redesign `scope-bindings-panel.tsx`: bảng rules (`overflow-x-auto` + `min-w-[600px]` — mobile rule AGENTS.md:270) với cột [Switch enabled, loại badge, scope_key, AccountSelect inline (sentinel `NONE`), priority number input, xóa]; add-row thêm lựa chọn `user` (backend sẵn); sau add thành công reset key + giữ loại. Inline đổi account gọi `upsertBinding` cùng scope/key (upsert đè).
10. `use-cloud.ts`: `CloudBinding` + `upsertBinding` input thêm `enabled?`, `priority?`.
11. `cloud-page.tsx`: thêm nút gear vào PageHeader actions cạnh Refresh; xóa BYO collapsible (504-518) và mount cũ của ScopeBindingsPanel (422) — grep `showByoSetup|ScopeBindingsPanel|ProviderClientSetup` để chắc không sót refer; state `settingsOpen` + `settingsProvider` local.
12. Checklist: `go fix ./...`, build 2 tags, `go vet`, unit test store + access; `pnpm build` trong `ui/web`; thử tay mobile viewport (DevTools) bảng rules scroll ngang.

## Success Criteria

- [ ] Migration PG 000121 up/down sạch trên DB có dữ liệu bindings cũ (giữ nguyên row, enabled=true, priority=100)
- [ ] SQLite fresh DB + incremental patch đều đúng (`SchemaVersion` = 84); desktop build compile
- [ ] Gear cạnh "Làm mới" mở sheet; BYO card + scope rules nằm trong sheet; collapsible cũ đã xóa sạch
- [ ] Add-row tạo được rule `user` và `group`; tạo nhiều rule liên tiếp không lỗi; rule mới hiện ngay trong bảng
- [ ] Switch enable/disable + xóa + đổi account inline hoạt động; binding disabled không còn ảnh hưởng resolve (unit test)
- [ ] 2 priority khác nhau cùng tier → priority nhỏ thắng (unit test)
- [ ] Mọi key i18n mới có ở đủ 5 locale; bảng rules `min-w-[600px]` trong `overflow-x-auto`

## Risk Assessment

| Risk | Observable signal | Response (pre-decided) |
|---|---|---|
| Quên SQLite twin → desktop crash khi migrate | `wails dev` / lite build lỗi opening DB | Bước 1 làm PG + SQLite trong cùng commit; checklist build 2 tags |
| Resolve đổi hành vi với dữ liệu cũ | Agent resolve ra account khác trước/sau deploy | Dữ liệu cũ enabled=true + priority=100 đồng nhất → sort priority ổn định; unit test regression trường hợp dữ liệu cũ |
| Radix Select value `""` crash (empty value forbidden) | Console error Radix | Copy sentinel `NONE = "__none__"` từ scope-bindings-panel.tsx:22 cho mọi Select mới |
| Xóa collapsible cũ sót refer → build fail UI | `pnpm build` lỗi import | Bước 11 grep đủ `showByoSetup|ScopeBindingsPanel|ProviderClientSetup` trước khi commit |
| Migration number collide nếu phase khác cũng thêm migration song song | `goose`/migrate lỗi version exists | Chuỗi version đã cấp trong plan.md (P2=121, P6=122, P7=123); rebase thì renumber |
