# Agents & Subagents

Every GoClaw agent runs with its own identity, tools, LLM provider, prompt
mode and context files. Agents can also **spawn subagents** and **delegate**
work to each other.

## Agent types

| Type | Context model | Best for |
|------|---------------|----------|
| `open` | Per-user private context (7 context files) | Personal assistants — every user talks to "their own" instance |
| `predefined` | Shared agent context + per-user `USER.md` | Purpose-built agents (researcher, designer) shared across users with a thin personal layer |

Context files live in two tables routed by a `ContextFileInterceptor`:
`agent_context_files` (shared, agent-level) and `user_context_files`
(per-user). Bootstrap templates (`SOUL.md`, `IDENTITY.md`, ...) are seeded
automatically and per-user.

Agent identity is dual: a **UUID** for database relations and events, an
**agent_key** slug for logs, paths and the UI.

## Creating agents

Create agents from the web dashboard (Agents page) or via the HTTP API
(`/v1/agents`). Each agent carries:

- A default **provider + model** (from the providers configured in
  `llm_providers`)
- A **tool allowlist** — which of the 30+ built-in tools the agent may use
- **Reasoning (thinking) configuration** — new agents default to `auto`,
  resolved against the provider's capability map (Anthropic → medium,
  OpenAI-compat reasoning models → low, unknown → off). Existing agents keep
  whatever was saved on them.
- **Skills granted** — see [Skills & Skill Market](/en/features/skills)

## Subagents

The `spawn` tool lets an agent fan work out into child tasks. Each spawned
task is a tracked row in `subagent_tasks` with a label, model, status and
summary — visible on every surface:

- **Web chat** — a pill next to the composer counts running/finished
  subagents; the panel lists every task (status icon, model, timing, summary)
  with **Cancel** for running tasks and **Archive** for finished ones, plus
  "Archive all completed". Completed subagents also render as cards in the
  chat timeline with an inline archive action.
- **Telegram** — the `/subagents` command lists tasks with the same status
  vocabulary (`queued`, `running`, `waiting`, `completed`, `failed`,
  `cancelled`), archive buttons per task and an "archive all completed"
  action. See [Telegram](/en/channels/telegram).
- **WebSocket API** — `subagents.list` (filter by `agentId` or `sessionKey`,
  optional `status` and `includeArchived`), `subagents.archive`,
  `subagents.archive_completed` and `subagents.cancel`. Ownership is enforced:
  you only ever see subagent tasks of agents you own.

Archived tasks disappear from default lists everywhere but remain queryable
with `includeArchived: true`.

::: tip Try it
Ask your agent: *"split this into 3 subagents — one per section"* — then watch
the subagent pill light up and archive the finished tasks from the panel.
:::

### How subagent quality works

Subagents **inherit the parent agent's effective config** — `max_tokens`,
`temperature` and reasoning/thinking level — unless the subagent definition
explicitly overrides them (`maxTokens`, `temperature`, `thinkingLevel`
fields). Historically subagents ran on hardcoded low parameters (4096 max
tokens, temperature 0.5), which produced visibly worse output than the parent
agent; inheritance fixed that. If subagent answers feel "dumber" than the
parent's, check the definition for leftover overrides — and see the
[checklist in Troubleshooting](/en/troubleshooting#why-replies-feel-dumb).

## Delegation between agents

`agent_links` define outbound, inbound or bidirectional permission edges
between agents. The `delegate` tool then hands a task from one agent to
another, in three orchestration modes:

| Mode | Behavior |
|------|----------|
| `auto` | The caller may delegate automatically when it judges another agent is better suited |
| `explicit` | The user (or prompt) names the target agent |
| `manual` | Delegation only with explicit confirmation |

Delegation runs **synchronously or asynchronously**, and exchanges files
through an isolated delegation workspace. Validated outputs are published
back under the caller's `.delegations/<delegation-id>/` directory.

## Teams

Agents can be grouped into **teams** with shared task boards (Kanban with
real-time updates), inter-agent messaging and token-aware work distribution.
In the Lite desktop edition teams are capped at 1 team / 5 members.
