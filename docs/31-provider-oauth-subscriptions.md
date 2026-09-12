# Provider OAuth Subscriptions (ChatGPT, Claude Pro/Max, GitHub Copilot)

GoClaw supports three OAuth subscription provider types — log in with a
consumer subscription account and the agent bills against that subscription
instead of an API key. All three share the same architecture: the access token
lives in `llm_providers.api_key` (AES-256 encrypted at rest), the refresh token
(where applicable) in `config_secrets`, expiry/metadata in
`llm_providers.settings` JSONB, and tokens refresh automatically on demand with
a 5-minute safety margin.

| Type | Flow | Refresh | Routes |
|------|------|---------|--------|
| `chatgpt_oauth` | OpenAI PKCE + paste-redirect (or loopback :1455) | automatic (refresh token) | `/v1/auth/chatgpt/{provider}/*` |
| `claude_oauth` | Claude.ai PKCE + console paste-back | automatic (refresh token) | `/v1/auth/claude/{provider}/*` |
| `copilot_oauth` | GitHub device flow (user code) | none needed (long-lived token) | `/v1/auth/copilot/{provider}/*` |

## Claude Pro/Max (`claude_oauth`)

1. Providers page → Add provider → **Claude Subscription (OAuth)**.
2. Pick an alias (default `claude-pro`) → **Sign in with Claude** opens
   claude.ai — log in with the Pro/Max account.
3. After authorizing, the console page shows a code — paste the full callback
   URL (or the bare `code#state` string) back into the dialog.
4. Inference goes to `api.anthropic.com/v1/messages` with
   `Authorization: Bearer <token>` + `anthropic-beta: oauth-2025-04-20`.

The public Claude Code PKCE client is used (no client secret). If Anthropic
rotates the client or endpoints, override them without a rebuild:

```bash
GOCLAW_CLAUDE_OAUTH_CLIENT_ID=...     # default 9d1c250a-e61b-44e9-96ed-4d3ce8e8c1af
GOCLAW_CLAUDE_OAUTH_AUTHORIZE_URL=... # default https://claude.ai/oauth/authorize
GOCLAW_CLAUDE_OAUTH_TOKEN_URL=...     # default https://platform.claude.com/v1/oauth/token
GOCLAW_CLAUDE_OAUTH_API_BASE=...      # default https://api.anthropic.com
GOCLAW_CLAUDE_OAUTH_SCOPES=...        # default "org:create_api_key user:profile user:inference"
```

Notes: `service_tier` / fast-mode are never sent on OAuth (Anthropic rejects
them); model aliases `opus` / `sonnet` / `haiku` resolve like the native
provider; OAuth requests cannot serve embeddings.

## GitHub Copilot (`copilot_oauth`)

1. Providers page → Add provider → **GitHub Copilot (OAuth)**.
2. Alias (default `github-copilot`) → **Sign in with GitHub Copilot** — the
   dialog shows a one-time code and opens github.com/login/device; enter the
   code there.
3. GoClaw validates Copilot access via `api.github.com/copilot_internal/user`
   and resolves the account-specific API base (default
   `https://api.individual.githubcopilot.com`).
4. Inference uses the OpenAI-compatible `/chat/completions` surface with the
   original GitHub token (the retired `/v2/token` ephemeral exchange is not
   needed) — default model `gpt-5`, changeable per provider.

Security: the resolved API base is allowlisted (`*.githubcopilot.com`,
`copilot-proxy.githubusercontent.com`, https only); anything else falls back to
the default base and logs `security.copilot_untrusted_api_endpoint`.

## CLI

```bash
goclaw auth status             # default ChatGPT alias
goclaw auth status claude-pro  # any OAuth alias — route resolved by provider type
goclaw auth logout claude-pro
```

## Troubleshooting

- **Claude: "invalid state parameter"** — the pasted string came from a
  different login attempt. Click Sign in again and paste the fresh callback.
- **Claude: 401 after working previously** — Anthropic likely rotated the
  client; set the `GOCLAW_CLAUDE_OAUTH_*` overrides above and re-login.
- **Copilot: 403 at validation** — the GitHub account has no Copilot
  subscription or the token lacks access; confirm at
  github.com/settings/copilot.
- **Refresh failures** never hard-fail a request while a stored token still
  works (stale-token fallback); fix connectivity and the next call refreshes.
