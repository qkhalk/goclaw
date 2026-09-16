---
phase: 1
title: "Author 12 design-studio skills"
status: pending       # pending | in-progress | completed
priority: P1          # P1 | P2 | P3
effort: "2d"
dependencies: []
---

# Phase 1: Author 12 design-studio skills

## Overview

Viết 12 thư mục `skills/<slug>/` nguyên bản: SKILL.md + `references/` + 4 script Python stdlib. Mỗi skill tự chứa workflow chính trong body; references chỉ là lookup tables.

## Requirements

- Functional: mỗi skill có frontmatter đúng YAML-subset (name, description ≤1024 chars, version, inputs/outputs block lists, allowed-tools khi cần), body <300 dòng tiếng Anh, bilingual EN+VI triggers + "Do NOT use for..." trong description
- Non-functional: parse sạch qua loader goclaw không warn; không dependencies ngoài stdlib; không copy văn bản từ bất kỳ skill cá nhân nào

## Architecture

Vị trí trong pipeline design (3 nhóm):
- **Bookend:** `creative-brief` (intake) ↔ `design-review` (critique) ↔ `design-handoff` (xuất khầu cho builder)
- **Hệ thống nền:** `color-system`, `typography`, `composition`, `motion-design`, `visual-styleguide` (consistency backbone)
- **Pipeline video:** `storyboard`, `image-direction`, `video-direction` (wrap tools `create_image`/`create_video`/`render_video` sẵn có — zero gateway change)

Scripts (4, stdlib only):
- `storyboard/scripts/shot_table.py` — shot list MD/CSV renderer (argparse, đọc JSON stdin/file)
- `color-system/scripts/contrast_check.py` — WCAG ratio từ cặp hex (pháp thức WCAG 2.1 relative luminance)
- `typography/scripts/scale_gen.py` — type scale table từ ratio + base size
- `design-handoff/scripts/export_tokens.py` — styleguide MD → CSS vars/JSON bundle

## Related Code Files

- Create: `skills/creative-brief/SKILL.md` (+ `references/brief-template.md`, `references/question-checklist.md`)
- Create: `skills/storyboard/SKILL.md` (+ `references/shot-grammar.md`, `references/continuity-rules.md`, `references/shot-list-schema.md`, `scripts/shot_table.py`)
- Create: `skills/color-system/SKILL.md` (+ `references/color-harmony.md`, `references/contrast-checks.md`, `references/token-export-format.md`, `scripts/contrast_check.py`)
- Create: `skills/typography/SKILL.md` (+ `references/type-scale.md`, `references/pairing-guide.md`, `references/diacritics-notes.md`, `scripts/scale_gen.py`)
- Create: `skills/composition/SKILL.md` (+ `references/grid-systems.md`, `references/aspect-ratio-specs.md`, `references/platform-deliverables.md`)
- Create: `skills/motion-design/SKILL.md` (+ `references/easing-reference.md`, `references/transition-grammar.md`, `references/timing-budget.md`)
- Create: `skills/visual-styleguide/SKILL.md` (+ `references/style-tile-workflow.md`, `references/consistency-audit.md`)
- Create: `skills/image-direction/SKILL.md` (+ `references/prompt-anatomy.md`, `references/style-vocabulary.md`, `references/iteration-playbook.md`)
- Create: `skills/video-direction/SKILL.md` (+ `references/video-prompt-anatomy.md`, `references/sequence-planning.md`, `references/render-spec-format.md`)
- Create: `skills/design-review/SKILL.md` (+ `references/critique-rubric.md`, `references/severity-scale.md`)
- Create: `skills/design-assets/SKILL.md` (+ `references/asset-naming.md`, `references/export-matrix.md`)
- Create: `skills/design-handoff/SKILL.md` (+ `references/token-bundle-schema.md`, `references/spec-sheet-template.md`, `scripts/export_tokens.py`)
- Modify: không file code nào
- Delete: không

## Implementation Steps

1. Viết `creative-brief` trước (entry point — các skill khác tham chiếu cấu trúc brief của nó)
2. Viết 5 skill hệ thống nền (color/typography/composition/motion/styleguide) — mỗi skill chuẩn hóa output là markdown section có heading cố định để skill khác consume
3. Viết 3 skill pipeline video — `storyboard` phải xuất shot list schema khớp storyboard contract v1 của video editor (đối chiếu `ui/web/src/pages/tools/video/` validation rules trước khi viết)
4. Viết 3 skill bookend (review/assets/handoff)
5. Viết 4 script Python + test thủ công từng script (run locally với sample input)
6. Pass quét nguyên bản: xem lại 12 skill, đảm bảo không câu văn nào lấy từ bộ skill ngoài

## Success Criteria

- [ ] 12/12 thư mục có SKILL.md parse được (kiểm tra bằng Go test nhỏ hoặc khởi gateway local đọc log scan)
- [ ] Mỗi description: ≤1024 chars, có ≥3 EN keywords + ≥3 VI triggers + 1 dòng "Do NOT use for..."
- [ ] 4 script chạy được với Python 3.10+ stdlib (`python3 script.py --help` exit 0)
- [ ] Không file nào >300 dòng; tổng SKILL.md của 3 skill pin < 30KB
- [ ] Self-scan nguyên bản: không trùng đoạn văn với nguồn ngoài

## Risk Assessment

- Viết dàn trải quá chi tiết → vượt budget inline: giữ body là workflow + quyết định, bảng tra số liệu đẩy xuống references. Tín hiệu vỡ: loader log fallback pointer-only — phản ứng: cắt body xuống <10KB/skill.
- Schema storyboard lệch contract video editor: verify validation rules client trước (file `render-shared.ts`/`loadJson` path) — nếu lệch, sửa references/shot-list-schema.md chứ không đổi contract editor.

