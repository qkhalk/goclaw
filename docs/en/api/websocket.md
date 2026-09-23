# WebSocket RPC

The gateway's primary API is a JSON RPC over WebSocket at
`ws://host:18790/ws` (the gateway port, default `18790`). The HTTP dashboard
and the CLI both speak this protocol. Wire types live in `pkg/protocol` in
the GoClaw repo.

## Frames

Every message is a JSON frame of one of three types:

| Type | Direction | Purpose |
|------|-----------|---------|
| `req` | client → server | Invoke a method; carries `id`, `method`, `params` |
| `res` | server → client | Reply to a `req` by `id` (success or error) |
| `event` | server → client | Unsolicited push (streaming deltas, lifecycle, presence) |

The **first request on a connection must be `connect`** — any other method
is rejected until the handshake completes.

```json
{"type":"req","id":"1","method":"connect","params":{"token":"...","locale":"en"}}
```

## Connect authentication

`connect` supports several auth paths (evaluated in order):

| Path | Credentials | Result |
|------|-------------|--------|
| Gateway token | `token` matches the configured gateway token (constant-time compare) | Owner/admin role; owner may narrow scope with `tenant_id` |
| API key | `token` is a valid `goclaw_`-prefixed API key | Role derived from the key's scopes; tenant from the key |
| No token configured | gateway runs without a token (explicit fallback allowed) | Operator role, master tenant |
| Paired browser reconnect | `sender_id` of a previously approved pairing | Operator role from tenant membership |
| Pairing-code flow | no token, no pairing | Responds `pending_pairing` with an 8-character `pairing_code`; the code is approved out-of-band with `goclaw pairing approve` (also `list`, `revoke`) |

Anything else is rejected (fail-closed).

`connect` also accepts a `locale` param (en, vi, zh) that persists for the
connection and localizes errors and prompts.

## Method families

Around 200 methods are registered. `pkg/protocol/methods.go` is the
authoritative list. Highlights:

| Family | Methods |
|--------|---------|
| Chat | `chat.send`, `chat.history`, `chat.abort`, `chat.inject` — with streaming events (chunks, tool calls/results, run lifecycle) |
| Agents | `agents.list/create/update/delete`, `agents.files.*` |
| Sessions | `sessions.list/preview/patch/delete/reset/compact/branch/archive/restore` |
| Agent links | `agents.links.list/create/update/delete` (delegation graph) |
| Channels | `channels.instances.*`, `channels.list/status/toggle` |
| Memory | `memory.write/get/search/supersede/archive` |
| Teams | `teams.*` incl. `teams.tasks.*` (create/assign/approve/reject/comments/events) |
| Tasks | `tasks.tree/create/updateStatus` |
| Cron | `cron.list/create/update/delete/toggle/status/run/runs` |
| Subagents | `subagents.list/get/archive/archive_completed/cancel` |
| Skills | `skills.list/get/update/approve/reject` |
| Hooks | `hooks.list/create/update/delete/toggle/test/history` |
| Config | `config.get/apply/patch/schema/defaults` |
| Runs | `runs.get/list/events/resume`, `runs.checkpoints.list`, `runs.replay` |
| Browser | `browser.act/snapshot/screenshot` (+ panel/remote variants) |
| Heartbeat | `heartbeat.get/set/toggle/test/logs`, `heartbeat.checklist.*`, `heartbeat.targets` |
| Usage | `usage.get`, `usage.summary` |
| Logs | `logs.tail` |
| Nodes & devices | `nodes.*`, `device.pair.*`, `node.hello/heartbeat/bye` |
| Workstations | `workstations.*` incl. permissions and activity |
| Multi-agent | `multiagent.formation`, `multiagent.jury`, `multiagent.negotiate` |
| Terminal | `terminal.create/list/attach/input/resize/close` — web PTY |
| Backup | `backup.schedule.get/set/run` |

## Events

The server pushes `event` frames from several families
(`pkg/protocol/events.go`):

- **Agent run lifecycle** — `run.started` / `run.completed` / `run.failed` /
  `run.cancelled`, `tool.call` / `tool.result`, `llm.started` /
  `llm.completed`, plus chat `chunk` / `thinking` streaming deltas
- **Presence & health** — `presence`, `health`, `heartbeat`
- **Delegation** — `delegation.started/progress/completed/failed/...`
- **Team tasks** — `team.task.created/claimed/completed/commented/...`,
  `team.message.sent`
- **Channel & pairing** — `whatsapp.qr.code/done`,
  `zalo.personal.qr.code/done`, `device.pair.requested/resolved`,
  `node.pair.requested/resolved`
- **Sessions** — `session.updated`
- **Approvals** — `exec.approval.requested/resolved`

## Conventions

- **Params are camelCase** — match the Go struct `json:"..."` tags
  (`teamId`, `taskId`, `sessionKey`)
- **Rate limiting** — optional per-connection RPM cap
  (`gateway.rate_limit_rpm` in the config; disabled by default)
- **RBAC** — every method is role-checked (`admin` / `operator` / `viewer`);
  unclassified methods fail closed
- Responses reference the request `id`; errors carry a protocol error code
  and a localized message

## Example

```json
{"type":"req","id":"2","method":"chat.send","params":{"agentId":"...","message":"hi"}}
{"type":"event","event":"chunk","payload":{"...":"streamed delta"}}
{"type":"res","id":"2","payload":{"sessionKey":"..."}}
```
