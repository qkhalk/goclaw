---
title: "Web /chat declutter — Paseo-aligned chat UX"
description: "Dọn trang /chat của Web UI: bỏ UI trùng lặp (3 nơi hiển thị team task, 2 agent picker, 2 run-phase indicator, 3 lần fetch /v1/agents), thu gọn cụm workspace console đang chiếm top bar, xóa UI chết (context badge không có data, label Ready), i18n hết chuỗi hardcode, rồi xây lại context meter thật theo pattern Paseo + nâng cấp composer pills / command palette theo triết lý 'chat là bề mặt hội thoại, mọi thứ khác để ngoài'."
status: completed
priority: P1
effort: "4d"
tags: [web-ui, chat, ux, declutter, paseo, i18n, ws-protocol]
created: "2026-09-12"
---

# Web /chat declutter — Paseo-aligned chat UX

## Overview

Anh thấy `/chat` (http://192.168.1.103:18790/chat, server đang chạy 3.18.0) có nhiều thứ thừa / không liên quan / không tác dụng. Khảo sát 3 nguồn xác nhận nguyên nhân:

1. **Audit code** (`ui/web/src/pages/chat/`, `ui/web/src/components/chat/`) — trang chat tích tụ qua các commit "paseo-style composer", "web terminal panel (Paseo Phase 4)", "jobs and task tree console panel", "slash command palette": mỗi đợt bolt thêm control lên cùng 1 trang.
2. **Live verify trên server** — top bar thực tế đang hiện: `Không có workspace` (picker) + 3 nút console (1 disable) + label `Ready` tiếng Anh giữa UI tiếng Việt; agent phải chọn ở 2 nơi (dropdown sidebar + grid giữa màn hình).
3. **Research Paseo** (github.com/getpaseo/paseo — "control plane for coding agents", ~17k sao) — triết lý cốt lõi áp dụng được: **chat là bề mặt hội thoại**; cấu hình/skills/history/schedule đặt ở screen riêng (GoClaw đã có đủ các page riêng rồi); task tracker là **pill nhỏ phía trên composer** (v0.5.0) chứ không phải cột panel; context meter là feature được khen — nhưng phải có data thật.

**Bằng chứng chính (file:line, đã verify):**

| Vấn đề | Bằng chứng |
|---|---|
| Team task hiển thị **3 nơi cùng lúc** | TaskPanel cột phải (`chat-page.tsx:324-327`, auto-open `:167-174`) + TeamActivityPanel inline (`chat-thread.tsx:165`) + chip "Team: N tasks" trên top bar (`chat-top-bar.tsx:164-173`) — cùng 1 mảng `teamTasks` |
| **2 agent picker** | AgentSelector sidebar (`chat-sidebar.tsx:34-36`) + AgentPickerPrompt grid (`chat-page.tsx:296-304`) |
| **3 lần fetch `/v1/agents`** trên 1 màn hình | `agent-selector.tsx:33`, `agent-picker-prompt.tsx:24`, `chat-top-bar.tsx:56` — không share cache |
| Run-phase label hiển thị **2 nơi** | top bar (`chat-top-bar.tsx:159-163`) trùng ActivityIndicator dưới thread (`activity-indicator.tsx:26-33`) |
| Workspace console (picker + files + jobs + terminal) chiếm **4/8 control top bar**, `workspaceId` **không ảnh hưởng gì đến chat session** | `chat-page.tsx:160` (state độc lập, không persist), picker luôn render `chat-top-bar.tsx:130`, nút files/jobs luôn enable → mở panel chỉ thấy empty state |
| **Context badge hoạt động nhưng thô** | ~~Audit ban đầu kết luận "UI chết" — RED-TEAM ĐÃ SỬA: đó là struct params của `handlePatch`, không phải response list.~~ Sự thật: WS `sessions.list` trả `SessionInfoRich` nguyên trạng (`internal/gateway/methods/sessions.go:72-76` → `session_store.go:95-104` có sẵn `estimatedTokens/contextWindow/compactionCount`; PG SQL `sessions_list.go:175-181` tính `last_prompt_tokens` fallback bytes/4+12k + join `agents.context_window` COALESCE 200000; SQLite có `ListPagedRich` + integration test `sessions_display_tokens_integration_test.go:68`; pipeline ghi `last_prompt_tokens` `loop_history_sanitize.go:433`, compaction ghi `last_compaction_at` `loop_pipeline_callbacks.go:916`). Vấn đề thực: badge chỉ có tooltip, `hidden sm:flex` (mất trên mobile), không click/popover được |
| Label `Ready` + phase labels + "Team: N" + "Running Tasks" + "Drop files here" + mô tả `gc:` commands **hardcode tiếng Anh** trong UI tiếng Việt | `chat-top-bar.tsx:36-43,171,174-176`, `task-panel.tsx:22,36`, `team-activity-panel.tsx:21`, `drop-zone.tsx:41`, `command-palette.tsx:38-53` |
| Command palette chỉ **chèn text**, 4/14 lệnh (status/runs/doctor/approve) trùng page riêng | `command-palette.tsx:77-83` |

**Nguyên tắc thiết kế lấy từ Paseo** (nguồn: releases + docs paseo.sh, chi tiết ở `reports/research-paseo.md`):
- Chat = composer + timeline + sessions. Cấu hình (provider/skills/schedules) ở screen riêng — GoClaw đã có `/providers`, `/skills`, `/cron`.
- Task tracker = "pills above the composer" (v0.5.0), KHÔNG phải cột panel tự mở.
- Context meter được khen ("no longer blanks out partway through") — làm thật, có data.
- Model/effort picker = pill gọn trong composer.
- Những gì Paseo cố tình để OUT khỏi chat view: settings, schedules, history browsing, provider config → GoClaw tương đồng nhờ hệ thống page sẵn có; việc cần làm là **bỏ bản sao trong chat**.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Mỗi mối quan tâm hiển thị đúng 1 nơi: 1 agent picker, 1 task surface, 1 run-phase indicator, 1 agents fetch | P1 |
| 2 | Top bar chat chỉ chứa thứ liên quan đến cuộc hội thoại; workspace console thu vào 1 nút, không còn text "Không có workspace" thường trực | P1 |
| 3 | Xóa UI chết (context badge không data, label Ready) và hết chuỗi hardcode tiếng Anh (i18n đủ 5 locale en/ko/ru/vi/zh) | P1 |
| 4 | Context meter hoàn chỉnh theo pattern Paseo: badge đã có data thật → nâng cấp UX: click mở popover (used/max/%, compaction count, lần compact gần nhất, link session), hiển thị cả mobile, live-update khi `session.updated` | P2 |
| 5 | Composer theo pattern Paseo: toolbar provider/model/thinking thành pill row gọn; command palette bỏ 4 lệnh trùng page | P2 |
| 6 | (Stretch, có điều kiện kích hoạt) Answer-form cards + @file mention — chỉ làm khi format ask-options từ plan Telegram (`260912-1116-telegram-interactive-ux-*`) chốt | P3 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: De-duplicate — one picker, one task surface, one fetch](./phase-01-start.md) | Completed |
| 2 | [Phase 2: De-noise top bar & console; i18n hết chuỗi hardcode](./phase-02-de-noise-composer-top-bar-i18n.md) | Completed |
| 3 | [Phase 3: Make the context meter real (backend + UI)](./phase-03-make-the-context-meter-real-backend-ui.md) | Completed |
| 4 | [Phase 4: Paseo-inspired upgrades: pills, palette, (stretch) answer forms + @file](./phase-04-paseo-inspired-upgrades-pills-answer-forms-palette.md) | Completed |

