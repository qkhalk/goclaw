# Telegram

Telegram is GoClaw's most complete channel: localized commands, inline
pickers, paged skill listings, subagent tracking — and (coming soon) archive
buttons for finished subagents.

## Connecting a bot

1. Create a bot with [@BotFather](https://t.me/BotFather) and copy the token.
2. In the GoClaw dashboard, open **Channels → Telegram**, paste the token and
   save. The gateway registers its webhook/updates loop automatically.
3. Start a private chat with your bot, or add it to a group. In groups, the
   bot follows the session's chat permissions — only members with writer
   rights can trigger control actions (see below).

Bot replies are localized: the bot replies in the language of the user
session (English, Vietnamese, Chinese, Korean or Russian).

## Commands

The bot exposes localized slash commands. The essentials:

| Command | Purpose |
|---------|---------|
| `/start`, `/help` | Register the chat and list what the bot can do |
| `/subagents` | List your agent's subagent tasks with live status |
| `/gc:` commands | Slash command palette for quick actions inside the chat |

`/subagents` rows use one status vocabulary shared with the web UI:
`queued` → `running` → `waiting` → `completed` / `failed` / `cancelled`.
Each task links to a detail view with its summary and result.

### Subagent archive buttons (coming)

Finished (`completed`/`failed`/`cancelled`) tasks listed by `/subagents` are
getting one-tap **archive** buttons — one per task plus an "archive all
completed" action. Archiving removes a task from the default list on every
surface (web panel and Telegram alike); running tasks cannot be archived
until they reach a terminal state, and in groups only chat writers may
archive.

## How replies are formatted

Model output goes through a dedicated formatting pipeline before Telegram
will accept it:

```
LLM output
  → SanitizeAssistantContent()      strip markup Telegram rejects
  → markdownToTelegramHTML()        markdown → Telegram HTML subset
  → chunkHTML()                     split within Telegram size limits
  → sendHTML()                      deliver
```

Tables are rendered as **ASCII inside `<pre>` tags** — Telegram has no table
markup — and long replies arrive as multiple messages chunked at safe
boundaries.

## Interactive elements

- **Inline pickers** — the bot answers option questions from the
  `ask_options` tool with inline keyboard buttons (1–4 options), same as the
  web chat.
- **Paged skills** — skill listings are paginated with inline navigation
  instead of one giant message.
- **Run announcements** — when a subagent finishes, the conversation gets an
  announcement message so you can pick up the result without opening the web
  UI.

## Notes for group chats

- The bot only reacts to messages it's configured to see (mentions, replies,
  commands) depending on the session preferences.
- Control actions (archiving subagents, task operations) require **writer**
  permission in the group.
- Telegram limits message edits to ~48 hours — for older listings the bot
  sends a fresh confirmation message instead of editing the original.
