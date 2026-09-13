---
phase: 1
title: "OAuth write scopes + re-consent"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: OAuth write scopes + re-consent

## Overview

Toàn bộ tính năng ghi (upload/mkdir/rename/move/delete/copy/share) bị chặn bởi hard blocker: scope OAuth hiện tại là read-only — Google `drive.readonly` (`internal/cloud/google.go:28-36`, `internal/cloud/credentials.go` `EmbeddedGoogleScopes`), OneDrive `Files.Read.All` với comment xác nhận "there is no write scope" (`internal/cloud/onedrive.go:31-36`) (scout GAPS #15). Phase này nâng scope set, thêm cờ `can_write` tính từ scope đã lưu của từng account, và thêm nút "Cấp quyền lại" (re-grant) chạy lại connect flow để lấy scope mới — tài khoản chưa re-grant vẫn dùng được toàn bộ đường read-only. Refresh token KHÔNG thể thêm scope (bắt buộc re-consent), nên đây phải là phase nền tảng.

## Surface parity

- **Backend:** sửa scope constants + helper `can_write` + field trong response list accounts.
- **API contract:** `GET /v1/cloud/accounts` thêm field `can_write` (bool) mỗi account; tài liệu endpoint.
- **Web UI:** nút "Cấp quyền lại" + chip "Chỉ đọc" trên account card; cập nhật setup guide i18n.
- **CLI/runtime:** N/A — cloud chưa có bề mặt CLI nào (scout §2.5: WS không có `cloud.*`).
- **Desktop UI:** N/A — `ui/desktop/frontend/src/` không có trang cloud; Lite edition tắt `CloudAccountsEnabled` (`internal/edition/edition.go:24,58`). Backend vẫn phải compile `-tags sqliteonly`.

## Requirements

### Functional
- Kết nối Google mới (BYO lẫn embedded client) yêu cầu scope `https://www.googleapis.com/auth/drive` (thay cho `drive.readonly` — superset).
- Kết nối OneDrive mới yêu cầu `Files.ReadWrite.All` (thay `Files.Read.All`).
- Tài khoản cũ (scope cũ trong DB `CloudAccount.Scopes`) được đánh dấu `can_write=false` và có nút "Cấp quyền lại" chạy lại flow `startConnect`/`completeConnect` hiện có; upsert theo conflict target `(tenant_id, user_id, provider, email)` (`internal/store/pg/cloud_accounts.go:67`) nên row account được cập nhật tại chỗ (không trùng).
- Endpoint ghi (Phase 4) sẽ từ chối 403 mã `cloud_write_scope_required` khi `can_write=false` — sentinel + helper đặt sẵn từ phase này.
- Đường read-only (list/about/mail/fetch) hoạt động nguyên trạng cho tài khoản chưa re-grant.

### Non-functional
- KHÔNG tự ý đánh dấu tài khoản cũ `expired`/`revoked` — chọn re-grant button thay vì force-expire để không phá flow đọc/agent của người không cần ghi.
- Giữ nguyên tính chất "order is stable, unit tests assert scope order" (`internal/cloud/google.go:26-28`, `internal/cloud/onedrive.go:31-33`): cập nhật test khẳng định thứ tự scope.

## Architecture

Data flow re-grant (tái dùng 100% flow connect hiện có):

1. UI gọi `startConnect(provider)` → `POST /v1/cloud/oauth/{provider}/start` (`internal/http/cloud.go:561-590`) → `Manager.BuildAuthURL` (`internal/cloud/manager.go:196-205`) — auth URL giờ chứa scope mới vì constants đã đổi.
2. Người dùng consent lại → callback mode hoặc paste-back mode (embedded client dùng loopback paste, `internal/cloud/credentials.go:32-33`).
3. `HandleCallback` (`internal/cloud/manager.go:325-359`) exchange + upsert row account (scopes mới đè scopes cũ).
4. UI invalidate `["cloud"]` (`ui/web/src/pages/cloud/hooks/use-cloud.ts:87-90`) → list account phản ánh `can_write=true`.

Helper phân loại (mới, ví dụ `internal/cloud/scopes.go`):
- `HasGoogleWriteScopes(scopesJSON string) bool` — match CHÍNH XẠC chuỗi `https://www.googleapis.com/auth/drive` (cẩn thận: substring `drive` sẽ khớp nhầm `drive.readonly` — phải so chuỗi đầy đủ).
- `HasMicrosoftWriteScopes(scopesJSON string) bool` — chứa `Files.ReadWrite.All`.
- `AccountCanWrite(acct *store.CloudAccount) bool` — dispatch theo `acct.Provider`.
- Sentinel error `ErrCloudWriteScopeRequired` cho endpoint ghi dùng ở Phase 4.

Lưu ý rclone remote: `ensureRemote` (`internal/cloud/storage_service.go:84-163`) pin `client_id`/`client_secret` và (từ commit 6c081728) pin `access_scopes` cho onedrive khi tạo remote. Remote hiện có tự self-heal rebuild từ DB khi gặp `InvalidAuthentication`/JWT error (cùng hàm). Cần verify access_scopes pin lấy từ scope account đã lưu (hoặc constants mới) để account re-grant tạo lại remote với scope ghi.

Điểm mạnh khả thi: embedded client chính là client của rclone (`internal/cloud/credentials.go` comment "identical to what rclone itself ships") — rclone drive backend mặc định dùng scope `drive` (full) và onedrive dùng `Files.ReadWrite.All`, nên consent qua client chia sẻ của rclone chấp nhận scope ghi mà không cần cấu hình gì.

## Related Code Files

**Modify:**
- `internal/cloud/google.go:28-36` — `GoogleScopes`: thay `.../auth/drive.readonly` → `.../auth/drive`.
- `internal/cloud/credentials.go` — `EmbeddedGoogleScopes`: thay `drive.readonly` → `drive`.
- `internal/cloud/onedrive.go:31-36` — `MicrosoftScopes`: thay `Files.Read.All` → `Files.ReadWrite.All`.
- `internal/cloud/storage_service.go:84-163` — verify/cập nhật pin `access_scopes` (onedrive) + token bootstrap lấy scope từ account/constants mới.
- `internal/http/cloud.go:214-228` — `handleList`: thêm `can_write` vào JSON response mỗi account.
- `ui/web/src/pages/cloud/hooks/use-cloud.ts:6-18` — `CloudAccount` thêm `can_write?: boolean`.
- `ui/web/src/pages/cloud/cloud-page.tsx:446-500` — account card: chip "Chỉ đọc" + nút "Cấp quyền lại".
- `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json` — keys mới (bước i18n bên dưới).
- Unit tests khẳng định thứ tự scope (grep `GoogleScopes`/`MicrosoftScopes` trong `internal/cloud/*_test.go` và cập nhật).

**Create:**
- `internal/cloud/scopes.go` — helpers `HasGoogleWriteScopes`/`HasMicrosoftWriteScopes`/`AccountCanWrite` + sentinel `ErrCloudWriteScopeRequired` (+ `scopes_test.go`).

## Implementation Steps

1. Đổi 3 scope constants (google.go, credentials.go, onedrive.go) theo Architecture. Chạy `go build ./...` — nếu test assert order fail, cập nhật expected order trong test (grep `GoogleScopes|MicrosoftScopes` trong `internal/cloud/`).
2. Tạo `internal/cloud/scopes.go` + test: parse `acct.Scopes` (JSON string), match chính xác chuỗi scope ghi theo provider; unit test phủ case `drive.readonly` KHÔNG được nhận là write.
3. `internal/http/cloud.go` `handleList` (214-228): map mỗi account thêm `"can_write": cloud.AccountCanWrite(&acct)` vào response struct.
4. Verify `ensureRemote` (storage_service.go:84-163): đọc logic pin `access_scopes`; nếu đang hard-code constants cũ thì chuyển sang lấy từ scope đã lưu của account (fallback constants mới). Ghi chú kết quả verify vào PR.
5. UI `use-cloud.ts`: thêm `can_write?: boolean` vào `CloudAccount` (snake_case JSON, không đổi shape khác).
6. UI `cloud-page.tsx`: trong account card (446-500), khi `account.can_write === false` render chip Badge "Chỉ đọc" (variant warning) + button "Cấp quyền lại" gọi `startConnect(provider)` và mở paste panel như flow connect hiện tại (212-247). Ẩn nút khi đang là account shared của người khác (chỉ owner re-grant được — token là của owner).
7. **i18n (bắt buộc trước khi build UI):** thêm keys vào `cloud.json` ở CẢ 5 locale `en, vi, zh, ko, ru`: `readonly_badge` ("Read-only" / "Chỉ đọc"), `regrant` ("Re-grant access" / "Cấp quyền lại"), `regrant_hint` (giải thích phải consent lại để ghi được file). Thiếu key = crash runtime (fallback en + console warn).
8. **Setup guide i18n:** cập nhật `setup.body`, `setup.google_step*`, `setup.onedrive_step*`, `setup.embedded_note` ở 5 locale: hướng dẫn BYO admin thêm scope `https://www.googleapis.com/auth/drive` vào GCP OAuth client và `Files.ReadWrite.All` vào Azure app registration (ghi chú work/school account có thể cần admin consent cho `Files.ReadWrite.All`); cập nhật note embedded client giờ yêu cầu scope Drive full.
9. Checklist post-implementation (AGENTS.md): `go fix ./...`, `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...`, chạy unit test package `internal/cloud`.

## Success Criteria

- [ ] `internal/cloud/google.go`, `credentials.go`, `onedrive.go` chứa scope ghi; unit test scope-order cập nhật và xanh
- [ ] `GET /v1/cloud/accounts` trả `can_write` đúng cho account scope cũ (false) và mới (true) — có handler test
- [ ] Account card hiện chip "Chỉ đọc" + nút "Cấp quyền lại"; click chạy lại OAuth flow và sau khi xong `can_write` thành true (thử tay với embedded client)
- [ ] Tài khoản chưa re-grant vẫn list/about/mail/files bình thường (regression thủ công)
- [ ] Setup guide (BYO) hiển thị đúng scope cần thêm ở 5 locale
- [ ] `go build -tags sqliteonly ./...` xanh

## Risk Assessment

| Risk | Observable signal | Response (pre-decided) |
|---|---|---|
| Match nhầm `drive.readonly` là write (substring) | Account cũ hiện `can_write=true` | So chuỗi chính xác đầy đủ + unit test case âm tính (bước 2) |
| rclone remote cũ vẫn giữ scope đọc (access_scopes/token cũ) sau re-grant | Ghi lỗi 403 từ provider dù DB có scope mới | Bước 4: pin access_scopes từ account scopes; nếu vẫn lỗi, xóa remote (`ConfigDelete` — wrapper có sẵn rc_client.go:97-99) để `ensureRemote` dựng lại; self-heal sẵn có (storage_service.go:84-163) bắt InvalidAuthentication |
| Google challenge bảo mật / OAuth verification với scope full trên embedded client | Consent hiện cảnh báo "unverified" | Embedded client là client rclone (đã verified với scope `drive`); với BYO client, setup guide cảnh báo bước publish app GCP |
| Azure work/school chặn `Files.ReadWrite.All` (cần admin consent) | User báo lỗi AADSTS65005/needs admin approval | Setup guide ghi chú rõ; UI lỗi connect hiển thị message gốc từ `?error=` banner (useCloudResult, cloud-page.tsx:42-57) |
| Account shared: owner không phải người đang đăng nhập → không tự re-grant được | Nút re-grant hiện cho member trên account shared | Ẩn nút khi `account.shared === true` và user không phải owner (UI chỉ còn chip cảnh báo) |
