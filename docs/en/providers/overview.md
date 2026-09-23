# Providers & Models

GoClaw talks to LLM providers through pluggable provider adapters. An agent
addresses a model as `provider/model`; the provider registry resolves the
adapter, applies reliability policies and streams responses back.

## Provider adapters

| Adapter | Wire protocol | Notes |
|---------|---------------|-------|
| `anthropic` | Native Anthropic HTTP + SSE | Splits the system prompt into cache blocks (prompt-cache boundary) for cheaper cached calls |
| `openai` | OpenAI-compatible HTTP + SSE | Used for most third-party gateways |
| `dashscope` | DashScope (Alibaba Qwen) | Native client |
| `codex` | OpenAI Responses (SSE) | Used by ChatGPT/Codex OAuth routing |
| `claude-cli` | Claude Code CLI subprocess | MCP bridge back into the gateway; per-agent MCP servers supported |
| `claude-oauth` | Anthropic over OAuth token source | Claude Pro/Max subscription accounts |
| `copilot-oauth` | GitHub Copilot over OAuth token source | Copilot subscription accounts |
| `codex-oauth` | OpenAI Responses over OAuth token source | ChatGPT/Codex subscription accounts with account pools |
| `ollama` | Native Ollama client | Local/self-hosted; per-model `num_ctx` resolution |
| `vertex` | Vertex AI (OpenAI-compatible endpoint) | OAuth2 service account (inline JSON or file) or Application Default Credentials |
| `acp` | JSON-RPC 2.0 over stdio | Orchestrates ACP-compatible agent CLIs: Claude Code, Codex, Gemini CLI |

## Config presets

Provider presets are registered at gateway startup from the config file
(`cmd/gateway_providers.go`) — each activates when its API key (or host, for
Ollama) is present:

`openai`, `anthropic`, `atlascloud`, `openrouter`, `groq`, `deepseek`,
`gemini`, `mistral`, `xai`, `minimax`, `cohere`, `perplexity`, `dashscope`,
`bailian`, `zai`, `zai-coding`, `ollama`, `ollama-cloud`, `novita`,
`byteplus`, `byteplus-coding`, `vertex`

Additional OpenAI-compatible presets (moonshot, together, fireworks,
cerebras, synthetic, kilocode, opencode, nvidia, stepfun, venice, baseten,
chutes, huggingface) follow the same pattern.

## Provider registry in the database

Providers configured through the UI/CLI are stored in the `llm_providers`
table. API keys are encrypted at rest with AES-256-GCM
(`internal/crypto`). The registry is tenant-scoped — each tenant can
register its own providers — with fallback to the master tenant's
registrations. DB providers are registered after config providers and take
precedence.

## OAuth subscription routing

Subscription accounts (ChatGPT/Codex, Claude, GitHub Copilot) are stored as
OAuth providers with token refresh handled by the gateway. Accounts form
pools that are shared round-robin across tenants per modality, so multiple
tenants can draw on one subscription without token conflicts.

- HTTP: `/v1/auth/chatgpt/*`, `/v1/auth/openai/*`, `/v1/auth/claude/*`,
  `/v1/auth/copilot/*` (start, callback, status, quota, logout)
- CLI: `goclaw auth status [provider]` / `goclaw auth logout [provider]` —
  authentication itself is completed through the web UI (Providers page)

## Models

- **Model registry with forward-compat resolver** — unknown model names fall
  back to sensible defaults instead of hard-failing, so new upstream models
  work before GoClaw learns about them
- **Reasoning capability resolution** — per-agent adaptive reasoning effort
  based on model capability (`internal/providerresolve`)
- **Model fallback chains** — when a primary model fails, the agent retries
  down a configured fallback chain
- **Embeddings** — OpenAI and Voyage adapters back the memory and knowledge
  features

## Reliability

All provider calls go through `internal/reliability` +
`internal/providers/retry.go`:

- **`RetryDo`** — 3 attempts by default with exponential backoff from 300 ms
  to a 30 s cap, plus jitter; retryable statuses include 429, 5xx and
  Cloudflare edge errors
- **Circuit breaker** — tracked per `provider:model` pair; an open circuit
  fails fast until a probe succeeds
- **Rate-limit coordinator** — one shared view of armed 429 cooldowns per
  provider:model, so concurrent runs stop burning quota against a
  known-closed window
- **Health registry** — per-provider health state surfaced to the dashboard

## Managing providers

| Surface | Path | Access |
|---------|------|--------|
| Web UI | `/providers` | Admin only |
| CLI | `goclaw providers list` / `add` / `update` / `delete` / `verify` | Requires a running gateway |
| HTTP | `/v1/providers` (+ `/{id}/verify`, `/{id}/models`) | Gateway token or API key |
| WebSocket | providers methods under the config/agents families | Role-checked per method |

`goclaw providers verify <id>` pings the provider (or a specific model) and
reports connectivity without touching agent configuration.
