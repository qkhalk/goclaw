# Hướng dẫn tự host

Chạy GoClaw trên server của riêng bạn: các biến thể Docker Compose, một
service gateway systemd, một sidecar video worker ffmpeg tùy chọn, kỷ luật
migration, reverse proxy, observability và backup. (Để cài Docker Compose lần
đầu, xem
[Cài đặt](/vi/getting-started/install#docker-compose) — `make up` xử lý phần
lớn việc này giúp bạn.)

## Docker Compose

Repo đi kèm một `docker-compose.yml` nền cộng với các file overlay cho
sidecar và hạ tầng tùy chọn:

| Overlay | Mục đích |
|---------|---------|
| `docker-compose.postgres.yml` | Cơ sở dữ liệu PostgreSQL (+ pgvector) |
| `docker-compose.redis.yml` | Redis |
| `docker-compose.sandbox.yml` | Sandbox thực thi mã |
| `docker-compose.browser.yml` | Sidecar tự động hóa browser |
| `docker-compose.claude-cli.yml` | Sidecar provider Claude CLI |
| `docker-compose.cloudflared.yml` | Mở ra qua Cloudflare Tunnel |
| `docker-compose.lightpanda.yml` | Backend browser Lightpanda |
| `docker-compose.otel.yml` | OpenTelemetry collector (`ENABLE_OTEL: "true"`) |
| `docker-compose.tailscale.yml` | Mở ra qua mạng Tailscale |
| `docker-compose.upgrade.yml` | Job nâng cấp one-shot |
| `docker-compose.selfservice.yml` | Portal onboarding tự phục vụ |
| `docker-compose.forkdev.yml` | Phát triển fork cục bộ |

`prepare-compose.sh` lắp danh sách `COMPOSE_FILE` đang hoạt động từ các mảnh
`compose.d/*.yml` và ghi vào `.env`; `prepare-env.sh` chuẩn bị file môi
trường; `docker-entrypoint.sh` xử lý khởi động.

### Images

Các image công khai được phát hành lên GHCR
(`ghcr.io/nextlevelbuilder/goclaw`) và Docker Hub (`digitop/goclaw`) với bốn
biến thể:

| Biến thể | Tag | Nội dung |
|---------|------|----------|
| latest | `:latest`, `:vX.Y.Z` | Backend + web UI + Python |
| base | `:base`, `:vX.Y.Z-base` | Chỉ backend |
| full | `:full`, `:vX.Y.Z-full` | Mọi runtime, skill cài sẵn |
| web | `-web:latest` | Web UI độc lập (Nginx) |

## Gateway dưới systemd

```ini
# /etc/systemd/system/goclaw.service
[Unit]
Description=GoClaw Gateway
After=network.target postgresql.service

[Service]
User=goclaw
ExecStart=/usr/local/bin/goclaw
Environment=GOCLAW_CONFIG=/opt/goclaw/config.json
EnvironmentFile=/opt/goclaw/goclaw.env
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
useradd -r -s /bin/false goclaw
mkdir -p /opt/goclaw
# Place the binary, config.json and env file, then:
systemctl daemon-reload
systemctl enable --now goclaw
curl http://localhost:18790/health
```

::: warning EnvironmentFile không có `export`
Các dòng `EnvironmentFile` của systemd phải là `KEY=value` thuần — file chứa
các dòng kiểu shell `export KEY=...` (như `.env.local`) **không** được phân
tích và các biến âm thầm không được áp dụng. Điều này từng gây sự cố cho một
triển khai production: cài đặt `GOCLAW_AUTO_UPGRADE` đặt qua file env có tiền
tố `export` không bao giờ tới được gateway. Hãy bỏ tiền tố `export` hoặc viết
rõ các biến quan trọng bằng chỉ thị `Environment=`.
:::

## Migrations (đọc trước khi triển khai thủ công)

Gateway đọc các file SQL migration **từ đĩa** — trên bố cục production, là
`/opt/goclaw/migrations`. Hai hệ quả:

1. **Triển khai binary thủ công phải kèm các file migration.** Khi bạn nâng
   cấp bằng cách chép binary mới, cũng hãy chép thư mục `migrations/` từ bản
   phát hành (tarball phát hành tiêu chuẩn có kèm nó). Binary có phiên bản
   schema yêu cầu cao hơn phiên bản đã áp dụng sẽ từ chối chạy thay vì chạy
   nửa vời migration.
2. **Chạy migration tường minh** khi user dịch vụ không thể tự migrate:

```bash
goclaw migrate up         # golang-migrate SQL migrations, idempotent
goclaw upgrade --status   # show schema/data upgrade state
goclaw upgrade --dry-run  # preview data migrations without applying
```

Theo dõi phiên bản schema được tích hợp sẵn — `migrate up` idempotent và chỉ
áp dụng phần còn thiếu. `GOCLAW_AUTO_UPGRADE=true` khiến gateway áp dụng các
migration schema/data đang chờ ngay khi khởi động.

## Video worker sidecar

Render video phía server chạy trên `videoworker`, một ffmpeg worker độc lập.
Nó là một **process riêng** với gateway, lắng nghe trên `127.0.0.1:18791`.

### Cài đặt

```bash
# ffmpeg + fonts (for captions)
apt update && apt install -y ffmpeg fonts-noto-core

# Build the worker from the goclaw repo
go build -o /usr/local/bin/videoworker ./cmd/videoworker/

# Service user and directories
useradd -r -s /bin/false goclaw
mkdir -p /var/lib/goclaw/video-tmp /var/www/goclaw/videos
chown goclaw:goclaw /var/lib/goclaw/video-tmp /var/www/goclaw/videos

# Systemd unit (deploy/videoworker.service in the repo)
cp deploy/videoworker.service /etc/systemd/system/
# Edit the unit to set your token:
#   Environment=GOCLAW_VIDEO_TOKEN=your-secret-token
systemctl daemon-reload
systemctl enable --now goclaw-videoworker
```

### Kiểm tra

```bash
systemctl status goclaw-videoworker
journalctl -u goclaw-videoworker -f
curl http://127.0.0.1:18791/health
```

### Các cờ cấu hình

| Cờ | Mặc định | Mô tả |
|------|---------|-------------|
| `--addr` | `127.0.0.1:18791` | Địa chỉ lắng nghe HTTP |
| `--token` | (trống) | Bearer auth token (trống = không xác thực) |
| `--work-dir` | `/tmp/videoworker` | Thư mục file tạm |
| `--output-dir` | `/var/www/videos` | Đầu ra video đã render |
| `--ffmpeg-path` | `ffmpeg` | Đường dẫn tới binary ffmpeg |
| `--font-file` | (trống) | Font cho phụ đề (bỏ qua phụ đề nếu trống) |
| `--max-scene-sec` | `30` | Số giây tối đa mỗi cảnh |
| `--max-queue` | `5` | Số job chờ tối đa (trả 409 khi đầy) |
| `--narr-voice` | `vi-VN-HoaiMyNeural` | Giọng Edge TTS mặc định |
| `--ttl-minutes` | `120` | TTL dọn thư mục tạm mồ côi |

Lời thoại yêu cầu `pip install edge-tts`. Unit ép `MemoryMax=600MB` (mức dùng
đỉnh ~200–350 MB mỗi lần render), `CPUQuota=100%` (một lõi, cố ý
`ffmpeg -threads 1`), và khởi động lại khi lỗi với backoff.

### Ghi chú vận hành

- Trạng thái worker nằm trong bộ nhớ: một lần crash làm mất các job đang
  chạy. Gateway phát hiện job mất qua poll timeout và đánh dấu chúng `failed`.
- Thư mục tạm mồ côi được dọn khi khởi động và mỗi 15 phút; dọn thủ công:
  `rm -rf /var/lib/goclaw/video-tmp/vwjob-*`.

## Reverse proxy (nginx)

Gateway phục vụ WebSocket và HTTP trên một cổng duy nhất (`18790`), nên một
upstream che phủ cả hai:

```nginx
# Gateway
location / {
    proxy_pass http://127.0.0.1:18790;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;      # WebSocket (/ws)
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
}

# Video worker — only if you expose it beyond localhost
location /v1/jobs {
    proxy_pass http://127.0.0.1:18791;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 600s;  # long render jobs
}
```

::: warning Webhook cần GOCLAW_ENCRYPTION_KEY
Gateway chỉ mount `/v1/webhooks/*` khi `GOCLAW_ENCRYPTION_KEY` được đặt —
không có nó, các endpoint đó trả về 404.
:::

**Tùy chọn Tailscale:** build với `-tags tsnet` và gateway tham gia tailnet
của bạn trực tiếp (`cmd/gateway_tsnet.go`) — không cần reverse proxy hay cổng
mở. Overlay `docker-compose.tailscale.yml` làm tương tự cho các triển khai
Compose.

## Backup và khôi phục

Trạng thái quan trọng là PostgreSQL (agent, session, bộ nhớ, provider key)
cộng với các file config/env:

```bash
# Nightly database dump
pg_dump -Fc goclaw > /var/backups/goclaw-$(date +%F).dump

# Config + secrets (keep offline)
tar czf /var/backups/goclaw-config-$(date +%F).tgz /opt/goclaw/config.json /opt/goclaw/goclaw.env
```

Khôi phục bằng `pg_restore -d goclaw --clean --if-exists <file>.dump`. Hãy
kiểm tra đường khôi phục ít nhất một lần — backup chưa từng thử khôi phục
không phải là backup.

GoClaw còn có các kho lưu trữ tích hợp sẵn:

```bash
goclaw backup <archive-path>          # full system backup (DB + filesystem)
goclaw restore <archive-path>         # restore a system archive

goclaw tenant-backup                  # tenant-scoped backup (DB rows + filesystem)
goclaw tenant-restore <archive-path>  # tenant-scoped restore
```

HTTP API (admin) phản chiếu điều này: `POST /v1/system/backup` và
`POST /v1/system/restore`, `GET /v1/system/backup/preflight`, tải xuống qua
`/v1/system/backup/download/{token}`, S3 dưới `/v1/system/backup/s3/*`
(config, upload, liệt kê), tương đương tenant dưới `/v1/tenant/backup*`.

**Backup định lịch:** cấu hình qua WebSocket (`backup.schedule.get`,
`backup.schedule.set`, `backup.schedule.run`) — chạy theo lịch với retention,
tùy chọn upload lên S3.

## Observability

- **Health:** `GET /health` cho liveness probe; số liệu host/system nuôi trang
  System của dashboard (`GET /v1/system/stats`)
- **Prometheus:** build với `-tags prometheus`, bật
  `telemetry.prometheus_enabled`, đặt `telemetry.prometheus_port` →
  `/metrics` (`cmd/gateway_prometheus.go`)
- **OpenTelemetry:** export OTLP tùy chọn — chạy với overlay
  `docker-compose.otel.yml` hoặc build với `ENABLE_OTEL=true`
  (`internal/tracing`)

## Cập nhật

```bash
# 1. Replace the binary (download the new release or re-run the install script)
# 2. Apply schema/data migrations, then restart
goclaw upgrade          # or: goclaw migrate up
systemctl restart goclaw
```

Không có lệnh tự cập nhật `goclaw update` trên bản server — hãy thay binary
thủ công. Chỉ ứng dụng desktop tự cập nhật bản thân (xem
[Desktop](./desktop#auto-update)). Gateway đang chạy có bộc lộ các HTTP
endpoint admin để kiểm tra và cài bản phát hành (`GET /v1/system/update`,
`POST /v1/system/update/install`).
