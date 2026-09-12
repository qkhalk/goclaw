---
phase: 5
title: "Dev mode v2 + polish"
status: completed
priority: P2
effort: "4h"
dependencies: ["phase-3", "phase-4"]
---

# Phase 5: Dev mode v2 + polish

## Overview

Nâng dev mode từ "prompt section" thành chế độ làm việc thực sự: /dev thành picker có nút, prompt section v2 dạy dùng ask_options + verify-before-conclude, status line chi tiết hơn khi dev, và các polish tổng (menu, docs).

## Requirements

- Functional:
  - `/dev` (no-arg) → inline keyboard: `[ON] [OFF]` (+ hàng thông tin hiện trạng trong text). Bấm → edit xác nhận (dùng cơ chế Phase 1).
  - **DevModePromptSection v2** (`internal/agent/dev_mode_prompt.go`): thêm 3 điều:
    1. Khi yêu cầu mơ hồ và có 2-4 cách hiểu → DÙNG tool `ask_options` (nêu tên tool) thay vì hỏi tự do.
    2. Verify trước khi kết luận: build/test sau thay đổi; không khẳng định "done" khi chưa chạy.
    3. Output đúng format làm việc: diff ngắn, lệnh đã chạy, kết quả — không văn hoa.
  - Khi dev mode ON và chưa có thinking override → status line hiện `Think: auto (dev suggests high)` KHÔNG tự ép (giữ tường minh — anh bấm /thinking high nếu muốn).
  - `/help` cập nhật các lệnh mới (picker, reasoning, ask). Menu commands thêm `reasoning`.
  - Docs `docs/25-telegram-runtime-commands.md` viết lại phần pickers + ask-options + locale (kèm ảnh mô tả flows).
  - CHANGELOG entry.
- Non-functional: không đổi persistence (metadata key như cũ); mọi string mới qua i18n (Phase 4 xong trước).

## Architecture

Thuần phần chồng lên nền đã có: picker dùng `pickers.go` Phase 1; prompt v2 chỉ sửa const + test; status line thêm 1 nhánh trong `commands_status.go`.

## Related Code Files

- Modify: `internal/agent/dev_mode_prompt.go` (+ test v2: chứa "ask_options", "verify")
- Modify: `internal/channels/telegram/commands_dev.go` (picker ON/OFF)
- Modify: `internal/channels/telegram/commands_status.go` (dev hint line)
- Modify: `internal/channels/telegram/commands.go` (/help), `commands_pairing.go` (menu + reasoning nếu Phase 1 chưa)
- Modify: `docs/25-telegram-runtime-commands.md`, `CHANGELOG.md`

## Implementation Steps

1. Prompt v2 + test.
2. /dev picker (reuse Phase 1 helpers) + test.
3. Status dev hint + help + menu.
4. Docs + changelog.
5. Manual E2E full flow: `/dev on` → yêu cầu mơ hồ → agent ask_options 3 lựa chọn → bấm → agent làm → `/status` thấy Mode: dev.
6. Build/vet/test chuẩn.

## Success Criteria

- [ ] `/dev` bấm ON → metadata chat_mode=dev + message edit xác nhận.
- [ ] Dev prompt chứa hướng dẫn ask_options + verify (test string).
- [ ] E2E thủ công pass (flow 5 ở steps).
- [ ] Docs phản ánh đúng hành vi mới.
- [ ] Build/vet/test sạch 2 mode.

## Risk Assessment

- Model yếu không theo prompt (không gọi ask_options) — đã ghi R6 plan.md; đo usage sau deploy, nếu thấp → tăng cường tool description thay vì ép code.
- Scope creep presets (Deep...) — để backlog Suggestions, KHÔNG làm trong phase này.
