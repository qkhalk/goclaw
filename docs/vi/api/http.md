# HTTP API

GoClaw để lộ một HTTP API dưới `/v1` bên cạnh WebSocket RPC. Mọi endpoint nằm
trên cổng gateway (mặc định `18790`).

Spec OpenAPI được phục vụ tại `/v1/openapi.json` và Swagger UI tương tác tại
`/docs`.

## Xác thực

Bearer token trong header `Authorization` — hoặc là gateway/operator token
hoặc là API key có phạm vi:

```bash
curl http://localhost:18790/v1/agents \
  -H "Authorization: Bearer $GOCLAW_GATEWAY_TOKEN"
```

API key có tiền tố `goclaw_`, được lưu dưới dạng hash SHA-256, và mang các
quyền hạn có phạm vi (mỗi key được xử lý thành một vai trò suy ra từ các
scope của nó). Quản lý chúng qua các endpoint `/v1/api-keys` (list, create,
revoke) hoặc web UI.

Các endpoint nhạy cảm ngôn ngữ tôn trọng header `Accept-Language` (en, vi,
zh).

## Các endpoint chính

| Method | Endpoint | Mục đích |
|--------|----------|---------|
| `POST` | `/v1/chat/completions` | Chat completions tương thích OpenAI với bất kỳ agent/model nào đã cấu hình |
| `POST` | `/v1/responses` | Endpoint tương thích OpenAI Responses |
| `POST` | `/v1/tools/invoke` | Gọi một tool trực tiếp |
| `GET` | `/v1/agents` | Liệt kê agent (kèm loại, provider, model) |
| `GET` | `/v1/skills` | Liệt kê skill đã cài |
| `GET` | `/health` | Thăm dò liveness/health |
| `GET` | `/v1/edition` | Thông tin edition (công khai, không cần xác thực) |

## Các nhóm endpoint REST

| Nhóm | Endpoint | Mục đích |
|--------|-----------|---------|
| Agent | `/v1/agents/*` | CRUD agent, instance và file theo user, bộ nhớ episodic, đồ thị tri thức (`kg/*`), vault (`vault/*`), metrics/đề xuất tiến hóa, cờ tính năng v3, export/import |
| Kênh | `/v1/channels/instances/*` | CRUD instance kênh, allowlist writer (`writers`, `writers/groups`, `writers/test`), phân giải nhóm/thành viên, xem xét trích xuất bộ nhớ |
| Skills | `/v1/skills/*` | CRUD skill, grant, dependency, tiến hóa, Chợ skill (`market/*`), upload/import/export |
| Knowledge Vault | `/v1/vault/*` | Tài liệu, wikilinks, cây, đồ thị, tìm kiếm, trạng thái enrichment |
| MCP | `/v1/mcp/*` | Sổ đăng ký server, grant, install job, OAuth, import/export, phê duyệt request |
| Bộ nhớ | `/v1/memory/documents` | Sổ đăng ký tài liệu toàn cục; theo agent dưới `/v1/agents/{id}/memory/*` |
| Tools | `/v1/tools/builtin`, `/v1/tools/builtin/{name}` | Sổ đăng ký tool builtin và cấu hình tenant theo tool |
| Webhook | `/v1/webhooks/*` | CRUD webhook, lịch sử gọi, cộng với runtime `POST /v1/webhooks/message` và `POST /v1/webhooks/llm` |
| API key | `/v1/api-keys` | Tạo, liệt kê, thu hồi key có phạm vi |
| Tenant & RBAC | `/v1/tenants*` | CRUD tenant, user, chính sách, vai trò và quyền |
| Usage & chi phí | `/v1/usage/*`, `/v1/costs/summary` | Timeseries, phân rã, tổng hợp, thống kê định tuyến |
| System | `/v1/system/stats`, `/v1/logs/runtime/aggregate`, `/v1/activity` | Metrics host, log runtime tổng hợp, dòng hoạt động (activity feed) |
| TTS | `/v1/tts/*` | Cấu hình TTS, capability, nhân bản giọng |
| Cloud | `/v1/cloud/*` | Tài khoản lưu trữ cloud, file, cặp đồng bộ, truyền dữ liệu |
| Packages | `/v1/packages/*` | Cài/cập nhật/gỡ runtime/gói |
| Tin nhắn chờ | `/v1/pending-messages` | Kiểm tra và nén tin nhắn kênh trong hàng đợi |
| CLI credentials | `/v1/cli-credentials/*` | Vault credential CLI dùng chung với grant theo agent/user |

Ghi vào Chợ skill **chỉ dành cho admin**; đọc khả dụng cho user đã đăng nhập.

## Chat completions

`POST /v1/chat/completions` tuân theo cấu trúc request/response của OpenAI,
nên client và SDK hiện có chạy không cần sửa — trỏ `base_url` vào gateway của
bạn và dùng gateway token làm API key. Request được định tuyến tới agent và
model bạn chỉ định, hỗ trợ streaming (SSE).

## Webhook API

Kích hoạt agent hoặc gửi tin nhắn kênh từ hệ thống bên ngoài **mà không cần
gateway token**. Tạo một webhook trong dashboard, rồi gọi nó theo một trong
hai lược đồ:

**Xác thực Bearer — gọi LLM đồng bộ:**

```bash
curl -X POST https://example.com/v1/webhooks/llm \
  -H "Authorization: Bearer wh_..." \
  -H "Content-Type: application/json" \
  -d '{"input":"Summarize today metrics","mode":"sync"}'
```

**Xác thực HMAC — ký body bằng `hmac_signing_key` từ response tạo webhook:**

```bash
TS=$(date +%s); BODY='{"input":"hi","mode":"sync"}'
SIG=$(echo -n "${TS}.${BODY}" | openssl dgst -sha256 -mac HMAC \
      -macopt "hexkey:${WEBHOOK_HMAC_KEY}" | awk '{print $2}')
curl -X POST https://example.com/v1/webhooks/llm \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Id: ${WEBHOOK_ID}" \
  -H "X-GoClaw-Signature: t=${TS},v1=${SIG}" \
  -d "$BODY"
```

Chế độ async (`"mode":"async"`) trả về ngay và gọi lại có retry — xem tài
liệu webhooks đầy đủ (`docs/webhooks.md` trong repo GoClaw) để biết lịch
retry và ma trận kênh.

::: warning Webhook yêu cầu GOCLAW_ENCRYPTION_KEY
Gateway từ chối mount `/v1/webhooks/*` khi `GOCLAW_ENCRYPTION_KEY` chưa được
đặt — các endpoint trả về 404. Hãy đặt env var này để bật subsystem webhook.
:::

`POST /v1/webhooks/message` dùng cùng các lược đồ xác thực cho một lần gửi
kênh đồng bộ (văn bản cộng media tùy chọn).

## WebSocket API

Dashboard và các client giàu tính năng dùng WebSocket API thay thế — xem
[WebSocket RPC](./websocket). Giao thức tóm tắt:

- Frame có kiểu `req` / `res` / `event`
- **Request đầu tiên trên một kết nối phải là `connect`** — nó xác thực và
  mang các tham số session (bao gồm `locale`)
- Frame `event` của server stream delta, event vòng đời LLM và cập nhật nhiệm
  vụ
