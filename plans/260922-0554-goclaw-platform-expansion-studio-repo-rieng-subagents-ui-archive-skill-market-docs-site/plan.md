---
title: "GoClaw platform expansion: Studio repo riêng, subagents UI + archive, skill market, docs site"
description: "5 hạng mục theo yêu cầu anh: (A) tách Tools Studio thành repo/domain riêng kèm admin quản lí model; (B) subagents cho agent /chat với UI kiểu Paseo + sửa phản hồi 'ngáo' + nút archive agents đã hoàn thành; (C) nút archive trên Telegram; (D) skill market từ goclaw-kit thay vì seed cứng; (E) docs site GitHub Pages kiểu 9router."
status: pending
priority: P1
effort: "~18 ngày (12 phase, 5 workstream)"
tags: [studio, subagents, archive, telegram, skills, market, docs, vitepress, new-repo]
created: "2026-09-22"
related: [260915-1800-goclaw-design-studio-skill-set, 260916-0545-overview-revamp-compact-recent-requests-9router-style-model-routing-system-stats-card]
---

# GoClaw platform expansion: Studio repo riêng, subagents UI + archive, skill market, docs site

## Overview

5 hạng mục độc lập theo yêu cầu, gom thành 5 workstream (A–E) / 12 phase. Mọi claim hạ tầng đều đã verify trong code (scout + spot-check 6/6 khớp):

| Hạng mục anh yêu cầu | Workstream | Phases |
|---|---|---|
| 1. TOOLS thành dự án riêng (repo + domain riêng, admin quản lí model) | **A — GoTools repo mới** | 1–4 |
| 2. Subagents cho /chat (UI theo Paseo) + sửa "ngáo" + nút archive agents hoàn thành | **B — subagents + chất lượng + archive web** | 5–8 |
| 3. Nút archive agents hoàn thành trên Telegram | **C — Telegram** (dùng nền phase 5) | 9 |
| 4. Gom skill goclaw-kit, market chọn skill cài (không add cứng) | **D — skill market** (wire KitManager đang là dead-code) | 10–11 |
| 5. GitHub Pages docs site kiểu 9router | **E — docs** (VitePress, cùng pattern trang 9router của anh) | 12 |

### Phát hiện then chốt từ scout (đã verify file:line)

