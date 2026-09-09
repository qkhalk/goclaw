---
title: "Phase 1: Telegram /skills + skill menu"
status: todo
---

# Phase 1: Telegram /skills + skill menu

## Overview

Thêm lệnh `/skills` (channel-side, tức thì, không qua LLM) liệt kê skill + mô tả; đăng ký 4–5 skill hay dùng vào bot command menu (gõ `/` là thấy); cấu hình được qua `channels.telegram.menu_skills`.

## Requirements

- [ ] `/skills` xử lý trong `handleBotCommand` (cạnh `/tasks`), trả lời ngay bằng `c.bot.SendMessage` HTML mode.
- [ ] Danh sách từ `skills.Loader.ListSkills(ctx)` inject qua Option mới `WithSkillsLister` (interface hẹp định nghĩa trong package telegram — tránh coupling + dễ fake trong test).
- [ ] Mỗi dòng: `/slug — mô tả` (mô tả truncate ~120 runes bằng `truncateStr` sẵn có, HTML-escape). Footer hướng dẫn: gõ `/<slug> <yêu cầu>` hoặc `/use <slug>` để chạy, `/gc:cook` style vẫn dùng tay được.
- [ ] Chunk an toàn: dùng `c.sendHTML(ctx, chatID, html, 0, threadID)` — `sendHTMLWithDepth` đã tự chunk qua `chunkHTML(html, telegramMaxMessageLen=4000)` (`send.go:305`, `format.go:395`); escape mô tả bằng `escapeHTML` (`format.go:206`, unexported, đủ bộ `& < >`).
- [ ] Group/topic có `skills` whitelist → chỉ list skill trong whitelist (dùng lại helper resolve topic/group config sẵn có — xem `topic_config.go`; `topicCfg.skills` là `[]string`, nil = tất cả).
- [ ] Menu: `DefaultMenuCommands()` thêm `{skills, "List available skills"}`; thêm `skillMenuCommands(slugs, lister)` sinh entry cho từng skill: slug phải khớp `^[a-z0-9_]{1,32}$` (bỏ qua + `slog.Warn` nếu không — ví dụ `ui-ux-pro-max` có gạch ngang), description = mô tả skill truncate 256 runes, fallback `"Run skill: <slug>"`. **telego v1.6.0 không validate charset phía client** (chỉ doc-comment; Telegram server mới reject) — validate locally là bắt buộc. Startup sync goroutine (`channel.go:263-289`) đổi `DefaultMenuCommands()` → `DefaultMenuCommands() + skillMenuCommands(...)`.
- [ ] Config: `TelegramConfig.MenuSkills []string json:"menu_skills,omitempty"` (nil = mặc định `[cook plan fix review test]`, `[]` = tắt); `telegramInstanceConfig` thêm `MenuSkills` map qua cho DB instance.
- [ ] `/help` text thêm dòng `/skills`.

## Architecture

Luồng: Telegram update → `handleMessage` → `handleBotCommand` (trước mention gate, `handlers.go:252`) → case `/skills` → `handleSkillsList` → lister.ListSkills → build HTML → chunk → sendHTML. Menu: startup `Start()` → goroutine sync → `SetMyCommands(default + skill entries)`.

Vì `handleBotCommand` chạy trước khi `resolvedMessageContext` được build, topic whitelist tra lại bằng `resolveTopicConfig(c.config, chatIDStr, messageThreadID)` (`topic_config.go:27`, trả `resolvedTopicConfig.skills []string` — nil = tất cả) — chính là helper production dùng ở `handlers.go:123`.

Injection (đã verify thứ tự startup trong `runGateway`, một scope hàm duy nhất): `skillsLoader` được build ở `cmd/gateway.go:627` TRƯỚC cả hai điểm wiring — `RegisterFactory` (`gateway.go:1035-1041`) và `registerConfigChannels` (`gateway.go:1103`). Do đó:
- `FactoryWithStoresAndAudio` thêm param `skillsLister` → caller duy nhất `cmd/gateway.go:1041` (`FactoryWithStores` wrapper không có caller ngoài package; `Factory` chỉ dùng trong test).
- `registerConfigChannels` (`gateway_channels_setup.go:33`, caller duy nhất `gateway.go:1103`) thêm param để truyền `skillsLoader` cho nhánh config-fallback `telegram.New(...)` (`gateway_channels_setup.go:52` — chỉ chạy khi instanceLoader == nil).

## Related Code Files

- Modify: `internal/channels/telegram/commands.go` (case `/skills` + `/help` text)
- Create: `internal/channels/telegram/commands_skills.go` (SkillsLister interface, WithSkillsLister Option, skillMenuCommands, handleSkillsList, buildSkillsListHTML)
- Modify: `internal/channels/telegram/commands_pairing.go` (DefaultMenuCommands + entry `skills`)
- Modify: `internal/channels/telegram/channel.go` (struct field `skillsLister`, startup sync dùng skill menu)
- Modify: `internal/channels/telegram/factory.go` (param + truyền Option)
- Modify: `internal/config/config_channels.go` (MenuSkills field)
- Modify: `cmd/gateway.go` (caller factory), `cmd/gateway_channels_setup.go` (config-based wiring)
- Create: `internal/channels/telegram/commands_skills_test.go`

## Implementation Steps

1. Config field + default helper `defaultMenuSkills`.
2. `commands_skills.go`: interface + Option + builder + handler.
3. Gắn case `/skills`, `/help`, menu sync, factory wiring.
4. Tests (xem dưới).
5. `go build ./... && go build -tags sqliteonly ./... && go vet ./... && go test ./internal/channels/telegram/ ./internal/config/`.

## Todo

- [ ] Config field + defaults
- [ ] commands_skills.go (Option, builder, handler)
- [ ] Switch/menu/help wiring + factory + cmd wiring
- [ ] Unit tests pass
- [ ] Build cả 2 mode + vet sạch

## Success Criteria

- [ ] `TestSkillMenuCommands_*`: slug hợp lệ giữ nguyên, slug có ký tự lạ bỏ qua, description truncate ≤256, fallback text khi lister nil/0 skill.
- [ ] `TestBuildSkillsListHTML_*`: có header + dòng `/slug — desc` escape HTML, truncate desc, whitelist filter.
- [ ] Nil lister → "Skills are not available." (mirror `/tasks` nil-store).
- [ ] Menu sync gồm entry `skills` + các entry skill mặc định.
- [ ] Test harness theo pattern `send_placeholder_update_test.go:18-31`: `telego.NewBot("123456:abc...", telego.WithAPICaller(recordingTelegramCaller), telego.WithDiscardLogger())` + `&Channel{...}` để assert message thật gửi đi; pure-function test dựng `&Channel{}` trực tiếp (precedent `handlers_utils_test.go:12`).

## Risk Assessment

- **Thứ tự startup loader-vs-channel** → nếu loader chưa sẵn lúc sync menu: menu dùng fallback description, `/skills` gọi lister lúc runtime (luôn mới). Tín hiệu vỡ: log "0 skills" lúc sync dù có skill → response: truyền *skills.Loader (method value) thay vì snapshot slice.
- **`handleBotCommand` chặn `/skills` trước mention gate trong group**: đúng như `/tasks` đã làm — hành vi mong muốn (lệnh bot trả lời trực tiếp).
- **Sync menu mỗi lần Start()**: đã có DeleteMyCommands trước SetMyCommands (idempotent) — reload instance không trùng lặp.
