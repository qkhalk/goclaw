# Provider & Model

GoClaw nói chuyện với các LLM provider thông qua provider adapter pluggable.
Agent gọi một model dưới dạng `provider/model`; provider registry phân giải
adapter, áp các chính sách reliability và stream phản hồi về.

## Provider adapter

| Adapter | Giao thức truyền | Ghi chú |
|---------|------------------|---------|
| `anthropic` | Native Anthropic HTTP + SSE | Chia system prompt thành các cache block (biên cache prompt) cho các lời gọi có cache rẻ hơn |
| `openai` | HTTP + SSE tương thích OpenAI | Dùng cho phần lớn gateway bên thứ ba |
| `dashscope` | DashScope (Alibaba Qwen) | Client native |
| `codex` | OpenAI Responses (SSE) | Dùng bởi routing OAuth ChatGPT/Codex |
| `claude-cli` | Tiến trình con Claude Code CLI | Cầu MCP ngược về gateway; hỗ trợ MCP server theo từng agent |
| `claude-oauth` | Anthropic qua nguồn OAuth token | Tài khoản gói đăng ký Claude Pro/Max |
| `copilot-oauth` | GitHub Copilot qua nguồn OAuth token | Tài khoản gói đăng ký Copilot |
| `codex-oauth` | OpenAI Responses qua nguồn OAuth token | Tài khoản gói ChatGPT/Codex với account pool |
| `ollama` | Client Ollama native | Cục bộ/tự host; phân giải `num_ctx` theo từng model |
| `vertex` | Vertex AI (endpoint tương thích OpenAI) | Service account OAuth2 (JSON inline hoặc file) hoặc Application Default Credentials |
| `acp` | JSON-RPC 2.0 qua stdio | Điều phối các agent CLI tương thích ACP: Claude Code, Codex, Gemini CLI |

## Preset config

Các preset provider được đăng ký lúc gateway khởi động từ file config
(`cmd/gateway_providers.go`) — mỗi preset kích hoạt khi có API key (hoặc
host, với Ollama) của nó:

`openai`, `anthropic`, `atlascloud`, `openrouter`, `groq`, `deepseek`,
`gemini`, `mistral`, `xai`, `minimax`, `cohere`, `perplexity`, `dashscope`,
`bailian`, `zai`, `zai-coding`, `ollama`, `ollama-cloud`, `novita`,
`byteplus`, `byteplus-coding`, `vertex`

Các preset tương thích OpenAI khác (moonshot, together, fireworks, cerebras,
synthetic, kilocode, opencode, nvidia, stepfun, venice, baseten, chutes,
huggingface) theo cùng pattern.

## Provider registry trong database

Provider cấu hình qua UI/CLI được lưu trong bảng `llm_providers`. API key
được mã hóa khi lưu trữ bằng AES-256-GCM (`internal/crypto`). Registry được
scope theo tenant — mỗi tenant có thể đăng ký provider riêng — với fallback
về các đăng ký của master tenant. Provider từ DB được đăng ký sau provider
từ config và có độ ưu tiên cao hơn.

## Routing gói đăng ký OAuth

Các tài khoản gói đăng ký (ChatGPT/Codex, Claude, GitHub Copilot) được lưu
dưới dạng OAuth provider, do gateway tự refresh token. Các tài khoản tạo
thành pool được chia sẻ round-robin giữa các tenant theo từng modality, nên
nhiều tenant có thể dùng chung một gói đăng ký mà không xung đột token.

- HTTP: `/v1/auth/chatgpt/*`, `/v1/auth/openai/*`, `/v1/auth/claude/*`,
  `/v1/auth/copilot/*` (start, callback, status, quota, logout)
- CLI: `goclaw auth status [provider]` / `goclaw auth logout [provider]` —
  việc xác thực được hoàn tất qua web UI (trang Providers)

## Model

- **Model registry với resolver tương thích tiến** — tên model lạ được hạ về
  default hợp lý thay vì fail cứng, nên model upstream mới chạy được trước
  cả khi GoClaw biết đến nó
- **Phân giải năng lực reasoning** — mức reasoning thích ứng theo từng agent
  dựa trên năng lực của model (`internal/providerresolve`)
- **Chuỗi fallback model** — khi model chính lỗi, agent thử lại theo một
  chuỗi fallback đã cấu hình
- **Embeddings** — các adapter OpenAI và Voyage hỗ trợ các tính năng memory
  và tri thức

## Reliability

Mọi lời gọi provider đi qua `internal/reliability` +
`internal/providers/retry.go`:

- **`RetryDo`** — mặc định 3 lần thử với exponential backoff từ 300 ms tới
  trần 30 s, kèm jitter; các trạng thái được retry gồm 429, 5xx và lỗi edge
  của Cloudflare
- **Circuit breaker** — theo dõi theo từng cặp `provider:model`; mạch mở sẽ
  fail nhanh cho tới khi một probe thành công
- **Điều phối rate-limit** — một cái nhìn chung về các cooldown 429 đang
  kích hoạt theo từng provider:model, để các lượt chạy đồng thời không đốt
  tiếp quota vào một cửa sổ đã biết là đóng
- **Health registry** — trạng thái sức khỏe theo từng provider được đẩy lên
  dashboard

## Quản lý provider

| Bề mặt | Đường dẫn | Quyền truy cập |
|--------|-----------|----------------|
| Web UI | `/providers` | Chỉ admin |
| CLI | `goclaw providers list` / `add` / `update` / `delete` / `verify` | Cần gateway đang chạy |
| HTTP | `/v1/providers` (+ `/{id}/verify`, `/{id}/models`) | Gateway token hoặc API key |
| WebSocket | các method providers trong nhóm config/agents | Kiểm tra vai trò theo từng method |

`goclaw providers verify <id>` ping provider (hoặc một model cụ thể) và báo
tình trạng kết nối mà không đụng đến cấu hình agent.
