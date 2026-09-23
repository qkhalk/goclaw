# Kênh

Kênh kết nối các agent GoClaw với các nền tảng nhắn tin. Gateway hỗ trợ 10
loại kênh, và mỗi loại có thể có nhiều tài khoản được cấu hình (**channel
instance**) chạy đồng thời.

## Các loại kênh được hỗ trợ

| Loại | Nền tảng | Ghi chú |
|------|----------|---------|
| `telegram` | Telegram | Lệnh bot, forum topic, STT tin nhắn voice, lệnh đa ngôn ngữ, inline picker chọn skill |
| `discord` | Discord | Bot guild/kênh với xử lý media |
| `facebook` | Facebook / Messenger | Nhắn tin qua Page |
| `feishu` | Feishu / Lark | Message card streaming và gửi media |
| `pancake` | Pancake | Nhắn tin qua nền tảng Pancake |
| `slack` | Slack | Bot workspace với xử lý mention |
| `whatsapp` | WhatsApp | Client multi-device native qua whatsmeow — đăng nhập QR, không qua Business API chính thức |
| `zalo_oa` | Zalo Official Account | Nhắn tin OA |
| `zalo_personal` | Zalo Personal | Đăng nhập QR, liệt kê danh bạ |
| `bitrix24` | Bitrix24 | Bot dựa trên imbot; portal được onboarding qua luồng cài đặt tự phục vụ |

Các hằng số loại kênh được định nghĩa trong `internal/channels/channel.go`.

## Channel instance

Một **channel instance** là một tài khoản được cấu hình của một loại kênh
(ví dụ hai bot Telegram, hoặc một số WhatsApp cho mỗi tenant). Instance được
lưu theo từng tenant và quản lý qua cả hai API:

- WebSocket RPC: `channels.instances.list` / `get` / `create` / `update` /
  `delete`
- HTTP: `/v1/channels/instances/*` (list, get, create, delete) cùng các
  endpoint theo từng instance cho context capability, phân giải
  group/thành viên, duyệt trích xuất bộ nhớ, và phạm vi credential

### Allowlist writer

Hội thoại nhóm được kiểm soát bởi allowlist writer — chỉ người gửi được liệt
kê mới có thể kích hoạt thao tác điều khiển (chạy agent, lệnh) trong group
hoặc forum:

- `writers` — các mục allowlist của một instance
- `writers/groups` — các group đủ điều kiện quản lý allowlist
- `writers/test` — kiểm tra một người gửi cụ thể hiện có phải writer hay
  không

Expose qua `GET/POST/DELETE /v1/channels/instances/{id}/writers*`.

## Trình quản lý kênh

Trình quản lý kênh (`internal/channels`) nắm vòng đời instance:

- `StartAll` / `StopAll` — khởi động hoặc dừng mọi instance đã đăng ký cùng
  gateway
- Health snapshot — mỗi kênh đang chạy báo một health snapshot cho dashboard
  hiển thị
- Ghi nhận lỗi — lỗi khởi động và lỗi runtime được phân loại và lưu theo từng
  instance để UI hiển thị được lỗi gần nhất
- Liệt kê group / phân giải thành viên — các kênh có group triển khai một
  provider interface dùng chung, phục vụ allowlist writer và việc chọn đích

## Pairing và đăng nhập QR

Một số kênh xác thực theo kiểu tương tác:

- **WhatsApp** và **Zalo Personal** dùng đăng nhập QR — bắt đầu luồng rồi
  quét bằng app trên điện thoại. Mã QR và xác nhận hoàn tất được đẩy về dạng
  server event (`whatsapp.qr.code` / `whatsapp.qr.done`,
  `zalo.personal.qr.code` / `zalo.personal.qr.done`), với điểm vào WS là
  `whatsapp.qr.start` và `zalo.personal.qr.start`
- Các kênh có bot nhắn tin trực tiếp (Telegram, Discord, Slack, ...) hỗ trợ
  **mã pairing** cho thiết bị: người dùng gửi yêu cầu pairing trong chat,
  một operator phê duyệt bằng `goclaw pairing approve`, và người dùng nhận
  quyền truy cập gateway theo đúng vai trò của mình

## Quản lý kênh

| Bề mặt | Đường dẫn | Quyền truy cập |
|--------|-----------|----------------|
| Web UI | `/channels` | Chỉ admin |
| CLI | `goclaw channels list` / `add` / `delete` | Cần gateway đang chạy |
| HTTP | `/v1/channels/instances/*` | Gateway token hoặc API key |
| WebSocket | `channels.instances.*` | Kiểm tra vai trò theo từng method |

CLI nói chuyện với gateway đang chạy qua HTTP — hãy khởi động gateway trước.

::: tip Tìm sâu về Telegram
Telegram là kênh đầy đủ tính năng nhất (slash command đa ngôn ngữ, theo dõi
subagent, picker chọn skill, pipeline định dạng). Xem
[Telegram](./telegram) để biết chi tiết.
:::