1. **Spawn subagent đã có sẵn cho MỌI agent chat** (`cmd/gateway.go:626` đăng ký global; mode auto resolve `internal/agent/orchestration_mode.go:43`). Chỉ thiếu: WS method expose `subagent_tasks` ra UI + surface UI. Không cần xây orchestration mới.
2. **Archive đã tồn tại ở tầng store nhưng chưa lộ ra đâu**: `subagent_tasks.archived_at` + TTL sweeper 60' (`migrations/000034`, `subagent_control.go:24-87`), nhưng `ListByParent` **không lọc archived** (`pg/subagent_tasks.go:148-179`) và không có method per-ID — Telegram `/subagents` đang hiện cả task đã archive.
3. **"Ngáo" có 3 thủ phạm cấu trúc**: subagent hardcode `max_tokens 4096 / temperature 0.5` (`subagent_exec.go:288-289`); reasoning mặc định **off** + fallback downgrade im lặng (`agent_store.go:192-199`, `resolver.go:186-200`); composer override per-message có thể đè model yếu hơn (`chat.go:151-163`).
4. **KitManager đầy đủ (manifest/install/update/rollback/lock) nhưng là dead-code** — chỉ được ref bởi chính nó (`internal/skills/kit_manager.go`, không handler/UI nào gọi). Skill hiện seed "hết mọi thứ trong bundled dir" (`gateway_setup.go:631-658`, Docker `Dockerfile:125` copy cả `skills/` 116 skill) → đây là chỗ "add cứng" phải sửa.
5. **MCP Tool Store (v4.9.4, PR #116) là template kiến trúc sẵn** cho skill market: catalog embed → install job pin version → validate → package row → uninstall (`internal/http/mcp_install.go`, `internal/mcp/installer/installer.go`).
6. **Go `internal/` không import cross-module** → repo Studio mới phải **copy-port** các slice (không `import goclaw/internal/...` được). Videoworker vốn đã standalone (`cmd/videoworker` HTTP riêng port 18791) nên port sạch.

### Quyết định kiến trúc đã chốt (cho validate review)

- **Studio = standalone tự chứa** (repo mới `GoTools`, Go server + SPA + SQLite riêng), KHÔNG proxy về goclaw gateway. Lý do: anh yêu cầu "tách hẳn khỏi dự án hiện tại, domain riêng"; tránh phụ thuộc triển khai goclaw. Trade-off: code copy-port phải đánh dấu PROVENANCE + sync tay.
- **Slim designer thay vì agent loop đầy đủ**: designer column trong Studio gọi endpoint completion đơn (system prompt + 2–3 skill file inline → 1 LLM call stream). UI đã parse markdown blocks client-side nên không cần 8-stage pipeline/tools.
- **Skill market = wire KitManager có sẵn + follow MCP installer pattern**, seed chuyển sang `seed_mode: all|core|none` (mặc định `all` để back-compat; server anh đổi sang `core`).
- **Docs = VitePress repo riêng `qkhalk/goclaw-docs`** (validate session 1 đổi từ "cùng repo") + workflow GitHub Pages riêng, locales en/vi, base `/goclaw-docs/` — đúng pattern trang 9router (VitePress + i18n `/en/` routing) của anh. Sync nội dung từ goclaw bằng PR thủ công + checklist dòng docs trong PR template goclaw.
- **Repo Studio = `qkhalk/gotools` (tên hiển thị GoTools)** — backend Go + SPA như thiết kế (validate: anh xác nhận web có backend Go). Thứ tự triển khai: **theo đúng thứ tự phase 1→12** (validate).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Repo `GoTools` deploy được ở domain riêng: watermark + PPTX + Video studio + admin model/providers, không cần goclaw chạy | P2 |
| 2 | Agent /chat dùng subagents (spawn có sẵn) + UI Paseo-style (pill + panel + timeline) | P1 |
| 3 | Phản hồi chat sắc hơn: subagent kế thừa config agent, reasoning mặc định theo provider, hiển thị model/thinking effective | P1 |
| 4 | Nút archive agents hoàn thành: web (phase 6/8) + Telegram `/subagents` + announce (phase 9) | P1 |
| 5 | Skill market: chọn skill từ goclaw-kit để cài, bỏ seed cứng | P1 |
| 6 | Docs site GitHub Pages online | P3 |

## Phases

| # | Phase | Status | Dep |
|---|-------|--------|-----|
| 1 | [Scaffold repo GoTools](./phase-01-start.md) | Pending | — |
| 2 | [Studio: port watermark + PPTX + slim designer endpoint](./phase-02-studio-port-watermark-pptx-slim-designer-endpoint.md) | Pending | 1 |
| 3 | [Studio: port video studio + videoworker sidecar](./phase-03-studio-port-video-studio-videoworker-sidecar.md) | Pending | 1, 2 |
| 4 | [Studio: admin quản lí model/providers](./phase-04-studio-admin-quan-li-modelproviders.md) | Pending | 1 |
| 5 | [Backend: WS subagents methods + archive store](./phase-05-backend-ws-subagents-methods-archive-store.md) | Pending | — |
| 6 | [Web: subagents UI kiểu Paseo](./phase-06-web-subagents-ui-kieu-paseo.md) | Pending | 5 |
| 7 | [Chat quality: reasoning mặc định + subagent params](./phase-07-chat-quality-reasoning-mac-dinh-subagent-params.md) | Pending | — |
| 8 | [Web: archive hoàn thành trong /chat (sessions)](./phase-08-web-archive-hoan-thanh-trong-chat.md) | Pending | — |
| 9 | [Telegram: nút archive agents hoàn thành](./phase-09-telegram-nut-archive-agents-hoan-thanh.md) | Pending | 5 |
| 10 | [Skills: wire KitManager + seed_mode + catalog API](./phase-10-skills-wire-kitmanager-seed-mode-catalog-api.md) | Pending | — |
| 11 | [Skills: market UI trong /skills](./phase-11-skills-market-ui-trong-skills.md) | Pending | 10 |
| 12 | [Docs: VitePress site + GitHub Pages (repo riêng goclaw-docs)](./phase-12-docs-vitepress-site-github-pages.md) | Pending | — |

## Cross-plan relations

- **`260922-0614-gotools-standalone-repo-kien-truc-chi-tiet-va-trien-khai` (plan con — CHI TIẾT hoá phase 1–4)**: GoTools repo mới được bung sâu thành 8 phase riêng (scaffold→shell→watermark→admin providers→designer engine→videoworker→video UI→deploy/release, ~12 ngày). **Cook workstream A từ plan con đó** — phase 1–4 của plan này chỉ còn giá trị tóm tắt, không cook trực tiếp. Quyết định validate (repo qkhalk/gotools, TTS không port, backend Go) áp dụng nguyên cho plan con.
- `260915-1800-goclaw-design-studio-skill-set` (pending): 12 skill design sẽ xuất hiện trong market như skill thường — **không block nhau**; market list đọc bundled dir nên tự thấy khi phase-02 của plan đó merge.
- `260916-0545-overview-revamp` (pending): khác surface hoàn toàn (overview page), không giao nhau.
- Nền tảng reuse: `capability-catalog.md` (Tool Store, done) + `mcp-catalog-installer.md` (PR #116, done) — pattern install job/catalog cho phase 10–11.
- `260912-1137-web-chat-declutter-paseo-aligned-chat-ux` (done): triết lý Paseo (chat = bề mặt hội thoại, tracker = pill) — phase 6 tuân theo.

## Success Criteria

- [ ] `GoTools` build + deploy 1 binary ở domain riêng, 3 tool dùng được end-to-end, admin thêm provider/model + test verify OK
- [ ] Agent chat spawn subagent → pill + panel hiện trạng thái → task xong có nút Archive → archived biến mất khỏi list (web + Telegram)
- [ ] Subagent call LLM mang max_tokens/temperature/reasoning của parent agent (verify qua trace), reasoning agent mới mặc định theo provider capability thay vì off
- [ ] Cài goclaw bản mới với `seed_mode: core` → chỉ skill core được seed; vào Market cài thêm 1 skill → agent search/use được; gỡ → sạch
- [ ] `https://<user>.github.io/goclaw-docs/` online với en/vi, search local hoạt động

## Open Questions — ĐÃ GIẢI QUYẾT (Validation Session 1, 2026-09-22)

1. ~~"Agents đã hoàn thành"~~ → **Cả subagent tasks (phase 6+9) VÀ sessions (phase 8)** — anh chọn phủ cả 2 cách hiểu.
2. ~~Tên repo Studio~~ → **GoTools** (repo `qkhalk/gotools`, module lowercase chuẩn Go, display name GoTools); anh xác nhận backend Go.
3. ~~TTS có sang Studio~~ → **Không** — TTS ở lại goclaw (non-goal của GoTools).
4. ~~reasoning auto chi phí~~ → **auto cho agent mới** (agent cũ giữ nguyên) — chấp nhận tăng token để hết "ngáo".
5. ~~Docs cùng repo hay riêng~~ → **Repo riêng `qkhalk/goclaw-docs`** + sync PR thủ công (checklist dòng docs trong PR template goclaw).
6. (thêm) Thứ tự triển khai → **theo thứ tự phase 1→12**.

## Validation Log

### Session 1 — 2026-09-22
**Trigger:** Post-plan validate interview (user chọn `/ak:plan validate` sau khi plan tạo xong)
**Questions asked:** 8 (3 lượt phỏng vấn)

#### Verification Results (chạy trước phỏng vấn)
- Claims checked: 23 (audit agent độc lập đối chiếu live codebase)
- Verified: 21 | Failed: 2 | Unverified: 0 — Tier: Full (12 phases)
- Failures đã sửa thẳng vào plan: (a) `agent-grants-dialog.tsx` không tồn tại → đúng là `skill-agent-grants-dialog.tsx` (phase 11); (b) run-phase label KHÔNG nằm ở `chat-top-bar.tsx` (file 96 dòng, chỉ spinner) → indicator đúng là `ui/web/src/components/chat/activity-indicator.tsx:42` (phase 7 đã đổi anchor).
- Minor drift đã ghi chú trong phase: `internal/sessions/key.go` WS builder :170-175; `internal/gateway/methods/agents_create.go:70-72,121-128`; seeder không có helper single-skill (phase 10 thêm bước extract `seedOne`).

#### Questions & Answers

1. **[Scope]** "Nút archive các agents đã hoàn thành" — đối tượng archive chính xác là gì?
   - Options: Cả subagent + session | Chỉ subagent tasks | Chỉ sessions
   - **Answer:** Cả subagent + session (Recommended)
   - **Rationale:** Phủ cả 2 cách hiểu, khớp web (pill + sidebar) lẫn Telegram (/subagents).
2. **[Assumptions]** Sửa "ngáo": reasoning mặc định cho agent mới?
   - Options: auto cho agent mới | Giữ off, chỉ thêm UI
   - **Answer:** auto cho agent mới (Recommended)
3. **[Architecture]** Repo mới cho Tools Studio đặt tên gì?
   - **Answer (custom):** "GoTools, và web sẽ có backend bằng go nhé" → repo `qkhalk/gotools`, display GoTools, backend Go xác nhận.
4. **[Architecture]** Docs site ở repo nào?
   - **Answer:** Repo docs riêng (khác recommendation) → `qkhalk/goclaw-docs`.
5. **[Scope]** TTS có đưa sang Studio không?
   - **Answer:** Không đưa sang (Recommended).
6. **[Scope]** Thứ tự triển khai 5 workstream?
   - **Answer:** Theo thứ tự plan 1→12.
7. **[Tradeoffs]** Sync docs từ goclaw sang repo docs riêng?
   - **Answer:** goclaw-docs + PR thủ công (Recommended).
8. **[Assumptions]** Tên cụ thể (sau khi anh yêu cầu suggestions: mediaforge/gocraft/aistudio/toolforge)?
   - **Answer (custom):** GoTools.

#### Confirmed Decisions
- Archive: subagent tasks + sessions (phase 6, 8, 9 giữ nguyên scope như thiết kế)
- Reasoning: auto cho agent mới, agent cũ giữ nguyên (phase 7 đúng như thiết kế)
- Studio repo: qkhalk/gotools (GoTools), backend Go + SPA (phase 1 đã rename toàn bộ)
- TTS: không port (non-goal giữ nguyên)
- Docs: repo riêng goclaw-docs + PR thủ công (phase 12 rewritten hoàn toàn)
- Thứ tự: phase 1→12 sequential

#### Impact on Phases
- Phase 1: rename goclaw-studio → GoTools/gotools (đã làm)
- Phase 12: rewrite cho repo riêng goclaw-docs (đã làm)
- Phase 7, 8, 9, 10, 11: không đổi nội dung (quyết định khớp thiết kế sẵn)
- Phase 2, 3, 4: không đổi (tên repo chỉ xuất hiện trong ngữ cảnh "repo mới"/GoTools)

## Risk Assessment

| Rủi ro | Mitigation |
|---|---|
| Copy-port Studio trôi diverge so với goclaw | Header PROVENANCE (`Ported from goclaw@<sha> internal/...`) mọi file port; giữ cấu trúc thư mục giống; changelog sync khi sửa bug cùng nguồn |
| Slim designer chất lượng kém hơn agent loop goclaw (không tool/skill search) | Skill designer inline toàn bộ trong system prompt (chúng vốn là prompt thuần); benchmark 5 prompt chuẩn so kết quả |
| Đổi mặc định reasoning ảnh hưởng agent hiện có | Chỉ áp dụng cho agent tạo mới / chưa set config tường minh; agent cũ giữ giá trị đã lưu; có config tắt |
| seed_mode core làm hổng skill mà agent đang dùng | Reconciler có sẵn giữ DB row; market hiện "Đã cài" cho mọi skill tồn tại — uninstall là hành động tường minh |
| Telegram callback 64-byte budget | UUID 36 chars + prefix `ar:` = 39 bytes — an toàn (comment ask_options.go:48) |
| VitePress base path sai khi repo name đổi | base đọc từ env `BASE` trong workflow; docs trỏ relative |

<!-- slug: goclaw-platform-expansion-studio-repo-rieng-subagents-ui-archive-skill-market-docs-site -->
