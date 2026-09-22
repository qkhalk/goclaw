---
phase: 3
title: "Grants for designer agent + server deploy + seeding verify"
status: pending
priority: P1
effort: "0.5d"
dependencies: [phase-02]
---

# Phase 3: Grants for designer agent + server deploy + seeding verify

## Overview

Deploy 12 skill + kit lên server 192.168.1.103, cấu hình pin 3 skill lõi cho agent `video-designer`, verify seeding vào DB và hiển thị đúng trong prompt.

## Requirements

- Functional: 12 skill + kit xuất hiện trong DB (`skills` table, `is_system=true`), agent `video-designer` có `pinned_skills` = [creative-brief, storyboard, visual-styleguide]
- Non-functional: restart gateway 1 lần duy nhất (dùng chung với phase deploy plan 260915-1243 nếu trùng lịch); seeding idempotent (chạy lại không nhân bản rows)

## Architecture

- Skill bundled seed `public` + `is_system` → mọi agent thấy qua `<available_skills>`/search; không cần grant row riêng cho visibility public (research §3.3: chỉ `internal` mới cần grants)
- Pin: cập nhật `agents.other_config.pinned_skills` của agent `video-designer` (agent do plan 260915-1243 Phase 1 ensure). Nếu agent đó chưa tồn tại (plan chưa chạy), pin qua config seed của ensure-agent hook luôn
- Deploy: `scp -r skills/<12 slug> skills/design-studio root@192.168.1.103:/opt/goclaw/skills/` → `systemctl restart goclaw`

## Related Code Files

- Modify: `internal/video/designer_agent.go` (nếu cần thêm pinned_skills vào ensure config — chỉ khi plan 260915-1243 đã tạo file này; nếu chưa, để Phase 1 plan đó consume)
- Create: không
- Delete: không

## Implementation Steps

1. Verify trước khi copy: server chưa có slug trùng (`ls /opt/goclaw/skills | grep -E 'creative-brief|storyboard|...'`)
2. Copy 13 thư mục (12 skill + kit) lên `/opt/goclaw/skills/`
3. Restart goclaw, theo dõi `journalctl -u goclaw` cho log seeding
4. Verify DB: `SELECT slug, is_system, visibility, status FROM skills WHERE slug IN (...)` — đủ 12 rows active
5. Verify prompt: mở chat với agent designer, kiểm tra system prompt/trace có `<skill_instructions>` của 3 skill pin + `<available_skills>` chứa đủ 12
6. Verify search: qua agent gọi `skill_search("bảng màu")` → trả color-system

## Success Criteria

- [ ] 12 rows `is_system=true, visibility=public, status=active` trong DB sau restart
- [ ] Agent designer system prompt inline 3 skill lõi (kiểm tra qua trace hoặc debug)
- [ ] `skill_search` VI + EN query đều hit đúng (thủ công qua chat)
- [ ] Restart lần 2 không tạo rows trùng (idempotency theo content-hash)
- [ ] journalctl không có `security.*` hay seeding error mới

## Risk Assessment

- Slug conflict với custom skill có sẵn trên server: seeder skip (`ErrSystemSkillSlugConflict`, seeder.go:134-138) — tín hiệu: log skip khi start; phản ứng: đổi tên slug hoặc gỡ skill cũ sau khi xác nhận chủ sở hữu.
- Agent `video-designer` chưa tồn tại (plan song song chưa merge): pin sau bằng UPDATE SQL hoặc WS `agents.update` — không block phase này.

