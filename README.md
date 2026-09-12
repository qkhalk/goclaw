<p align="center">
  <img src="_statics/goclaw-logo.svg" alt="GoClaw" height="200" />
</p>

<p align="center"><strong>Multi-Tenant AI Agent Platform</strong></p>

<p align="center">
Multi-agent AI gateway built in Go. 20+ LLM providers. 7 channels. Multi-tenant PostgreSQL.<br/>
Single binary. Production-tested. Agents that orchestrate for you.
</p>

<p align="center">
  <a href="https://github.com/qkhalk/goclaw/releases">Releases</a> •
  <a href="https://github.com/qkhalk/goclaw#quick-start">Quick Start</a>
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go_1.26-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" /></a>
  <a href="https://www.postgresql.org/"><img src="https://img.shields.io/badge/PostgreSQL_18-316192?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL" /></a>
  <a href="https://www.docker.com/"><img src="https://img.shields.io/badge/Docker-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker" /></a>
  <a href="https://developer.mozilla.org/en-US/docs/Web/API/WebSocket"><img src="https://img.shields.io/badge/WebSocket-010101?style=flat-square&logo=socket.io&logoColor=white" alt="WebSocket" /></a>
  <a href="https://opentelemetry.io/"><img src="https://img.shields.io/badge/OpenTelemetry-000000?style=flat-square&logo=opentelemetry&logoColor=white" alt="OpenTelemetry" /></a>
  <a href="https://www.anthropic.com/"><img src="https://img.shields.io/badge/Anthropic-191919?style=flat-square&logo=anthropic&logoColor=white" alt="Anthropic" /></a>
  <a href="https://openai.com/"><img src="https://img.shields.io/badge/OpenAI_Compatible-412991?style=flat-square&logo=openai&logoColor=white" alt="OpenAI" /></a>
  <img src="https://img.shields.io/badge/License-CC%20BY--NC%204.0-lightgrey?style=flat-square" alt="License: CC BY-NC 4.0" />
</p>

## Features

- **8-Stage Agent Pipeline** — context → history → prompt → think → act → observe → memory → summarize. Pluggable stages, always-on execution
- **4-Mode Prompt System** — Full / Task / Minimal / None with section gating, cache boundary optimization, and per-session mode resolution
- **3-Tier Memory** — Working (conversation) → Episodic (session summaries) → Semantic (knowledge graph). Progressive loading L0/L1/L2
- **Knowledge Vault** — Document registry with [[wikilinks]], hybrid search (FTS + pgvector), filesystem sync
- **Cloud Accounts** — Connect your own Google accounts (Gmail + Drive) per user; agents search/read/clean mail (no permanent delete, consent-gated unsubscribe) and browse/download Drive files via rclone
- **Agent Teams & Orchestration** — Shared task boards, inter-agent delegation (sync/async), 3 orchestration modes (auto/explicit/manual)
- **Self-Evolution** — Metrics → suggestions → auto-adapt with guardrails. Agents refine their own communication style
- **Multi-Tenant PostgreSQL** — Per-user workspaces, per-user context files, encrypted API keys (AES-256-GCM), RBAC, isolated sessions
- **20+ LLM Providers** — Anthropic (native HTTP+SSE with prompt caching), OpenAI, OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI, MiniMax, DashScope, Claude CLI, Codex, ACP, Parallel web search, and any OpenAI-compatible endpoint
- **7 Messaging Channels** — Telegram, Discord, Slack, Zalo OA, Zalo Personal, Feishu/Lark, WhatsApp
- **Production Security** — 5-layer permission system, rate limiting, prompt injection detection, SSRF protection, AES-256-GCM encryption
- **Single Binary** — ~25 MB static Go binary, no Node.js runtime, <1s startup, runs on a $5 VPS
- **Observability** — Built-in LLM call tracing with spans and prompt cache metrics, optional OpenTelemetry OTLP export

## Reliability Layer

**`internal/reliability/`** — unified reliability infrastructure with unit tests:

