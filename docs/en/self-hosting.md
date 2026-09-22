# Self-Hosting Guide

Running GoClaw on your own server: a systemd gateway service, an optional
ffmpeg video worker sidecar, migrations discipline, reverse proxy and
backups. (For Docker Compose deployments, see
[Installation](/en/getting-started/install#docker-compose) — `make up`
handles most of this for you.)

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
goclaw migrate up
```

Schema version tracking is built in — `migrate up` is idempotent and applies
only what's missing.

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

```nginx
# Gateway
location / {
    proxy_pass http://127.0.0.1:18790;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;      # WebSocket
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

## Backup

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

## Updating

```bash
# 1. Ship new binary AND migrations/
goclaw migrate up
systemctl restart goclaw

# Built-in self-update (downloads, verifies SHA256, swaps, restarts)
goclaw update --apply
```
