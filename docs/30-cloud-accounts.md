# Clouds — Google Drive / OneDrive / Gmail (OAuth)

Kết nối tài khoản cloud của bạn với GoClaw để agent đọc/tìm kiếm/dọn hộp thư
(Gmail) và duyệt/tải file Drive/OneDrive. Tài khoản thuộc về **mỗi user** —
mỗi thành viên tự kết nối tài khoản của mình trên trang **Clouds**.

## Cách hoạt động

1. Bạn bấm **Connect Google** trên trang Cloud → GoClaw chuyển bạn sang trang
   đồng ý của Google (xác thực OAuth 2.0 + PKCE).
2. Sau khi đồng ý, Google redirect về GoClaw; access/refresh token được **mã
   hóa AES-256-GCM** trước khi lưu vào DB (`cloud_accounts`).
3. Agent dùng tool `cloud_accounts` → `mail_search` / `mail_read` /
   `mail_archive` / `mail_unsubscribe` và `cloud_ls` / `cloud_read` /
   `cloud_fetch` / `cloud_about` để thao tác.

Bảo mật:

- Token mã hóa theo key `GOCLAW_ENCRYPTION_KEY`; không bao giờ log hay trả về
  client/UI.
- Không có đường **xóa vĩnh viễn** mail trong bất kỳ tool nào — trash có thể
  khôi phục.
- `mail_unsubscribe` luôn phân tích trước; chỉ thực thi (POST RFC 8058) sau
  khi bạn đồng ý rõ ràng với sender đó.
- State OAuth ký HMAC + hết hạn 10 phút (chống CSRF).

## Zero-config: bấm Connect là chạy (mặc định)

Từ v4.0.6, GoClaw nhúng sẵn **OAuth client dùng chung của rclone** (cùng cơ
chế với rclone CLI). Không cần cấu hình gì:

1. Trang **Clouds** → chọn **Google Drive** hoặc **Microsoft OneDrive** →
   bấm **Kết nối**.
2. Trình duyệt mở trang đồng ý của Google/Microsoft (bạn sẽ thấy tên app
   "rclone" — đúng như vậy, storage đi qua engine rclone).
3. Sau khi đồng ý, trình duyệt dừng ở một trang **không tải được**
   (`http://127.0.0.1:53682/...`) — đó là bình thường. **Copy toàn bộ URL
   trên thanh địa chỉ**, dán vào ô trên trang Clouds rồi bấm **Hoàn tất**.

Giới hạn của đường zero-config (do dùng client dùng chung):

- **Quota chia sẻ** với toàn bộ người dùng rclone — Google/Microsoft có thể
  throttle khi quá tải.
- Google chỉ cấp scope **Drive (đọc)** — **không có Gmail**. Muốn dùng mail
  tools, hãy cấu hình OAuth client riêng (phần dưới).
- rclone có thể rotate secret trong bản phát hành tương lai; khi đó cần cập
  nhật GoClaw.
- Refresh token gắn với client đã cấp quyền: tài khoản kết nối nhanh luôn
  refresh bằng client nhúng, tài khoản BYO luôn refresh bằng client BYO
  (client_id được ghi vào `cloud_accounts.settings`).

## Thiết lập OAuth client riêng — BYO (tùy chọn, cho Gmail/quota riêng)

Muốn dùng Gmail tools (và có quota/branding riêng), admin tự đăng ký OAuth
client một lần ngay trên Web UI.

### Bước chuẩn bị trên Google Cloud Console

