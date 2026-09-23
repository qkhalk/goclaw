# Configuration

GoClaw separates **what you configure** (config file) from **what you keep
secret** (environment). Never put API keys in the config file.

## The config file

GoClaw reads a **JSON5** configuration file — comments and trailing commas
are allowed:

```json5
{
  // Fragment — shows commonly used sections.
  // Real defaults are seeded by `goclaw onboard`.

  agents: {
    // Reasoning/thinking default for NEWLY created agents:
    // auto | off | low | medium | high (default: auto)
    // Existing agents keep the value saved on the agent.
    reasoning_default: "auto"
  },

  skills: {
    // Which skills are seeded on a fresh install:
    // all   — full bundle (legacy default, back-compat)
    // core  — ~10 runtime-critical skills only
    // none  — nothing; install from the Skill Market later
    seed_mode: "all"
  }
}
```

### Config file resolution order

The config path is resolved in this order (see `resolveConfigPath` in
`cmd/root.go`):

1. The `--config` command-line flag, if set.
2. The `GOCLAW_CONFIG` environment variable, if set.
3. `config.json` in the **current working directory**.
4. Built-in defaults — only if no file exists at the resolved path.

In practice `goclaw onboard` writes `config.json` into the directory you
start the gateway from, so step 3 is the normal case.

## Environment variable overlay

Environment variables override config file values. The core variables:

| Variable | Purpose |
|----------|---------|
| `GOCLAW_CONFIG` | Path to the JSON5 config file |
| `GOCLAW_POSTGRES_DSN` | PostgreSQL connection string (env-only, never stored in config) |
| `GOCLAW_GATEWAY_TOKEN` | Bearer token for the HTTP API and operator CLI |
| `GOCLAW_ENCRYPTION_KEY` | AES-256-GCM key encrypting provider API keys at rest |
| `GOCLAW_PORT` | Gateway listen port (default `18790`) |
| `GOCLAW_STORAGE_BACKEND` | `postgres` (default) or `sqlite` — env-only |

Config sub-sections map to prefixed env vars. Provider keys are the most
common example; a few other verified mappings:

```bash
export GOCLAW_ANTHROPIC_API_KEY=sk-ant-...       # providers.anthropic.api_key
export GOCLAW_OPENROUTER_API_KEY=sk-or-...       # providers.openrouter.api_key
export GOCLAW_MODEL=anthropic/claude-sonnet-4    # agents.defaults.model
export GOCLAW_SKILLS_SEED_MODE=core              # skills.seed_mode
```

`GOCLAW_MODE` is **deprecated** and ignored — setting it only produces a
startup warning.

## Secrets

Secrets live in `.env.local` (or real environment variables) — **never in
`config.json`**:

```bash
# .env.local — source it before starting the gateway
source .env.local && goclaw
```

Provider keys stored in the `llm_providers` database table are encrypted at
rest with **AES-256-GCM** using `GOCLAW_ENCRYPTION_KEY`.

::: warning
`.env.local` is for your shell. If you run the gateway under systemd, note
that systemd does **not** parse shell-style `export` lines from arbitrary
files — pass variables through `Environment=` directives or an
`EnvironmentFile` without the `export` prefix. This is a common reason an
env-var-driven setting silently doesn't apply. See
[Self-Hosting](../self-hosting).
:::

## Hot reload

The gateway watches the config file (fsnotify) and applies changes without a
restart. Some settings that construct long-lived resources (for example
channel binaries) are only picked up on restart; the config reference in
`internal/config` calls these out where applicable.

The dashboard edits the same configuration over WebSocket RPC methods:
`config.get`, `config.apply`, `config.patch`, `config.schema` and
`config.defaults`. From the CLI:

```bash
goclaw config show       # effective config, secrets redacted
goclaw config path       # print the resolved config file path
goclaw config validate   # validate the config file
```

## Database

- **Standard (server):** PostgreSQL 18 with the **pgvector** extension.
  Migrations run during `goclaw onboard` / `make up`, or manually with
  `goclaw migrate up`.
- **Desktop (Lite):** SQLite at `~/.goclaw/data/` — zero configuration.
  Secrets use the OS keyring with a file fallback at `~/.goclaw/secrets/`.
  See [Desktop](../desktop).

## Agents

Agent defaults live under `agents.defaults` in the config (model,
temperature, max tokens, provider, reasoning level), and `agents.list`
defines predefined agents. The `GOCLAW_MODEL` env var overrides
`agents.defaults.model`.

Each agent carries its own provider/model, tools, prompt mode and reasoning
configuration. Notable behaviors:

- **`agents.reasoning_default`** (default `auto`) — applies to *newly
  created* agents only. Existing agents keep their saved value, so upgrades
  never change behavior. With `auto`, the effective thinking level is
  resolved from the provider's capability map (e.g. Anthropic → medium,
  OpenAI-compat reasoning models → low, unknown → off).
- **Subagents inherit** the parent agent's effective `max_tokens`,
  `temperature` and reasoning config unless the subagent definition overrides
  them explicitly. See [Agents & Subagents](../features/agents).

## First-run flow

With the config in place, open the web dashboard. On a fresh database the UI
redirects to the `/setup` wizard, which configures, in order: a provider,
a model, an agent, and (optionally) a channel. After that you land on the
dashboard. Provider credentials can also be managed later from the dashboard
or via [Channels](../channels/telegram) setup.

## Video worker

The standalone ffmpeg render worker is a separate binary with its own CLI
flags (`--addr`, `--token`, `--work-dir`, ...). It is documented in the
[Self-Hosting Guide](../self-hosting).

## Where to go next

- [Installation](./install) — onboarding wizard and deployment options
- [Self-Hosting Guide](../self-hosting) — systemd units, env handling, backups
- [Architecture](../architecture) — how config fits into the runtime