| Module | File | Description |
|--------|------|-------------|
| Error taxonomy | `errors.go` | Canonical `ErrorCode` (`provider.*`, `model.*`, `runtime.*`, `tool.*`) with retryability + severity. `ReliabilityError`, `ClassifyError` (HTTP status/body → code), `IsRetryable` |
| Circuit breaker | `circuitbreaker.go` | State machine per provider:model (Healthy → Degraded → Open → HalfOpen), consecutive-failure counting, cooldown, `ProbeTimeout` |
| Health registry | `health.go` | Per-key runtime reliability scoring (success ratio − stall/tool-error penalties) |
| Rate-limit coordinator | `ratelimit.go` | Single-flight cooldown against retry storms; fail-closed `ErrMaxPendingWaiters` when waiter cap exceeded; stale waiter cannot delete newer cooldown |
| Metrics | `metrics.go` | `atomic` counters + `Snapshot`, global swap-safe `Sink`, `Flush` drain per-counter |

## Releases

Releases are created manually via GitHub Actions (`release-fork.yaml`) — builds binaries for 5 platforms (linux/amd64 + arm64, darwin/amd64 + arm64, windows/amd64) with embedded web UI, Docker images (`ghcr.io/qkhalk/goclaw:{tag}` + `-full`), and GitHub Releases.

| Tag | Type | Changes | Docker |
|-----|------|---------|--------|
| `v3.16.1` | stable | **Phase B — Resource leak & retry hardening:** B1 scheduler session eviction (janitor idle reaping, configurable `SessionIdleEvictMs`), B2 watchdog age-based eviction (`MaxRunDuration` 30m, prevents re-abort loops), B3 timeline delta coalescing (adjacent chunk/thinking merged into single DB rows), B4 retry admission hardening (Codex/Ollama migrated to `RetryDoFor`, fail-closed `ErrMaxPendingWaiters`, dead `ShouldWait`/`BeginWait`/`Waiters` removed). **C3** SQLite partial index `idx_webhook_calls_running_heartbeat` for `ReclaimStale`. 17 files, +757/-198. **Phase A:** security P0 fixes + slash command palette. Reliability layer; `/gc:` command system (plan/fix/cook/review + 7-phase kit infrastructure). **Parallel web search provider** (11 files) | `ghcr.io/qkhalk/goclaw:v3.16.1`, `:v3.16.1-full` |

## Desktop Edition (GoClaw Lite)

A native desktop app for local AI agents — no Docker, no PostgreSQL, no infrastructure.

**macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/qkhalk/goclaw/dev/scripts/install-lite.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/qkhalk/goclaw/dev/scripts/install-lite.ps1 | iex
```

### What's Included
- Single native app (Wails v2 + React), ~30 MB
- SQLite database (zero setup)
- Chat with agents (streaming, tools, media, file attachments)
- Agent management (max 5), provider config, MCP servers, skills, cron
- Team tasks with Kanban board and real-time updates
- Auto-update from GitHub Releases

### Lite vs Standard

| Feature | Lite (Desktop) | Standard (Server) |
|---------|---------------|-------------------|
| Agents | Max 5 | Unlimited |
| Teams | Max 1 (5 members) | Unlimited |
| Database | SQLite (local) | PostgreSQL |
| Memory | FTS5 text search | pgvector semantic |
| Channels | — | Telegram, Discord, Slack, Zalo, Feishu, WhatsApp |
| Knowledge Graph | — | Full |
| RBAC / Multi-tenant | — | Full |
| Auto-update | GitHub Releases | Docker / binary |

### Building from Source
```bash
# Prerequisites: Go 1.26+, pnpm, Wails CLI (go install github.com/wailsapp/wails/v2/cmd/wails@latest)
make desktop-build                    # Build .app (macOS) or .exe (Windows)
make desktop-dmg VERSION=0.1.0        # Create .dmg installer (macOS only)
make desktop-dev                      # Dev mode with hot reload
```

### Desktop Releases
Desktop uses independent versioning with `lite-v*` tags:
```bash
git tag lite-v0.1.0 && git push origin lite-v0.1.0
# → GitHub Actions builds macOS (.dmg + .tar.gz) + Windows (.zip)
# → Creates GitHub Release with all assets
```

## Architecture

<p align="center">
  <img src="_statics/Multi-Tenant Architecture.jpg" alt="Multi-Tenant Architecture" width="800" />
</p>

<p align="center">
  <img src="_statics/3-Tier Memory Architecture.jpg" alt="3-Tier Memory" width="800" />
</p>

<p align="center">
  <img src="_statics/8-Stage Agent Pipeline.jpg" alt="8-Stage Agent Pipeline" width="800" />
</p>

<p align="center">
  <img src="_statics/Mode Prompt System.jpg" alt="4-Mode Prompt System" width="800" />
</p>

## Quick Start

**Prerequisites:** Go 1.26+, PostgreSQL 18 with pgvector, Docker (optional)

### Install (one-liner)

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

Sau khi cài: `goclaw onboard` (wizard, tự chạy migrations) rồi `goclaw` để khởi động gateway. Web dashboard tại `http://localhost:18790`.

