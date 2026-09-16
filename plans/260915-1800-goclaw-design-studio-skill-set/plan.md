---
title: "GoClaw design-studio skill set"
description: "Bộ 12 skill design gốc cho goclaw (creative-brief → storyboard → color/typography/composition/motion → styleguide → image/video-direction → review/assets/handoff), authored theo đúng SKILL.md contract hiện có, kèm kit.yaml design-studio, grants cho agent designer, deploy lên server và verify E2E. Không port skill cá nhân của ai — 100% nội dung nguyên bản viết cho goclaw."
status: pending
priority: P1
effort: "~3-4 ngày (4 phase)"
tags: [skills, design, video, seeding, kit]
created: 2026-09-15
related: [260915-1243-video-designer-agent-column]
---

# GoClaw design-studio skill set

## Overview

GoClaw hiện có 34 skill deployed nhưng **không có skill design nào** (gap analysis: `plans/reports/goclaw-skills-research.md` §5 — thiếu hoàn toàn storyboard, color system, typography, motion, composition, style consistency, generation prompt craft, design critique). Bộ "design-studio" lấp đầy gap bằng **12 skill nguyên bản** viết riêng cho goclaw contract:

1. `creative-brief` — intake yêu cầu → brief có cấu trúc (entry point)
2. `storyboard` — brief/script → shot plan + generation prompts
3. `color-system` — palette + WCAG contrast + token export + grading intent
4. `typography` — type scale + pairing + diacritics (vi/cjk) + kinetic type
5. `composition` — grid, aspect ratio, safe areas, focal hierarchy
6. `motion-design` — easing/timing tables + transition grammar
7. `visual-styleguide` — style tile nhất quán cho cả project
8. `image-direction` — prompt craft cho tool `create_image` sẵn có
9. `video-direction` — prompt craft + sequencing cho `create_video`/`render_video`
10. `design-review` — critique theo rubric, severity-ranked (như skill `review`)
11. `design-assets` — naming/versioning/export conventions
12. `design-handoff` — token bundle + spec sheet cho builder agents

**Ràng buộc cứng từ anh:** nội dung phải nguyên bản cho goclaw, tuyệt đối không port skill cá nhân từ bộ 106-skill của anh. Chỉ mượn *pattern cấu trúc* của chính goclaw (bilingual triggers, progressive disclosure, `{baseDir}` references) — đã đúc kết trong research report.

**Zero gateway code change** — skill wrap quanh các tool sẵn có (`create_image`, `create_video`, `render_video`, `read_image`, `read_video`); shipping = thư mục `skills/<slug>/` + `kit.yaml`, seeder tự đưa vào DB khi gateway start (§2.2 research).

## Kiến trúc đã chọn (từ research report, file:line verified)

- **SKILL.md contract:** slug = directory name `^[a-z0-9][a-z0-9-]*[a-z0-9]$` (`internal/skills/helpers.go:10`); YAML-subset frontmatter (không flow lists) (`loader.go:625-666`); `description` ≤1024 ký tự là corpus BM25 — nhúng trigger keywords **EN + VI** + "Do NOT use for..." anti-triggers; body <300 dòng; `references/*.md` <300 dòng mỗi file qua `{baseDir}` placeholder; inline budget 10KB/skill, 30KB tổng (`loader.go:483-492`).
- **Bundled path:** repo `skills/<slug>/` → server `/opt/goclaw/skills/<slug>/` (`GOCLAW_BUNDLED_SKILLS_DIR`) → `Seeder.Seed()` upsert system skill (visibility `public`, `is_system`) idempotent theo content-hash (`seeder.go:55-174`).
- **Grant cho designer agent:** seed `public` cho mọi agent dùng được; riêng agent `video-designer` **pin** 3 skill lõi (`creative-brief`, `storyboard`, `visual-styleguide`) qua `other_config.pinned_skills` (`internal/store/agent_store.go:316-326`) để luôn inline SKILL.md body (`loader.go:494-555`).
- **Kit:** `skills/design-studio/kit.yaml` manifest `{name, version, description, skills: [12 slugs]}` theo `internal/skills/kit_manager.go:25-31` (pattern của `go-claw-engineer` kit deployed).
- **Lite/desktop safe:** chỉ 4 script Python stdlib (không deps ngoài) → không cần `skill_manage`/`publish_skill` (không đăng ký trên lite, `gateway_setup.go:680-692`); `requires:` chỉ dùng khi thật sự cần binary.

## Cross-surface parity

