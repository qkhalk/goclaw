---
title: "Journal 2026-09-15: Verifier recover chạy được trên fresh-run + tool plan first-class"
date: 2026-09-15
---

# 2026-09-15 — Verifier recover (fix fresh-Run) + tool `plan` + plan card

Cook từ journal cùng ngày (mục "Next"): bỏ dev-mode web theo yêu cầu anh (dev mode giữ Telegram-only), làm A (verifier) + B (plan tool). Scope né toàn bộ file đang bị agent song song sửa.

## A. Completion verifier recover — sửa đường fresh-Run
- **Root cause:** `Run` gọi `runViaPipeline(ctx, req, nil, …)` rồi truyền `state = nil` vào `gateCompletion` → nhánh recover luôn rơi vào `recover_unavailable` → **recover biến dạng hard** (incomplete lần 1 = FAIL, không retry). ResumeRun thì có state (RestoreCheckpoint) nên đúng.
- **Fix (commit 629fe3fd9, nhánh `feat/verifier-recover-plan-tool`):**
  - `runViaPipeline` trả thêm `*pipeline.RunState`; Run giữ `passState` qua các pass retry.
  - `RunState.MarkContinuation()` mới (run_state.go): `resuming = true` + `ExitCode = Continue` — hình trạng giống hệt RestoreCheckpoint (checkpoint không persist ExitCode, zero = Continue) → pass 2 skip setup, vào lại iteration N, consume `ContinueAfterFinal` ở BreakLoop đầu.
  - Guard mới ở nhánh recover: không continue khi `ExitCode == AbortRun` hoặc `Tool.LoopKilled`.
  - ResumeRun nhận lại state từ return → sửa luôn fallback fresh-start (checkpoint hỏng).
  - Advisory byte-identical như cũ; hard không đổi.
- Test: `run_state_continuation_test.go` pin ngữ nghĩa (skip setup, resume iteration, consume flag); suite verifier green.

## B. Tool `plan`
- `internal/tools/plan.go`: action `set` (thay toàn bộ, ≤20 bước × 100 runes) / `update` (step 1-based + status, chặn step lẻ) / `get`. Persist vào session metadata key `plan` (SetSessionMetadata merge + Save) — không đụng key khác. Không set Deliverable (verifier không tính planning là work output). Rune-safe truncation.
- Wire: `gateway_tools_wiring.go` (register + vòng SessionStoreAware), seed `gateway_builtin_tools.go` (category sessions). Profile mặc định = full nên không phải đụng policy.go (né `designer_policy_test.go` của agent song song). Designer agent vẫn không thấy tool (allowlist 3 tool fail-closed).
- UI: `plan-card.tsx` — parse JSON line đầu của `result` (fallback arguments cho `set` đang stream), phase=error không render. i18n `planCard.*` trong `common.json` ×5 (né `chat.json` đang bị sửa).
- **Bẫy:** `truncateStr` trong `loop_tools.go:80` giữ PHẦN ĐUÔI — result >1000 ký tự làm vỡ JSON đầu dòng. Fix: `toolResultPreviewCap("plan") = 4096`. Kèm theo đó ForLLM của plan là JSON compact duy nhất (bỏ render), step 100 runes.
- `DevModePromptSection` giờ chỉ định dùng plan tool khi có sẵn.

## Quy trình đặc biệt (agent song song)
- Agent khác đang sửa: web_fetch.go, browser-panel/chat-page/chat-sidebar/use-browser-panel, chat.json ×5, internal/agent (json_repair, team_work_directive + 6 test), internal/pipeline (final_request_guard, 3 test), cmd/gateway.go, internal/video/*.
- Commit chọn lọc: riêng `run_state.go` chỉ stage đúng hunk MarkContinuation (git apply --cached), bỏ lại gofmt realignment của họ (vẫn unstaged trong worktree). `plan_test.go` dính 1 dòng maps.Copy của họ — chấp nhận, note trong commit.
- **Pre-existing failures trên Windows (xác minh bằng worktree HEAD sạch, KHÔNG phải do ai):** `TestMediaEgressRoots`, `TestEnrichImageIDs_*` (path /tmp semantics) và hang `TestDelegateAsyncDurableGetIsSourceAgentScoped` (tools suite timeout).

## Next (chờ anh)
- A3: bật trên server = thêm vào config JSON5: `reliability.completion_verifier.mode: "recover"` (+ optional `reliability.premature_completion.enabled: true`) rồi restart goclaw — **chờ khi không còn agent nào đang chạy**.
- Đẩy nhánh `feat/verifier-recover-plan-tool` + verify live trên server sau deploy.
- Desktop không có plan card (render generic) — chấp nhận.
