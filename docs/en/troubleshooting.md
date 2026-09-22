# Troubleshooting

## Why replies feel "dumb"

If an agent's answers are noticeably shallower than expected — short,
generic, missing reasoning steps — walk this checklist. The three causes
below account for most cases.

### 1. Reasoning is off (and downgrades are silent)

The model's **thinking/reasoning level** dramatically changes answer depth.

- New agents default to `auto`: the level is resolved from the provider's
  capability map — Anthropic → medium, OpenAI-compatible reasoning models →
  low, unknown models → **off**. Agents created earlier keep whatever was
  saved on them, which for old agents is often plain **off**.
- When a provider can't serve the requested level, GoClaw falls back to a
  lower one. Historically this downgrade was **silent**; the run trace now
  records it (a note like "reasoning medium → off (provider not supported)").
- **Check:** open the agent settings and look at its reasoning config; on
  providers without reasoning support, pick a model that has it.

### 2. Subagent parameters

Subagents used to run on **hardcoded low parameters** (`max_tokens: 4096`,
`temperature: 0.5`) — visibly worse than the parent agent. They now
**inherit the parent's effective config** unless the subagent definition
overrides it.

- **Check:** if a subagent's output is still shallow, inspect its definition
  for explicit `maxTokens` / `temperature` / `thinkingLevel` overrides left
  over from earlier setups, and remove the ones you don't intend.

### 3. Provider fallback and routing

A reliability layer circuit-breaks struggling providers and routes around
them — meaning your request may land on a different (weaker) provider/model
than you think.

- **Check the traces:**

```bash
goclaw traces list --status error
goclaw traces get <trace-id> -o json
```

The trace shows which provider/model actually served each call, the effective
thinking level, and any downgrade note. The web dashboard shows the same
per-run metadata in the chat activity indicator.

## Installation and startup

**`make up` fails with "port 5432 already allocated"**
Another Postgres owns the port. Set another host port in `.env`
(`POSTGRES_PORT=5433`) and retry.

**Gateway starts but the dashboard is empty / methods fail**
Migrations likely didn't run. Run `goclaw migrate up` and restart. On manual
server deploys, confirm the `migrations/` directory actually shipped to
`/opt/goclaw/migrations` — see [Self-Hosting](/en/self-hosting#migrations-read-this-before-manual-deploys).

**Env variables from my env file never apply under systemd**
systemd doesn't parse `export KEY=...` lines. Use plain `KEY=value` in the
`EnvironmentFile`, or explicit `Environment=` directives.

**Health check**

```bash
curl http://localhost:18790/health
```

**Logs**

```bash
make logs                # Docker Compose
journalctl -u goclaw -f  # systemd
```

## Video rendering

- `render_video` jobs stuck at queued → check the worker:
  `curl http://127.0.0.1:18791/health` and
  `journalctl -u goclaw-videoworker -f`. The queue cap (`--max-queue`,
  default 5) returns 409 when full.
- Renders failed after a worker crash → expected: worker state is in-memory.
  The gateway marks orphaned jobs `failed`; re-run them. Orphan temp dirs are
  cleaned automatically every 15 minutes.
- Captions missing → `--font-file` is empty or the font path is wrong.

## Telegram

- Bot answers but control buttons do nothing in groups → group actions
  require **writer** permission for your member role.
- An edited message never updates → Telegram refuses edits on messages older
  than ~48 hours; the bot sends a fresh confirmation message instead.
- Garbled tables → tables render as ASCII in `<pre>` by design; Telegram has
  no table markup.

## Skills

- Skill Market install job fails → installs read from the **bundled skills
  directory on disk**; if you run a bare binary outside its release layout,
  point `GOCLAW_BUNDLED_SKILLS_DIR` at the bundled dir from the release.
- Custom skill can't be uninstalled from the market → by design; the market
  only removes skills it installed (system origin). Delete custom skills
  manually.
- Switched `seed_mode` to `core` but old skills are still there → expected:
  seed mode only affects **fresh** installs; the reconciler never removes
  already-installed skills. Uninstall the ones you don't want.
