# Telegram

Telegram là kênh hoàn chỉnh nhất của GoClaw: lệnh đa ngôn ngữ, inline picker,
danh sách skill phân trang, theo dõi subagent — và (sắp ra mắt) nút archive
cho subagent đã xong việc.

## Kết nối bot

1. Tạo bot với [@BotFather](https://t.me/BotFather) và copy token.
2. Trong dashboard GoClaw, mở **Channels → Telegram**, dán token và lưu.
   Gateway tự đăng ký vòng lặp webhook/updates.
3. Bắt đầu chat riêng với bot của bạn, hoặc thêm bot vào một nhóm. Trong
   nhóm, bot tuân theo quyền chat của session — chỉ thành viên có quyền
   writer mới kích hoạt được thao tác điều khiển (xem bên dưới).

Câu trả lời của bot được bản địa hóa: bot trả lời theo ngôn ngữ của session
người dùng (tiếng Anh, tiếng Việt, tiếng Trung, tiếng Hàn hoặc tiếng Nga).

## Các lệnh

Bot cung cấp slash command đa ngôn ngữ. Những lệnh cơ bản:

| Lệnh | Mục đích |
|------|----------|
| `/start`, `/help` | Đăng ký hội thoại và liệt kê bot làm được gì |
| `/subagents` | Liệt kê task subagent của agent kèm trạng thái trực tiếp |
| Lệnh `/gc:` | Bảng slash command cho thao tác nhanh ngay trong chat |

Các dòng của `/subagents` dùng chung một bộ trạng thái với web UI:
`queued` → `running` → `waiting` → `completed` / `failed` / `cancelled`.
Mỗi task dẫn tới trang chi tiết với tóm tắt và kết quả.

### Nút archive subagent (sắp ra mắt)

Các task đã xong (`completed`/`failed`/`cancelled`) trong danh sách
`/subagents` sẽ có nút **archive** một chạm — một nút mỗi task cùng một thao
tác "archive tất cả task đã xong". Archive đưa task ra khỏi danh sách mặc
định trên mọi bề mặt (cả web panel lẫn Telegram); task đang chạy không thể
archive cho tới khi đạt trạng thái kết thúc, và trong nhóm chỉ chat writer
mới được archive.

## Cách câu trả lời được định dạng

Output của model đi qua một pipeline định dạng chuyên dụng trước khi
Telegram chấp nhận:

```
LLM output
  → SanitizeAssistantContent()      strip markup Telegram rejects
  → markdownToTelegramHTML()        markdown → Telegram HTML subset
  → chunkHTML()                     split within Telegram size limits
  → sendHTML()                      deliver
```

Bảng được render dạng **ASCII bên trong thẻ `<pre>`** — Telegram không có
markup bảng — và câu trả lời dài được gửi thành nhiều message, chia tại các
ranh giới an toàn.

## Các thành phần tương tác

- **Inline picker** — bot trả lời câu hỏi dạng lựa chọn từ tool `ask_options`
  bằng các nút bàn phím inline (1–4 lựa chọn), giống hệt web chat.
- **Skill phân trang** — danh sách skill được phân trang với điều hướng
  inline thay vì một message khổng lồ.
- **Thông báo lượt chạy** — khi một subagent xong việc, hội thoại nhận được
  một message thông báo để bạn nhận kết quả mà không cần mở web UI.

## Lưu ý cho chat nhóm

- Bot chỉ phản ứng với những message được cấu hình để thấy (mention, reply,
  lệnh), tùy theo tùy chọn của session.
- Thao tác điều khiển (archive subagent, thao tác task) cần quyền **writer**
  trong nhóm.
- Telegram giới hạn sửa tin nhắn trong ~48 giờ — với các danh sách cũ hơn,
  bot gửi một tin xác nhận mới thay vì sửa tin gốc.