### From Source

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw
make build
./goclaw onboard        # Interactive setup wizard
source .env.local && ./goclaw
```

### With Docker

```bash
# Generate .env with auto-generated secrets
chmod +x prepare-env.sh && ./prepare-env.sh

# Add at least one GOCLAW_*_API_KEY to .env, then:
make up

# If Postgres fails to start ("port 5432 already allocated"), set another host
# port in .env, e.g. POSTGRES_PORT=5433 (see .env.example).

# Web Dashboard at http://localhost:18790 (built-in)
# Health check: curl http://localhost:18790/health

# Optional: separate nginx for custom SSL/reverse proxy
# make up WITH_WEB_NGINX=1  → Dashboard at http://localhost:3000
```

`make up` creates a Docker network, embeds the correct version from git tags, builds and starts all services, and runs database migrations automatically.

**Common commands:**

```bash
make up                # Start all services (build + migrate)
make down              # Stop all services
make logs              # Tail logs (goclaw service)
make reset             # Wipe volumes and rebuild from scratch
```

**Operator CLI:**

The main `goclaw` binary can also inspect local or remote gateways:

```bash
goclaw traces list --status error
goclaw traces get <trace-id> -o json
goclaw --server https://goclaw.example.com --token "$GOCLAW_GATEWAY_TOKEN" traces follow --session <session-key>
```

**Optional services** — enable with `WITH_*` flags:

| Flag | Service | What it does |
|------|---------|-------------|
| `WITH_BROWSER=1` | Headless Chrome | Enables `browser` tool for web scraping, screenshots, automation |
| `WITH_OTEL=1` | Jaeger | OpenTelemetry tracing UI for debugging LLM calls and latency |
| `WITH_SANDBOX=1` | Docker sandbox | Isolated container for running untrusted code from agents |
| `WITH_TAILSCALE=1` | Tailscale | Expose gateway over Tailscale private network |
| `WITH_REDIS=1` | Redis | Redis-backed caching layer |

Flags can be combined and work with all commands:

```bash
# Start with browser automation and tracing
make up WITH_BROWSER=1 WITH_OTEL=1

