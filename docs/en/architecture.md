# Architecture

GoClaw is a multi-tenant AI agent gateway: one Go binary containing a
WebSocket RPC + HTTP API server, an agent runtime, channel connectors, and an
embedded web dashboard. This page is a public map of how the pieces fit.

## Tech stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.26, Cobra CLI |
| Realtime | gorilla/websocket (frames: `req` / `res` / `event`) |
| Database | PostgreSQL 18 + pgvector (standard); SQLite via `modernc.org/sqlite` (desktop) — raw SQL via `database/sql` + pgx/v5, no ORM |
| Migrations | golang-migrate (PG), embedded incremental schema (SQLite) |
| Browser automation | go-rod |
| Web UI | React 19, Vite 6, TypeScript, Tailwind CSS 4, Radix UI, Zustand, React Router 7 |
| Desktop app | Wails v2 (build tag `sqliteonly`) embedding the gateway + React frontend in one binary |

## Repository layout

```
cmd/                     CLI commands, gateway startup, onboard wizard, migrations
internal/
  agent/                 Agent loop (think→act→observe), router, resolver
  pipeline/              8-stage agent pipeline
  providers/             LLM providers (Anthropic, OpenAI-compat, DashScope, Vertex, ...)
  providerresolve/       Provider adapter + model registry, forward-compat resolver
  gateway/               WS + HTTP server, client, method router (methods/)
  http/                  HTTP API (/v1/chat/completions, /v1/agents, /v1/skills, ...)
  channels/              Telegram, Feishu/Lark, Zalo, Discord, WhatsApp, ...
  memory/                3-tier memory (pgvector)
  knowledgegraph/        Knowledge graph storage and traversal
  vault/                 Knowledge Vault: wikilinks, hybrid search, FS sync
  skills/                SKILL.md loader, BM25 search, kit manager + market
  tools/                 Tool registry, filesystem, exec, web, subagent, delegate
  scheduler/             Lane-based concurrency (main/subagent/cron)
  consolidation/         Memory consolidation workers (episodic, semantic, dreaming)
  eventbus/              Domain event bus: worker pool, dedup, retry
  cron/                  Cron scheduling (at/every/cron expressions)
  store/                 Store interfaces + PostgreSQL (pg/) and SQLite (sqlitestore/) impls
  config/                Config loading (JSON5) + env var overlay
  crypto/                AES-256-GCM encryption for API keys
  edition/               Edition system (Lite, Standard) with feature gating
  i18n/                  Message catalog, T(locale, key, args...)
  mcp/                   Model Context Protocol bridge/server
  sandbox/               Docker-based code execution sandbox
  tts/                   Text-to-Speech (OpenAI, ElevenLabs, Edge, MiniMax)
pkg/protocol/            Wire types: frames, methods, errors, events
pkg/browser/             Browser automation (Rod + CDP)
migrations/              PostgreSQL migration files
ui/web/                  React SPA dashboard
ui/desktop/              Wails v2 desktop app
```

## The agent pipeline

Every run flows through an **8-stage pipeline**:

```
context → history → prompt → think → act → observe → memory → summarize
```

Stages are pluggable callbacks on an always-on execution path. A **4-mode
prompt system** (Full / Task / Minimal / None) gates prompt sections per
session and optimizes cache boundaries for prompt-caching providers.

## Agent types and identity

- **`open` agents** — each user gets a private context (7 context files).
- **`predefined` agents** — shared context plus a per-user `USER.md`.

Context files are routed through a `ContextFileInterceptor` from two tables:
`agent_context_files` (agent-level) and `user_context_files` (per-user).

Identity follows a dual-id convention: **UUID** for database foreign keys,
events and internal references; **agent_key** (a human-readable slug) for
logs, filesystem paths and the UI.

## Memory

Three tiers with progressive loading (L0/L1/L2, auto-inject for L0):

1. **Working** — the live conversation.
2. **Episodic** — session summaries, consolidated asynchronously.
3. **Semantic** — the knowledge graph.

