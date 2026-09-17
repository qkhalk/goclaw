# Cloud native storage + agent permissions (2026-09-16)

## Yêu cầu của anh (message gốc)
1. Kiểm tra trang /cloud (đang chết: "rclone đã cài trên server chưa?").
2. Đổi "Ổ của tôi" → "Dashboard", bỏ mục "Ổ của tôi" trong dashboard.
3. Thanh tìm tài khoản trong mỗi provider.
4. Preview file thay vì icon + click mở ở GIỮA (không phải panel bên cạnh).
5. Giải thích "Phạm vi sử dụng" / "Mặc định tổ chức".
6. Đọc dự án rclone để TÍCH HỢP (không cài binary).
7. Tuỳ chỉnh quyền agent theo từng tài khoản mail/drive.
8. Thiết kế chuẩn doanh nghiệp.

## Nghiên cứu rclone (kết luận then chốt)
- rclone KHÔNG phải library: global registries (fs.Registry/config), init() side effects,
  API nội bộ đổi mỗi release, không có docs library; librclone là C-API (cgo).
  → Import làm Go module là ngõ cụt (đúng như dự đoán; không có project nào làm thật).
- Cách đúng: đọc CÁCH rclone nói chuyện với Drive v3/Graph rồi viết native client:
  - Drive path→ID walk + dir cache 5 phút (parity --dir-cache-time), query escape ' và \,
    trashed=false everywhere, orderBy modifiedTime desc (duplicate names newest-wins).
  - Drive delete mặc định là TRASH (rclone --drive-use-trash=true) — comment cũ "no trash"
    của layer rclone là SAI; giữ nguyên semantics recoverable qua PATCH trashed:true.
  - Upload: multipart ≤5MB + resumable session single-PUT; Graph ≤4MB PUT
    (conflictBehavior=replace) + upload session chunk 10MiB (bội của 320KiB).
  - mtime propagation: Drive metadata.modifiedTime / Graph fileSystemInfo PATCH —
    không có cái này thì sync-pair quét lại toàn bộ tree mỗi lần (rclone skip identical).
- Cloud page hiện tại chết vì server chưa có rclone binary → giờ backend không cần binary nữa.

## Kiến trúc mới
- internal/cloud/storage: Backend interface + DriveBackend + GraphBackend; xoá rcd.go/rc_client.go.
- StorageService giữ nguyên public surface (trừ FS/DownloadAccount/ErrRCloneMissing đã chết),
  backend cache per-account invalided theo UpdatedAt (re-grant tự rebuild).
- Copy job engine riêng (copyjob.go): goroutine detached + registry oldest-ID eviction;
  TransferAccount single-file = stream trực tiếp; folder = async job có progress.
- Download streaming + Range relay; signed URL ngắn hạn (HMAC tenant|user|account|path,
  10 phút) cho <video>/<iframe> — handler đăng ký KHÔNG requireAuth, tự xác thực 2 chế độ.

## Quyền agent
- Ladder none<read<write<full; default 'read' (legacy rows).
- AgentAccount resolve TRƯỚC rồi mới check level → account tồn tại nhưng thiếu quyền
  báo lỗi rõ ("admin can raise it on the Clouds page") thay vì "not found".
- Write tools mới: cloud_write/mkdir/copy (write), move/delete/share (full).
- UI: Select trên AccountCard (admin) + i18n 5 ngôn ngữ.

## Review tự động (code-reviewer) — đã sửa hết blocker + major
- BLOCKER: ListAccount dùng CleanRemotePath chặn "/" → root không list được → CleanRemoteDir.
- MAJOR: Drive upload tạo duplicate (Drive cho trùng tên) → overwrite in-place qua
  PATCH multipart khi path đã có file; childID orderBy newest.
- MAJOR: delete isDir lệch kiểu → xoá cả cây; Drive DELETE vĩnh viễn → Stat guard + trash.
- MAJOR: mtime không propagate → sync quét lại hết → metadata.modifiedTime/fileSystemInfo.
- MAJOR: stream size-unknown (copyurl chunked) gãy Content-Range → spool temp file.
- MEDIUM đã sửa: SSRF guard cho CopyURL, preview query key theo path (không phải name),
  refetchInterval 5' cho signed URL (TTL 10'), blob cache (URL revoke per-mount),
  dir-cache TTL, input rail text-base md:text-sm (iOS zoom), sign-expiry race (bỏ expiry
  khỏi payload — suffix của token đã bind).

## Agent song song
- Vẫn đang chạy (video-designer/pptx/skills); đã né toàn bộ file M của họ
  (credentials.go, sync_service_test.go, sidebar*, routes*, chat*, toolbox.json...).
- Họ tự vá mock SetAgentAccess trong cloud_files_test.go (trùng interface mới) — đã Stage cùng.
- Commit trên nhánh chung feat/verifier-recover-plan-tool (740f8044d) — KHÔNG đổi HEAD.

## Kết quả verify
- go build ./... (PG) + -tags sqliteonly: OK; go vet scoped: OK.
- Tests: internal/cloud + storage + http (Cloud*) + tools (mail/cloud_accounts): PASS.
  Hang có sẵn Windows: tools Delegate (skip), media tests internal/agent — đã chứng minh
  pre-existing bằng worktree sạch ở phiên trước.
- tsc -b: 0 lỗi; pnpm build (Vite prod): OK 1m02s.
- i18n: cloud.json ×5 +20/-4 mỗi file, key mới đủ (drive.dashboard, search_accounts,
  no_accounts_match, agent_access.*, preview.close, scope.tenant_default_hint).

## Giải thích "Phạm vi sử dụng" (cho anh)
- Là cloud_account_bindings: chọn TÀI KHOẢN nào agent dùng khi không chỉ định.
- Thứ tự: chỉ định tường minh > group chat > user > MẶC ĐỊNH TỔ CHỨC > tài khoản cá nhân.
- "Mặc định tổ chức" hiện một email = tài khoản đó thành default toàn tenant
  (agent/cloud page sẽ dùng nó khi không naming). Không phải quyền — chỉ là "chọn ổ".
- Quyền THẬT của agent giờ nằm ở "Quyền agent" trên account card (none/read/write/full).

## Còn lại / Next
- DEPLOY chờ anh xác nhận không còn agent chạy (phải restart goclaw; build embedui:
  cp -r ui/web/dist internal/webui/dist trước khi go build -tags embedui).
- Server 192.168.1.103:18790 hiện không phản hồi từ máy này (curl 000) — kiểm tra lại
  khi deploy.
- 416 pass-through, Drive cache TTL <5' vẫn còn như ghi nhận minor (không chặn).
