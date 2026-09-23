# Agents

An agent is the core runnable unit of GoClaw: an LLM-backed assistant with its
own identity, provider, model, prompt mode, context files, tools and budget.
Agents chat over [WebSocket RPC](../api/websocket), [HTTP](../api/http) and
[messaging channels](../channels/telegram), and can fan work out to other
agents — see [Orchestration & Teams](./orchestration).

## Agent types

| Type | Context model | Best for |
|------|---------------|----------|
| `open` | Per-user private context (7 context files seeded per user on first chat) | Personal assistants — every user talks to "their own" instance |
| `predefined` | Shared agent-level context plus a thin per-user `USER.md` | Purpose-built agents (researcher, designer) shared across users, with a summoning lifecycle and per-user instances |

Context files live in two tables routed by a `ContextFileInterceptor`:
`agent_context_files` (shared, agent-level) and `user_context_files`
(per-user). Bootstrap templates are seeded automatically per agent type and,
for `open` agents, per user.

### Dual identity

Every agent has two identifiers:

- **UUID** — used for database relations, foreign keys and events.
- **agent_key** — human-readable slug used for logs, workspace paths, session
  keys and the UI.

WebSocket params accept either form; APIs that return agent references include
both. See [Agent identity conventions](https://github.com/qkhalk/goclaw/blob/main/docs/agent-identity-conventions.md)
in the repo for the full rules.

## Per-agent configuration

Each agent carries its own runtime configuration, editable on the agent detail
page in the web UI or via `agents.update` / `PUT /v1/agents/{id}`:

| Field | Purpose |
|-------|---------|
| `provider` / `model` | Default LLM routing (from the providers registered in `llm_providers`) |
| `context_window` | Context window override for the model |
| `max_tool_iterations` | Cap on think→act loops per run |
| `workspace` + `restrict_to_workspace` | Working directory and filesystem confinement |
| `budget_monthly_cents` | Monthly token/cost budget |
| `temperature`, `thinking_level` | Sampling and baseline reasoning effort |
| `tools_config` | Tool policy — enable/disable per tool, per-agent allow/deny |
| `sandbox_config` | Docker sandbox for `exec` (see [Tools](./tools#sandbox)) |
| `subagents_config` | Spawn templates and subagent limits (see [Orchestration](./orchestration)) |
| `memory_config` | Memory auto-injection, episodic TTL, dreaming (see [Memory & Knowledge](./memory)) |
| `compaction_config` | Session history compaction behavior |
| `context_pruning` | Context pruning strategy |
| `reasoning_config` | Reasoning effort, including adaptive effort |
| `model_fallback` | Fallback chain when the primary model fails |
| `shell_deny_groups` | Extra shell command deny patterns |
| `kg_dedup_config` | Knowledge-graph dedup tuning |
| `self_evolve`, `skill_evolve` | Self-evolution flags (see [Orchestration](./orchestration#self-evolution)) |

## Prompt modes

Each agent has a prompt mode controlling how much of the system prompt is
assembled per run:

| Mode | Content |
|------|---------|
| `full` | All sections — main conversational agents |
| `task` | Lean but capable — automation runs |
| `minimal` | Reduced sections — periodic check-ins |
| `none` | Identity line only |

The effective mode is resolved per run with the precedence
**runtime override > auto-detect > agent config > default (`full`)**.
Auto-detection caps headless runs: heartbeat sessions run at most `minimal`,
subagent and cron sessions at most `task`. A stricter agent config always wins
over a looser auto-detected mode.

## Context files

Agent behavior is shaped by markdown context files seeded from
`internal/bootstrap/templates/`:

- `AGENTS.md` — operating instructions and environment
- `IDENTITY.md` — name, persona, emoji
- `SOUL.md` — values, tone and evolving self-model
- `TOOLS.md` — tool usage notes
- `USER.md` — per-user profile (per-user layer for both agent types)
- `BOOTSTRAP.md` — first-run onboarding checklist

All files are editable from the web UI: **Agents → agent detail → Files**
(backed by the `agents.files.list` / `agents.files.get` / `agents.files.set`
WebSocket methods).

## Heartbeats

Agents can run periodic check-ins driven by a `HEARTBEAT.md` checklist file:

- The heartbeat scheduler fires on configured intervals, optionally restricted
  to **active-hours windows** (per-heartbeat `active_hours` start/end).
- The run executes on the dedicated cron scheduler lane, so background
  check-ins never contend with interactive chats.
- If nothing needs attention the agent replies containing `HEARTBEAT_OK`, and
  the delivery is suppressed — no message is sent to the user.

## Managing agents

Surfaces for agent lifecycle management:

- **WebSocket** — `agents.list`, `agents.create`, `agents.update`,
  `agents.delete`, plus `agents.files.*` for context files and
  `agents.links.*` for delegation edges.
- **HTTP** — `GET/POST /v1/agents`, `PUT/DELETE /v1/agents/{id}`,
  `POST /v1/agents/{id}/resummon`, `GET /v1/agents/{id}/instances` and more.
- **Web UI** — the Agents page covers creation, per-agent settings, files,
  subagent definitions and the Evolution tab.
- **Import/export** — full agent archives:
  `GET /v1/agents/{id}/export` (with `/export/preview` and a download token)
  and `POST /v1/agents/import` / `POST /v1/agents/{id}/import` for restore or
  merge. Import requires admin role.

::: tip
`predefined` agents have a summoning lifecycle: a summoner bootstraps the
agent's context from a description, and instances per user can be inspected
and re-summoned (`/resummon`) when the shared definition changes.
:::

## Where to next

- Spawn subagents, delegate work and form teams: [Orchestration & Teams](./orchestration)
- Memory tiers, knowledge graph and vault: [Memory & Knowledge](./memory)
- Grant capabilities: [Skills & Skill Market](./skills)
- Built-in tools, sandbox, browser and MCP: [Tools, Browser & MCP](./tools)
