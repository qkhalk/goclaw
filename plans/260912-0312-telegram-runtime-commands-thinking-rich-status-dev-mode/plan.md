---
title: "Telegram runtime commands: thinking, rich status, dev mode"
description: "Nâng cấp runtime UX trên Telegram: lệnh /thinking bật-tắt mức suy luận theo chat, /status mở rộng (version, uptime gateway+system, model/session, cost, context, compactions, queue, link docs nội bộ) kèm verbosity toggle, /dev bật dev mode chuyên code với hành vi hỏi xác nhận như CLI agent, docs page trong repo thay link openclaw, bộ skill kiểm thử hiện trong menu / + nâng cấp sâu + 3 skill mới, và fix đường video Telegram→agent (download retry + provider video)."
status: completed
priority: P1
effort: "3d"
tags: [telegram, ux, thinking, status, dev-mode, skills, docs]
created: 2026-09-12
---

# Telegram runtime commands: thinking, rich status, dev mode

## Overview

Anh muốn trải nghiệm GoClaw trên Telegram gần với CLI agent (zcode/claude code): điều khiển được mức thinking, nhìn thấy trạng thái runtime đầy đủ (kiểu status của OpenClaw), có dev mode chuyên code tự hỏi xác nhận khi chưa rõ, và docs nội bộ thay link `docs.openclaw.ai`.

**Phạm vi hôm nay tách làm 2 nhóm:**

- **ĐÃ XONG** (plan `260910-0216-telegram-skill-menu-and-testing-skills`, đã lên production từ 3.17.x — verify bằng code hiện tại): menu `/` hiện skill hay dùng qua `menu_skills` (`internal/config/config_channels.go:92`, default `cook/plan/fix/review/test` qua `defaultMenuSkills` `commands_skills.go:32`), lệnh `/skills` liệt kê skill + mô tả (`commands_skills.go:85-122`), 4 skill kiểm thử `security-audit` / `loadtest` (L7) / `netstress` (L4) / `ssl-audit` đã trong `skills/`, Docker full variant có sẵn `nmap nikto wrk hey iperf3` (`Dockerfile:92-98`). **Không làm lại.**
- **MỚI (plan này):** `/thinking`, `/status` mở rộng + toggle verbosity, `/dev` (dev mode), docs page trong repo, **testing skill suite hiện trong menu `/` + nâng cấp sâu 4 skill + 3 skill mới**, **fix đường video input** (2 nguyên nhân production-verified: download body không retry + không có provider video nào cấu hình).

Ví dụ `/status` mục tiêu (ánh xạ sang dữ liệu GoClaw THẬT có — mỗi dòng ghi nguồn ở Phase 3):

```
🦊 GoClaw 3.17.9
⏱️ Uptime: gateway 2d 3h · system 5d 21h
🤖 Agent: fox-spirit · 🧠 Model: oc/mimo-v2.5-free
🧵 Session: agent:fox-spirit:telegram:direct:386246614 · updated 5m ago
💵 Cost (session): $0.0074 · 🔢 Tokens: 12.3k in / 4.5k out
📚 Context: 37k/200k (18%) · 🧹 Compactions: 0
⚙️ Think: high (chat) · Mode: dev · 🪢 Queue: main 2 active / 1 pending
📖 Docs: https://github.com/qkhalk/goclaw/blob/dev/docs/25-telegram-runtime-commands.md
```

## Scope Challenge (Step 0)

