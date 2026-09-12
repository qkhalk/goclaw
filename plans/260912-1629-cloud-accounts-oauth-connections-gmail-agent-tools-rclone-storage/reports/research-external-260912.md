# Research Report — External (openclaw, rclone, Gmail API, OAuth, prior art)

Ngày: 2026-09-12. Researcher agent (WebFetch trực tiếp nguồn chính; web-search bị rate-limit phiên này).

## 1. OpenClaw (openclaw/openclaw)

- Personal AI assistant / agent gateway, Node.js/TypeScript pnpm workspace, MIT. Lịch sử tên: Warelay → CLAWDIS → Clawdbot → Moltbot → OpenClaw (Wikipedia: https://en.wikipedia.org/wiki/OpenClaw). Docs: https://docs.openclaw.ai/
- **KHÔNG có OAuth cloud connectors**: channels chỉ chat (Discord, Google Chat, iMessage, MS Teams, Signal, Slack, Telegram, WhatsApp, WebChat; Matrix/Nostr/Twitch/Zalo/Feishu qua plugins). Tool list (https://docs.openclaw.ai/tools) không có email/Gmail/calendar/drive.
- Skills = SKILL.md markdown dạy agent dùng tool sẵn có ("an eligible skill does not grant tool access" — https://docs.openclaw.ai/tools/skills). Third-party qua ClawHub (https://docs.openclaw.ai/clawhub).
- Token storage không tài liệu hóa; v2.0 bị chê unencrypted secrets at rest + sandbox off default (Wikipedia). → GoClaw Cloud section là khác biệt hóa, và phải mã hóa token từ ngày 1.

## 2. rclone (rclone/rclone)

- Backends cho "Cloud": Google Drive (T1), OneDrive (T1), Dropbox (T1), S3 (T1), Google Photos (T5), GCS (T1), Box/Mega/pCloud/Proton/Storj/WebDAV/SFTP/SMB/Azure/B2 (https://rclone.org/overview/). **KHÔNG có backend mail nào** (grep imap/gmail/mail = 0; "Mail.ru Cloud" là storage).
- Library Go: module `github.com/rclone/rclone` MIT, tag v1.75.1 (2026-09-04) — nhưng là application-first, kéo ~40 backend deps + global state, không cam kết API ổn định (wiki "Importing rclone" không fetch được — unverified).
- **rc API (khuyến nghị)**: `rclone rcd --rc-addr :5572` + `--rc-user/--rc-pass`; mọi call POST JSON (https://rclone.org/rc/):
  - `operations/`: list (opt incl. metadata), stat, mkdir/rmdirs/purge, delete, copyfile, movefile, uploadfile, about (quota), size, publiclink, fsinfo, cleanup.
  - `sync/`: copy, move, sync, bisync (`_async=true`).
  - `core/`: command (**shell-equivalent — cấm dùng**), stats, version, ping-via-version, quit, obscure.
  - `config/`: create, update, delete, get, listremotes, dump (**leak credentials — cấm gọi**).
  - `job/`: poll async.
  - Bảo mật: "rc access is equivalent to shell access as the user running rclone"; CORS allow-all; auth all-or-nothing → chỉ loopback + strong basic-auth + không expose.
- `rclone authorize` (https://rclone.org/commands/rclone_authorize/): backend Drive tên **`drive`**; flow headless (https://rclone.org/remote_setup/): `rclone config` trên server → "n" → chạy `rclone authorize "drive"` trên máy có browser → paste token block về. GoClaw KHÔNG cần runtime — gateway tự chạy web flow + inject token qua rc config/create.
- Config: INI (`type = drive`…), password obfuscation (reversible KHÔNG phải encryption — `rclone obscure`); remotes có thể tạo qua rc JSON; env `RCLONE_CONFIG_<NAME>_<OPTION>` (well-known, docs page bị truncate khi fetch — flag).

## 3. Mail: Gmail API vs IMAP vs Graph

- **Gmail API** (https://developers.google.com/gmail/api):
  - Scopes (https://developers.google.com/gmail/api/auth/scopes): `gmail.labels` non-sensitive; `gmail.readonly`/`gmail.modify` restricted; `https://mail.google.com/` restricted full (chỉ dùng khi cần delete vĩnh viễn — v1 không dùng).
  - Search: `GET /gmail/v1/users/{userId}/messages?q=<gmail-search>&labelIds&maxResults≤500&pageToken` → trả id/threadId; body cần `messages.get` (https://developers.google.com/gmail/api/reference/rest/v1/users.messages/list).
  - Archive/trash: `users.messages.modify` `addLabelIds[]/removeLabelIds[]` (gỡ INBOX = archive; thêm TRASH) (https://developers.google.com/gmail/api/reference/rest/v1/users.messages/modify). `delete/batchDelete` = vĩnh viễn — không implement v1.
- **IMAP generic**: `github.com/emersion/go-imap/v2` còn "in development"; Gmail-qua-IMAP XOAUTH2 **bắt buộc scope `https://mail.google.com/`** rộng nhất, Google khuyến nghị chuyển sang API (https://developers.google.com/gmail/imap/xoauth2-protocol) → loại v1.
- **Microsoft Graph** (https://learn.microsoft.com/en-us/graph/api/user-list-messages): `GET /v1.0/me/messages` OData `$search/$filter/$top≤1000`; delegated `Mail.ReadBasic/Mail.Read/Mail.ReadWrite`; move `POST /me/messages/{id}/move` — backlog v2.

## 4. OAuth clients

- **Google web-server flow** (https://developers.google.com/identity/protocols/oauth2/web-server): client type "Web application", redirect URI exact-match (scheme/host/trailing slash); HTTPS bắt buộc trừ localhost; state CSRF; token endpoint POST code. `access_type=offline` mới có refresh token; `prompt=consent` re-issue.
  - Testing vs production (https://developers.google.com/identity/protocols/oauth2; https://support.google.com/cloud/answer/13464323): testing refresh token **7 ngày** + **100 test users** + "unverified app" screen. Internal org app (Workspace) không bị. → **BYO client per install**; docs phải chỉ dẫn publish-to-production unverified.
  - Refresh token invalidation: user revoke, 6 tháng không dùng, đổi password (Gmail scopes), token count limit per client-user (cũ bị silent invalidate).
  - PKCE cho web-server: doc fetch không thấy nhắc (unverified) — vẫn tự áp S256 (tốt hơn, không hại).
- **Microsoft Entra v2** (https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-auth-code-flow): PKCE "recommended for all application types"; `offline_access` phải xin riêng (https://learn.microsoft.com/en-us/entra/identity-platform/scopes-oidc); RT 90 ngày default, rotate-on-use giữ cũ đến hết hạn (https://learn.microsoft.com/en-us/entra/identity-platform/refresh-tokens) — lưu RT mới nhất mỗi lần refresh.

## 5. Prior art + ranking features

- **n8n** (https://docs.n8n.io): model tốt nhất cho UI "Connections" — credential per-service, OAuth, allowed-domains chống exfiltration, test-on-save, encryption key riêng (https://docs.n8n.io/deploy/host-n8n/configure-n8n/basic-configuration/configuration-examples/set-a-custom-encryption-key.md) → mirroring GoClaw pattern llm_providers.
- **Khoj** (https://github.com/khoj-ai/khoj): RAG docs local, automations/newsletter — không có OAuth connectors. **Letta/MemGPT**: channels only (unverified sâu). **ChatGPT connectors**: không verify được (403).

Ranking (value/effort) cho GoClaw Cloud:
1. Gmail read/search — 2. Archive/trash/labels — 3. One-click unsubscribe (RFC 8058) — 4. rclone storage browse/read (mở ~40 backend) — 5. Cloud→workspace ingest — 6. Workspace backup/sync — 7. Google Calendar — 8. Outlook/Graph — 9. Google Photos — 10. Quota panel — 11. Contacts — 12. IMAP generic (defer).

## RFC 8058 unsubscribe (https://datatracker.ietf.org/doc/html/rfc8058)

- Headers `List-Unsubscribe` (https URI) + `List-Unsubscribe-Post: List-Unsubscribe=One-Click` → POST body `List-Unsubscribe=One-Click` (form-urlencoded/multipart), no cookies/auth, sender không redirect, DKIM-signed. **"The mail receiver MUST NOT perform a POST on the HTTPS URI without user consent."** Fallbacks: mailto: entry (gửi từ địa chỉ user), GET https (fetch + confirm). → GoClaw tool phải consent-gated.

## Khuyến nghị chốt (đã đưa vào plan)

1. Storage: rclone binary + rc API (KHÔNG import library).
2. OAuth: BYO client, gateway web flow, HMAC state, token AES-256-GCM trong bảng riêng.
3. Mail v1: Gmail API (`gmail.readonly`+`gmail.labels`+`gmail.modify`); không delete vĩnh viễn; unsubscribe consent-gated.
4. Agent tools gating trước model call + UI Cloud page + docs verification-aware.
