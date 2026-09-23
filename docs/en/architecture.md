# Architecture

## The big picture

GoClaw is a multi-tenant AI agent gateway delivered as a **single Go binary**
that contains:

- a **WebSocket RPC + HTTP API gateway** (frames: `req` / `res` / `event`),
- the **agent runtime** — the 8-stage pipeline, tools, memory, scheduling,
- channel connectors (Telegram, Discord, WhatsApp, Feishu/Lark, Zalo, ...),
- the **embedded web dashboard** (React SPA served by the same binary).

The standard deployment stores everything in **PostgreSQL 18 with pgvector**
(raw SQL over `database/sql` + pgx/v5, no ORM). The desktop build compiles the
same runtime with the `sqliteonly` build tag against an embedded SQLite store
— see [Desktop](../desktop).

| Layer | Technology |
|-------|------------|
| Language | Go 1.26, Cobra CLI |
| Realtime | gorilla/websocket |
| Database | PostgreSQL 18 + pgvector; SQLite (`modernc.org/sqlite`) for desktop |
| Web UI | React 19, Vite 6, TypeScript, Tailwind CSS 4, Radix UI, Zustand |
| Desktop app | Wails v2 (`//go:build sqliteonly`) |
| Migrations | golang-migrate (PG), embedded incremental schema (SQLite) |

## The 8-stage pipeline

Every agent run executes through a pipeline of pluggable stages built on a
small `Stage` interface (`Execute(ctx, *RunState) error`, plus a `Name()`).
The pipeline has three phases (`internal/pipeline/pipeline.go`):

```
Setup      [context]               runs once
Iteration  [prune, think, continuation gate, tools, observe, checkpoint]   runs per turn
Finalize   [finalize]              runs once after the loop
```

- **context** (setup) — resolves the workspace, loads context files, builds
  the filtered tool list and the system prompt.
- **prune** — compacts/trims history to fit the token budget before each turn.
- **think** — calls the LLM; may produce a final answer or tool calls.
- **continuation gate** — guards against weak models ending runs prematurely
  (empty or cut-off replies get one bounded nudge instead).
- **tools** — dispatches tool calls for the turn.
- **observe** — feeds tool results back into the conversation.
- **checkpoint** — flushes pending messages to the session store each
  iteration and writes durable checkpoints on a cadence.
- **finalize** — closes out the run (summarization hooks run here).

**Runs are resumable:** because state is checkpointed to the session store,
a crashed or restarted gateway resumes the run from the checkpointed
iteration instead of starting over.

## 3-tier memory

Memory is layered, with progressive loading (L0 is auto-injected, L1/L2 are
loaded on demand):

| Tier | Name | Content |
|------|------|---------|
| L0 | Working | Compact abstracts of past sessions, auto-injected into the prompt under a ~200-token budget |
| L1 | Episodic | Per-session summaries generated as sessions close |
| L2 | Semantic | Knowledge graph entities and relations |

Consolidation is **event-driven**: domain events flow through the
**DomainEventBus** (`internal/eventbus` — typed events, worker pool, dedup,
retry) into the consolidation workers (`internal/consolidation`): episodic
summarization, semantic KG extraction, and a **dreaming** worker that
promotes/distills episodic content in the background.

## Knowledge layer

- **Knowledge graph** (`internal/knowledgegraph`) — entities and relations
  extracted by the LLM from conversations, stored in PostgreSQL (pgvector for
  semantic lookup) and traversable at query time.
- **Knowledge Vault** (`internal/vault`) — a document registry with
  `[[wikilinks]]` between documents, **hybrid search** combining full-text
  and vector scoring, and filesystem sync so vault documents can be edited
  on disk.

## Orchestration

Agents coordinate through several mechanisms:

- **Subagents** — an agent spawns child tasks (`spawn` tool, tracked in
  `subagent_tasks`) that run on their own pipeline with inherited model
  parameters.
- **Delegation** — the `delegate` tool hands work to another agent over
  **`agent_links`** permission edges, synchronously or asynchronously, with
  results returned as artifacts.
- **Teams** — agents grouped under a team lead with shared boards and
  member roles (Standard edition; Lite restricts team actions).
- **Jury / negotiate** — multi-agent decision tools: several agents weigh in
  and a verdict is aggregated (`internal/tools/jury_tool.go`,
  `internal/tools/negotiate_tool.go`, `internal/orchestration`).

## Self-evolution

The self-evolution loop runs in three progressive stages
(`internal/agent/suggestion_engine.go`, `evolution_guardrails.go`):

1. **Metrics collection** — per-agent tool metrics are aggregated over a
   rolling 7-day window.
2. **Suggestion analysis** — rules over those metrics produce actionable
   suggestions (e.g. repeated tool failures, prompt adjustments).
3. **Guardrail-protected apply/rollback** — suggestions are applied behind
   guardrails and can be rolled back; nothing mutates agents silently.

## Scheduler

Concurrency is organized into **four lanes** (`internal/scheduler/lanes.go`)
so background work never starves interactive chat:

| Lane | Default concurrency | Env override |
|------|--------------------|--------------|
| `main` | 30 | `GOCLAW_LANE_MAIN` |
| `subagent` | 50 | `GOCLAW_LANE_SUBAGENT` |
| `team` | 100 | `GOCLAW_LANE_TEAM` |
| `cron` | 30 | `GOCLAW_LANE_CRON` |

Sessions queue per session key within their lane, preserving per-session
ordering.

## Editions

`internal/edition` gates features between two presets:

- **Standard** — PostgreSQL server, all features: knowledge graph, RBAC,
  multi-tenancy, channels, vector search, dependency installers.
- **Lite** — the desktop preset: 5 agents, 1 team / 5 members, FTS-only
  search, no knowledge graph, no RBAC. Full list in [Desktop](../desktop).

`GET /v1/edition` (no auth) reports the active edition so the UI can adapt.

## Cross-cutting concerns

- **Store layer** — interface-based stores (`store.SessionStore`,
  `store.AgentStore`, ...) implemented twice (PostgreSQL and SQLite) behind a
  shared Dialect pattern. Tenant/user/agent/locale context propagates
  explicitly via context helpers (`store.WithTenantID(ctx)`,
  `store.WithUserID(ctx)`, ...); admin role is never treated as a tenant
  check on its own.
- **WebSocket protocol** — the first request on a connection must be
  `connect` (auth + locale); then `req` frames invoke RPC methods and the
  server pushes `event` frames (streaming deltas, lifecycle events). All
  params are camelCase, mirroring Go `json` tags. See
  [HTTP API](../api/http) for the REST surface.
- **Providers** — Anthropic, OpenAI-compatible, DashScope, Vertex AI, Codex
  CLI and more behind a single adapter interface with a forward-compatible
  model registry; API keys are AES-256-GCM encrypted in the `llm_providers`
  table.
- **Security** — rate limiting, detection-only input guard, CORS, shell deny
  patterns, SSRF protection, path traversal prevention, and a Docker sandbox
  for untrusted code. Security events log as `slog.Warn("security.*")`.
- **Localization** — backend catalog with `i18n.T(locale, key, args...)`
  (en/vi/zh); the web UI ships en, vi, zh, ko, ru. Locale arrives via the WS
  `connect` param or the HTTP `Accept-Language` header.

## Where to go next

- [Agents](../features/agents) — agent types, context files, subagents
- [Skills](../features/skills) — SKILL.md loading and the Skill Market
- [Telegram channel](../channels/telegram) — the most complete channel surface
- [Troubleshooting](../troubleshooting) — diagnosing routing and reasoning issues