- **Existing code (tái dùng, không viết lại):**
  - Thinking là subsystem hoàn chỉnh: 3 lớp cấu hình (agent `thinking_level`/`reasoning_config` — `internal/store/agent_store.go:76,81`; provider defaults; per-request `RunRequest.ThinkingLevelOverride` — `loop_types.go:721`) + resolution engine `providers.ResolveReasoningDecision` (`reasoning_resolution.go:49-118`), override thắng agent config tại `loop_pipeline_callbacks.go:572-574`. Path `chat.send` đã validate mức (`gateway/methods/chat.go:163-168`). Chỉ thiếu: điểm điều khiển từ Telegram.
  - Per-chat persistence có sẵn: `sessions.metadata` JSONB (migration `000011`), API `GetSessionMetadata`/`SetSessionMetadata` (interface `session_store.go:135-152`, PG `pg/sessions.go:259-279`, SQLite `sqlitestore/sessions.go:188-206`), WS method đã dùng pattern `SetSessionMetadata` + `Save` (`gateway/methods/sessions.go:169-173`). Save persist cột metadata (`pg/sessions_list.go:235,255,265`).
  - Session key builder exported: `sessions.BuildSessionKey` / `BuildGroupTopicSessionKey` / `BuildDMThreadSessionKey` (`internal/sessions/key.go:48,56,64`).
  - `/status` hiện tại là stub 3 dòng hardcode (`commands.go:200-205`) — thay hoàn toàn, không phá contract nào.
  - Dữ liệu rich-status đều có nguồn: version `cmd.Version` (`cmd/root.go:12`), gateway uptime `Server.StartedAt()` (`gateway/server.go:1030`, server tạo tại `cmd/gateway.go:646`), lane queue `Scheduler.LaneStats()` (`scheduler/lanes.go:112-128`, scheduler tạo `cmd/gateway.go:1175` — hiện chưa có caller nào ngoài package, đây là lần đầu expose), session tokens/compactions/context window từ `SessionData` (`session_store.go:27-55`), cost theo session từ `traces.total_cost` + index `idx_traces_session` (migration 000001:340,364). Thiếu duy nhất: system uptime → đọc `/proc/uptime` (Linux-only, omit nếu không có).
  - ask_user đã tồn tại như reminder-model (`tools/team_tasks_followup.go:12-61`, ticker `tasks/task_ticker.go:338-434`, auto-clear khi user trả lời `gateway_consumer_normal.go:202-212`) — KHÔNG pause run. Dev mode dùng prompt guidance + ask_user reminder; không xây pause/resume mới (InjectCh `loop_types.go:729-732` là cơ chế riêng cho mid-run injection, out of scope).
  - PromptMode 4 lớp đã có (`systemprompt.go:39-87`) nhưng là auto-detect theo session type — dev mode KHÔNG map vào PromptMode (vẫn `full`), mà inject section riêng qua `ExtraPrompt` (`loop_history.go:265`).
- **Requested scope:** 5 mục của anh (thinking, rich status + toggle, docs nội bộ, dev mode, thêm skills) — deliver đủ, không cắt.
- **Complexity:** ~25 file sửa/tạo, 0 service mới (2 interface nhỏ `StatusProvider`/`SessionPrefsStore` + 1 store method `SessionTotalCost` + 1 field config `MenuTestingSkills`). 5 phase: 1-3 tuần tự (chung nền prefs), 4-5 độc lập chạy song song được. Gộp hơn sẽ rối trách nhiệm — giữ 5.
- **Selected mode: HOLD SCOPE** (auto-detect → hard mode: research đã chạy 3 scout song song + verify tay; red-team chạy sau khi viết plan).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | `/thinking [off\|low\|medium\|high\|auto\|adaptive\|...]` — xem/bật/tắt mức thinking theo chat, persist qua session metadata, consumer áp vào `ThinkingLevelOverride` từ tin nhắn kế tiếp | P1 |
| 2 | `/dev on\|off` — dev mode chuyên code: inject prompt section (plan-first, hỏi xác nhận khi chưa rõ, confirm trước thao tác phá hủy), hiện trong /status | P1 |
| 3 | `/status` rich (theo mẫu trên) + `/status full\|short` toggle verbosity per-chat; group mặc định short, DM mặc định full | P1 |
| 6 | Docs page `docs/25-telegram-runtime-commands.md` trong repo (gồm mục Video input + hướng dẫn testing skills), link từ /status thay cho link openclaw | P2 |
| 4 | Testing skill suite: hiện trong menu `/` (config `menu_testing_skills`, sanitize `-`→`_` cho command + matching `_`≡`-`), nâng cấp sâu 4 skill theo khung chuẩn (gate/pre-flight/phases/abort/report), thêm `recon` + `fuzz` + `dns-audit` | P1 |
| 5 | Video input reliability: retry tải body trong `downloadMedia` (production-verified: connection reset VN→Telegram DC làm mất video) + ops cấu hình provider video (Gemini/OpenRouter) + docs | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Nền per-chat prefs + /thinking](./phase-01-start.md) | Pending |
| 2 | [Phase 2: Dev mode /dev](./phase-02-dev-mode.md) | Pending |
| 3 | [Phase 3: Rich /status + verbosity + docs page](./phase-03-rich-status.md) | Pending |
| 4 | [Phase 4: Testing skill suite — menu /, deep upgrade, skills mới](./phase-04-extra-skills.md) | Pending |
| 5 | [Phase 5: Video input reliability](./phase-05-video-input-reliability.md) | Pending |

