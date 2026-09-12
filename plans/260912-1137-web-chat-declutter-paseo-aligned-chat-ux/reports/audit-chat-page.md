# Audit: /chat page — element inventory + redundancy evidence

_Ngày 2026-09-12. Explore agent, very thorough. Repo `C:\Users\DORA\Downloads\goclaw-mod` (dev @ 2e421795). Live confirm trên http://192.168.1.103:18790/chat (3.18.0)._

## Layout state machine (`chat-page.tsx`)

- Route `/chat/:sessionKey?` (`lib/routes.ts:4-5`, mount `routes.tsx:173`) trong `AppLayout` → **luôn có** global nav (w-64/16) + Topbar quanh chat. Desktop /chat có thể 3 cột: global nav (256px) + chat sidebar (w-72=288px) + right panel (w-72).
- State: `urlSessionKey` (source of truth) → `agentConfirmed`; `isMobile` (≤768px) → drawer + panel overlay; `taskPanelOpen` auto-open/close theo team tasks (`:167-174`); `workspaceId` local-only KHÔNG persist, KHÔNG liên kết session/agent (`:160`); `filesPanelOpen/jobsPanelOpen/termOpen` mutual exclusion (`:325-342`).
- Composer gating: `!isOwn` → read-only banner; `!agentConfirmed` → AgentPickerPrompt; else ChatInput (`:288-304`).

## Inventory theo vùng (tóm tắt — bảng đầy đủ trong transcript audit)

**Global chrome**: nav ~30 items/7 nhóm (sidebar.tsx:89-158) + topbar (lang/timezone/settings/theme/user).

**Chat sidebar (chat-sidebar.tsx:20-62)**: AgentSelector (GET /v1/agents) · "Trò chuyện mới" (buildNewSessionKey) · SessionSwitcher (WS `sessions.list {channel:"ws"}` — không search, subset của Sessions page) · mobile drawer.

**Top bar (chat-top-bar.tsx:45-192)**: agent emoji+tên (fetch /v1/agents riêng `:56`) · context badge (`:111-127` — UI CHẾT: sessions.list không trả estimatedTokens/contextWindow/compactionCount, verify `internal/gateway/methods/sessions.go:130-133` chỉ có key/label/model/metadata) · WorkspacePicker luôn render (`:130`) · toggle files (`:133-140`) · toggle jobs (`:141-148`) · toggle terminal (`:150-158`, disable khi chưa chọn workspace) · run phase text+spinner (`:159-163`) · chip "Team: N tasks" (`:164-173`) · "Ready" (`:174-176`).

**Thread (chat-thread.tsx:77-204)**: MessageBubble · RichContent (forward/reply/location/video-notice — pattern channel, hiếm khi xuất hiện vì sidebar chỉ list session ws) · MediaGallery + ImageLightbox · MergedToolGroup · ToolCallCard · ThinkingBlock · **SystemNotification** (team events pseudo-messages, use-chat-team-tasks.ts:96-109) · **TeamActivityPanel inline** (`:165`) · ActiveRunZone (streaming + **ActivityIndicator** — trùng phase text top bar) · DropZone.

**Composer (chat-input.tsx:43-346 + composer-toolbar.tsx)**: file chips + hidden input + Paperclip · CommandPalette slash `/` (14 `gc:*` hardcoded `command-palette.tsx:38-53` + skills WS skills.list; chọn chỉ set text `:77-83`) · textarea auto-resize IME-safe · voice recorder waveform · toolbar: Provider select (`:71-100`, override localStorage `goclaw.composer-override` `chat-input.tsx:26-64`) · Model select (`:103-127`) · Thinking select (`:130-156`) · Mic / Cancel+Stop / Send+Stop-abort (`chat.abort`) / Send.

**Right panels (w-72)**: TaskPanel (teamTasks — TRÙNG Teams page + TeamActivityPanel + chip + SystemNotification) · FileExplorerPanel (WS workspace.files.list/read — **read-only by design**, header comment "write/mkdir arrive with editor integration" chưa từng land; WORKSPACE_FILES_WRITE/MKDIR có trong protocol.ts:100-102 nhưng không UI nào gọi) · JobsTasksPanel (Jobs tab: jobs.list/cancel — TRÙNG Runs page + Cron; Tasks tab: tasks.tree — hệ task THỨ BA surfaced trong chat) · TerminalPanel (xterm PTY, WS terminal.*).

**Overlays**: delete-session dialog · media/file preview · image lightbox · AgentSelector dropdown · WorkspacePicker dropdown (+inline create) · CommandPalette popup · mobile backdrops.

## Overlap với page riêng (10 cặp)

