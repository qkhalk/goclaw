# Cài đặt

GoClaw phân phối dưới ba hình thức: một binary Go tĩnh (~25 MB), image
Docker, và ứng dụng desktop (GoClaw Lite). Chọn hình thức phù hợp với bạn.

**Yêu cầu trước (bản server):** PostgreSQL 18 kèm pgvector. Phiên bản
[desktop](#phiên-bản-desktop-goclaw-lite) không cần gì thêm — SQLite đã nhúng
sẵn.

## Cài bằng một dòng lệnh

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

Sau khi cài, chạy wizard cấu hình tương tác — wizard hỏi khóa nhà cung cấp
LLM, chạy migration cơ sở dữ liệu và seed dữ liệu mặc định:

```bash
goclaw onboard
```

Rồi khởi động gateway:

```bash
source .env.local && goclaw
```

Bảng điều khiển web được nhúng ngay trong binary, phục vụ tại
`http://localhost:18790`.

::: tip Onboard không cần tương tác
Khi các biến môi trường `GOCLAW_*_API_KEY` đã được set, gateway tự onboard
không cần prompt — tự dò nhà cung cấp, chạy migration và seed mặc định. Rất
tiện cho server headless.
:::

## Build từ source

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw
make build
./goclaw onboard        # Wizard cấu hình tương tác
source .env.local && ./goclaw
```

Build từ source cần Go 1.26+. Web UI được nhúng vào binary — không cần
Node.js khi chạy.

## Docker Compose

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw

# Sinh file .env với mật khẩu tự động
chmod +x prepare-env.sh && ./prepare-env.sh

# Thêm ít nhất một GOCLAW_*_API_KEY vào .env, rồi:
make up

# Nếu Postgres không start ("port 5432 already allocated"), đổi cổng host
# trong .env, ví dụ POSTGRES_PORT=5433 (xem .env.example).

# Dashboard: http://localhost:18790
# Health check: curl http://localhost:18790/health
```

Các lệnh thường dùng:

| Lệnh | Chức năng |
|------|-----------|
| `make up` | Khởi động toàn bộ dịch vụ (build + migrate) |
| `make down` | Dừng toàn bộ dịch vụ |
| `make logs` | Xem log gateway |
| `make reset` | Xóa volume và build lại từ đầu |

### Dịch vụ tùy chọn

Bật bằng cờ `WITH_*` — kết hợp tự do:

```bash
make up WITH_BROWSER=1 WITH_OTEL=1
```

| Cờ | Dịch vụ | Chức năng |
|----|---------|-----------|
| `WITH_BROWSER=1` | Headless Chrome | Bật tool `browser` để cạo dữ liệu, chụp màn hình, tự động hóa |
| `WITH_OTEL=1` | Jaeger | Giao diện OpenTelemetry tracing cho LLM call và độ trễ |
| `WITH_SANDBOX=1` | Docker sandbox | Container cách ly chạy code không tin cậy từ agent |
| `WITH_TAILSCALE=1` | Tailscale | Mở gateway qua mạng riêng Tailscale |
| `WITH_REDIS=1` | Redis | Lớp cache dùng Redis |

### Các biến thể image Docker

| Image | Mô tả |
|-------|-------|
| `ghcr.io/qkhalk/goclaw:v4.5.0` | Backend + web UI nhúng + Python (**khuyên dùng**) |
| `ghcr.io/qkhalk/goclaw:v4.5.0-base` | Chỉ backend API, không web UI, không runtime |
| `ghcr.io/qkhalk/goclaw:v4.5.0-full` | Đủ runtime + phụ thuộc skill cài sẵn |
| `ghcr.io/qkhalk/goclaw:latest` | Bí danh của stable mới nhất |

## Phiên bản desktop (GoClaw Lite)

Ứng dụng desktop native cho AI agent cục bộ — không Docker, không
PostgreSQL, không hạ tầng. Một ứng dụng duy nhất (Wails v2 + React) ~30 MB,
SQLite không cần cấu hình, quản lý agent, cấu hình provider, MCP server,
skill, cron và bảng Kanban của team. Tự cập nhật từ GitHub Releases.

```bash
# macOS
curl -fsSL https://raw.githubusercontent.com/qkhalk/goclaw/dev/scripts/install-lite.sh | bash
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/qkhalk/goclaw/dev/scripts/install-lite.ps1 | iex
```

Giới hạn Lite: 5 agent, 1 team (5 thành viên), 50 phiên. Không có kênh nhắn
tin, knowledge graph, RBAC hay đa thuê — đó là tính năng bản Standard
(server).

| Tính năng | Lite (Desktop) | Standard (Server) |
|-----------|---------------|-------------------|
| Agent | Tối đa 5 | Không giới hạn |
| Cơ sở dữ liệu | SQLite (cục bộ) | PostgreSQL |
| Bộ nhớ | Tìm kiếm văn bản FTS5 | Ngữ nghĩa pgvector |
| Kênh nhắn tin | — | Telegram, Discord, Slack, Facebook, Zalo, Feishu/Lark, WhatsApp, Bitrix24, Pancake |
| Tự cập nhật | GitHub Releases | Docker / binary |

## Cập nhật

```bash
# Docker
docker compose pull && docker compose up -d

# Binary (web UI nhúng kèm)
goclaw update --apply    # Tải về, kiểm tra SHA256, đổi binary, khởi động lại
```

Hoặc từ dashboard web: mở **About** → **Update Now** (chỉ admin).

## Đi tiếp đâu

- [Cấu hình (EN)](/en/getting-started/configuration) — file JSON5, env overlay, khóa bí mật
- [Hướng dẫn tự host (EN)](/en/self-hosting) — systemd, video worker sidecar, migration, backup
- [Kiến trúc (EN)](/en/architecture) — tổng quan nền tảng
- [Skill & Chợ Skill](/vi/features/skills) — hệ thống skill tiếng Việt
