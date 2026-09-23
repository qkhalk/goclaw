# HTTP API

GoClaw exposes an HTTP API under `/v1` alongside its WebSocket RPC. All
endpoints live on the gateway port (default `18790`).

An OpenAPI spec is served at `/v1/openapi.json` and an interactive
Swagger UI at `/docs`.

## Authentication

Bearer token in the `Authorization` header — either the gateway/operator
token or a scoped API key:

```bash
curl http://localhost:18790/v1/agents \
  -H "Authorization: Bearer $GOCLAW_GATEWAY_TOKEN"
```

API keys are `goclaw_`-prefixed, stored SHA-256-hashed, and carry scoped
permissions (each key resolves to a role derived from its scopes). Manage
them via the `/v1/api-keys` endpoints (list, create, revoke) or the web UI.

Locale-sensitive endpoints honor the `Accept-Language` header (en, vi, zh).

## Main endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `POST` | `/v1/chat/completions` | OpenAI-compatible chat completions against any configured agent/model |
| `POST` | `/v1/responses` | OpenAI Responses-compatible endpoint |
| `POST` | `/v1/tools/invoke` | Invoke a tool directly |
| `GET` | `/v1/agents` | List agents (with type, provider, model) |
| `GET` | `/v1/skills` | List installed skills |
| `GET` | `/health` | Liveness/health probe |
| `GET` | `/v1/edition` | Edition info (public, no auth) |

## REST endpoint families

| Family | Endpoints | Purpose |
|--------|-----------|---------|
| Agents | `/v1/agents/*` | Agent CRUD, instances and per-user files, episodic memory, knowledge graph (`kg/*`), vault (`vault/*`), evolution metrics/suggestions, v3 feature flags, export/import |
| Channels | `/v1/channels/instances/*` | Channel instance CRUD, writer allowlists (`writers`, `writers/groups`, `writers/test`), group/member resolution, memory-extraction review |
| Skills | `/v1/skills/*` | Skill CRUD, grants, dependencies, evolution, Skill Market (`market/*`), upload/import/export |
| Knowledge Vault | `/v1/vault/*` | Documents, wikilinks, tree, graph, search, enrichment status |
| MCP | `/v1/mcp/*` | Server registry, grants, install jobs, OAuth, import/export, request approval |
| Memory | `/v1/memory/documents` | Global document registry; per-agent under `/v1/agents/{id}/memory/*` |
| Tools | `/v1/tools/builtin`, `/v1/tools/builtin/{name}` | Builtin tool registry and per-tool tenant config |
| Webhooks | `/v1/webhooks/*` | Webhook CRUD, call history, plus runtime `POST /v1/webhooks/message` and `POST /v1/webhooks/llm` |
| API keys | `/v1/api-keys` | Create, list, revoke scoped keys |
| Tenants & RBAC | `/v1/tenants*` | Tenant CRUD, users, policies, roles and permissions |
| Usage & costs | `/v1/usage/*`, `/v1/costs/summary` | Timeseries, breakdowns, summaries, routing stats |
| System | `/v1/system/stats`, `/v1/logs/runtime/aggregate`, `/v1/activity` | Host metrics, aggregated runtime logs, activity feed |
| TTS | `/v1/tts/*` | TTS config, capabilities, voice cloning |
| Cloud | `/v1/cloud/*` | Cloud storage accounts, files, sync pairs, transfers |
| Packages | `/v1/packages/*` | Runtime/package install, update, uninstall |
| Pending messages | `/v1/pending-messages` | Inspect and compact queued channel messages |
| CLI credentials | `/v1/cli-credentials/*` | Shared CLI credential vault with agent/user grants |

Skill Market writes are **admin-only**; reads are available to signed-in
users.

## Chat completions

`POST /v1/chat/completions` follows the OpenAI request/response shape, so
existing clients and SDKs work unmodified — point `base_url` at your gateway
and use the gateway token as the API key. Requests are routed to the agent
and model you address, with streaming (SSE) supported.

## Webhook API

Trigger agents or send channel messages from external systems **without the
gateway token**. Create a webhook in the dashboard, then call it with either
scheme:

**Bearer auth — synchronous LLM call:**

```bash
curl -X POST https://example.com/v1/webhooks/llm \
  -H "Authorization: Bearer wh_..." \
  -H "Content-Type: application/json" \
  -d '{"input":"Summarize today metrics","mode":"sync"}'
```

**HMAC auth — sign the body with the `hmac_signing_key` from the create
response:**

```bash
TS=$(date +%s); BODY='{"input":"hi","mode":"sync"}'
SIG=$(echo -n "${TS}.${BODY}" | openssl dgst -sha256 -mac HMAC \
      -macopt "hexkey:${WEBHOOK_HMAC_KEY}" | awk '{print $2}')
curl -X POST https://example.com/v1/webhooks/llm \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Id: ${WEBHOOK_ID}" \
  -H "X-GoClaw-Signature: t=${TS},v1=${SIG}" \
  -d "$BODY"
```

Async mode (`"mode":"async"`) returns immediately and calls back with retry —
see the full webhooks reference (`docs/webhooks.md` in the GoClaw repo) for
the retry schedule and channel matrix.

::: warning Webhooks require GOCLAW_ENCRYPTION_KEY
The gateway refuses to mount `/v1/webhooks/*` when `GOCLAW_ENCRYPTION_KEY`
is unset — the endpoints return 404. Set the env var to enable the webhook
subsystem.
:::

`POST /v1/webhooks/message` uses the same auth schemes for a synchronous
channel send (text plus optional media).

## WebSocket API

The dashboard and rich clients use the WebSocket API instead — see
[WebSocket RPC](./websocket). The protocol in brief:

- Frames are typed `req` / `res` / `event`
- The **first request on a connection must be `connect`** — it authenticates
  and carries session parameters (including `locale`)
- Server `event` frames stream deltas, LLM lifecycle events and task updates
