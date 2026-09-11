---
phase: 2
title: "Dev mode /dev"
status: pending
priority: P1
effort: "4h"
dependencies: ["phase-1"]
---

# Phase 2: Dev mode /dev

## Overview

Lệnh `/dev on|off` bật dev mode chuyên code cho chat: inject section hành vi "senior engineer + plan-first + hỏi làm rõ khi mơ hồ + xác nhận trước thao tác phá hủy" vào system prompt qua `ExtraPrompt`, phụ trợ bằng ask_user reminder đã có. Đây là hành vi "đặt câu hỏi xác nhận như zcode/claude code/codex" mà anh muốn — mức prompt-guided, KHÔNG xây cơ chế pause/resume run mới.

## Requirements

- Functional:
  - `/dev` (không tham số) → hiện mode hiện tại (dev/normal) của chat.
  - `/dev on` → set `chat_mode=dev` trong session metadata (pattern Phase 1), reply mô tả ngắn hành vi mới.
  - `/dev off` → set `chat_mode=""` (về normal).
  - Group: writer-only (như Phase 1); DM tự do.
  - Khi dev mode ON, mọi run của chat có system prompt chứa dev section (verify qua span input_preview); khi OFF không còn.
  - `/status` (Phase 3) hiển thị `Mode: dev`.
- Non-functional: prompt section tiếng Anh (bootstrap templates English-only — AGENTS.md); không đổi PromptMode (vẫn `full`); không migration.

## Architecture

**Prompt section** — nội dung (draft, tinh chỉnh lúc impl, giữ ngắn <40 dòng để không phình system prompt):

```markdown
## DEV MODE ACTIVE

You are operating as a hands-on software engineer inside the user's repository.
- Plan before acting: for non-trivial changes, state a short plan (files, approach) first.
- Ask before assuming: if the request is ambiguous, missing context, or has multiple
  valid interpretations, ASK ONE clarifying question instead of guessing.
- Confirm destructive or slow operations (deletes, bulk rewrites, long installs,
  deploys) before running them.
- Prefer minimal diffs; match existing code style; never leave the build broken —
  run build/tests after changes when feasible.
- Report honestly: failures, skipped steps, and verification results.
When you ask a question, end your turn and wait. Optionally set an ask_user
reminder as a follow-up nudge.
```

Cuối cùng dựa trên cơ chế thật: ask_user chỉ đặt reminder (`resolver_helpers.go:117-119` guidance + `team_tasks_followup.go`), không pause — câu "end your turn and wait" là hành vi prompt thuần, đúng sự thật.

**Data flow (audit-verified — thiết kế consumer-prepend, KHÔNG thêm field RunRequest mới):**
1. `/dev on` → channel handler → `SetSessionMetadata(key, {"chat_mode":"dev"})` + `Save` (helper Phase 1).
2. Consumer build `extraSystemPrompt` tại `cmd/gateway_consumer_normal.go:294-309` (group prompt + topic prompt) → cùng block đọc metadata Phase 1 (ngay trước literal `:482`): nếu `chat_mode=="dev"` → prepend `agent.DevModePromptSection` + `"\n\n"` vào `extraSystemPrompt` TRƯỚC khi gán `RunRequest.ExtraSystemPrompt` (`:507`).
3. Chuỗi sẵn có lo phần còn lại: `RunRequest.ExtraSystemPrompt` → `pipeline.RunInput` (`loop_pipeline_adapter.go:290`) → context_stage append memory (`context_stage.go:77-80`) → `makeBuildMessages` append TeamWork (`loop_pipeline_callbacks.go:145-150`) → `buildMessages` (call site duy nhất `:151`) → `SystemPromptConfig.ExtraPrompt` (`loop_history.go:265`). **Không sửa gì trong chuỗi này.**
4. Resume/checkpoint: `loop_run.go:512` copy `ExtraSystemPrompt` nguyên vẹn → dev section tự sống qua resumed runs (đây là lý do chọn consumer-prepend thay vì field mới — field mới phải sửa thêm resume path).

**Không làm (ghi rõ để khỏi scope creep):** không pause/resume run thật; không đổi tool whitelist theo mode (tool policy đã có per-agent/per-request `RunRequest.ToolAllow` — nếu sau này muốn dev mode khóa tool thì là 1 dòng nữa, khỏi trước); không map vào PromptMode; không thêm field RunRequest.

## Related Code Files

- Create: `internal/channels/telegram/commands_dev.go` (handleDevCommand — dùng SessionPrefsStore + chatSessionKey từ Phase 1)
- Create: `internal/channels/telegram/commands_dev_test.go`
- Create: `internal/agent/dev_mode_prompt.go` (const `DevModePromptSection` + helper `ApplyDevMode(extra string) string`) — đặt cạnh systemprompt để dễ test
- Create: `internal/agent/dev_mode_prompt_test.go`
- Modify: `internal/channels/telegram/commands.go` (case `/dev`, `/help`)
- Modify: `internal/channels/telegram/commands_pairing.go` (menu entry `dev`)
- Modify: `cmd/gateway_consumer_normal.go` (đọc `chat_mode` + prepend DevModePromptSection vào extraSystemPrompt tại :294-309/:507)

## Implementation Steps

1. `dev_mode_prompt.go` + test (ApplyDevMode: mode off → extra nguyên bản; on → section + "\n\n" + extra; extra rỗng → chỉ section).
2. Consumer wiring: cùng block đọc metadata với thinking (tách hàm chung `chatPrefsFromMetadata(map) (thinking, mode string)`), prepend section vào extraSystemPrompt.
3. `commands_dev.go` + case + help + menu (test mirror Phase 1: on/off/show, perm group, persist Set+Save).
4. Build/vet/test toàn bộ (lệnh chuẩn Phase 1).

## Success Criteria

- [ ] Dev mode ON → system prompt chứa "DEV MODE ACTIVE" ở đầu ExtraPrompt (unit test assert prompt assembly; thủ công check span input_preview trên dev); OFF → không chứa, kể cả resumed run (resume copy ExtraSystemPrompt `loop_run.go:512`).
- [ ] Thử nghiệm hành vi: gửi yêu cầu mơ hồ ("sửa cái đó đi") trong dev mode → agent hỏi lại câu làm rõ thay vì tự đoán (manual eval — prompt không guarantee tuyệt đối, ghi nhận kết quả; nếu model yếu vẫn đoán → tăng cường section, không thêm cơ chế mới).
- [ ] Toggle persist qua restart; group writer-only.
- [ ] Build/vet/test sạch 2 mode.

## Risk Assessment

- **Ordering trong ExtraPrompt:** dev section prepend TRƯỚC group/topic prompt của consumer → section nằm đầu; context_stage append memory và makeBuildMessages append TeamWork SAU → nằm cuối. Thứ tự ổn (section định tính hành vi nên đầu là đúng chỗ). Test assert vị trí đầu.
- **Prompt không đủ ràng buộc model yếu (mimo free):** chấp nhận — dev mode là prompt-guided; mitigation: section viết mệnh lệnh ngắn, dứt khoát; không build cơ chế pause/resume trong plan này (nếu anh muốn sau này, đó là feature riêng dùng InjectCh `loop_types.go:729-732`).
- **System prompt phình:** section cố định ~15 dòng, không lặp theo iteration — không ảnh hưởng đáng kể context budget.
- **Preview/replay paths không qua consumer:** preview_prompt.go/replay.go build prompt không có consumer → không thấy dev section. Chấp nhận (dev mode là runtime UX của channel thật, không phải preview); ghi chú 1 dòng trong docs page.
