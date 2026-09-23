# Xử lý sự cố

## Gateway không khởi động được

Làm theo thứ tự sau:

1. **Chạy lệnh health check:**

   ```bash
   goclaw doctor
   ```

   Lệnh này kiểm tra môi trường hệ thống và cấu hình, và chỉ đúng ra thứ đầu
   tiên bị lỗi.

2. **Kiểm tra PostgreSQL có thể kết nối và pgvector đã được cài.** Schema
   gốc cần các extension `pgvector` (và `pgcrypto`). Thử DSN từ
   `.env.local`:

   ```bash
   psql "$GOCLAW_POSTGRES_DSN" -c "SELECT extname FROM pg_extension;"
   ```

   Nếu thiếu `vector`: `CREATE EXTENSION vector;` (extension này phải được
   cài trên server trước — Docker image đi kèm `pgvector/pgvector:pg18` có
   sẵn).

3. **Áp dụng migration**, rồi khởi động lại:

   ```bash
   goclaw migrate up
   ```

**Health check**

```bash
curl http://localhost:18790/health
```

**Log**

```bash
make logs                # Docker Compose
journalctl -u goclaw -f  # systemd
```

## Vấn đề đăng nhập và token

**Dashboard hoặc CLI nhận 401/forbidden**
Client phải trình ra đúng `GOCLAW_GATEWAY_TOKEN` mà gateway đã khởi động
cùng. Nguyên nhân phổ biến nhất là không khớp giữa giá trị trong `.env.local`
của gateway và cái client gửi (token lưu trong trình duyệt, hoặc `--token`
trên operator CLI). Sửa giá trị phía client, hoặc cập nhật `.env.local` rồi
khởi động lại gateway — sau đó xác thực lại mọi client.

