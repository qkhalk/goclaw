# Telegram Runtime Commands

Runtime control surface for the Telegram channel: per-chat preferences
(thinking level, dev mode, status verbosity), the rich `/status` card, and the
testing-skill menu. All commands reply instantly — none spend an LLM call.

## Commands

| Command | What it does |
|---------|--------------|
| `/thinking` | Show the chat's thinking-level override + the agent default |
| `/thinking <level>` | Set the override: `off` `minimal` `low` `medium` `high` `xhigh` `auto` `adaptive` |
| `/thinking default` | Clear the override — the agent config applies again |
| `/dev` | Show whether dev mode is on |
| `/dev on` / `/dev off` | Toggle dev mode for this chat |
| `/status` | Rich status card (verbosity below) |
| `/status full` / `/status short` | Set the verbosity for this chat and render it |
| `/skills` | List installed skills with descriptions |

Group chats gate `/thinking` and `/dev` behind the file-writer permission
(same rule as `/reset`; DB check failures fail open). DMs are unrestricted.

## /thinking — how it works

The level is stored in the chat session's `metadata.thinking_level` and read
by the gateway consumer when it builds the next `RunRequest`, landing in
`RunRequest.ThinkingLevelOverride` — which outranks the agent's configured
reasoning settings. Effects start from the **next** message.

Semantics that matter:

- `off` **disables reasoning entirely**: the reasoning option is omitted from
  the LLM request (Anthropic/OpenAI/Codex/Ollama all honor this; Ollama sends
  `think=false`).
- `none` is **rejected** by the Telegram command: in the provider layer
  `none` passes through and on Claude models it *enables* thinking with a
  default budget — the opposite of what people expect. Use `off`.
- `default` clears the override (back to agent config).
- Known caveat: models routed through the Gemini OpenAI-compat path ignore
  `off` (Gemini 3 defaults to high thinking when the parameter is absent).
- Voice messages may route to a different agent (`voice_agent_id`), which has
  its own session — toggles set here don't apply to voice turns.

## /dev — dev mode

`/dev on` stores `metadata.chat_mode=dev`. The consumer prepends a
`DEV MODE ACTIVE` behavior section to the system prompt of every run in the
chat: plan before acting, ask one clarifying question when a request is
ambiguous, confirm destructive/slow operations, prefer minimal diffs, report
honestly. It is prompt-guided behavior (plus the existing `ask_user`
reminder mechanics) — there is no run pause/resume behind it. Prompt preview
and replay surfaces don't include the section; live channel runs do.

## /status — the card

`full` (default in DMs):

```
🦊 GoClaw 3.18.0
⏱️ Uptime: gateway 2d 3h · system 5d 21h
🤖 Agent: fox-spirit · 🧠 Model: oc/mimo-v2.5-free
🧵 Session: telegram:direct:386246614 · updated 5m
💵 Cost (session): $0.0074 · 🔢 Tokens: 12.3k in / 4.5k out
📚 Context: 37k/200k (18%) · 🧹 Compactions: 0
⚙️ Think: high · Mode: dev · 🪢 Queue: main 1/4 active, 2 pending
📖 Docs: https://github.com/qkhalk/goclaw/blob/dev/docs/25-telegram-runtime-commands.md
```

`short` (default in groups): version, uptime, agent/model, session-updated.
Every line's source:

| Line | Source |
|------|--------|
| Version | `cmd.Version` build stamp |
| Gateway uptime | gateway server start time |
| System uptime | `/proc/uptime` (Linux; omitted elsewhere) |
| Agent / Model | session record (`model`, `provider`) |
| Session + updated | session key + `updated_at` |
| Cost | `SUM(traces.total_cost)` for the session key |
| Tokens | session `input_tokens` / `output_tokens` counters |
| Context | last prompt tokens vs the agent context window (`?` until the first LLM response) |
| Compactions | session compaction counter |
| Think / Mode | session metadata (this doc's toggles) + agent default |
| Queue | scheduler `main` lane utilization |

## Skill menu (`/`)

The bot command menu merges: default bot commands + `menu_skills`
(default `cook plan fix review test`) + `menu_testing_skills` (default
`security-audit loadtest netstress ssl-audit recon fuzz dns-audit`).

Telegram bot commands only accept `[a-z0-9_]`, so hyphenated slugs are
sanitized: `/security-audit` → `/security_audit`. The skill command matcher
treats `_` and `-` as equivalent, so the menu entry and typing
`/security-audit` by hand activate the same skill. Configure via the channel
instance config:

```json
{ "menu_skills": ["cook", "plan"], "menu_testing_skills": ["loadtest"] }
```

`[]` disables a group; omit the key for defaults.

## Testing skills

The bundled testing suite (all with an authorization gate — use them only on
systems you own or are authorized to test):

| Skill | Purpose | Tools |
|-------|---------|-------|
| `security-audit` | Pre-production security assessment with severity-ranked findings | nmap, nikto, sqlmap, testssl.sh |
| `loadtest` | HTTP Layer-7 capacity testing with ramp/soak profiles and an SLO verdict | wrk, hey |
| `netstress` | Layer-4 throughput / packet-rate / connection-rate ceilings | iperf3, hping3 |
| `ssl-audit` | TLS/SSL health check with a grade table | testssl.sh, openssl |
| `recon` | Service discovery + attack-surface map (discovery only) | nmap |
| `fuzz` | Content/endpoint discovery with rate-limited ffuf | ffuf |
| `dns-audit` | SPF/DKIM/DMARC email-safety and zone health | dig |

Docker `full` variant pre-installs: nmap, nikto, wrk, hey, iperf3, ffuf,
bind-tools (dig), sqlmap (pip). Missing binaries install on demand via the
skill dependency installer.

## Video input

Sending a video to the bot downloads it and exposes it to the agent as a
`<media:video>` tag; the agent analyzes it with the `read_video` tool, which
uploads the video to a video-capable provider (priority: Gemini →
OpenRouter). **No ffmpeg is involved.**

Requirements in practice:

1. **A video-capable provider must be configured** (Providers in the web
   UI). With only an OpenAI-compatible chat endpoint configured, `read_video`
   has no provider and fails.
2. **The download must survive the network.** Downloads retry up to 3 times
   (connection resets from Telegram file DCs happen); after that the message
   is skipped with a notice.
3. **~20 MB limit on the standard Bot API** — Telegram's bot `getFile` only
   serves files up to 20 MB. Larger videos need a local Bot API server
   (`api_server` channel config); there is no code path around this limit.
