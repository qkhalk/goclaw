---
title: "Telegram Skill Menu and Testing Skills"
description: "Telegram '/' menu hiển thị skill hay dùng + lệnh /skills liệt kê skill kèm mô tả; bộ skill kiểm thử mới (security-audit, loadtest L7, netstress L4, ssl-audit) cho sản phẩm trước khi lên production."
status: completed
priority: P1
effort: "1.5d"
tags: [telegram, skills, security, loadtest]
created: 2026-09-10
---

# Telegram Skill Menu and Testing Skills

## Overview

Người dùng muốn: (1) trong giao diện Telegram, gõ `/` hiện ~4–5 skill hay dùng (kiểu `gc:cook`, `gc:plan`) trong bot command menu; (2) một lệnh `/skills` liệt kê toàn bộ skill kèm mô tả tác dụng; (3) thêm các skill kiểm thử trước production: audit bảo mật (burp-like), load test Layer 7, stress Layer 4, và các skill kiểm thử khác.

Hai fact bắt buộc từ codebase (đã verify):

- **Telegram BotCommand không cho phép ký tự `:`** — `telego.BotCommand.Command` giới hạn 1–32 ký tự, chỉ `[a-z0-9_]` (types.go:4941, telego v1.6.0). Do đó menu sẽ đăng ký dạng `/cook`, `/plan`, `/fix`… (chính là cú pháp slash-command kích hoạt skill sẵn có của agent — `internal/agent/skill_slash_commands.go`), không thể đăng ký `/gc:cook` nguyên văn. `/gc:cook <text>` vẫn dùng được khi gõ tay.
- **`setMyCommands` đã có sẵn**: `SyncMenuCommands` (`internal/channels/telegram/commands_pairing.go:143`) + `DefaultMenuCommands()` (`:162`) + goroutine sync 3-retry trong `Start()` (`channel.go:263-289`). Chỉ cần mở rộng danh sách lệnh.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Gõ `/` trong Telegram hiện 4–5 skill hay dùng (cấu hình được qua `channels.telegram.menu_skills`, mặc định cook/plan/fix/review/test) + lệnh `/skills` | P1 |
| 2 | Lệnh `/skills` trả lời tức thì (không qua LLM): danh sách slug + mô tả, tôn trọng skill whitelist của group/topic khi có | P1 |
| 3 | 4 skill bundled mới: `security-audit`, `loadtest` (L7), `netstress` (L4), `ssl-audit` — khai báo deps hệ thống qua frontmatter `deps:` để cơ chế install-deps sẵn có tự cài | P1 |
| 4 | Docker full variant có sẵn công cụ; docs cập nhật; cross-surface parity được kiểm toán | P2 |

## Scope Challenge (đã cân nhắc)

- **Không** dùng "top-N theo usage metrics" cho menu: sync menu chỉ chạy lúc startup, usage đổi liên tục → heuristic lệch giá trị. Chọn explicit config (đúng quy tắc AGENTS.md "Prefer explicit configuration over runtime heuristics").
- **Không** thêm CommandKind `/gc:` mới cho 4 skill test: generic `/<slug>` đã kích hoạt được; thêm kind là scope creep.
- **Không** sửa web UI: skills.list WS + /v1/skills HTTP đã có; menu Telegram là surface duy nhất thay đổi.
- Skill content phải có **authorization gate** ở đầu mỗi skill (chỉ test hạ tầng mình sở hữu/có giấy phép) — đây là điều kiện để tính năng "kiểm tra chịu tải/độ an toàn sản phẩm trước production" an toàn.

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Telegram /skills + skill menu](./phase-01-start.md) | Pending |
| 2 | [Phase 2: Bundled testing skills](./phase-02-bundled-testing-skills.md) | Pending |
| 3 | [Phase 3: Docker packages and verification](./phase-03-docker-packages-and-verification.md) | Pending |

## Success Criteria