Thứ tự: 1 → 2 → 3 tuần tự (chung nền prefs; 3 hiển thị kết quả 1+2). Phase 4 và Phase 5 độc lập hoàn toàn, chạy song song được với 1-3.

## Success Criteria

- [x] `/thinking` không tham số → hiện mức hiện tại (chat override + default của agent); tham số hợp lệ → persist + xác nhận; tham số sai → list mức hợp lệ. Từ tin kế tiếp, trace/span thấy reasoning effort đúng mức (verify qua span input hoặc log `thinking_level`).
- [x] `/thinking off` tắt THẬT: override="off" → `RequestEffort()` rỗng → option `OptThinkingLevel` vắng mặt trong request LLM (semantics đã audit-verify: `reasoning_resolution.go:62-68` short-circuit "off", `:120-125` `RequestEffort()` trả "" khi off/absent; mọi provider guard `!= "" && != "off"` — anthropic_request.go:228, openai_request.go:267, codex_build.go:139; Ollama ép `think=false` openai_request.go:323).
- [x] `/thinking default` (keyword riêng) xóa override → về cấu hình agent. `none` bị TỪ CHỐI trong lệnh TG (audit: "none" pass-through và BẬT thinking 10k budget trên Claude — anthropic_request.go:278-289 default branch). Caveat đã biết, ghi docs: route Gemini-compat không tắt được thinking khi "off" (mặc định high — openai_request.go:273-284).
- [x] Group: `/thinking`, `/dev` yêu cầu quyền writer (reuse `CheckPermission` như `/reset` `commands.go:107-124`, copy cả hành vi fail-open khi DB lỗi `:115-117`); DM: tự do.
- [x] `/dev on` → system prompt chứa dev section (verify span input_preview); agent hỏi lại câu hỏi làm rõ thay vì đoán khi yêu cầu mơ hồ (thử nghiệm thủ công + eval prompt có trong phase).
- [x] `/status` full hiển thị đủ 8 dòng mẫu; short = 4 dòng (version, uptime, model, session updated); `/status full|short` persist per-chat; session chưa tồn tại → "No session yet" grace.
- [x] Restart gateway: các toggle (thinking/dev/verbosity) còn nguyên (metadata persist DB, không chỉ in-memory).
- [x] Gõ `/` trong Telegram: thấy nhóm testing skill (`/security_audit`, `/loadtest`, `/netstress`, `/ssl_audit` + 3 skill mới) — slug có gạch ngang không còn bị loại; gõ tay `/security_audit <target>` và bấm menu đều kích hoạt đúng skill; `/loadtest` cũ không regression matching.
- [x] 4 SKILL.md kiểm thử nâng cấp đủ khung: authorization gate / pre-flight / phases / abort criteria / report format.
- [x] Video: gửi lại video trên Telegram sau deploy → tải về thành công (nếu route reset, log thấy retry attempt 2+ thay vì fail ngay); sau ops thêm provider video → agent mô tả được nội dung video.
- [x] 3 skill mới seed lúc startup, frontmatter parse sạch, deps check không chặn gateway khi thiếu tool (status `archived` + `missing_deps` hiển thị trong `/skills` pattern hiện có).
- [x] `go build ./...` + `go build -tags sqliteonly ./...` + `go vet ./...` sạch; test mới pass cả PG logic (unit + fake) lẫn SQLite (metadata methods đã có — chỉ cần không regress).
- [x] Không migration schema nào (metadata column đã có từ 000011; SQLite schema verify có cột — nếu thiếu thì phase 1 phải patch theo luật dual-DB, xem Risk).

