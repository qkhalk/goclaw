# Cấu hình

GoClaw tách biệt **phần bạn cấu hình** (file config) khỏi **phần cần giữ bí
mật** (biến môi trường). Đừng bao giờ đặt API key vào file config.

## File config

GoClaw đọc file cấu hình dạng **JSON5** — cho phép comment và dấu phẩy thừa
ở cuối:

```json5
{
  // Fragment — shows commonly used sections.
  // Real defaults are seeded by `goclaw onboard`.

  agents: {
    // Reasoning/thinking default for NEWLY created agents:
    // auto | off | low | medium | high (default: auto)
    // Existing agents keep the value saved on the agent.
    reasoning_default: "auto"
  },

  skills: {
    // Which skills are seeded on a fresh install:
    // all   — full bundle (legacy default, back-compat)
    // core  — ~10 runtime-critical skills only
    // none  — nothing; install from the Skill Market later
    seed_mode: "all"
  }
}
```

### Thứ tự tìm file config

Đường dẫn config được xác định theo thứ tự sau (xem `resolveConfigPath` trong
`cmd/root.go`):

1. Flag `--config` trên dòng lệnh, nếu được đặt.
2. Biến môi trường `GOCLAW_CONFIG`, nếu được đặt.
3. `config.json` trong **thư mục làm việc hiện tại**.
4. Giá trị mặc định tích hợp sẵn — chỉ khi không có file nào tại đường dẫn đã
   xác định.

Thực tế `goclaw onboard` ghi `config.json` vào thư mục bạn khởi động gateway,
nên bước 3 là trường hợp phổ biến.

## Lớp phủ biến môi trường (env overlay)

Biến môi trường ghi đè giá trị trong file config. Các biến chính:

| Biến | Mục đích |
|------|----------|
| `GOCLAW_CONFIG` | Đường dẫn file config JSON5 |
| `GOCLAW_POSTGRES_DSN` | Chuỗi kết nối PostgreSQL (chỉ qua env, không bao giờ lưu trong config) |
| `GOCLAW_GATEWAY_TOKEN` | Bearer token cho HTTP API và operator CLI |
| `GOCLAW_ENCRYPTION_KEY` | Khóa AES-256-GCM mã hóa API key của provider khi lưu trữ |
| `GOCLAW_PORT` | Cổng lắng nghe của gateway (mặc định `18790`) |
| `GOCLAW_STORAGE_BACKEND` | `postgres` (mặc định) hoặc `sqlite` — chỉ qua env |

Các phân đoạn con của config ánh xạ sang biến env có tiền tố tương ứng. API
key của provider là ví dụ phổ biến nhất; một vài ánh xạ đã kiểm chứng khác:

```bash
export GOCLAW_ANTHROPIC_API_KEY=sk-ant-...       # providers.anthropic.api_key
export GOCLAW_OPENROUTER_API_KEY=sk-or-...       # providers.openrouter.api_key
export GOCLAW_MODEL=anthropic/claude-sonnet-4    # agents.defaults.model
export GOCLAW_SKILLS_SEED_MODE=core              # skills.seed_mode
```

`GOCLAW_MODE` đã **bị loại bỏ (deprecated)** và bị bỏ qua — đặt nó chỉ sinh
ra một cảnh báo khi khởi động.

## Secrets

Secrets nằm trong `.env.local` (hoặc biến môi trường thật) — **không bao giờ
trong `config.json`**:

```bash
# .env.local — source it before starting the gateway
source .env.local && goclaw
```

API key của provider lưu trong bảng database `llm_providers` được mã hóa khi
lưu trữ bằng **AES-256-GCM** với `GOCLAW_ENCRYPTION_KEY`.

::: warning
`.env.local` dành cho shell của bạn. Nếu chạy gateway dưới systemd, lưu ý
systemd **không** parse các dòng `export` kiểu shell từ file tùy ý — hãy
truyền biến qua directive `Environment=` hoặc một `EnvironmentFile` không có
tiền tố `export`. Đây là lý do phổ biến khiến một thiết lập qua biến env âm
thầm không có hiệu lực. Xem [Tự host](../self-hosting).
:::

## Hot reload

Gateway theo dõi file config (fsnotify) và áp dụng thay đổi mà không cần
khởi động lại. Một số thiết lập tạo tài nguyên tồn tại lâu dài (ví dụ binary
của kênh) chỉ được nhận khi khởi động lại; tài liệu tham khảo config trong
`internal/config` ghi rõ những trường hợp này.

Dashboard chỉnh sửa cùng cấu hình đó qua các WebSocket RPC method:
`config.get`, `config.apply`, `config.patch`, `config.schema` và
`config.defaults`. Từ CLI:

```bash
goclaw config show       # effective config, secrets redacted
goclaw config path       # print the resolved config file path
goclaw config validate   # validate the config file
```

## Database

- **Standard (server):** PostgreSQL 18 với extension **pgvector**. Migration
  chạy trong lúc `goclaw onboard` / `make up`, hoặc thủ công bằng
  `goclaw migrate up`.
- **Desktop (Lite):** SQLite tại `~/.goclaw/data/` — không cần cấu hình gì.
  Secrets dùng keyring của hệ điều hành với phương án dự phòng là file tại
  `~/.goclaw/secrets/`. Xem [Desktop](../desktop).

## Agents

Giá trị mặc định của agent nằm dưới `agents.defaults` trong config (model,
temperature, max tokens, provider, mức reasoning), và `agents.list` định
nghĩa các predefined agent. Biến env `GOCLAW_MODEL` ghi đè
`agents.defaults.model`.

Mỗi agent mang cấu hình provider/model, tool, chế độ prompt và reasoning
riêng. Một số hành vi đáng chú ý:

- **`agents.reasoning_default`** (mặc định `auto`) — chỉ áp dụng cho agent
  *mới tạo*. Agent hiện có giữ nguyên giá trị đã lưu, nên việc nâng cấp không
  bao giờ làm thay đổi hành vi. Với `auto`, mức thinking hiệu lực được suy ra
  từ capability map của provider (ví dụ Anthropic → medium, model reasoning
  tương thích OpenAI → low, không rõ → off).
- **Subagent kế thừa** `max_tokens`, `temperature` và cấu hình reasoning hiệu
  lực của agent cha, trừ khi định nghĩa subagent ghi đè tường minh. Xem
  [Agents & Subagents](../features/agents).

## Luồng chạy lần đầu

Khi config đã sẵn sàng, mở web dashboard. Với database mới tinh, UI chuyển
tới wizard `/setup`, cấu hình lần lượt: một provider, một model, một agent,
và (tùy chọn) một kênh. Sau đó bạn đến trang dashboard. Thông tin đăng nhập
provider cũng có thể quản lý sau từ dashboard hoặc qua phần setup
[Kênh](../channels/telegram).

## Video worker

FFmpeg render worker độc lập là một binary riêng với các CLI flag riêng
(`--addr`, `--token`, `--work-dir`, ...). Nó được tài liệu hóa trong
[Hướng dẫn tự host](../self-hosting).

## Bước tiếp theo

- [Cài đặt](./install) — wizard onboarding và các lựa chọn triển khai
- [Hướng dẫn tự host](../self-hosting) — systemd unit, xử lý env, backup
- [Kiến trúc](../architecture) — config nằm ở đâu trong runtime
