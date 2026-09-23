# Desktop App (GoClaw Lite)

GoClaw Desktop (the **Lite** edition) is a native desktop app for running
agents locally. It is a [Wails v2](https://wails.io) single-window app that
wraps the same gateway binary as the server edition, compiled with the
`sqliteonly` build tag:

- **Embedded SQLite store** — no PostgreSQL, no Docker.
- **Embedded web UI** — the React dashboard is compiled into the binary and
  rendered in the app window.
- One process, one data directory, zero infrastructure.

## Install

```bash
# macOS
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.ps1 | iex"
```

Releases are published on GitHub Releases under `lite-v*` tags for macOS
(arm64/amd64) and Windows (amd64).

## Runtime behavior

| Aspect | Value |
|--------|-------|
| Listen address | `127.0.0.1` only (localhost, not exposed to the network) |
| Port | `18790` (override with `GOCLAW_PORT`) |
| Data directory | `~/.goclaw/data/` (SQLite database `goclaw.db`, configs) |
| Workspace | `~/.goclaw/workspace/` (agent files, team workspace) |

There is no `goclaw onboard` step in the desktop app: the encryption key and
gateway token are generated automatically on first launch.

## Secrets

Secrets are stored in the **OS keyring** (`go-keyring`) with a file fallback
at `~/.goclaw/secrets/`. This covers the encryption key and gateway token;
provider API keys are encrypted at rest the same way as the server edition.

## Auto-update

The app checks GitHub Releases for newer `lite-v*` tags:

- once at startup, and
- every 6 hours while running.

When an update is found, an in-app update banner appears; applying it
downloads the new build, swaps the binary and restarts the app. There is no
separate updater daemon.

The app version comes from `cmd.Version`, injected via `-ldflags` at build
time. The running edition is exposed by the gateway at `GET /v1/edition`
(no auth), which the UI uses to adapt feature availability.

## Lite edition limits

Limits are enforced by `internal/edition/edition.go` (`edition.Lite`):

| Capability | Lite | Standard |
|------------|------|----------|
| Agents | 5 | Unlimited |
| Teams | 1 (5 members) | Unlimited |
| Channels | 1 Telegram + 1 Discord | All supported channel types |
| Concurrent subagents | 2 | Unlimited |
| Delegation depth | 1 | Unlimited |
| Memory search | Full-text (FTS) only | FTS + pgvector semantic |
| Knowledge graph | Not available | Available |
| RBAC / multi-tenancy | Not available | Available |
| pip/npm/apk dependency installers | Not available | Available |
| Cloud accounts (OAuth cloud connections) | Not available | Available |

## Build from source

```bash
# Dev mode with hot reload
cd ui/desktop && wails dev -tags sqliteonly

# Production builds (from the repo root)
make desktop-build VERSION=0.1.0   # .app (macOS) or .exe (Windows)
make desktop-dmg VERSION=0.1.0     # .dmg installer (macOS only)
```

## Data reset

**Settings → About → Reset Database** deletes `goclaw.db` (including its
`-wal` and `-shm` files) from the data directory and restarts the app with a
fresh database. Workspace files are not removed. If the app misbehaves after a
failed upgrade, this is the fastest way back to a clean state — see
[Troubleshooting](../troubleshooting).

## Where to go next

- [Installation](./getting-started/install) — server editions and Docker
- [Configuration](./getting-started/configuration) — config file and env vars
- [Troubleshooting](./troubleshooting) — common problems
