# Self-Hosting Guide

Running GoClaw on your own server: Docker Compose variants, a systemd
gateway service, an optional ffmpeg video worker sidecar, migrations
discipline, reverse proxy, observability and backups. (For a first Docker
Compose install, see
[Installation](/en/getting-started/install#docker-compose) — `make up`
handles most of this for you.)

## Docker Compose

The repo ships a base `docker-compose.yml` plus overlay files for sidecars
and optional infrastructure:

| Overlay | Purpose |
|---------|---------|
| `docker-compose.postgres.yml` | PostgreSQL (+ pgvector) database |
| `docker-compose.redis.yml` | Redis |
| `docker-compose.sandbox.yml` | Code-execution sandbox |
| `docker-compose.browser.yml` | Browser automation sidecar |
| `docker-compose.claude-cli.yml` | Claude CLI provider sidecar |
| `docker-compose.cloudflared.yml` | Cloudflare Tunnel exposure |
| `docker-compose.lightpanda.yml` | Lightpanda browser backend |
| `docker-compose.otel.yml` | OpenTelemetry collector (`ENABLE_OTEL: "true"`) |
| `docker-compose.tailscale.yml` | Tailscale network exposure |
| `docker-compose.upgrade.yml` | One-shot upgrade job |
| `docker-compose.selfservice.yml` | Self-service portal onboarding |
| `docker-compose.forkdev.yml` | Local fork development |

`prepare-compose.sh` assembles the active `COMPOSE_FILE` list from
`compose.d/*.yml` fragments and writes it to `.env`; `prepare-env.sh`
prepares the environment file; `docker-entrypoint.sh` handles startup.

### Images

Public images are published to GHCR (`ghcr.io/nextlevelbuilder/goclaw`) and
Docker Hub (`digitop/goclaw`) in four variants:

| Variant | Tags | Contents |
|---------|------|----------|
| latest | `:latest`, `:vX.Y.Z` | Backend + web UI + Python |
| base | `:base`, `:vX.Y.Z-base` | Backend only |
| full | `:full`, `:vX.Y.Z-full` | All runtimes, skills pre-installed |
| web | `-web:latest` | Standalone web UI (Nginx) |

## The gateway under systemd

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

::: warning EnvironmentFile has no `export`
systemd `EnvironmentFile` lines must be plain `KEY=value` — a file with
shell-style `export KEY=...` lines (like `.env.local`) is **not** parsed and
the variables silently don't apply. This bit one production deploy: an
`GOCLAW_AUTO_UPGRADE` setting set via an `export`-prefixed env file never
reached the gateway. Either strip the `export` prefixes or spell critical
variables out as `Environment=` directives.
:::

## Migrations (read this before manual deploys)

The gateway reads migration SQL files **from disk** — on a production layout,
`/opt/goclaw/migrations`. Two consequences:

1. **Manual binary deploys must ship the migration files.** When you upgrade
   by copying a new binary, also copy the `migrations/` directory from the
   release (the standard release tarball includes it). A binary whose
   required schema version is higher than the applied one will refuse to run
   half-migrated.
2. **Run migrations explicitly** when the service user can't auto-migrate:

```bash
goclaw migrate up         # golang-migrate SQL migrations, idempotent
goclaw upgrade --status   # show schema/data upgrade state
goclaw upgrade --dry-run  # preview data migrations without applying
```

Schema version tracking is built in — `migrate up` is idempotent and applies
only what's missing. `GOCLAW_AUTO_UPGRADE=true` makes the gateway apply
pending schema/data migrations inline at startup.

## Video worker sidecar

Server-side video rendering runs on `videoworker`, a standalone ffmpeg
worker. It is a **separate process** from the gateway, listening on
`127.0.0.1:18791`.

### Install

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

### Verify

```bash
systemctl status goclaw-videoworker
journalctl -u goclaw-videoworker -f
curl http://127.0.0.1:18791/health
```

### Configuration flags

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `127.0.0.1:18791` | HTTP listen address |
| `--token` | (empty) | Bearer auth token (empty = no auth) |
| `--work-dir` | `/tmp/videoworker` | Temp files directory |
| `--output-dir` | `/var/www/videos` | Rendered video output |
| `--ffmpeg-path` | `ffmpeg` | Path to ffmpeg binary |
| `--font-file` | (empty) | Font for captions (captions skipped if empty) |
| `--max-scene-sec` | `30` | Max seconds per scene |
| `--max-queue` | `5` | Max queued jobs (409 when full) |
| `--narr-voice` | `vi-VN-HoaiMyNeural` | Default Edge TTS voice |
| `--ttl-minutes` | `120` | Orphan temp dir cleanup TTL |

Narration requires `pip install edge-tts`. The unit enforces `MemoryMax=600MB`
(peak usage is ~200–350 MB per render), `CPUQuota=100%` (single core, by
design `ffmpeg -threads 1`), and restarts on failure with backoff.

### Operational notes

- Worker state is in-memory: a crash loses running jobs. The gateway detects
  lost jobs via poll timeout and marks them `failed`.
- Orphaned temp dirs are cleaned at startup and every 15 minutes; manual
  cleanup: `rm -rf /var/lib/goclaw/video-tmp/vwjob-*`.

## Reverse proxy (nginx)

The gateway serves WebSocket and HTTP on a single port (`18790`), so one
upstream covers both:

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

::: warning Webhooks need GOCLAW_ENCRYPTION_KEY
The gateway mounts `/v1/webhooks/*` only when `GOCLAW_ENCRYPTION_KEY` is
set — without it those endpoints return 404.
:::

**Tailscale option:** build with `-tags tsnet` and the gateway joins your
tailnet directly (`cmd/gateway_tsnet.go`) — no reverse proxy or open ports
needed. The `docker-compose.tailscale.yml` overlay does the same for Compose
deploys.

## Backup and restore

The state that matters is PostgreSQL (agents, sessions, memory, provider
keys) plus the config/env files:

```bash
# Nightly database dump
pg_dump -Fc goclaw > /var/backups/goclaw-$(date +%F).dump

# Config + secrets (keep offline)
tar czf /var/backups/goclaw-config-$(date +%F).tgz /opt/goclaw/config.json /opt/goclaw/goclaw.env
```

Restore with `pg_restore -d goclaw --clean --if-exists <file>.dump`. Test the
restore path at least once — an untested backup is not a backup.

GoClaw also has built-in archives:

```bash
goclaw backup <archive-path>          # full system backup (DB + filesystem)
goclaw restore <archive-path>         # restore a system archive

goclaw tenant-backup                  # tenant-scoped backup (DB rows + filesystem)
goclaw tenant-restore <archive-path>  # tenant-scoped restore
```

The HTTP API (admin) mirrors this: `POST /v1/system/backup` and
`POST /v1/system/restore`, `GET /v1/system/backup/preflight`, download via
`/v1/system/backup/download/{token}`, S3 under `/v1/system/backup/s3/*`
(config, upload, list), tenant equivalents under `/v1/tenant/backup*`.

**Scheduled backups:** configure via WebSocket (`backup.schedule.get`,
`backup.schedule.set`, `backup.schedule.run`) — scheduled runs with
retention, optionally uploading to S3.

## Observability

- **Health:** `GET /health` for liveness probes; host/system stats power the
  dashboard System page (`GET /v1/system/stats`)
- **Prometheus:** build with `-tags prometheus`, enable
  `telemetry.prometheus_enabled`, set `telemetry.prometheus_port` → `/metrics`
  (`cmd/gateway_prometheus.go`)
- **OpenTelemetry:** optional OTLP export — run with the
  `docker-compose.otel.yml` overlay or build with `ENABLE_OTEL=true`
  (`internal/tracing`)

## Updating

```bash
# 1. Replace the binary (download the new release or re-run the install script)
# 2. Apply schema/data migrations, then restart
goclaw upgrade          # or: goclaw migrate up
systemctl restart goclaw
```

There is no `goclaw update` self-update command on the server edition —
replace the binary manually. Only the desktop app updates itself (see
[Desktop](./desktop#auto-update)). The running gateway does expose admin
HTTP endpoints to check and install releases (`GET /v1/system/update`,
`POST /v1/system/update/install`).