- **Gateway server:** N/A because zero code change — chỉ file skills + kit manifest (auto-seeded).
- **API contract:** N/A because dùng nguyên endpoints upload/grant sẵn có.
- **Web UI:** N/A because trang Skills render từ DB rows tự động; không thêm UI string.
- **CLI/runtime:** N/A because `goclaw` skill commands hoạt động trên seeded rows không đổi.
- **Editions:** chạy cả Standard + Lite (consumption read-only).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | 12 skill design nguyên bản đúng goclaw contract, parse + search được | P1 |
| 2 | Kit `design-studio` nhận diện được qua kit manager | P2 |
| 3 | Agent `video-designer` pin 3 skill lõi, thấy đủ 12 skill | P1 |
| 4 | Deploy server + verify seeding (DB rows, BM25 hit, pinned inline) | P1 |
| 5 | E2E: bài báo công nghệ → brief → storyboard → storyboard JSON agent dùng được | P1 |

## Phases

| # | Phase | Status | Priority |
|---|-------|--------|----------|
| 1 | [Phase 1: Author 12 design-studio skills](./phase-01-start.md) | Pending | P1 |
| 2 | [Phase 2: Kit manifest + repo bundling + loader validation](./phase-02-kit-manifest-repo-bundling-loader-validation.md) | Pending | P1 |
| 3 | [Phase 3: Grants + server deploy + seeding verify](./phase-03-grants-for-designer-agent-server-deploy-seeding-verify.md) | Pending | P1 |
| 4 | [Phase 4: E2E validation + docs](./phase-04-e2e-validation-docs.md) | Pending | P2 |

## Success Criteria

- [ ] Loader parse sạch 12/12 skill (gateway scan log không warn; test Go nhỏ gọi `skills.Load` trên thư mục bundle)
- [ ] `skill_search` với query tiếng Việt ("kịch bản phân cảnh bảng màu") và tiếng Anh ("storyboard color palette") đều trả đúng skill trong top-3
- [ ] Agent `video-designer` có `pinned_skills` = 3 skill lõi → body inline trong system prompt; 12 skill đều xuất hiện trong `<available_skills>` hoặc search được
- [ ] Server restart sau copy: journalctl không có slug conflict/seeding error; 12 rows `is_system` mới trong DB (`skills` table)
- [ ] E2E Phase 4: feed 1 bài báo công nghệ → agent dùng `creative-brief` → `storyboard` → storyboard JSON hợp lệ contract v1 (video editor apply được — nối với plan 260915-1243 Phase 3)
- [ ] GoClaw Lite (sqliteonly build) vẫn load đủ 12 skill (không deps ngoài stdlib)
- [ ] Kiểm tra nguyên bản: quét nội dung 12 skill không trùng văn bản với bộ skill cá nhân của anh (chỉ khái niệm chung ngành được trùng)

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| BM25 không hit vì description thiếu từ khóa | Phase 1 viết description theo formula đã verify ở 34 skill deployed: EN keywords + VI triggers ("kịch bản, bảng màu, kiểu chữ, chuyển động") + anti-triggers; Phase 4 test search thực tế |
| YAML-subset parser từ chối frontmatter phức tạp | Chỉ dùng `key: value` + block lists (`inputs:\n  - x`) — không flow lists, không nested maps (loader.go:730-799 đã map rõ subset) |
| Trùng slug với skill tương lai của upstream | Tên đặc thù đủ (`visual-styleguide`, `design-handoff`); kiểm tra `bundled-skills` upstream trước khi merge |
| Skill quá dài vượt budget inline 10KB | SKILL.md giữ <300 dòng; chi tiết đẩy xuống `references/`; chỉ 3 skill pinned nên 30KB tổng budget dư sức |
| Agent không tự đọc reference files | SKILL.md body phải tự chứa workflow chính; references chỉ là lookup tables (pattern của `pdf/reference.md`) |
| Deploy server cần restart gateway | Trùng lịch với phase deploy của plan video-designer — làm chung 1 lần restart |

## Non-Goals

- KHÔNG port bất kỳ skill nào từ bộ 106-skill của anh (ràng buộc tuyệt đối).
- KHÔNG thêm gateway code, WS method, hay migration DB.
- KHÔNG làm UI mới cho skills management (trang sẵn có đủ).
- KHÔNG skill `deck-design` thứ 13 (defer — `pptx` đã cover mechanics).
- KHÔNG benchmark/load test (per AGENTS.md skip rule).

## Conventions

- Skill content: tiếng Anh (LLM consumption — cùng policy bootstrap templates); VI triggers nhúng inline trong description, không đụng locale files.
- Script Python: stdlib only, có `if __name__ == "__main__"`, docstring usage, exit code đúng.
- Mọi claim file:line trong plan phải verify lại trước khi implement (AGENTS.md Plan Verification Rules).

<!-- slug: goclaw-design-studio-skill-set -->