## Quyết định thiết kế (khuyến nghị, anh duyệt khi review plan)

| # | Quyết định | Lựa chọn | Lý do |
|---|---|---|---|
| D1 | Workspace console (picker + files/jobs/terminal panels) | **Thu vào 1 nút popover** ở top bar, KHÔNG xóa tính năng | Console là feature đã ship (Paseo §25 Phase 3/4); vấn đề là trình bày (4 control thường trực + text "Không có workspace"). Progressive disclosure giữ feature, bỏ noise |
| D2 | Team task 3 nơi → 1 nơi | **Pill trên composer** (pattern Paseo v0.5.0), bỏ TaskPanel cột + TeamActivityPanel inline + chip top bar; GIỮ SystemNotification trong timeline (đó là marker lịch sử hội thoại, Paseo cũng keep "progress through the timeline") | Tracker gần nơi gõ tin nhắn; không còn panel tự mở giữa chừng chat |
| D3 | 2 agent picker → 1 | **Giữ AgentSelector sidebar**, bỏ grid AgentPickerPrompt giữa màn; empty-state thay bằng CTA mở selector | Sidebar selector luôn hiển thị, là chỗ chọn agent tự nhiên; grid giữa màn chỉ xuất hiện khi chưa từng chọn agent |
| D4 | Context badge | **Giữ và nâng cấp ở Phase 3** (popover + mobile + live-update). Phase 2 chỉ dọn quanh nó, KHÔNG xóa | Red-team sửa: audit đầu tiên kết luận "UI chết" do đọc nhầm struct `handlePatch` params làm response list. Data pipeline đã đầy đủ PG lẫn SQLite |
| D5 | Run-phase 2 nơi → 1 | Giữ ActivityIndicator dưới thread (gần composer theo Paseo), top bar chỉ còn spinner mờ khi đang chạy | Người dùng nhìn cuối màn khi chờ phản hồi |
| D6 | 4 lệnh `gc:status/runs/doctor/approve` trong palette | **Bỏ khỏi palette** (trùng `/runs`, `/approvals`...); giữ 10 lệnh prompt-style (plan/fix/cook/... gửi cho agent) | Palette hiện tại chỉ chèn text — lệnh control không có tác dụng UI, trang riêng đã có |