## Cross-Plan Relationships

- **Supersedes phần chưa làm của** `260910-0216-telegram-skill-menu-and-testing-skills`: plan đó đã implement đủ 3 phase (menu + /skills + 4 skill test + Docker packages) và sẽ được đóng. Plan này chỉ thêm mới, không đụng file `commands_skills.go` trừ help text.
- `260910-0450-telegram-text-coalesce-and-version-stamp`: đã hoàn thành qua PR #58/#59 (deploy 3.17.8) — đóng luôn.
- Không plan mở nào khác đụng `internal/channels/telegram/` hay consumer.

## Surface Parity (kỳ vọng khi hoàn tất)

- **Gateway server:** telegram channel (3 lệnh mới + status provider wiring), config (không field mới — dùng session metadata, không thêm config), consumer (đọc metadata → ThinkingLevelOverride + prepend dev section vào ExtraSystemPrompt), `store.TracingStore` (+1 method đọc). Thay đổi chính.
- **API contract:** N/A — không thêm/đổi WS method hay HTTP endpoint; không field RunRequest mới; `SessionTotalCost` là method store interface internal (không wire). Session metadata đã có WS `sessions.setMetadata` từ trước, shape không đổi.
- **Web UI:** N/A — feature TG-only. Session metadata hiện không render ở UI nào nên không có consumer phải cập nhật.
- **CLI/runtime package:** N/A — không command CLI nào đọc các toggle này; skill files là dữ liệu bundled.
- **Docs:** tạo `docs/25-telegram-runtime-commands.md` (mới), cập nhật `docs/15-core-skills-system.md` (bundled list +3), `docs/06`-series channels doc nếu có mục lệnh (verify lúc viết).
- **Migrations:** KHÔNG migration — mọi persistence dùng cột có sẵn.

## Key References (verify-passed 2026-09-12)

Lệnh & wiring:
- `internal/channels/telegram/commands.go:75-262` — switch `handleBotCommand`; `/status` stub `:200-205`; `/reset` group gate pattern `:107-124`; `/help` text `:81-100`; `resolveAgentUUID` `:19-37`.
- `internal/channels/telegram/commands_pairing.go:162-183` — `DefaultMenuCommands()` (18 lệnh); `SyncMenuCommands` `:143-159` (DeleteMyCommands → SetMyCommands, chưa dùng scope).
- `internal/channels/telegram/commands_skills.go` — pattern inject + handler instant-reply (`:85-122`), `skillMenuCommands` `:60-81`, merge menu tại `channel.go:267-269`.
- `internal/channels/telegram/commands_tasks.go:52-131` — pattern handler (nil-store guard, send closure, truncateStr `:34-40`).
- `internal/channels/telegram/factory.go:62-77` — `FactoryWithStoresAndAudio`; caller DUY NHẤT của factory: `cmd/gateway.go:1049`. **LƯU Ý (audit):** config-branch `registerConfigChannels` (`cmd/gateway_channels_setup.go:33`) KHÔNG dùng factory — gọi `telegram.New(...)` trực tiếp kèm Options tại `:52` (nhánh chạy khi instanceLoader == nil). Mọi Option mới phải wire Ở CẢ HAI chỗ.
- `internal/channels/telegram/channel.go:27-62` — Channel struct + các store đã inject; Option pattern `:72-98`.
- telego v1.6.0: `SetMyCommandsParams` hỗ trợ `Scope` + `LanguageCode` (`methods.go:3189-3201`) — chưa cần, ghi chú cho tương lai (menu riêng DM/group).
- `telegramMaxMessageLen=4000` (`constants.go:6-8`), `chunkHTML` (`format.go:395-450`), `sendHTML` (`send.go:424`), `escapeHTML` (`format.go:206`).

