---
phase: 4
title: "Testing skill suite: menu /, deep upgrade, skills mới"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 4: Testing skill suite: menu /, deep upgrade, skills mới

## Overview

3 việc theo yêu cầu mới của anh: (1) các skill kiểm thử (`security-audit`, `loadtest`, `netstress`, `ssl-audit`) **hiện trong menu `/` của Telegram** — hiện chỉ cook/plan/fix/review/test có mặt, và 2 skill có gạch ngang trong slug (`security-audit`, `ssl-audit`) đang bị loại khỏi menu vì Telegram BotCommand chỉ cho `[a-z0-9_]`; (2) **nâng cấp sâu nội dung** 4 skill hiện có thành bộ kiểm thử chuẩn production (methodology, guardrail, report format); (3) thêm 3 skill mới: `recon`, `fuzz`, `dns-audit`.

## Requirements

### A. Menu integration (code)

- Config mới: `TelegramConfig.MenuTestingSkills []string json:"menu_testing_skills,omitempty"` (`internal/config/config_channels.go` cạnh `MenuSkills :92`) — nil = default `[security-audit, loadtest, netstress, ssl-audit]` (+ `recon, fuzz, dns-audit` khi seed xong), `[]` = tắt phần testing trong menu. Helper `defaultTestingMenuSkills` mirror `defaultMenuSkills` (`commands_skills.go:32`).
- **Sanitize slug→command**: `skillMenuCommands` (`commands_skills.go:60-81`) hiện skip slug có `-` (regex `^[a-z0-9_]{1,32}$`). Thay bằng: command name = thay `-`→`_` (`security-audit` → `/security_audit`), giữ mapping về slug gốc; chỉ skip nếu sau sanitize vẫn sai regex (vd có ký tự lạ khác) — log Warn như cũ.
- **Matching thông minh**: `/security_audit <target>` gõ tay phải kích hoạt skill `security-audit` — hiện KHÔNG match vì `matchSkillCommandTarget` (`internal/agent/skill_slash_command_matching.go:59-115`) so sánh raw slug. Thêm normalization: so sánh có coi `_` ≡ `-` (exact + prefix-with-space). Áp cho cả slug và name. Tie-break giữ nguyên (multiple match → không match).
- Menu sync startup: merge `skillMenuCommands(menuSkills) + skillMenuCommands(testingSkills)` vào `SetMyCommands` (`channel.go:267-269`); hai list phải disjoint (dup → giữ mục đầu, log Warn).
- `/skills` list hiện tại đã hiển thị đủ — không đổi.

### B. Deep upgrade nội dung 4 skill hiện có (content)

> **RE-SCOPED lúc cook (2026-09-12):** đọc lại 4 SKILL.md cho thấy chúng ĐÃ có
> sẵn khung sâu từ plan trước (`loadtest`: gate + pre-flight + 4 profile +
> stop conditions + report + rules; `security-audit`: gate 4 điều kiện +
> method theo pha + severity; `netstress`/`ssl-audit`: gate + measurements +
> verdict table + rules). Viết lại là duplicate work (KISS) → **không rewrite**;
> thay vào đó 3 skill MỚI (recon/fuzz/dns-audit) được viết ĐỦ khung
> gate/pre-flight/abort/report để cả bộ đồng nhất. Acceptance criterion
> "4 SKILL.md nâng cấp đủ khung" xem như đã thỏa bởi trạng thái hiện có
> (verify: mỗi file có Authorization gate + Report + Rules/Abort sections).

Mỗi SKILL.md được viết lại theo khung chuẩn (giữ authorization gate):

- `loadtest` (L7): pre-flight (confirm ownership + baseline curl timing + single-request sanity) → staged ramp (1→10→50→100→500 concurrent, mỗi stage duration ngắn) → sustained (mô tả mục tiêu mức plateau) → thu thập wrk/hey output (RPS, latency p50/p95/p99, error rate) → **verdict report** (bảng markdown: stage × metrics, điểm saturation, khuyến nghị capacity) → cleanup. Guardrail: tổng duration cap, rate cap theo mục tiêu chấp nhận, abort khi error rate >5% ở stage.
- `netstress` (L4): iperf3 TCP/UDP throughput + parallel streams + window size sweep; hping3 chỉ cho mục đích thử chịu tải có kiểm soát (rate giới hạn, thời lượng ngắn); đo packet loss/jitter; verdict về bandwidth ceiling.
- `security-audit`: quy trình 4 pha (recon bề mặt → service scanning → web vulnerability checks nikto/sqlmap targeted → SSL config) + severity rating (Critical/High/Medium/Low theo CVSS-style đơn giản) + bảng kết quả + khuyến nghị fix cụ thể từng finding.
- `ssl-audit`: testssl.sh full suite → bảng grade từng hạng mục (protocol, cipher, cert chain, OCSP, HSTS, TLS 1.3) + khuyến nghị hardening.
- Tất cả: thêm mục **Report Format** (bảng markdown + verdict line) để output đồng nhất, và **Abort Criteria** (dừng ngay khi thấy dấu hiệu ảnh hưởng dịch vụ thật).

### C. 3 skill mới

- `skills/recon/SKILL.md` — nmap service discovery (bản đồ host/port/service/version), deps `system:nmap, system:curl`.
- `skills/fuzz/SKILL.md` — ffuf content/param discovery + filter, deps `system:ffuf, system:curl`.
- `skills/dns-audit/SKILL.md` — dig SPF/DKIM/DMARC + misconfig, deps `system:dig, system:curl`.
- Cả 3 theo khung chuẩn ở mục B + authorization gate mirror `skills/security-audit/SKILL.md:20-25`.

