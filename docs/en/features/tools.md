# Tools, Browser & MCP

Tools are the agent's hands: built-in tools cover the filesystem, shell,
web, memory, messaging and cloud surfaces, extended by the Tool Store,
Docker-sandboxed execution, server-side browser automation, external MCP
servers and credentialed CLI access.

## Built-in tool catalog

The catalog is seeded into the database and managed in the web UI
(**/builtin-tools**: enable/disable per tool, per-tenant configuration via
`/v1/tools/builtin/{name}/tenant-config`).

| Category | Tools |
|----------|-------|
| Filesystem | `read_file`, `write_file`, `edit`, `list_files` |
| Runtime | `exec` (approval-gated), `wait`, `datetime`, `workstation_exec`, `claude_remote` |
| Web | `web_search`, `web_fetch` |
| Memory | `memory_search`, `memory_get`, `memory_expand`, `knowledge_graph_search` |
| Vault | `vault_search`, `vault_read` |
| Media | `read_image`, `read_document`, `create_image`, `read_audio`, `read_video`, `create_video`, `create_audio`, `tts`, `stt` (capability-gated) |
| Browser | `browser` — server-side automation via go-rod |
| Sessions | `sessions_list`, `session_status`, `sessions_history`, `sessions_send`, `plan` |
| Messaging | `message`, `ask_options`, `send_file`, `create_forum_topic`, `list_group_members`, `zalo_list_groups` |
| Cloud | `cloud_accounts`, `mail_search`, `mail_read`, `mail_archive`, `mail_unsubscribe`, `cloud_ls`, `cloud_read`, `cloud_fetch`, `cloud_about`, `cloud_write`, `cloud_mkdir`, `cloud_copy`, `cloud_move`, `cloud_delete`, `cloud_share` |
| Scheduling | `cron`, `heartbeat` |
| Subagents | `spawn`, `delegate` (availability depends on the [orchestration mode](./orchestration#orchestration-modes)) |
| Skills | `skill_search`, `use_skill`, `publish_skill`, `skill_manage`, `mcp_tool_search` |
| Teams | `team_tasks` |
| Studio | `pptx_studio`, `video_studio`, `watermark_studio` — see [Creative Studio](./tools-studio) |

### Deferred loading

Most common tools are always in the model's tool list; less-common tools are
**deferred** and discovered on demand through the BM25-based `tool_search`
meta-tool, which loads matching tools into the registry for the rest of the
run. This keeps prompt size small without sacrificing reach.

### Web search provider chain

`web_search` routes through a per-tenant provider chain — Brave, Tavily, Exa,
SearXNG, Parallel — with DuckDuckGo always last as the free fallback. The
chain is first-success-wins; a call can also force a specific provider for
cross-engine corroboration.

## Tool policy and approvals

- **Per-agent policy** (`tools_config`): enable/disable individual tools; a
  `toolCallPrefix` maps prefixed model output (e.g. `proxy_exec`) back to the
  canonical tool name.
- **Exec approvals**: `exec` runs are gated by a security mode; when a command
  requires approval it lands in a queue that admins resolve with
  approve/deny. Grants are scoped (`once` default, `session`, allow-always)
  and can carry an expiry. WS methods: `exec.approval.list`, `.approve`,
  `.deny`, `.history`. UI: **/approvals**.

## Extending the toolset

There is no runtime "custom tool" CRUD — the toolset is extended through
verified surfaces instead:

- **Builtin tool management**: enable/disable any builtin tool with
  per-tool settings via `GET/PUT /v1/tools/builtin/{name}`, global or
  per-tenant (`tenant-config`). UI: **/builtin-tools**.
- **Tool Store packages**: install/uninstall packages that ship additional
  tooling (`POST /v1/packages/install`, `POST /v1/packages/uninstall`,
  `GET /v1/packages/runtimes`). UI: **/packages**.
- **Credentialed exec**: known CLIs (`git`, `gh`, `psql`, `kubectl`, `aws`,
  `gcloud`, `terraform`) run through credential adapters with ephemeral
  credentials and regex deny patterns blocking dangerous arguments
  (`/v1/cli-credentials/*`).
- **Shell deny groups**: `shell_deny_groups` on the agent plus global
  deny patterns (`GET /v1/shell-deny-groups`) block dangerous constructs
  before `exec` runs.
- **MCP servers**: attach external MCP servers to gain their tools
  dynamically (see [MCP](#mcp-bridge) below).

## Sandbox

`exec` can run inside a Docker sandbox configured per agent
(`sandbox_config`):

| Field | Values (default first) |
|-------|------------------------|
| `mode` | `off`, `non-main` (subagents/delegates only), `all` |
| `image` | `goclaw-sandbox:bookworm-slim` |
| `workspace_access` | `rw`, `ro`, `none` |
| `scope` | `session`, `agent`, `shared` |
| `memory_mb` / `cpus` | 512 MB / 1.0 CPU |
| `network_enabled` | `false` by default |
| `read_only_root` | `true` by default |
| `setup_command` | Run once after container creation |

Idle containers are pruned automatically (default: idle 24 h, max age 7 days).

## Browser automation

The `browser` tool drives a real browser **server-side** via
[go-rod](https://github.com/go-rod/rod) (`pkg/browser`) — navigation,
clicking, form fills and extraction, with no local browser needed on the
client device. WebSocket surface: `browser.act`, `browser.snapshot`,
`browser.screenshot` (plus pairing, panel relay and remote sessions). The web
chat includes a browser panel that renders live snapshots relayed through the
gateway.

## MCP bridge

Connect external [Model Context Protocol](https://modelcontextprotocol.io)
servers over `stdio`, `sse` or `streamable-http`:

- Registered MCP tools enter the normal tool registry with `mcp_` prefixing
  and optional `{prefix}__{tool}` naming to avoid collisions.
- **Per-agent and per-user grants** with tool allow/deny lists; users can
  request access and admins review (`POST /v1/mcp/requests/{id}/review`).
- `mcp_tool_search` searches across connected servers; full management under
  `/v1/mcp/*` and the **/mcp** UI page.
- GoClaw can also **expose its own tools as an MCP server** for outside
  consumers (`internal/mcp` bridge server), and
  `goclaw mcp new <name>` scaffolds a new tool-server project.

## Credentialed exec

For trusted CLI workflows, credential adapters inject ephemeral credentials
into `exec` calls so agents can use authenticated tools without seeing long
lived secrets:

- Presets: `git`, `psql`, `gh`, `kubectl`, `aws`, `gcloud`, `terraform`.
- Credentials are scoped per user and per agent, validated against the target
  binary, and issued ephemerally per execution.
- HTTP surface: `/v1/cli-credentials` (presets, binaries check, per-account
  user/agent credentials and grants, connectivity test).
- Web UI: **/packages → CLI credentials** tab.

## Where to next

- Tool policy fields per agent: [Agents](./agents)
- Tools the memory tools query: [Memory & Knowledge](./memory)
- Skill tools and their access model: [Skills & Skill Market](./skills)