Thinking:
- `internal/agent/loop_types.go:721` — `ThinkingLevelOverride` (validated levels + "adaptive").
- `internal/gateway/methods/chat.go:157,163-168` — `thinkingOverrideFor` (chuẩn giá trị hợp lệ để reuse).
- `internal/providers/reasoning_resolution.go:49-118,131-139` — `ResolveReasoningDecision`, `NormalizeReasoningEffort`.
- `internal/agent/loop_pipeline_callbacks.go:566-600` — áp override vào `chatReq.Options[OptThinkingLevel]` (`:572-574,595-596`).
- `internal/store/agent_store.go:76,81,391-430` — `thinking_level`, `reasoning_config`, `ResolveEffectiveReasoningConfig`.

Session metadata & keys:
- `internal/sessions/key.go:48,56,64` — `BuildSessionKey(agentKey, channel, kind, chatID)` = `agent:{agentKey}:{channel}:{kind}:{chatID}`; forum/DM-thread variants. **CRITICAL: dùng đúng builder này ở channel để khớp key consumer dùng** (xem Risk R1).
- `internal/store/session_store.go:27-55,135-152` — `SessionData` (Metadata `:49`), `SessionMetadataStore`.
- `internal/store/pg/sessions.go:259-279` — `Get/SetSessionMetadata` (Set dùng `getOrInit` — tự tạo in-memory nếu chưa có).
- `internal/store/pg/sessions_list.go:235,255,265` — Save persist cột `metadata` (UPDATE + INSERT + ON CONFLICT).
- `internal/gateway/methods/sessions.go:169-173` — pattern bắt buộc: `SetSessionMetadata(...)` rồi `Save(...)` mới xuống DB.
- `cmd/gateway_consumer_normal.go:482` — điểm build `agent.RunRequest{...}` (scope có sẵn `sessionKey` :83, `agentID` :37, `peerKind` :68, `agentLoop` :42); consumer deps field `SessStore` (`gateway_consumer.go:59`, đã là `store.SessionStore` đầy đủ).

Status data:
- `cmd/root.go:12` (`Version`), `cmd/gateway.go:646` (server), `internal/gateway/server.go:1030` (`StartedAt()`).
- `cmd/gateway.go:1175` (`sched := scheduler.NewScheduler`), `internal/scheduler/lanes.go:112-128` (`LaneStats{Name,Concurrency,Active,Pending}`), `scheduler.go:246-248`.
- Migration 000001: `traces` có `session_key` (`:340`), `total_cost` (`:351`), index `idx_traces_session` (`:364`); `sessions` tokens/compactions (`:139-142`), `last_prompt_tokens` (session_store.go:52-54).
- `store.TracingStore` interface TỒN TẠI (`internal/store/tracing_store.go:164-196`, đã có `GetMonthlyAgentCost`/`GetCostSummary`) + impls `pg/tracing.go`, `sqlitestore/tracing.go`; runtime write qua `tracing.Collector` (`internal/tracing/collector.go:120`, tạo tại `cmd/gateway.go:371`). → `SessionTotalCost` thêm thẳng vào interface này.
- Consumer deps: field `deps.SessStore` (`cmd/gateway_consumer.go:59`) đã là `store.SessionStore` đầy đủ (embed SessionMetadataStore + Save) — KHÔNG cần type-assert.
- Session key consumer-side: `processNormalMessage` tự build tại `cmd/gateway_consumer_normal.go:83-110` — `BuildScopedSessionKey` (:83, delegate `BuildSessionKey` key.go:127), override topic/forum (:95-101) / DM-thread (:104-110) dựa trên metadata `is_forum`/`dm_thread_id`+`message_thread_id` do channel gắn tại `handlers.go:699-706`.
- ExtraSystemPrompt chain (đường truyền dev mode): consumer build `gateway_consumer_normal.go:294-309` → `RunRequest.ExtraSystemPrompt` (:507) → `pipeline.RunInput` (`loop_pipeline_adapter.go:290`) → context_stage append memory (`context_stage.go:77-80`) → `makeBuildMessages` append TeamWork directive (`loop_pipeline_callbacks.go:145-150`) → `buildMessages` (call site DUY NHẤT `loop_pipeline_callbacks.go:151`; `ExtraPrompt` literal `loop_history.go:265`). Resume path copy `ExtraSystemPrompt` tại `loop_run.go:512`.
- Prompt assembly: `internal/agent/loop_history.go:21,54-57,265` — `buildMessages(..., extraSystemPrompt, ...)`; `SystemPromptConfig.ExtraPrompt`.