1. SessionSwitcher ↔ `/sessions` (page giàu hơn: search/pagination/all-channels/reset/compact/branch/run-timeline)
2. TaskPanel + TeamActivityPanel + SystemNotification ↔ `/teams` task board (3 bản render trong chat)
3. JobsTasksPanel Jobs ↔ `/runs` + `/cron`
4. JobsTasksPanel Tasks (tasks.tree) ↔ Teams tasks + TaskPanel (khái niệm task thứ 3)
5. FileExplorerPanel ↔ Teams workspace files (teams.workspace.*) — 2 hệ file browser song song
6. TerminalPanel ↔ `/workstations`
7. Palette skills ↔ `/skills`; gc:status/runs ↔ `/runs`
8. Composer provider/model ↔ `/providers` + agent model config
9. ToolCallCard ↔ RunReplayPage ToolItem + RunTimelinePanel (3 renderer tool)
10. Top-bar phase label ↔ ActivityIndicator (cùng chuỗi, 2 nơi, 1 màn hình) — và AgentSelector ↔ AgentPickerPrompt (2 picker) + **3 fetch /v1/agents** (`agent-selector.tsx:33`, `agent-picker-prompt.tsx:24`, `chat-top-bar.tsx:56`)

## Dead / useless (bằng chứng)

1. Palette chỉ chèn text (`command-palette.tsx:77-83`); 5/14 lệnh là control commands trùng page
2. SystemNotification pseudo-messages + panels = 3 surface cùng event
3. "Ready" — noise (`chat-top-bar.tsx:174-176`)
4. Context badge — dead khi backend không populate (đã verify response struct)
5. Workspace axis = 4/8 top-bar control, không liên quan conversation; panels mở ra chỉ empty state khi chưa có workspace
6. RichContent channel badges gần như không xuất hiện trên route này (sidebar chỉ list channel "ws")
7. FileExplorerPanel read-only (write/mkdir chưa bao giờ được wiring UI)
8. TaskPanel auto-open ép panel mở giữa chừng chat
9. Không có edition gating nào trong /chat
10. Zero TODO/FIXME trong pages/chat + components/chat (comment "Phase 3/4/5" là dấu vết plan cũ)
11. Hardcoded EN strings: phase labels `chat-top-bar.tsx:36-43`, "Team: N task(s)" `:171`, TaskPanel `task-panel.tsx:22,36`, "Team: N tasks active" `team-activity-panel.tsx:21`, "Drop files here" `drop-zone.tsx:41`, GC_COMMANDS descriptions `command-palette.tsx:38-53` — vi phạm i18n 5-locale (en/ko/ru/vi/zh)
12. Triple /v1/agents fetch — không share cache

## Live confirm (192.168.1.103:18790/chat, domSnapshot 2026-09-12)

Top bar thực tế: `Không có workspace` · `Open files panel` · `Open jobs/tasks panel` · `Chọn một workspace để mở terminal` [disabled] · `Ready`. Thread: empty state + `Chọn agent` prompt grid (`🦊 Fox Spirit Default`). Sidebar: `Chọn agent` + `Trò chuyện mới` + 2 sessions. → Đúng từng điểm audit.

## PHỤ LỤC — Sửa lỗi sau red-team (2026-09-12)

- **SAI: "Context badge là UI chết / sessions.list chỉ trả key/label/model/metadata."** Struct `sessions.go:130-133` tôi từng dẫn là **params của `handlePatch`**, không phải response list. Thật: handler list (`sessions.go:72-76`) gọi `ListPagedRich` và trả `result.Sessions` nguyên trạng — `SessionInfoRich` (`session_store.go:95-104`) có đủ `estimatedTokens` (computed: `metadata.last_prompt_tokens` fallback `octet_length/4+12000`, SQL `sessions_list.go:175-181`), `contextWindow` (join `agents.context_window` COALESCE 200000), `compactionCount`. SQLite cũng có `ListPagedRich` (+ integration test). Pipeline ghi `last_prompt_tokens` (`loop_history_sanitize.go:433`); compaction ghi `last_compaction_at` (`loop_pipeline_callbacks.go:916`). Badge chỉ không hiện trên live snapshot vì chưa chọn session nào (`session` prop null). Vấn đề thật của badge: chỉ tooltip, `hidden sm:flex` mất mobile, không popover → Phase 3 re-scope thành UX upgrade.
- **ĐÃ XÁC NHẬN ĐÚNG**: 3 fetch /v1/agents; 3 surface team task; 2 picker (AgentSelector uncontrolled — useState open `agent-selector.tsx:26`); palette 4 lệnh control (`command-palette.tsx:49-52`), override key `goclaw.composer-override` (`chat-input.tsx:26`); 3 component định xóa chỉ được import bởi `chat-page.tsx:13,19` + `chat-thread.tsx:7` (an toàn xóa); dự án KHÔNG có Radix Popover wrapper — dropdowns là custom `createPortal` (`workspace-picker.tsx:93`); `session.updated` là event protocol có thật (`ui/web/src/api/protocol.ts:322`); 5 locale đều có chat.json.
