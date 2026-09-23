# Orchestration & Teams

GoClaw agents do not run alone: they can spawn subagents, delegate tasks to
linked agents and work in teams with a shared task board. Background work is
handled by cron jobs, and agents can improve themselves through a
guardrailed self-evolution loop.

## Orchestration modes

Every agent resolves to one orchestration mode at run start, by priority
**team > delegate > spawn**:

| Mode | Available inter-agent tools |
|------|------------------------------|
| `spawn` | `spawn` only (self-clones) |
| `delegate` | `spawn` + `delegate` (linked agents) |
| `team` | `spawn` + `delegate` + `team_tasks` (full team tools) |

Resolution: a member of any team gets `team`; otherwise an agent with
outbound delegation links gets `delegate`; otherwise `spawn`. Tools outside
the mode are hidden from the model, not just discouraged.

## Subagents

The `spawn` tool creates child agents that run tasks in isolation and report
back:

- **Spawn templates** — named reusable definitions persisted on the agent
  (`subagents_config.definitions`), each able to override model, prompt and
  allowlist. Without a template the child is a default self-clone of the
  parent's effective config.
- **Roster** — `spawn` with a roster action lists running and finished child
  tasks; running tasks can be **cancelled** or **steered**, finished tasks
  **archived** (single or all).
- **Announce** — completed subagents publish their results back to the parent
  session through the message bus.
- **Edition limits** — Lite caps concurrent subagents at 2 and depth at 1;
  the standard default concurrency limit is 20 (`subagents_config`).

Subagent definitions are managed on the agent detail page in the web UI and
over WS (`subagents.list`, `subagents.get`, `subagents.cancel`,
`subagents.archive`, `subagents.archive_completed`).

## Agent delegation

`agent_links` are explicit edges between agents:

| Field | Values |
|-------|--------|
| `direction` | `outbound`, `inbound`, `bidirectional` |
| `description` | Free text injected into the caller's prompt (who to hand what) |
| `max_concurrent` | Concurrency cap on delegations over the link |
| `status` | `active`, `disabled` |
| `team_id` | Set when the link was auto-created by a team |

The `delegate` tool hands a task to a linked agent in two modes:

- **Sync** — blocks the caller until the target answers; timeout configurable
  per call (default 300 s, hard cap 600 s).
- **Async** — fire-and-forget; the target announces its result back through
  the message bus when done.

Delegation exchanges files through an isolated delegation workspace; validated
outputs are published back under the caller's `.delegations/<delegation-id>/`
directory.

Link management: WS `agents.links.list` / `create` / `update` / `delete`.

## Teams

Teams group agents with shared context and coordination tools:

- **CRUD + members** — WS `teams.create`, `teams.update`, `teams.delete`,
  `teams.get`, `teams.list`, `teams.members.add`, `teams.members.remove`,
  plus `teams.scopes` and `teams.known_users`.
- **Shared team workspace** — files visible to all members
  (`teams.workspace.list` / `read` / `delete`).
- **Task board** — create, assign, claim, progress, review, approve, reject
  and comment on tasks (`teams.tasks.*`), with live events
  (`teams.tasks.events`, `teams.events.list`). Team-work classification
  routes team-scoped sessions automatically.
- **Lite gating** — `TeamActionPolicy` blocks the destructive/coordination
  actions (`comment`, `review`, `approve`, `reject`, `attach`, `ask_user`)
  in the Lite edition; agents are told to escalate blockers via comment
  only in full mode.

## Multi-agent rounds

Beyond one-to-one delegation, GoClaw supports structured multi-agent rounds:

- **Dynamic team formation** (`multiagent.formation`) — assemble a panel of
  agents for a task.
- **Jury** (`multiagent.jury`) — several agents answer independently and a
  verdict is aggregated; verdict history is queryable.
- **Negotiation** (`multiagent.negotiate`) — agents iterate toward agreement
  with visible negotiation state.

Round execution is driven by the jury/negotiate tools in the agent loop; the
RPC methods expose formation, verdict and state history. Result aggregation
uses `BatchQueue[T]` in `internal/orchestration`.

## Cron & tasks

Scheduled work runs through cron jobs and a task tree:

- **Schedule kinds** — `at` (one-shot timestamp), `every` (fixed interval) and
  `cron` (5-field [gronx](https://github.com/adhocore/gronx) expression).
- **Isolation** — jobs execute through the `cronexec` runner (its own process
  group) and agent runs are scheduled on the dedicated **cron lane** of the
  scheduler, separate from main and subagent lanes.
- **Retry** — failed executions retry with backoff.
- **Task tree** — lightweight parent/child tasks for structure, and the team
  task board (above) for full workflows.

Cron jobs are managed in the web UI (/cron) and over WS (`cron.create`,
`cron.list`, `cron.get`, `cron.delete`, ...).

## Self-evolution

Self-evolution is a metrics → suggestions → apply loop with admin review at
the apply step:

1. **Metrics collection** — every run records tool usage and retrieval
   metrics per agent.
2. **Suggestion analysis** — a periodic engine evaluates 7-day aggregates
   with three rules:

   | Rule | Trigger |
   |------|---------|
   | Low retrieval usage | Usage rate < 20% over 50+ queries for a source |
   | Tool failure | Success rate < 10% over 20+ calls for a tool |
   | Repeated pattern | A single tool > 100 successful calls/week |

3. **Review** — suggestions land as `pending`; admins approve, reject or roll
   back. Guardrails bound what can be applied and require recent metric data
   before any change.

Two per-agent flags extend the loop:

- `self_evolve` — the agent may rewrite its own `SOUL.md` under guardrails.
- `skill_evolve` — adds skill-creation nudges when repeated workflows suggest
  a missing skill.

Endpoints: `GET /v1/agents/{id}/evolution/metrics`,
`GET /v1/agents/{id}/evolution/suggestions`,
`PATCH /v1/agents/{id}/evolution/suggestions/{suggestionId}`.
Per-skill evolution has its own CLI — see
[Skills](./skills#per-skill-evolution). The web UI exposes this as the
**Evolution tab** on the agent detail page.

## Where to next

- Agent types and configuration: [Agents](./agents)
- Memory that delegation and teams share: [Memory & Knowledge](./memory)
- Scheduling details: [WebSocket RPC](../api/websocket) and [HTTP API](../api/http)