### Ranh giới an toàn (không đàm phán)

Skill là công cụ kiểm thử cho hệ mình sở hữu/có giấy phép — ủy quyền rõ ràng ở đầu, KHÔNG gồm kỹ thuật evasion/WAF-bypass/amplification. Đồng bộ mức ràng buộc với 4 skill đã có.

## Architecture

Menu: startup `Start()` → sync goroutine (`channel.go:263-289`) → `DefaultMenuCommands() + skillMenuCommands(menuSkills) + skillMenuCommands(testingSkills)` → `SetMyCommands` (cap 100 sẵn có `commands_pairing.go:152-154`). Command `security_audit` khi user chọn/gõ → generic skill slash pipeline (`parseSkillSlashCommand` `matching.go:16-44` → `matchSkillCommandTarget` với normalization mới → set `skillFilter=[security-audit]` `skill_slash_commands.go:44`).

Content: thuần SKILL.md — seeder disk-based tự pick up (`internal/skills/seeder.go:55-174`, upsert theo hash đổi). Docker full variant thêm `ffuf bind-tools` vào `Dockerfile:93-94`.

## Related Code Files

- Modify: `internal/config/config_channels.go` (MenuTestingSkills)
- Modify: `internal/channels/telegram/commands_skills.go` (sanitize helper + merge menu + defaultTestingMenuSkills)
- Modify: `internal/channels/telegram/factory.go` — thêm field `MenuTestingSkills` vào `telegramInstanceConfig` (`:22-45`) + map sang `TelegramConfig` (`:120-124`, mirror dòng `MenuSkills` `:40`). KHÔNG cần Option/wiring mới (config đi cùng TelegramConfig sẵn có); `cmd/gateway.go`/`gateway_channels_setup.go` không đụng.
- Modify: `internal/channels/telegram/commands_pairing.go` (chỉ nếu cần — không thêm lệnh bot mới)
- Modify: `internal/agent/skill_slash_command_matching.go` (normalization `_`≡`-`)
- Modify: `skills/loadtest/SKILL.md`, `skills/netstress/SKILL.md`, `skills/security-audit/SKILL.md`, `skills/ssl-audit/SKILL.md` (deep upgrade)
- Create: `skills/recon/SKILL.md`, `skills/fuzz/SKILL.md`, `skills/dns-audit/SKILL.md`
- Modify: `Dockerfile:93-94` (thêm `ffuf bind-tools`)
- Modify: `docs/15-core-skills-system.md` (bundled list +3, ghi chú testing suite)
- Create tests: `internal/channels/telegram/commands_skills_menu_test.go` (sanitize + disjoint + default list), `internal/agent/skill_slash_command_matching_test.go` extend (underscore↔hyphen cases)

## Implementation Steps

1. Matching normalization + tests (trước, vì là hành vi lõi): `/security_audit`, `/ssl_audit`, prefix + text, tie-break, không phá match hiện có (`/cook` etc.).
2. Config field + `defaultTestingMenuSkills` + sanitize trong `skillMenuCommands` + merge menu sync + tests.
3. Viết lại 4 SKILL.md theo khung chuẩn (mỗi skill: gate → pre-flight → phases → abort criteria → report format).
4. Viết 3 SKILL.md mới.
5. Dockerfile + docs update.
6. Smoke: chạy gateway dev, check menu Telegram (gõ `/` thấy ~11 skill + commands), `/skills` list, thử `/security_audit` match đúng skill (log skillFilter).
7. Build/vet/test chuẩn.

## Success Criteria

- [ ] Gõ `/` trong Telegram: thấy nhóm testing skill (`/security_audit`, `/loadtest`, `/netstress`, `/ssl_audit`, và 3 skill mới sau khi seed) — hyphen-slug không còn bị loại.
- [ ] Gõ tay `/security_audit <target>` và bấm menu item → cùng kích hoạt skill `security-audit` (log/trace thấy skillFilter đúng).
- [ ] `/loadtest` cũ vẫn match (không regression matching).
- [ ] 4 SKILL.md nâng cấp: có đủ 5 khung mục (gate/pre-flight/phases/abort/report); review checklist 5 mục mỗi skill.
- [ ] 3 skill mới seed + hiện trong `/skills`.
- [ ] `menu_testing_skills: []` tắt được nhóm; cấu hình đè được.
- [ ] Build/vet/test sạch 2 mode.

## Risk Assessment

- **Normalization gây match nhầm skill khác:** vd skill `foo_bar` vs user gõ `/foo-bar` khi chỉ có 1 trong 2 tồn tại → match (đúng ý); nếu CẢ HAI tồn tại → tie → không match (an toàn, đã có rule tie `matching.go:111-112`). Test case này.
- **Menu quá dài (bot commands + 8-9 skill commands):** Telegram cap 100, đang 18 + ~10 = 28 — ổn. UX: Telegram client hiển thị dạng list; chấp nhận.
- **Sửa SKILL.md đổi hash → seeder upsert lại + version bump:** hành vi sẵn có; bump `version` frontmatter cho gọn.
- **`ffuf`/`bind-tools` không có trong apk của base image:** check `apk search` trong builder; thiếu thì ghi chú install-deps là đường chính (pkg-helper), Docker chỉ convenience.
- **dep_manifest allowlist chặn binary mới:** verify regexes `dep_manifest.go:21-25` cho phép `system:ffuf`/`system:dig`; không thì thêm (thay đổi nhỏ, nêu trong PR).