# Stop everything including optional services
make down WITH_BROWSER=1 WITH_OTEL=1
```

When `GOCLAW_*_API_KEY` environment variables are set, the gateway auto-onboards without interactive prompts — detects provider, runs migrations, and seeds default data.

> **Docker image variants:**
> | Image | Description |
> |-------|-------------|
> | `ghcr.io/qkhalk/goclaw:v3.16.1` | Backend + embedded web UI + Python (**recommended**) |
> | `ghcr.io/qkhalk/goclaw:v3.16.1-base` | Backend API-only, no web UI, no runtimes |
> | `ghcr.io/qkhalk/goclaw:v3.16.1-full` | All runtimes + skill dependencies pre-installed |
> | `ghcr.io/qkhalk/goclaw:latest` | Alias指向 latest stable tag |
>
> For custom builds (Tailscale, Redis): `docker build --build-arg ENABLE_TSNET=true ...`
> See the [Deployment Guide](https://docs.goclaw.sh/#deploy-docker-compose) for details.

## Updating

### Docker
```bash
docker compose pull && docker compose up -d
```

### Binary (with embedded web UI)
```bash
goclaw update --apply    # Downloads, verifies SHA256, swaps binary, restarts
```

### Web Dashboard
Open **About** dialog → click **Update Now** (admin only). The update includes both backend and web dashboard when using the default `latest` image.

## Multi-Agent Orchestration

<p align="center">
  <img src="_statics/Agent Orchestration.jpg" alt="Agent Orchestration" width="800" />
</p>

Each agent runs with its own identity, tools, LLM provider, and context files.
Agent Links define outbound, inbound, or bidirectional permission edges.
Delegation can run synchronously or asynchronously and exchanges files through
an isolated delegation workspace; validated outputs are published back under
the caller's `.delegations/<delegation-id>/` directory.

> Details: [Agent Teams docs](https://docs.goclaw.sh/#teams-what-are-teams)

## Knowledge Vault

<p align="center">
  <img src="_statics/Knowledge Vault.jpg" alt="Knowledge Vault" width="800" />
</p>

Document registry with `[[wikilinks]]` for bidirectional linking. Hybrid search combines full-text (BM25) and semantic (pgvector) for precise retrieval. Filesystem sync keeps vault in sync with on-disk files.

## Cloud Accounts

Connect your own Google accounts (Gmail + Drive) per user and let agents work with them as tools — a differentiator among self-hosted agent gateways:

- **First-run setup in the browser** — an admin pastes a Google OAuth Client ID/Secret into the Cloud page once; credentials are stored encrypted and the setup form disappears. No config file editing, no restart. (Bring-your-own client: each install registers its own GCP OAuth app — see [docs/30-cloud-accounts.md](docs/30-cloud-accounts.md).)
- **Multi-account, per-user isolation** — every user connects their own Google accounts; tokens are AES-256-GCM encrypted at rest and scoped by tenant + user.
- **Mail tools** — `mail_search` (Gmail search syntax), `mail_read`, `mail_archive` (archive/trash/label; permanent delete does not exist by construction), and `mail_unsubscribe` (RFC 8058 — analyze first, executes only with explicit user consent).
- **Drive tools** — `cloud_ls` / `cloud_read` / `cloud_fetch` / `cloud_about` backed by a supervised rclone daemon on loopback (bundled in every Docker variant).
- **mail-digest skill** — schedule a daily digest by cron: 24h mail grouped by sender with newsletter candidates proposed for unsubscription (never auto-executed).

## Self-Evolution

<p align="center">
  <img src="_statics/Self-Evolution System.jpg" alt="Self-Evolution" width="800" />
</p>

Agents improve themselves through a 3-stage guardrailed pipeline: metrics collection → suggestion analysis → auto-adaptation. Can refine communication style and domain expertise (CAPABILITIES.md) — but never change identity, name, or core purpose.

## Provider Adapters

<p align="center">
  <img src="_statics/Provider Adapter System.jpg" alt="Provider Adapters" width="800" />
</p>

20+ LLM providers unified through a single adapter interface. Capability-based routing, encrypted API keys (AES-256-GCM), extended thinking support per-provider, and prompt caching for Anthropic + OpenAI.

## Event-Driven Architecture

<p align="center">
  <img src="_statics/DomainEventBus.jpg" alt="DomainEventBus" width="800" />
</p>

Typed domain events power the consolidation pipeline — session summaries, knowledge graph extraction, and dreaming promotion all run asynchronously via worker pools with dedup and retry.

## Built-in Tools

30+ tools across 8 categories:

| Category | Tools | Description |
|----------|-------|-------------|
| **Filesystem** | `read_file`, `write_file`, `edit_file`, `list_files`, `search`, `glob` | File operations with virtual FS routing |
| **Runtime** | `exec`, `browser` | Shell commands (approval workflow) + browser automation |
| **Web** | `web_search`, `web_fetch`, `parallel_search` | Search (Brave, DuckDuckGo, Parallel) + content extraction |
| **Memory** | `memory_search`, `memory_get`, `knowledge_graph_search` | 3-tier memory + KG traversal |
| **Media** | `create_image`, `create_audio`, `create_video`, `read_*`, `tts` | Generation + analysis (multi-provider) |
| **Skills** | `skill_search`, `use_skill`, `skill_manage` | BM25 + semantic hybrid search |
| **Teams** | `team_tasks`, `spawn`, `delegate`, `message` | Task board + orchestration + messaging |
| **Automation** | `cron`, `heartbeat`, `sessions_*` | Scheduling + session management |

> Full tool reference at [docs.goclaw.sh](https://docs.goclaw.sh/#custom-tools)

## Webhook API

Trigger agents or send channel messages from external systems without the gateway token.

```bash
# Bearer auth — sync LLM call
curl -X POST https://example.com/v1/webhooks/llm \
  -H "Authorization: Bearer wh_..." \
  -H "Content-Type: application/json" \
  -d '{"input":"Summarize today metrics","mode":"sync"}'

