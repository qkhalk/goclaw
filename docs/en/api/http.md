# HTTP API

GoClaw exposes an HTTP API under `/v1` alongside its WebSocket RPC. All
endpoints live on the gateway port (default `18790`).

## Authentication

Bearer token in the `Authorization` header (the gateway/operator token):

```bash
curl http://localhost:18790/v1/agents \
  -H "Authorization: Bearer $GOCLAW_GATEWAY_TOKEN"
```

Locale-sensitive endpoints honor the `Accept-Language` header (en, vi, zh).

## Main endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `POST` | `/v1/chat/completions` | OpenAI-compatible chat completions against any configured agent/model |
| `GET` | `/v1/agents` | List agents (with type, provider, model) |
| `GET` | `/v1/skills` | List installed skills |
| `GET` | `/v1/skills/market` | Skill Market catalog (name, category, version, installed, grants) |
| `POST` | `/v1/skills/market/install` | Install skills — `{ slugs: [...], grantAgentIds? }` background job |
| `POST` | `/v1/skills/market/update/{slug}` | Update one installed skill |
| `DELETE` | `/v1/skills/market/installed/{slug}` | Uninstall a managed skill |
| `POST` | `/v1/webhooks/llm` | Trigger an agent from an external system (see below) |
| `GET` | `/health` | Liveness/health probe |

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

## WebSocket API

The dashboard and rich clients use the WebSocket API instead. The protocol
in brief:

- Frames are typed `req` / `res` / `event`
- The **first request on a connection must be `connect`** — it authenticates
  and carries session parameters (including `locale`)
- Subsequent `req` frames invoke methods (`chat.*`, `agents.*`,
  `sessions.*`, `subagents.*`, `config.*`, `skills.*`, `cron.*`, ...)
- Server `event` frames stream deltas, LLM lifecycle events and task updates

All WS method params are camelCase (`teamId`, `taskId`, `sessionKey`).
The wire types live in `pkg/protocol` in the GoClaw repo.
