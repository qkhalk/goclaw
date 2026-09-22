# Installation

GoClaw ships as a single static Go binary (~25 MB), as Docker images, and as a
desktop app (GoClaw Lite). Pick the flavor that fits you.

**Prerequisites (server editions):** PostgreSQL 18 with pgvector. The
[desktop edition](#desktop-edition-goclaw-lite) needs nothing — it embeds SQLite.

## One-liner install

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

After installing, run the interactive setup wizard — it asks for your LLM
provider keys, runs database migrations and seeds default data:

```bash
goclaw onboard
```

Then start the gateway:

```bash
source .env.local && goclaw
```

The web dashboard is built into the binary and served at
`http://localhost:18790`.

::: tip Zero-prompt onboarding
When `GOCLAW_*_API_KEY` environment variables are set, the gateway
auto-onboards without interactive prompts — it detects the provider, runs
migrations and seeds defaults. Ideal for headless servers.
:::

## From source

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw
make build
./goclaw onboard        # Interactive setup wizard
source .env.local && ./goclaw
```

Building from source requires Go 1.26+. The web UI is embedded into the
binary — no Node.js runtime is needed at runtime.

## Docker Compose

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw

# Generate .env with auto-generated secrets
chmod +x prepare-env.sh && ./prepare-env.sh

# Add at least one GOCLAW_*_API_KEY to .env, then:
make up

# If Postgres fails to start ("port 5432 already allocated"), set another host
# port in .env, e.g. POSTGRES_PORT=5433 (see .env.example).

# Web dashboard: http://localhost:18790
# Health check:  curl http://localhost:18790/health
```

Common commands:

| Command | What it does |
|---------|--------------|
| `make up` | Start all services (build + migrate) |
| `make down` | Stop all services |
| `make logs` | Tail gateway logs |
| `make reset` | Wipe volumes and rebuild from scratch |

### Optional services

Enable with `WITH_*` flags — they combine freely:

```bash
make up WITH_BROWSER=1 WITH_OTEL=1
```

| Flag | Service | What it does |
|------|---------|--------------|
| `WITH_BROWSER=1` | Headless Chrome | Enables the `browser` tool for scraping, screenshots, automation |
| `WITH_OTEL=1` | Jaeger | OpenTelemetry tracing UI for LLM calls and latency |
| `WITH_SANDBOX=1` | Docker sandbox | Isolated container for running untrusted agent code |
| `WITH_TAILSCALE=1` | Tailscale | Expose the gateway over a Tailscale private network |
| `WITH_REDIS=1` | Redis | Redis-backed caching layer |

### Docker image variants

| Image | Description |
|-------|-------------|
| `ghcr.io/qkhalk/goclaw:v4.5.0` | Backend + embedded web UI + Python (**recommended**) |
| `ghcr.io/qkhalk/goclaw:v4.5.0-base` | Backend API only, no web UI, no runtimes |
| `ghcr.io/qkhalk/goclaw:v4.5.0-full` | All runtimes + skill dependencies pre-installed |
| `ghcr.io/qkhalk/goclaw:latest` | Alias for the latest stable tag |

## Desktop Edition (GoClaw Lite)

A native desktop app for local AI agents — no Docker, no PostgreSQL, no
infrastructure. Single app (Wails v2 + React), ~30 MB, SQLite database with
zero setup, agent management, provider config, MCP servers, skills, cron and a
team Kanban board. Auto-updates from GitHub Releases.

```bash
# macOS
curl -fsSL https://raw.githubusercontent.com/qkhalk/goclaw/dev/scripts/install-lite.sh | bash
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/qkhalk/goclaw/dev/scripts/install-lite.ps1 | iex
```

Lite limits: 5 agents, 1 team (5 members), 50 sessions. No channels,
knowledge graph, RBAC or multi-tenancy — those are Standard (server) features.

| Feature | Lite (Desktop) | Standard (Server) |
|---------|---------------|-------------------|
| Agents | Max 5 | Unlimited |
| Database | SQLite (local) | PostgreSQL |
| Memory | FTS5 text search | pgvector semantic |
| Channels | — | Telegram, Discord, Slack, Facebook, Zalo, Feishu/Lark, WhatsApp, Bitrix24, Pancake |
| Auto-update | GitHub Releases | Docker / binary |

## Updating

```bash
# Docker
docker compose pull && docker compose up -d

# Binary (with embedded web UI)
goclaw update --apply    # Downloads, verifies SHA256, swaps binary, restarts
```

Or from the web dashboard: open **About** → **Update Now** (admin only).

## Operator CLI

The `goclaw` binary can also inspect local or remote gateways:

```bash
goclaw traces list --status error
goclaw traces get <trace-id> -o json
goclaw --server https://goclaw.example.com --token "$GOCLAW_GATEWAY_TOKEN" \
  traces follow --session <session-key>
```

## Where to go next

- [Configuration](/en/getting-started/configuration) — the JSON5 config file, env overlay, secrets
- [Self-Hosting Guide](/en/self-hosting) — systemd, video worker sidecar, migrations, backups
- [Architecture](/en/architecture) — how the platform fits together