Skills & menu:
- `skills/` 23 dir; seeder disk-based `internal/skills/seeder.go:55-174` (skip `_`, cần `SKILL.md`, upsert system skill theo hash); wiring `cmd/gateway_setup.go:591-656`.
- Menu skill hiện tại: `skillMenuCommands` (`commands_skills.go:60-81`) — skip slug không khớp `^[a-z0-9_]{1,32}$` (gạch ngang bị loại — lý do `security-audit`/`ssl-audit` vắng mặt menu); merge vào SetMyCommands tại `channel.go:267-269`; `menu_skills` config `config_channels.go:92`, default `defaultMenuSkills` `commands_skills.go:32`.
- Skill slash matching: `parseSkillSlashCommand`/`matchSkillCommandTarget` `internal/agent/skill_slash_command_matching.go:16-115` — so slug + name raw, tie → không match (`:111-112`); activate set `skillFilter` `skill_slash_commands.go:44`.
- Deps frontmatter: `skills/security-audit/SKILL.md:20-25` (mẫu `system:nmap, pip:sqlmap`); checker `dep_checker.go:18-51`; installer `dep_installer.go:82+` (apk qua pkg-helper `Dockerfile:67,121`).
- Docker full variant: `Dockerfile:92-98` (`ENABLE_FULL_SKILLS=true`: `nmap nikto wrk hey iperf3` + pip sqlmap). Thiếu: `ffuf`, `bind-tools` (dig) → Phase 4 thêm.

Video (production-verified 2026-09-11 07:00 UTC):
- Telegram video → `handlers.go:545-546` (MediaRef pipeline) → `media.go:89-130` download → `<media:video>` tag (`internal/agent/media.go:448`) → `read_video` tool (`internal/tools/read_video.go:98-189`, max 100MB `:35`).
- **Lỗi production:** `downloadMedia` (`media.go:204-325`) chỉ retry `GetFile` (`:208-224`, `downloadMaxRetries=3` `:36`) — phần HTTP GET + `io.Copy` body KHÔNG retry (`:283-325`); log server: `"failed to download video" ... read tcp ...149.154.166.110:443: connection reset by peer"` → video mất trắng, user nhận skip notice (`handlers.go:577-596`).
- **Provider video:** chain mặc định `videoProviderPriority=[gemini, openrouter]` (`read_video.go:41`), model defaults `gemini-2.5-flash` (`:44-47`); resolve qua `ResolveMediaProviderChain` (`media_provider_chain.go:65-104`: per-agent > builtin_tools.settings chain > default theo registry). Server hiện CHỈ có provider `openai-compat` (ai.doralove.io.vn) → chain rỗng → read_video fail dù tải xong. KHÔNG dùng ffmpeg.

## Risk Assessment