1. Vào [Google Cloud Console](https://console.cloud.google.com/) → tạo project
   (hoặc dùng project có sẵn).
2. **APIs & Services → Library**: bật **Gmail API** và **Google Drive API**.
3. **APIs & Services → OAuth consent screen**:
   - User type **Internal** (Google Workspace) hoặc **External**.
   - Nếu External: bấm **Publish App** (chế độ "In production", không cần
     verify). ⚠️ **Không giữ chế độ Testing** — refresh token trong Testing chỉ
     sống **7 ngày** và tối đa 100 test users.
   - Scopes cần thiết: `gmail.readonly`, `gmail.labels`, `gmail.modify`,
     `drive.readonly`, `userinfo.email`, `userinfo.profile`.
4. **Credentials → Create credentials → OAuth client ID**:
   - Type: **Web application**.
   - **Authorized redirect URIs**: thêm chính xác
     `https://<goclaw-cua-ban>/v1/cloud/oauth/callback`
     (trang Cloud hiển thị đúng chuỗi này kèm nút copy khi chưa cấu hình).

### Nhập vào GoClaw

Mở trang **Cloud** (quyền admin) → điền **Client ID** + **Client Secret** →
bấm **Lưu**. Credentials được mã hóa AES-256-GCM trong `config_secrets` và
form setup **không hiện lại** (còn nút nhỏ "Cập nhật credentials" khi cần
rotate). Không cần khởi động lại gateway — kết nối Google dùng được ngay.

> Cách thay thế (tự host bằng file config): đặt `cloud.google.client_id`
> trong config.json5 và `GOCLAW_CLOUD_GOOGLE_CLIENT_SECRET` trong
> `.env.local`. Web UI sẽ không hiện form setup nếu env đã đủ.

## Cấu hình GoClaw

`config.json5`:

```json5
{
  cloud: {
    // redirect_base_url: "https://goclaw-cua-ban", // chỉ cần khi auto-detect sai (reverse proxy)
    google: {
      client_id: "1234-abc.apps.googleusercontent.com",
      // KHÔNG đặt client_secret ở đây — dùng env:
    },
    mail_rate_per_minute: 10,   // giới hạn gọi Gmail API mỗi tài khoản
    mail_read_max_bytes: 8192,  // truncate nội dung mail_read
    fetch_size_cap_mb: 100,     // giới hạn cloud_fetch
    rclone_path: "rclone",      // đường dẫn binary rclone (phần Drive)
  },
}
```

Biến môi trường (`.env.local`):

```
GOCLAW_CLOUD_GOOGLE_CLIENT_SECRET=GOCSPX-...
```

Khởi động lại gateway → trang **Cloud** hiện nút **Connect Google**.

## Lưu trữ Drive (rclone)

Phần Drive đi qua [rclone](https://rclone.org) chạy nội bộ (`rclone rcd`,
chỉ 127.0.0.1, basic-auth ngẫu nhiên) — không expose ra ngoài.

- **Docker: rclone đã có sẵn trong mọi image variant** (base/latest/full) —
  không cần cài gì thêm. Bare-metal: `apt install rclone` / `brew install rclone`.
- Thiếu rclone: tool mail vẫn hoạt động bình thường; tool Drive trả lỗi có
  hướng dẫn cài.

Ghi chú: rclone **tự refresh** token của nó (ghi vào file config nội bộ của
nó). Nếu token trong GoClaw hết hiệu lực (đổi mật khẩu Google, thu hồi), hãy
**Disconnect → Connect lại** trên trang Cloud.

## Troubleshooting

| Triệu chứng | Nguyên nhân | Cách xử lý |
|---|---|---|
| `redirect_uri_mismatch` khi đăng nhập | Redirect URI chưa khớp 100% | Copy đúng chuỗi từ trang Cloud paste vào GCP console (scheme, host, không thừa `/`) |
| Token chết sau ~7 ngày | OAuth app đang ở chế độ **Testing** | Publish to production (unverified là đủ) hoặc dùng Internal app |
| `invalid_grant` khi dùng tool | Refresh token bị thu hồi (đổi mật khẩu / thu hồi quyền) | Disconnect → Connect lại trên trang Cloud |
| Tool Drive báo "rclone is not installed" | Thiếu binary | Cài rclone hoặc đặt `cloud.rclone_path` |
| Trang Cloud không hiện | Edition Lite, hoặc chưa cấu hình `client_id`/secret | Cấu hình theo phần trên; Lite (desktop) không có Cloud |

## Bề mặt API

| Endpoint | Mô tả |
|---|---|
| `GET /v1/cloud/status` | Cloud bật chưa, provider nào đã cấu hình |
| `GET /v1/cloud/settings` | Admin: xem OAuth client đã cấu hình (không trả secret) |
| `PUT /v1/cloud/settings` | Admin: lưu OAuth client từ form Web UI (mã hóa) |
| `GET /v1/cloud/accounts` | Danh sách tài khoản của user (không trả token) |
| `DELETE /v1/cloud/accounts/{id}` | Ngắt kết nối |
| `POST /v1/cloud/oauth/google/start` | Lấy `auth_url` + `redirect_uri` |
| `GET /v1/cloud/oauth/callback` | Redirect target của Google (state ký HMAC) |
