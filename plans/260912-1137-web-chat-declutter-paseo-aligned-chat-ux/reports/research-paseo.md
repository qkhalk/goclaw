# Research: Paseo (getpaseo/paseo) — chat UX inventory

_Ngày 2026-09-12. Nguồn: github.com/getpaseo/paseo (README, releases trang 1-5/12/19, source tree `packages/app/src/{composer,timeline,tool-calls,command-center}`), paseo.sh + /docs (workspaces, skills, schedules-chat, voice, web-ui), HN items 47397226 / 47530027 / 48377250 (92 pts), mockup hero/mobile._

## (a) Product overview

- Paseo = **"control plane for coding agents"** (orchestrates Claude Code, Codex, Copilot, OpenCode, Pi, +34 via ACP), self-hosted, Apache-2.0, ~17k sao, TypeScript monorepo, solo dev Mohamed Boudra. KHÔNG phải browser extension, KHÔNG liên quan OpenClaw/Clawdbot.
- Platforms: Desktop (Electron), Mobile (iOS/Android Expo, "full parity"), Web (app.paseo.sh), CLI, VS Code extension, headless daemon. Local daemon + WS; optional E2EE relay.
- Triết lý: local-first, no telemetry, no login; "optionality and freedom of choice".

## (b) Chat interface inventory

**Composer**: placeholder dạy affordance ("Message the agent, tag @files, or use /commands and /skills"); @file mentions; attachment pills + image paste; slash commands + skills (/paseo, /paseo-handoff, /paseo-committee, /paseo-advisor); autocomplete; **model/effort/reasoning pills inline** (v0.2+); **contextual pills trên composer** — subagent/task trackers (v0.5.0), live workspace change counts, diff-stat pill; **live task progress above the composer** (v0.4.0); **steering** — send vào turn đang chạy thay vì interrupt (v0.5.0); **answer forms/question cards** khi agent hỏi (v0.1.104, v0.8.0) — inline buttons "Looks good"/"Needs work"; dictation + voice mode; persistent drafts; plugin composer pills.

**Timeline (không bubbles)**: markdown prose + compact tool-call rows + summary blocks; steady-rate streaming (v0.7.0); tool calls collapse 1 item (v0.1.108), configurable detail-level, badge mở file trực tiếp; reasoning blocks + setting auto-expand; **subagent track** (v0.1.107); images persist as attachments; **interactive Mermaid** (v0.4.0); rich file links `foo.ts:42`; partial-selection copy giữ format; **context meter** per-session (được khen, fix "no longer blanks out" v0.2.4); reading-position preservation khi reload/expand; infinite scroll older history (v0.2.3); 32k render cap (v0.7.2); rewind; plugin-rendered timeline items (v0.8.0).

**Sessions/workspaces**: IA = **Projects → Workspaces → Sessions** — "Paseo is organized around workspaces, not chats"; session mở dạng tab; statuses ready-to-review/working/done + pin; fork từ failed turn (v0.1.108) / fork đang chạy (v0.3.0); history search by workspace/agent/branch (v0.3.0); auto titles/branch/PR drafts (v0.4.0); recent chats sống background.

**Panels quanh chat**: Explorer pane riêng có tabs (Files/Changes/PR, v0.5-0.6); diff/review view; terminal panes; in-app browser agent điều khiển được; side panel độc lập; mobile = slide-over sheets.

**Commands**: **Command Center** (Cmd/Ctrl+P) — workspace + file search, git actions, đổi model/reasoning/mode/plan, panel/tab actions, relevance ranking (v0.2→v0.6.1). **Scheduled chats bằng ngôn ngữ tự nhiên** qua MCP + Heartbeats (agent tự đặt lịch).

## (c) UX được khen (HN 92pts)

"Very focused and clean UI"; mobile = killer app ("unglued me from my computer", "multi-agent workflow unlocked"); "focus on usability instead of adding endless features". Chê: design "unopinionated" neutral grey. Signature: workspaces-not-chats; non-destructive steering; **performance obsession as UX** (steady streaming, frame budget, in-memory timeline, reading position); conversational control plane; local-first.

## (d) Deliberately OUT of chat view

| OUT | Vị trí |
|---|---|
| Provider/model setup, permissions, skills install, host pairing | Settings screens |
| Schedules management | Schedules screen + CLI (tạo có thể qua chat MCP) |
| History/archive + search | History surface riêng |
| Files/changes/PR review | Explorer pane + Review view (tab riêng, không trong chat column) |
| Terminal, browser | Pane/tab riêng trong workspace |
| Global actions + model switching | Command Center palette |
| Automation triggers | Paseo Hub (sản phẩm riêng) |

→ **Bài học cho GoClaw /chat**: GoClaw đã có page riêng cho tất cả các thứ trên (providers/skills/cron/sessions/runs...) — /chat chỉ cần composer + timeline + session list + pill tracker + context meter; console panels thu hẹp lại, không chiếm chrome thường trực.

## (e) Releases gần đây (chat-related)

v0.1.60-63 (attachments pills/lightbox, file-link badges) → v0.1.104-108 (question cards, autocomplete, fork, collapsed tool summaries, subagent track, pin) → v0.2.x (Command Center model switching, add files from panes, history restore, context meter fix) → v0.3.x (fork running, partial copy, history search, mobile terminal) → v0.4.0 (live task progress above composer, Mermaid, auto titles) → v0.5.x (**steering**, tracker pills above composer, Explorer tabbed pane, pane tabs) → v0.6.x (Explorer layout, palette ranking) → v0.7.x (Apache-2.0, steady-rate streaming, plugins timeline, rewind, 32k cap) → v0.8.0 (2026-09-10: Codex answer forms, plugin composer actions, session import).