**Gateway khởi động nhưng dashboard trống / các method thất bại**
Có thể migration chưa chạy. Chạy `goclaw migrate up` rồi khởi động lại. Với
triển khai server thủ công, xác nhận thư mục `migrations/` thực sự được đóng
gói — xem
[Tự host](./self-hosting#migrations-read-this-before-manual-deploys).

## Vì sao câu trả lời có vẻ "ngu"

Nếu câu trả lời của agent nông hơn hẳn mong đợi — ngắn, chung chung, thiếu
các bước suy luận — hãy đi qua checklist này. Ba nguyên nhân dưới đây chiếm
phần lớn các trường hợp.

### 1. Reasoning bị tắt (và việc hạ cấp diễn ra im lặng)

Mức **thinking/reasoning** của model thay đổi rõ rệt độ sâu của câu trả lời.

- Agent mới mặc định là `auto`: mức này được suy ra từ capability map của
  provider — Anthropic → medium, model reasoning tương thích OpenAI → low,
  model không rõ → **off**. Agent tạo từ trước giữ nguyên giá trị đã lưu trên
  chúng, với agent cũ thường là **off** hẳn.
- Khi provider không phục vụ được mức yêu cầu, GoClaw hạ xuống mức thấp hơn.
  Trước đây việc hạ cấp này diễn ra **im lặng**; giờ run trace ghi lại nó
  (một ghi chú dạng "reasoning medium → off (provider not supported)").
- **Kiểm tra:** mở phần cài đặt agent và xem cấu hình reasoning của nó; với
  provider không hỗ trợ reasoning, hãy chọn một model có hỗ trợ.

### 2. Tham số subagent

Subagent từng chạy với **tham số thấp được hardcode** (`max_tokens: 4096`,
`temperature: 0.5`) — tệ thấy rõ so với agent cha. Giờ chúng **kế thừa cấu
hình hiệu lực của agent cha**, trừ khi định nghĩa subagent ghi đè.

- **Kiểm tra:** nếu output của subagent vẫn nông, rà định nghĩa của nó để tìm
  các ghi đè tường minh `maxTokens` / `temperature` / `thinkingLevel` sót lại
  từ những setup trước, và xóa những cái bạn không chủ ý.

### 3. Fallback và routing của provider

Tầng reliability sẽ circuit-break các provider đang gặp vấn đề và định tuyến
vòng qua chúng — nghĩa là request của bạn có thể rơi vào một provider/model
khác (yếu hơn) so với những gì bạn nghĩ.

- **Xem trace:**

```bash
goclaw traces list --status error
goclaw traces get <trace-id> -o json
```

Trace cho biết provider/model nào thực sự phục vụ từng lời gọi, mức thinking
hiệu lực, và ghi chú hạ cấp (nếu có). Web dashboard hiển thị cùng metadata
theo từng lượt chạy trong chỉ báo hoạt động của chat.

## Ngôn ngữ và locale

**UI hiển thị sai ngôn ngữ hoặc hiện key chưa dịch**
Ngôn ngữ web UI được đặt từ bộ chọn trên thanh trên cùng và lưu cục bộ; các
locale UI được hỗ trợ là en, vi, zh, ko, ru. Nếu các mục hiện dưới dạng key
thô, hãy hard-refresh trang để tải lại bundle locale. Catalog thông điệp
backend (câu trả lời của agent, message hệ thống) hỗ trợ en, vi, zh — locale
đến backend qua tham số `connect` của WebSocket hoặc header HTTP
`Accept-Language`, nên thiếu header có thể khiến câu trả lời rơi về tiếng
Anh.

## Cài đặt và khởi động

**`make up` thất bại với lỗi "port 5432 already allocated"**
Một Postgres khác đang chiếm cổng. Đặt cổng host khác trong `.env`
(`POSTGRES_PORT=5433`) rồi thử lại.

**Biến môi trường từ file env không bao giờ có hiệu lực dưới systemd**
systemd không parse các dòng `export KEY=...`. Dùng dạng thuần `KEY=value`
trong `EnvironmentFile`, hoặc directive `Environment=` tường minh. Xem
[Cấu hình](./getting-started/configuration).

## Ứng dụng desktop

**Reset dữ liệu**
**Settings → About → Reset Database** trong ứng dụng desktop xóa `goclaw.db`
(cùng `-wal`/`-shm`) khỏi `~/.goclaw/data/` và khởi động lại ứng dụng với
một database mới tinh. Dùng sau khi nâng cấp thất bại hoặc trạng thái bị
hỏng. File trong workspace được giữ lại. Xem [Desktop](./desktop).

**Cập nhật không bao giờ hoàn tất**
Bản cập nhật desktop tải từ `lite-v*` GitHub Releases; banner áp dụng bản
cập nhật và khởi động lại ứng dụng. Nếu việc kiểm tra liên tục thất bại,
xác nhận máy có với tới được github.com hay không — ứng dụng desktop không
có lệnh cập nhật thủ công. Các bản server cũng không có lệnh tự cập nhật;
thay binary hoặc pull image mới như mô tả trong
[Cài đặt](./getting-started/install#updating).

## Render video

- Job `render_video` kẹt ở queued → kiểm tra worker:
  `curl http://127.0.0.1:18791/health` và
  `journalctl -u goclaw-videoworker -f`. Giới hạn hàng đợi (`--max-queue`,
  mặc định 5) trả về 409 khi đầy.
- Render thất bại sau khi worker crash → đúng như dự kiến: trạng thái worker
  nằm trong bộ nhớ. Gateway đánh dấu các job mồ côi là `failed`; hãy chạy
  lại chúng. Thư mục tạm mồ côi được dọn tự động mỗi 15 phút.
- Thiếu phụ đề → `--font-file` trống hoặc đường dẫn font sai.

## Telegram

- Bot trả lời nhưng nút điều khiển không tác dụng trong nhóm → thao tác nhóm
  cần quyền **writer** cho vai trò thành viên của bạn.
- Tin nhắn đã sửa không bao giờ cập nhật → Telegram từ chối sửa tin nhắn cũ
  hơn ~48 giờ; thay vào đó bot gửi một tin xác nhận mới.
- Bảng hiển thị lỗi → bảng được render dạng ASCII trong `<pre>` một cách cố
  ý; Telegram không có markup bảng.

## Skills

- Job cài từ chợ skill thất bại → việc cài đọc từ **thư mục skill bundled
  trên đĩa**; nếu bạn chạy binary trần bên ngoài cấu trúc release, hãy trỏ
  `GOCLAW_BUNDLED_SKILLS_DIR` tới thư mục bundled trong bản release.
- Skill tùy chỉnh không gỡ được từ chợ → đúng thiết kế; chợ chỉ gỡ những
  skill do chính nó cài (nguồn gốc system). Hãy xóa skill tùy chỉnh thủ công.
- Đã đổi `seed_mode` sang `core` nhưng skill cũ vẫn còn → đúng dự kiến: seed
  mode chỉ ảnh hưởng các bản cài **mới tinh**; reconciler không bao giờ xóa
  skill đã cài. Hãy gỡ những cái bạn không muốn.
