---
title: "Phase 10: Skills: wire KitManager + seed_mode + catalog API"
status: todo
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 10: Skills: wire KitManager + seed_mode + catalog API

## Overview
Biến bộ skill goclaw-kit thành **market chọn-cài-được**: wire `KitManager` (hiện dead-code — chỉ ref bởi chính nó) vào HTTP layer, thêm config `skills.seed_mode` để bỏ seed cứng "cả 116 skill", API catalog + install/uninstall job theo template MCP installer (v4.9.4).

## Requirements
- Functional: `GET /v1/skills/market` liệt kê skill trong goclaw-kit (name, description, category, version, installed?, granted); `POST /v1/skills/market/install {slugs[], grantAgentIds?}` job nền copy → seed DB → grants; `DELETE /v1/skills/market/installed/{slug}` gỡ (file managed + DB row); `POST /v1/skills/market/update/{slug}` (KitManager.Update có sẵn); config `skills.seed_mode: all|core|none` điều khiển seeding khởi động.
- Non-functional: install từ **bundled dir nội bộ** (không tải mạng — khác MCP installer); admin-only (requireTenantAdmin — skill là tenant-scoped data); không migration DB mới (dùng skills table + skills-store dir hiện có); Reconciler giữ skill đã cài sống sót qua upgrade (có sẵn `gateway_setup.go:660-676`).

## Architecture
- **Catalog sinh từ nguồn sự thật duy nhất**: đọc `bundled dir` (resolver hiện `gateway_setup.go:610-613`/`GOCLAW_BUNDLED_SKILLS_DIR`) — mỗi subdir có SKILL.md → parse frontmatter (loader.go:625-666 parser có sẵn) + đọc `skills/goclaw-kit/kit.yaml` cho grouping/category. Response row thêm `installed` (exists trong skills DB active?) + `grantedAgents` count (ListAccessible ngược).
- **Enrich metadata**: thêm `category` + `icon` vào frontmatter skill trong `skills/` dần (115 skill — chỉ enrich tối thiểu ~12 category chính theo thư mục nhóm hiện có; thiếu thì fallback "general"). Không block phase.
- **Install job** (`internal/http/skills_market.go` + `internal/skills/market.go`):
  1. Validate slugs tồn tại trong catalog + parse SKILL.md OK ("smoke test" phiên bản skill).
  2. `KitManager.InstallFrom` (đã có: resolve dependency closure `resolver.go`, copy vào `<managedDir>/<slug>/<int-version>/`, ghi `.goclaw/kit.lock` — `kit_manager.go:231-335`) với source = bundled dir.
  3. Seeder upsert từng skill — KHÔNG có helper single-skill sẵn (logic nằm inline trong loop `Seed` :55-174, audit) → extract hàm `seedOne(dir)` dùng primitives `store.UpsertSystemSkill` + `CopyDir` (`seeder.go:376`); Seed loop gọi lại hàm này (refactor không đổi hành vi, test lại idempotent).
  4. Grants nếu có `grantAgentIds` (`skill_store.go:234-242` GrantToAgent có sẵn).
  - Job nền + progress + log: tái dùng lớp job của `internal/http/mcp_install.go:64-127` (in-memory job, poll GET).
- **Uninstall**: disable per-tenant có sẵn + xoá row DB + xoá managed dir (chỉ khi skill origin=bundled; skill custom của user từ chối — check owner_id=system).
- **seed_mode**: `setupSkillsSystem` (`gateway_setup.go:631-658`) đọc config: `all` = như hiện tại (back-compat mặc định); `core` = chỉ seed core list (định nghĩa `internal/skills/core_list.go` ~10: review, plan, cook, skill_search runtime deps + 4 skill designer cho studio agents); `none` = không seed gì. Server anh set `GOCLAW_SKILLS_SEED_MODE=core` qua env overlay (config env pattern có sẵn).
- **goclaw-kit folder**: giữ `skills/goclaw-kit/kit.yaml` là manifest nguồn (115 slugs, có sẵn checksum ComputeChecksum); market KHÔNG add cứng danh sách trong code.

## Related Code Files
- Create: `internal/skills/market.go` (catalog builder + install pipeline gọi KitManager), `internal/skills/core_list.go`, `internal/http/skills_market.go` (handlers + job layer), tests
- Modify: `internal/skills/kit_manager.go` (chỉ expose nếu thiếu method cần), `internal/skills/seeder.go` (extract seedOne), `cmd/gateway_setup.go:631-658` (seed_mode), `internal/config/config.go:518-523` (SkillsConfig thêm SeedMode — hiện chưa có option nào giống, audit), Docker env
- Reference: `internal/http/mcp_install.go` (job template), `internal/mcp/installer/installer.go:96-170` (pipeline pattern), `internal/skills/seeder.go:55-174`, `internal/skills/kit_manager.go:231-432`, `internal/store/skill_store.go:234-242`

## Implementation Steps
1. seed_mode config + core_list + sửa setupSkillsSystem + test 3 mode (fresh data dir).
2. Catalog builder (bundled dir → rows) + endpoint list + test golden file (115 slugs, shape ổn định).
3. Install pipeline (validate → KitManager copy → seed single → grants) + job layer + test E2E local: cài `pdf` skill → skill_search thấy → agent dùng được.
4. Uninstall + update endpoints + test (skill custom bị từ chối gỡ).
5. Enrich category cho kit.yaml grouping (batch script 1 lần trong repo).
6. Surface parity: Web UI phase 11; CLI `cmd/skills_cmd.go` thêm `market list/install` subcommand (parity cmd surface); docs phase 12.

## Success Criteria
- [ ] Fresh install `seed_mode=core` → chỉ ~10 skill core; market hiện đủ 115+ mục với badge "Đã cài" đúng
- [ ] Cài 1 skill qua API → agent search/use được ngay (E2E local); gỡ → sạch file + DB
- [ ] Kit lock ghi đúng sau install (`.goclaw/kit.lock` checksum verify)
- [ ] Skill user-custom KHÔNG bị gỡ được qua market uninstall
- [ ] seed_mode=all (mặc định) hành động y hệt bản cũ (back-compat test)

## Risk Assessment
- Server anh đang chạy đã seed đủ 116 skill: chuyển `core` không GỠ skill đã có (reconciler giữ) — chỉ ảnh hưởng fresh install; market hiện "Đã cài" hết → anh gỡ manual những bộ không muốn. Đây là behaviour mong muốn, ghi chú docs.
- Dependency closure install chọn lẻ (skill A cần _shared): KitManager.Resolve đã lo `_`-prefix shared copy (seeder.go:68-71 pattern) — test case skill có shared dep.
- Nếu kit.yaml listing lệch thư mục thật (115 vs 116): catalog builder lấy thư mục làm nguồn chính, kit.yaml chỉ supplement metadata — tránh dual source of truth đảo ngược.