- [ ] Gõ `/` trong Telegram bot thấy các skill mặc định (cook, plan, fix, review, test) + `skills`; cấu hình `menu_skills` override được; slug không hợp lệ bị bỏ qua + log warn (không crash).
- [ ] `/skills` trả lời trong ~1s không tốn LLM token: mỗi skill 1 dòng `/slug — mô tả` (mô tả truncate), chunk an toàn dưới 4000 ký tự HTML, HTML-escape mô tả.
- [ ] Group/topic có `skills` whitelist → `/skills` chỉ list skill trong whitelist.
- [ ] 4 skill mới seed vào DB khi khởi động (seeder path), frontmatter parse sạch (bundled smoke test pass), deps hiển thị missing trên máy thiếu tool mà không chặn gateway.
- [ ] `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...` sạch; test mới pass; CI Linux xanh (lỗi Windows sẵn có ở internal/agent + internal/tools là pre-existing, đã verify ở session trước).

## Cross-Plan Relationships

Không có plan đang mở nào chồng lấn. Các plan `2608xx-*` trong `plans/` là việc đã hoàn thành & merge (PR #49, #50 và các phase trước) — index `ak plan list` chưa đánh dấu completed nhưng file thực tế đã triển khai xong.

## Surface Parity (kỳ vọng khi hoàn tất)

- **Gateway server:** telegram channel + config + factory wiring (thay đổi chính).
- **API contract:** N/A — không thêm WS method/HTTP endpoint mới (skills.list/GET /v1/skills đã tồn tại, không đổi shape).
- **Web UI:** N/A — không màn hình nào thay đổi; menu Telegram là surface riêng của channel.
- **CLI/runtime package:** N/A — không có command CLI nào đọc menu_skills; skill files là dữ liệu bundled, `goclaw` CLI không consume.
- **Docs:** cập nhật `docs/15-core-skills-system.md` (bundled list) + `docs/14-skills-runtime.md` (full-variant packages) nếu phase 3 thêm package.

## Key References (verify-passed)

- `internal/channels/telegram/commands.go:41` — `handleBotCommand` switch; `/skills` case sẽ đặt cạnh `/tasks` (`:206`).
- `internal/channels/telegram/commands_pairing.go:143,162` — `SyncMenuCommands` / `DefaultMenuCommands` (17 lệnh hiện tại).
- `internal/channels/telegram/channel.go:263-289` — startup menu sync (3 retry × backoff).
- `internal/channels/telegram/channel.go:72-96` — Option pattern (`WithAgentStore`, `WithTeamStore`…) — thêm `WithSkillsLister`.
- `internal/channels/telegram/factory.go:59-70` — `FactoryWithStoresAndAudio` (cần thêm param + update caller duy nhất `cmd/gateway.go:1041`).
- `internal/channels/telegram/commands_tasks.go:52-79` — pattern handler list (nil-store guard, `send` helper, `truncateStr`).
- `internal/config/config_channels.go:74-118` — `TelegramConfig` (thêm `MenuSkills []string json:"menu_skills,omitempty"`); `factory.go:22-45` `telegramInstanceConfig` thêm field tương ứng.
- `internal/skills/loader.go:45-57,150` — `Info{Name,Slug,Description,...}`, `ListSkills(ctx)`.
- `internal/skills/dep_scanner.go:30-32`, `loader.go:723-725` — frontmatter `deps: ["system:nmap", "pip:sqlmap"]` là nguồn authoritative cho install-deps.
- `internal/skills/seeder.go:55-100,299` — bundled skill tự seed + CheckDepsAsync khi startup.
- `internal/agent/skill_slash_commands.go:31` + `skill_slash_command_matching.go:16` — `/cook <text>` đã kích hoạt skill khi message rơi xuống agent loop (unknown command từ `handleBotCommand` return false → passthrough).
- `cmd/gateway_channels_setup.go:52` — config-based channel wiring (cần truyền skills loader).