On top sits the **Knowledge Vault**: a document registry with `[[wikilinks]]`,
hybrid full-text (BM25) + semantic (pgvector) search, and filesystem sync.
Typed domain events (worker pool, dedup, retry) drive the consolidation
pipeline — session summaries, KG extraction, and "dreaming" promotion all run
asynchronously.

## Providers

40+ providers behind a single `ProviderAdapter` interface with a
forward-compatible model registry:

- **Anthropic** — native HTTP+SSE, prompt caching, extended thinking
- **OpenAI-compatible** — HTTP+SSE (OpenAI, OpenRouter, Groq, DeepSeek, ...)
- **DashScope** (Alibaba Qwen), **Vertex AI** (GCP service account/ADC),
  **Codex CLI** (stdio+MCP bridge), **ACP**, and more
- **OAuth subscriptions** — ChatGPT, Claude Pro/Max, GitHub Copilot

All providers share one `RetryDo()` retry path and an SSE scanner. Provider
records live in the `llm_providers` table with AES-256-GCM encrypted keys. A
reliability layer underneath provides per-provider circuit breaking,
per-key health scoring, rate-limit coordination and metrics.

## Multi-tenancy and the store layer

Stores are interface-based (`store.SessionStore`, `store.AgentStore`, ...) with
a shared **Dialect** pattern: the same interface is implemented twice —
PostgreSQL (`store/pg/`) and SQLite (`store/sqlitestore/`) — over raw SQL with
positional parameters (`$1` for PG, `?` for SQLite).

Tenant and user context propagates explicitly through the request chain:
`store.WithTenantID(ctx)`, `store.WithUserID(ctx)`, `store.WithAgentID(ctx)`,
`store.WithLocale(ctx)`. Admin role is not a tenant check — writes to
tenant-scoped tables always pair an admin gate with a `WHERE tenant_id = $n`
clause.

## WebSocket protocol

The dashboard talks to the gateway over WebSocket frames of type `req`,
`res`, and `event`:

1. The **first request on a connection must be `connect`** (carries auth,
   locale, and session parameters).
2. Subsequent `req` frames invoke RPC methods (`chat.*`, `agents.*`,
   `sessions.*`, `subagents.*`, ...) routed by a method registry.
3. The server pushes `event` frames (streaming deltas, LLM lifecycle events,
   task updates) that the client fans out to stores and the UI.

All WS method params are **camelCase** (`teamId`, `taskId`, `sessionKey`),
mirroring the Go structs' `json` tags.

## Channels

Each channel connector (Telegram, Discord, Slack, Facebook/Messenger, Zalo,
Feishu/Lark, WhatsApp, Bitrix24, Pancake) adapts platform messages into the
gateway and back. **Telegram** is the most complete surface — inline pickers,
paged skill listings, localized commands, and HTML-formatted replies (see
[Telegram channel](/en/channels/telegram)).

## Tools and orchestration

30+ built-in tools across filesystem, exec, web search, memory, media,
video rendering, cloud accounts (Drive/Gmail via rclone), skills, teams, and
interactive `ask_options` questions. Agents coordinate through:

- **spawn** — launch subagent tasks (tracked in `subagent_tasks`)
- **delegate** — inter-agent task delegation over `agent_links` permission
  edges, in three modes: auto / explicit / manual
- **BatchQueue[T]** — generic parallel result aggregation

The scheduler enforces lane-based concurrency (main / subagent / cron) so
background work never starves interactive chat.

## Editions

An edition system gates features between **Standard** (PostgreSQL server,
full feature set) and **Lite** (SQLite desktop: 5 agents, 1 team, no channels
or multi-tenancy). The desktop binary is built with the `sqliteonly` tag and
talks only to SQLite.

## Localization

English (default), Vietnamese and Chinese in the backend message catalog
(`i18n.T(locale, key, args...)`); the web UI ships five locales
(en, vi, zh, ko, ru). Locale propagates via the WS `connect` parameter or the
HTTP `Accept-Language` header.

## Security

Rate limiting, detection-only input guarding, CORS, shell deny patterns,
SSRF protection, path traversal prevention, AES-256-GCM secret encryption,
a 5-layer permission system for tools, and sandboxed execution for untrusted
code.
