---
phase: 1
title: "Designer agent backend — ensure agent + tool allowlist + design skills"
status: done
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Designer agent backend

## Overview
Tạo agent `video-designer` (predefined) được ensure tự động lúc gateway start khi video surface bật: tool allowlist fail-closed (design-only), persona IDENTITY.md, và 2 skill design builtin được grant riêng cho agent.

## Requirements
- Functional:
  - Agent tồn tại sau start (idempotent: tạo nếu thiếu theo agent_key, không đè nếu admin đã sửa)
  - Agent chỉ có tool: `skill_search`, `use_skill`, `session_status`
  - Agent load được 2 skill design qua `use_skill`; skill không hiện với agent khác (visibility internal + grant)
  - System prompt instruct agent luôn kết thúc design bằng fenced block ` ```storyboard {json} ` `
- Non-functional:
  - Ensure-agent chạy async sau start, không block gateway readiness (giống seeder async hiện có)
  - Zero migration DB (dùng cột `tools_config` + bảng `skill_agent_grants` sẵn có)

## Architecture
- `cmd/gateway_video.go` (nơi đã wire video handler/worker) gọi thêm `video.EnsureDesignerAgent(ctx, deps)` — idempotent: `agentStore.GetByKey("video-designer")` → nếu chưa có, build `AgentData`:
  - `AgentKey: "video-designer"`, `DisplayName: "Video Designer"`, `AgentType: predefined`, `Status: active`
  - `ToolsConfig`: `{"allow": ["skill_search", "use_skill", "session_status"]}` — qua `config.ToolPolicySpec` (`internal/config/config_channels.go:676-689`)
  - Provider/Model: default từ `cfg.Agents.Defaults` (pattern `agents_create.go:121-128`)
  - Sau create: set IDENTITY.md (pattern `agents_create.go:183-193`, qua `bootstrap.SeedToStore` + context-file write)
- IDENTITY.md persona (English, LLM consumption): video designer, chỉ design storyboard theo contract v1 (`internal/video/types.go:130-186`), ALWAYS kết thúc bằng ` ```storyboard ` fenced block, KHÔNG có quyền chạy hệ thống, từ chối lịch sự yêu cầu ngoài design, mặc định scene `type:"color"` khi user không cung cấp ảnh, duration 1-30s/cảnh.
- Skills: 2 file SKILL.md mới trong builtin tier của `internal/skills/loader.go` (tier 5, embedded):
  - `video-storyboard-design`: cấu trúc storyboard, nhịp cảnh, caption ngắn ≤8 từ, transition chọn theo mood
  - `video-color-motion`: palette gradient an toàn (hex chuẩn), Ken Burns zoom nhẹ 1.0→1.12, rotate ≤6°, filter sáng/tương phản hợp lý
  - Frontmatter `allowed-tools:` trống (skill chỉ là knowledge, không cần tool)
  - Đăng ký visibility `internal` + `GrantToAgent(designer)` (pattern `internal/store/skill_store.go:235`, HTTP precedent `internal/http/skills.go:124`) — làm thẳng qua store trong ensure hook, không qua HTTP
- Không cho agent vào team nào (tránh `agentToolPolicyWithWorkspace` thêm `write_file` — `internal/agent/resolver_helpers.go:179-192`).

## Related Code Files
- Modify: `cmd/gateway_video.go` (gọi ensure hook)
- Create: `internal/video/designer_agent.go` (EnsureDesignerAgent + persona + skill payloads)
- Create: skill files trong builtin skills dir (xác định vị trí chính xác lúc impl: tier "builtin" của loader — `internal/skills/loader.go:69-73`)
- Read-only reference: `internal/store/agent_store.go:44-113`, `internal/gateway/methods/agents_create.go:24-210`, `internal/tools/policy.go:15-53,240-283`

## Implementation Steps
1. Viết `internal/video/designer_agent.go`: `EnsureDesignerAgent(ctx, cfg, agentStore, skillStore) error` — Get theo key, nếu tồn tại return; ngược lại build AgentData + Create + seed IDENTITY.md
2. IDENTITY.md nhúng const string (pattern bootstrap templates)
3. Viết 2 SKILL.md builtin; nếu builtin tier là embed FS thì thêm file vào đúng embed dir
4. Trong ensure: tạo skill record internal-visibility (nếu chưa theo slug) + `GrantToAgent` cho UUID agent mới
5. Wire gọi từ `cmd/gateway_video.go` (goroutine async, log `video: designer agent ensured`)
6. Test Go: ensure idempotent (gọi 2 lần không duplicate), tools_config parse ra đúng allowlist, agent không có team

## Success Criteria
- [ ] Sau start gateway: `agents.list` (WS) thấy `video-designer` active
- [ ] `tools_config` = đúng 3 tool; FilterTools cho agent này chỉ còn 3 definitions (unit test assertion)
- [ ] Agent resolver: `SkillAllowList` chứa đúng 2 slug skill design (unit test)
- [ ] Go build thường + sqliteonly + vet sạch

## Risk Assessment
- Builtin skills dir có thể là embed FS_readonly → thêm file cần rebuild (bình thường với Go embed). Tín hiệu: loader không thấy skill mới → check embed directive.
- SkillStore interface có thể thiếu Get-by-slug cho idempotency → dùng ListAccessible/List theo pattern `skills_grants.go:410-468`; nếu không có, thêm method nhỏ (không đổi interface công khai khác).
- Nếu gateway chạy nhiều instance (không phải case hiện tại) ensure có thể đua → chấp nhận unique constraint trên agent_key chặn, log ignore duplicate.
