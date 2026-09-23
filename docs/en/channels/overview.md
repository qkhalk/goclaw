# Channels

Channels connect GoClaw agents to messaging platforms. The gateway supports
10 channel types, and each type can have multiple configured accounts
(**channel instances**) running at the same time.

## Supported channel types

| Type | Platform | Notes |
|------|----------|-------|
| `telegram` | Telegram | Bot commands, forum topics, voice message STT, localized commands, inline skills picker |
| `discord` | Discord | Guild/channel bot with media handling |
| `facebook` | Facebook / Messenger | Page messaging |
| `feishu` | Feishu / Lark | Streaming message cards and media delivery |
| `pancake` | Pancake | Messaging via the Pancake platform |
| `slack` | Slack | Workspace bot with mention handling |
| `whatsapp` | WhatsApp | Native multi-device client via whatsmeow — QR login, no official Business API |
| `zalo_oa` | Zalo Official Account | OA messaging |
| `zalo_personal` | Zalo Personal | QR login, contact listing |
| `bitrix24` | Bitrix24 | imbot-based bot; portals are onboarded through a self-service install flow |

Type constants are defined in `internal/channels/channel.go`.

## Channel instances

A **channel instance** is one configured account of a channel type (for
example, two Telegram bots, or one WhatsApp number per tenant). Instances are
stored per tenant and managed over both APIs:

- WebSocket RPC: `channels.instances.list` / `get` / `create` / `update` /
  `delete`
- HTTP: `/v1/channels/instances/*` (list, get, create, delete) plus
  per-instance endpoints for context capabilities, group/member resolution,
  memory-extraction review, and credential scopes

### Writer allowlists

Group conversations are gated by writer allowlists — only listed senders can
trigger control actions (agent runs, commands) in a group or forum:

- `writers` — the allowlist entries for an instance
- `writers/groups` — groups eligible for allowlist management
- `writers/test` — check whether a specific sender is currently a writer

Expose via `GET/POST/DELETE /v1/channels/instances/{id}/writers*`.

## Channel manager

The channel manager (`internal/channels`) owns instance lifecycle:

- `StartAll` / `StopAll` — start or stop every registered instance with the
  gateway
- Health snapshots — each running channel reports a health snapshot the
  dashboard displays
- Failure recording — startup and runtime failures are classified and stored
  per instance so the UI can show the last error
- Group listing / member resolution — channels that expose groups implement a
  shared provider interface used by writer allowlists and targeting

## Pairing and QR login

Some channels authenticate interactively:

- **WhatsApp** and **Zalo Personal** use QR login — start the flow and scan
  with the phone app. QR codes and completion are pushed as server events
  (`whatsapp.qr.code` / `whatsapp.qr.done`, `zalo.personal.qr.code` /
  `zalo.personal.qr.done`), with WS entry points `whatsapp.qr.start` and
  `zalo.personal.qr.start`
- Channels with direct-message bots (Telegram, Discord, Slack, ...) support
  device **pairing codes**: a user sends a pairing request in the chat, an
  operator approves it with `goclaw pairing approve`, and the user gets
  gateway access scoped to their role

## Managing channels

| Surface | Path | Access |
|---------|------|--------|
| Web UI | `/channels` | Admin only |
| CLI | `goclaw channels list` / `add` / `delete` | Requires a running gateway |
| HTTP | `/v1/channels/instances/*` | Gateway token or API key |
| WebSocket | `channels.instances.*` | Role-checked per method |

The CLI talks to the running gateway over HTTP — start the gateway first.

::: tip Telegram deep dive
Telegram is the most feature-complete channel (localized slash commands,
subagent tracking, skills picker, formatting pipeline). See
[Telegram](./telegram) for details.
:::
