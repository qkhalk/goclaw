# Cài đặt

GoClaw được phân phối dưới dạng một binary Go tĩnh duy nhất (~25 MB), các
Docker image, và một ứng dụng desktop riêng. Trang này nói về các bản server;
ứng dụng desktop được mô tả trong [Desktop](../desktop).

## Yêu cầu

| Thành phần | Yêu cầu |
|------------|---------|
| Database | PostgreSQL 18 với extension **pgvector** (schema gốc cũng dùng pgcrypto) |
| Go | 1.26+ — chỉ cần khi build từ mã nguồn |
| Runtime | Không cần. Binary là bản tĩnh và đã nhúng sẵn web dashboard |

## Cài đặt một dòng lệnh

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

## Onboarding

Sau khi cài xong, chạy wizard onboarding:

```bash
goclaw onboard
```

Wizard đi qua bảy bước:

1. Hỏi DSN của PostgreSQL.
2. Kiểm tra kết nối database.
3. Tạo gateway token (`GOCLAW_GATEWAY_TOKEN`) và khóa mã hóa
   (`GOCLAW_ENCRYPTION_KEY`).
4. Chạy database migration.
5. Khởi tạo các provider giữ chỗ (placeholder).
6. Ghi file `config.json` (không chứa secrets).
7. Ghi file `.env.local` chứa các secrets đã tạo.

Sau đó khởi động gateway:

```bash
source .env.local && goclaw
```

Web dashboard được nhúng sẵn trong binary và phục vụ tại
`http://localhost:18790`.

## Các lệnh CLI hữu ích

| Lệnh | Mục đích |
|------|----------|
| `goclaw setup` | TUI wizard (provider, agent, kênh) để cấu hình sau cài đặt |
| `goclaw doctor` | Health check: môi trường hệ thống và cấu hình |
| `goclaw migrate up` | Áp dụng các migration PostgreSQL còn pending |
| `goclaw upgrade` | Áp dụng migration schema + dữ liệu (có flag `--dry-run`, `--status`) |
| `goclaw version` | In phiên bản binary và phiên bản giao thức wire |
| `goclaw config show` | In cấu hình đang hiệu lực với secrets được che |

## Build từ mã nguồn

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw
make build
./goclaw onboard
source .env.local && ./goclaw
```

Build từ mã nguồn cần Go 1.26+. Web UI được nhúng vào binary lúc build —
server không cần Node.js runtime.

## Docker Compose

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw

# Generate .env with auto-generated secrets
./prepare-env.sh

# Start the stack (creates the network, builds, starts, runs migrations)
make up

# Health check
curl http://localhost:18790/health
```

Các lệnh thường dùng:

| Lệnh | Tác dụng |
|------|----------|
| `make up` | Pull/khởi động toàn bộ service và chạy migration |
| `make down` | Dừng toàn bộ service |
| `make logs` | Xem log gateway liên tục |
| `make reset` | Xóa volumes và dựng lại từ đầu |

### Service tùy chọn

Bật bằng các flag `WITH_*` — có thể kết hợp tự do:

```bash
make up WITH_BROWSER=1 WITH_OTEL=1
```

| Flag | Service | Tác dụng |
|------|---------|----------|
| `WITH_BROWSER=1` | Headless Chrome | Bật tool `browser` để scraping, chụp ảnh màn hình, tự động hóa |
| `WITH_OTEL=1` | Jaeger | Tracing UI OpenTelemetry cho các lời gọi LLM và độ trễ |
| `WITH_SANDBOX=1` | Docker sandbox | Container cô lập để chạy code agent không tin cậy |
| `WITH_TAILSCALE=1` | Tailscale | Expose gateway qua mạng riêng Tailscale |
| `WITH_REDIS=1` | Redis | Tầng cache dựa trên Redis |

### Các biến thể Docker image

Image được publish lên GHCR (`ghcr.io/nextlevelbuilder/goclaw`) và mirror
trên Docker Hub (`digitop/goclaw`).

| Tag | Nội dung |
|-----|----------|
| `:latest`, `:vX.Y.Z` | Backend + web UI nhúng + Python |
| `:base`, `:vX.Y.Z-base` | Chỉ backend, không có web UI lẫn runtime |
| `:full`, `:vX.Y.Z-full` | Đầy đủ runtime + phụ thuộc skill cài sẵn |

## Bản desktop (GoClaw Lite)

Ứng dụng desktop native cho agent chạy cục bộ — không cần Docker, không cần
PostgreSQL. Nó nhúng chính gateway đó, biên dịch với SQLite, kèm sẵn web UI.
Bộ cài:

```bash
# macOS
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.ps1 | iex"
```

Xem [Desktop](../desktop) để biết giới hạn bản Lite, hành vi tự cập nhật và
hướng dẫn build.

## Cập nhật

- **Docker:** `make up` pull image mới nhất đã publish rồi khởi động lại;
  hoặc `docker compose pull && docker compose up -d`.
- **Binary:** tải bản release mới từ GitHub Releases (hoặc chạy lại script
  cài đặt) và thay thế binary. Không có lệnh tự cập nhật `goclaw update` —
  tự cập nhật binary chỉ có trên ứng dụng desktop. Sau khi thay binary, chạy
  `goclaw upgrade` để áp dụng migration schema/dữ liệu của database (đây là
  lệnh migration database, không phải tự cập nhật).
- **Desktop:** tự cập nhật ngay trong ứng dụng từ các bản `lite-v*` GitHub
  Releases — xem [Desktop](../desktop#auto-update).

### Artifact của bản release

Release được build bởi GitHub Actions kích hoạt theo tag. Binary được đóng
cho linux (amd64/arm64), macOS (amd64/arm64) và Windows (amd64), cùng các
biến thể Docker image liệt kê ở trên.

## Bước tiếp theo

- [Cấu hình](./configuration) — file config JSON5, env overlay, secrets
- [Hướng dẫn tự host](../self-hosting) — systemd, migration, backup
- [Kiến trúc](../architecture) — nền tảng vận hành như thế nào
- [Xử lý sự cố](../troubleshooting) — các lỗi khởi động và runtime thường gặp
