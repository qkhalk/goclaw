---
phase: 4
title: "E2E validation + docs"
status: pending
priority: P2
effort: "0.5d"
dependencies: [phase-03]
---

# Phase 4: E2E validation + docs

## Overview

Chạy kịch bản E2E dùng bài báo công nghệ thật: brief → storyboard → storyboard JSON apply vào video editor; viết docs cho bộ skill.

## Requirements

- Functional: agent designer (hoặc agent test có skill) consume 1 bài báo công nghệ → trả storyboard JSON hợp lệ contract v1
- Non-functional: toàn bộ flow qua skill (creative-brief → storyboard), không can thiệp thủ công giữa chừng

## Architecture

Kịch bản E2E (trùng yêu cầu của anh ở item 2 — dùng chung kết quả với plan 260915-1243 Phase 5):
1. Lấy 1 bài báo công nghệ (URL thật, ví dụ bài từ The Verge/Ars TechnicaVN)
2. Chat với agent designer: "Đọc bài này và làm video 30s giới thiệu nội dung" (kèm URL)
3. Agent: `creative-brief` → tóm tắt thông điệp chính → `storyboard` → shot list + storyboard JSON
4. Verify JSON qua validation rules của video editor (`hasValidationErrors`)
5. Nếu plan 260915-1243 Phase 3 đã ship StoryboardCard: Apply vào timeline và render — full E2E; nếu chưa: verify JSON schema bằng unit rules

Docs: `skills/design-studio/README.md` (mục đích bộ skill, sơ đồ pipeline, cách pin/grant, cách đóng góp skill mới).

## Related Code Files

- Create: `skills/design-studio/README.md`
- Modify: không
- Delete: không

## Implementation Steps

1. Viết README.md cho kit
2. Chọn bài báo công nghệ (tiếng Việt ưu tiên — test diacritics + VI triggers)
3. Chạy kịch bản E2E qua UI http://192.168.1.103:18790 (agent designer) hoặc API chat
4. Ghi lại: screenshot storyboard card, JSON output, kết quả validation
5. Báo cáo: những chỗ skill hướng agent sai/hụt → quay lại Phase 1 sửa description/workflow

## Success Criteria

- [ ] Agent tự dùng `creative-brief` + `storyboard` (thấy trong trace/tool calls) không cần nhắc tên skill
- [ ] Storyboard JSON pass validation của editor (scenes có duration, source hợp lệ)
- [ ] README giải thích đủ để người khác thêm skill thứ 13 mà không hỏi
- [ ] Báo cáo E2E lưu `plans/reports/design-studio-e2e-<date>.md`

## Risk Assessment

- Agent không tự chọn đúng skill: BM25 ranking yếu hoặc description chung chung — tín hiệu: agent trả lời không dùng skill; phản ứng: cải thiện description keywords (thêm VI triggers), không hard-code workflow.
- Bài báo dài vượt context: `creative-brief` hướng dẫn trích xuất theo mục lục nội dung (headline, lead, 3 điểm chính) — nếu vẫn tràn, giảm maxChars qua tham số tool đọc web sẵn có.