## Success Criteria

- [ ] `/chat` không còn: AgentPickerPrompt grid, TeamActivityPanel, TaskPanel cột + auto-open, chip "Team: N tasks", label "Ready", context badge không data, text "Không có workspace" thường trực
- [ ] Team task: 1 pill trên composer có popover danh sách; timeline vẫn có SystemNotification
- [ ] Top bar chat: tên agent + spinner (khi chạy) + 1 nút console popover — hết khác
- [ ] `/v1/agents` gọi 1 lần/màn hình (shared react-query)
- [ ] Không còn chuỗi hardcode tiếng Anh trong `pages/chat/**` + `components/chat/**` (grep "Ready\|Running Tasks\|Drop files" = 0); key mới có đủ 5 locale
- [ ] Context meter: click badge mở popover đủ 4 thông tin (used/max/%, compaction count, lần compact gần nhất, link session); badge hiện cả mobile; cập nhật live khi session có run mới
- [ ] Backend chỉ thay đổi nếu `session.updated` event thiếu field token (kiểm tra trước — additive nếu có); `go build ./...`, `go vet ./...`, `go build -tags sqliteonly ./...` pass; `pnpm build` (ui/web) pass
- [ ] Live verify trên 192.168.1.103 sau deploy: chat gửi/nhận bình thường, pill task hiện đúng khi có team task, console popover mở files/terminal được khi có workspace

## Surface parity

- **Gateway server:** chỉ Phase 3, và chỉ khi verification cho thấy `session.updated` event thiếu field token (sửa additive). Phases 1-2-4 không đụng backend — `sessions.list` đã trả đủ data qua `SessionInfoRich`.
- **Web UI:** toàn bộ plan (`ui/web`).
- **CLI/runtime:** N/A — không có CLI surface cho chat UI.
- **Desktop UI:** N/A — `ui/desktop/frontend/` là SPA lite riêng, không share code với `ui/web` chat. Các quyết định declutter (D1-D6) có thể port sau nếu anh muốn; không đưa vào scope plan này.

## Cross-plan

- `260912-1116-telegram-interactive-ux-inline-pickers-ask-options-locale-i18n-dev-mode-v2` (pending): khác surface (Telegram channel vs web chat), không chồng file → không phụ thuộc. RIÊNG mục stretch Phase 4 (answer-form cards) ăn khớp concept "ask-options 3 lựa chọn + Other" của plan đó — nếu plan Telegram làm trước thì web tái dùng cùng format block; nếu plan này làm trước thì thông báo lại cho plan Telegram dùng chung.
- Không có plan nào khác đang sửa `ui/web/src/pages/chat/**` hay `components/chat/**`.

## Open Questions

None — câu hỏi nguồn `estimatedTokens` đã red-team giải xong: store trả sẵn qua `SessionInfoRich` (PG `sessions_list.go:175-181`, SQLite `ListPagedRich` + test), nguồn ghi là `metadata.last_prompt_tokens` từ pipeline (`loop_history_sanitize.go:433`) với fallback heuristic bytes/4+12k. Phase 3 chỉ còn verify `session.updated` event payload khi implement.
