# Troubleshooting

## Gateway won't start

Work through this in order:

1. **Run the health check command:**

   ```bash
   goclaw doctor
   ```

   It verifies the system environment and configuration and names the first
   thing that is wrong.

2. **Check PostgreSQL is reachable and pgvector is installed.** The base
   schema requires the `pgvector` (and `pgcrypto`) extensions. Test the DSN
   from `.env.local`:

   ```bash
   psql "$GOCLAW_POSTGRES_DSN" -c "SELECT extname FROM pg_extension;"
   ```

   If `vector` is missing: `CREATE EXTENSION vector;` (it must be installed
   on the server first — the bundled Docker image `pgvector/pgvector:pg18`
   ships it).

3. **Apply migrations**, then restart:

   ```bash
   goclaw migrate up
   ```

**Health check**

```bash
curl http://localhost:18790/health
```

**Logs**

```bash
make logs                # Docker Compose
journalctl -u goclaw -f  # systemd
```

## Login and token issues

**Dashboard or CLI gets 401/forbidden**
The client must present the same `GOCLAW_GATEWAY_TOKEN` the gateway booted
with. The most common cause is a mismatch between the value in the
gateway's `.env.local` and what the client sends (browser-stored token, or
`--token` on the operator CLI). Fix the client value, or update
`.env.local` and restart the gateway — then re-authenticate every client.

**Gateway starts but the dashboard is empty / methods fail**
Migrations likely didn't run. Run `goclaw migrate up` and restart. On manual
server deploys, confirm the `migrations/` directory actually shipped — see
[Self-Hosting](./self-hosting#migrations-read-this-before-manual-deploys).

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

## Language and locale

**The UI shows the wrong language or untranslated keys**
The web UI language is set from the selector in the top bar and stored
locally; supported UI locales are en, vi, zh, ko, ru. If entries appear as
raw keys, hard-refresh the page to reload the locale bundles. The backend
message catalog (agent replies, system messages) supports en, vi, zh — the
locale reaches the backend via the WebSocket `connect` parameter or the HTTP
`Accept-Language` header, so a missing header can make replies fall back to
English.

## Installation and startup

**`make up` fails with "port 5432 already allocated"**
Another Postgres owns the port. Set another host port in `.env`
(`POSTGRES_PORT=5433`) and retry.

**Env variables from my env file never apply under systemd**
systemd doesn't parse `export KEY=...` lines. Use plain `KEY=value` in the
`EnvironmentFile`, or explicit `Environment=` directives. See
[Configuration](./getting-started/configuration).

## Desktop app

**Reset the data**
**Settings → About → Reset Database** in the desktop app deletes
`goclaw.db` (plus `-wal`/`-shm`) from `~/.goclaw/data/` and restarts the app
with a fresh database. Use it after a failed upgrade or corrupted state.
Workspace files are kept. See [Desktop](./desktop).

**Update never completes**
Desktop updates download from `lite-v*` GitHub Releases; the banner applies
the update and restarts the app. If the check keeps failing, verify the
machine can reach github.com — there is no manual update command in the
desktop app. Server editions have no self-update command either; replace the
binary or pull a new image as described in
[Installation](./getting-started/install#updating).

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