# HMAC auth — sign with hmac_signing_key from create response
TS=$(date +%s); BODY='{"input":"hi","mode":"sync"}'
SIG=$(echo -n "${TS}.${BODY}" | openssl dgst -sha256 -mac HMAC \
      -macopt "hexkey:${WEBHOOK_HMAC_KEY}" | awk '{print $2}')
curl -X POST https://example.com/v1/webhooks/llm \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Id: ${WEBHOOK_ID}" \
  -H "X-GoClaw-Signature: t=${TS},v1=${SIG}" \
  -d "$BODY"
```

See **[docs/webhooks.md](docs/webhooks.md)** for the full reference: auth, async callbacks, retry schedule, HMAC examples, channel matrix.

## Documentation

Full documentation at **[docs.goclaw.sh](https://docs.goclaw.sh)**

| Section | Topics |
|---------|--------|
| [Getting Started](https://docs.goclaw.sh/#what-is-goclaw) | Installation, Quick Start, Configuration, Web Dashboard Tour |
| [Core Concepts](https://docs.goclaw.sh/#how-goclaw-works) | Agent Loop, Sessions, Tools, Memory, Multi-Tenancy |
| [Agents](https://docs.goclaw.sh/#creating-agents) | Creating Agents, Context Files, Personality, Sharing & Access |
| [Providers](https://docs.goclaw.sh/#providers-overview) | Anthropic, OpenAI, OpenRouter, Gemini, DeepSeek, +15 more |
| [Channels](https://docs.goclaw.sh/#channels-overview) | Telegram, Discord, Slack, Feishu, Zalo, WhatsApp, WebSocket |
| [Agent Teams](https://docs.goclaw.sh/#teams-what-are-teams) | Teams, Task Board, Messaging, Delegation & Handoff |
| [Advanced](https://docs.goclaw.sh/#custom-tools) | Custom Tools, MCP, Skills, Cron, Sandbox, Hooks, RBAC |
| [Deployment](https://docs.goclaw.sh/#deploy-docker-compose) | Docker Compose, Database, Security, Observability, Tailscale |
| [Reference](https://docs.goclaw.sh/#cli-commands) | CLI Commands, REST API, WebSocket Protocol, Environment Variables |

## Testing

```bash
go test ./...                                    # Unit tests
go test -v ./tests/integration/ -timeout 120s    # Integration tests (requires running gateway)
```

## Project Status

See [CHANGELOG.md](CHANGELOG.md) for detailed feature status including what's been tested in production and what's still in progress.

## License

[CC BY-NC 4.0](LICENSE) — Creative Commons Attribution-NonCommercial 4.0 International

## Star History

<a href="https://www.star-history.com/?repos=qkhalk%2Fgoclaw&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/image?repos=qkhalk/goclaw&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/image?repos=qkhalk/goclaw&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/image?repos=qkhalk/goclaw&type=date&legend=top-left" />
 </picture>
</a>
