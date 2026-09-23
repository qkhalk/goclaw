# Installation

GoClaw ships as a single static Go binary (~25 MB), as Docker images, and as a
separate desktop app. This page covers the server editions; the desktop app is
described in [Desktop](../desktop).

## Requirements

| Component | Requirement |
|-----------|-------------|
| Database | PostgreSQL 18 with the **pgvector** extension (base schema also uses pgcrypto) |
| Go | 1.26+ — only needed if building from source |
| Runtime | None. The binary is static and embeds the web dashboard |

## One-liner install

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

## Onboarding

After installing, run the onboarding wizard:

```bash
goclaw onboard
```

The wizard walks through seven steps:

1. Ask for the PostgreSQL DSN.
2. Test the database connection.
3. Generate a gateway token (`GOCLAW_GATEWAY_TOKEN`) and an encryption key
   (`GOCLAW_ENCRYPTION_KEY`).
4. Run database migrations.
5. Seed placeholder providers.
6. Write `config.json` (contains no secrets).
7. Write `.env.local` containing the generated secrets.

Then start the gateway:

```bash
source .env.local && goclaw
```

The web dashboard is built into the binary and served at
`http://localhost:18790`.

## Useful CLI commands

| Command | Purpose |
|---------|---------|
| `goclaw setup` | TUI wizard (providers, agents, channels) for post-install configuration |
| `goclaw doctor` | Health check: system environment and configuration |
| `goclaw migrate up` | Apply pending PostgreSQL migrations |
| `goclaw upgrade` | Apply schema + data migrations (`--dry-run`, `--status` flags available) |
| `goclaw version` | Print the binary version and wire protocol version |
| `goclaw config show` | Print the effective configuration with secrets redacted |

## From source

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw
make build
./goclaw onboard
source .env.local && ./goclaw
```

Building from source requires Go 1.26+. The web UI is embedded into the binary
at build time — no Node.js runtime is needed on the server.

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

Common commands:

| Command | What it does |
|---------|--------------|
| `make up` | Pull/start all services and run migrations |
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

Images are published to GHCR (`ghcr.io/nextlevelbuilder/goclaw`) and mirrored
on Docker Hub (`digitop/goclaw`).

| Tag | Contents |
|-----|----------|
| `:latest`, `:vX.Y.Z` | Backend + embedded web UI + Python |
| `:base`, `:vX.Y.Z-base` | Backend only, no web UI or runtimes |
| `:full`, `:vX.Y.Z-full` | All runtimes + skill dependencies pre-installed |

## Desktop edition (GoClaw Lite)

A native desktop app for local agents — no Docker, no PostgreSQL. It embeds
the same gateway compiled against SQLite, with the web UI built in. Installers:

```bash
# macOS
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.ps1 | iex"
```

See [Desktop](../desktop) for the Lite edition limits, auto-update behavior
and build instructions.

## Updating

- **Docker:** `make up` pulls the latest published image and restarts; or
  `docker compose pull && docker compose up -d`.
- **Binary:** download the new release from GitHub Releases (or re-run the
  install script) and replace the binary. There is no `goclaw update`
  self-update command — binary self-update exists only in the desktop app.
  After replacing the binary, run `goclaw upgrade` to apply database
  schema/data migrations (this is a database migration command, not a
  self-update).
- **Desktop:** auto-updates in-app from `lite-v*` GitHub Releases — see
  [Desktop](../desktop#auto-update).

### Release artifacts

Releases are tag-triggered GitHub Actions builds. Binaries are produced for
linux (amd64/arm64), macOS (amd64/arm64) and Windows (amd64), plus the Docker
image variants listed above.

## Where to go next

- [Configuration](./configuration) — the JSON5 config file, env overlay, secrets
- [Self-Hosting Guide](../self-hosting) — systemd, migrations, backups
- [Architecture](../architecture) — how the platform fits together
- [Troubleshooting](../troubleshooting) — common startup and runtime problems
