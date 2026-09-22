# Configuration

GoClaw separates **what you configure** (config file) from **what you keep
secret** (environment). Never put API keys in the config file.

## The config file

GoClaw reads a **JSON5** configuration file whose path comes from the
`GOCLAW_CONFIG` environment variable. JSON5 means you get comments and
trailing commas:

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

If `GOCLAW_CONFIG` is not set, the gateway falls back to its built-in
defaults and interactive onboarding.

## Environment variable overlay

Every setting in the config file can be overridden by an environment
variable. Two env vars are central:

| Variable | Purpose |
|----------|---------|
| `GOCLAW_CONFIG` | Path to the JSON5 config file |
| `GOCLAW_PORT` | Gateway listen port (default `18790`) |

Config sub-sections map to prefixed env vars, e.g. the skills seed mode from
the fragment above can be set without touching the file:

```bash
export GOCLAW_SKILLS_SEED_MODE=core
```

## Secrets

API keys and tokens live in `.env.local` (or real environment variables) —
**never in config.json**:

```bash
# .env.local — source it before starting the gateway
export GOCLAW_ANTHROPIC_API_KEY=sk-ant-...
export GOCLAW_OPENAI_API_KEY=sk-...
export GOCLAW_GATEWAY_TOKEN=...        # operator CLI / remote access token
```

```bash
source .env.local && goclaw
```

Two layers of protection apply:

- **At rest:** provider keys stored in the `llm_providers` database table are
  encrypted with **AES-256-GCM**.
- **At boot:** when `GOCLAW_*_API_KEY` variables are present, the gateway
  auto-onboards — provider detection, migrations and seeding run without
  interactive prompts.

::: warning
`.env.local` is for your shell. If you run the gateway under systemd, note
that systemd does **not** parse shell-style `export` lines from arbitrary
files — pass variables through `Environment=` directives or an
`EnvironmentFile` without the `export` prefix. This is a common reason an
env-var-driven setting silently doesn't apply. See
[Self-Hosting](/en/self-hosting).
:::

## Database

- **Standard (server):** PostgreSQL 18 with the **pgvector** extension.
  Migrations run automatically on `make up` / `goclaw onboard`, or manually
  with `goclaw migrate up`.
- **Desktop (Lite):** SQLite at `~/.goclaw/data/` — zero configuration.
- **Desktop secrets:** the OS keyring (`go-keyring`) with a file fallback at
  `~/.goclaw/secrets/`.

## Providers

Providers are not hardcoded in the config file. They live in the
`llm_providers` table (managed from the web dashboard or onboarding), each
with an encrypted API key. GoClaw supports 40+ providers out of the box —
Anthropic (native HTTP+SSE with prompt caching), OpenAI and any
OpenAI-compatible endpoint, OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI,
MiniMax, DashScope, Moonshot/Kimi, Vertex AI, and OAuth subscriptions
(ChatGPT, Claude Pro/Max, GitHub Copilot). See the
[Architecture page](/en/architecture#providers) for the adapter model.

## Per-agent settings

Each agent carries its own provider/model, tools, prompt mode and reasoning
(thinking) configuration. Notable defaults:

- **`agents.reasoning_default`** (config, default `auto`) — applies to
  *newly created* agents only. Agents created before this setting existed
  keep their saved value, so upgrades never change existing behavior. With
  `auto`, the effective thinking level is resolved from the provider's
  capability map (e.g. Anthropic → medium, OpenAI-compat reasoning models →
  low, unknown → off).
- **Subagents inherit** the parent agent's effective `max_tokens`,
  `temperature` and reasoning config unless the subagent definition overrides
  them explicitly. See [Agents & Subagents](/en/features/agents).

## Video worker

The standalone ffmpeg render worker is a separate binary with its own CLI
flags (`--addr`, `--token`, `--work-dir`, ...). It is documented in the
[Self-Hosting Guide](/en/self-hosting#video-worker-sidecar).