- **R1 — Session key mismatch channel vs consumer (CRITICAL):** consumer build key tại `gateway_consumer_normal.go:83-110` (BuildScopedSessionKey + các bản sao topic/forum/DM-thread theo metadata `is_forum`/`dm_thread_id` mà channel tự gắn `handlers.go:699-706`). Channel-side helper phải tái lập đúng chuỗi đó. Hai residual đã biết (ghi docs, không chặn): (a) voice message route sang `VoiceAgentID` → sessionKey khác (handlers.go:728-739) — toggle đặt bằng lệnh text không áp cho voice session; (b) khi `msg.AgentID` rỗng, routing-rules có thể chọn agent khác `c.AgentID()` (`gateway_consumer_normal.go:37-40`) — hiếm, chấp nhận. Phòng: test characterization 4 case (DM/group/forum/DM-thread) so key 2 path.
- **R2 — Metadata lost nếu chỉ Set không Save:** `SetSessionMetadata` chỉ ghi in-memory cache (`pg/sessions.go:259-279`). Handler PHẢI gọi `Save` ngay sau (pattern `gateway/methods/sessions.go:169-173`). Test: set → new store instance đọc từ DB (hoặc fake assert Save gọi).
- **R3 — SQLite metadata column: ĐÃ VERIFY CÓ** (audit 2026-09-12: `sqlitestore/schema.sql:281` `metadata TEXT DEFAULT '{}'`; Save persist cả UPDATE/INSERT `sessions_ops.go`). Không cần patch. Lưu ý: SQLite Save tự inject `last_prompt_tokens`/`last_message_count` vào chính map metadata (`sessions_ops.go:39-42`) → handler chỉ được MERGE key (maps.Copy của SetSessionMetadata đã thế), không thay cả map.
- **R4 — "off"/"none" semantics: ĐÃ VERIFY QUA AUDIT** (xem Success Criteria): "off" = disable thật (option vắng mặt), "none" = pass-through BẬT thinking trên Claude → lệnh TG từ chối "none". Caveat Gemini-compat ghi docs. Không cần characterization test mở rộng — audit đã truy hết đường code; giữ 1 unit test khóa regression cho hành vi "off" → option absent.
- **R5 — /status read latency:** /status đọc ~4 nguồn (server in-mem, sched in-mem, session store, trace store) — tất cả local/index, không qua LLM. Giữ handler < 1s; queue depth + system uptime là in-process/proc read.
- **R6 — Group spam:** rich status trong group dễ nhiễu → default short ở group (đã thiết kế), full chỉ khi gọi `/status full` hoặc set.
- **R7 — Skill bảo mật bị dùng ngoài phạm vi:** mọi skill kiểm thử đều phải có AUTHORIZATION GATE ở đầu (pattern `security-audit` hiện có) — chỉ target hạ tầng mình sở hữu/có giấy phép viết; refusals khi không xác định được ownership. Đây là điều kiện tồn tại của Phase 4, không phải nice-to-have.
- **R8 — Wiring 2 đường tạo channel (audit FAIL đã sửa):** Option mới (SessionPrefs, StatusProvider) phải gắn cả factory path (`cmd/gateway.go:1049`) LẪN direct `telegram.New(...)` path (`cmd/gateway_channels_setup.go:52`) — nhánh config-fallback không đi qua factory. Gate: test/greps đảm bảo cả 2 call site đều truyền Option; quên 1 nhánh = feature chết âm thầm khi deploy không có channel instances trong DB.
- **R9 — Video phụ thuộc 2 yếu tố ngoài code:** (a) route mạng VN→Telegram DC reset kéo dài thì retry 3 lần có thể vẫn fail → phản ứng: cấu hình `proxy` (TelegramConfig.Proxy `config_channels.go:77`) hoặc local Bot API server (`api_server` — `media.go:249-256` hỗ trợ đọc FS, cũng là đường duy nhất cho video >20MB vì Bot API chuẩn giới hạn GetFile); (b) cần 1 provider video (Gemini key free / OpenRouter) trong `llm_providers` — không có thì read_video fail dù tải xong. Cả 2 đều là ops step kèm theo Phase 5, có docs.
- **R10 — Normalization `_`≡`-` trong skill matching:** nếu 2 skill khác nhau chỉ khác nhau bằng `_`/`-` trong slug → tie → không match (an toàn sẵn có `matching.go:111-112`); test case này trong Phase 4.
