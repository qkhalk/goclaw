---
title: "Phase 7: Chat quality: reasoning mặc định + subagent params"
status: todo
priority: P1
effort: "1d"
dependencies: []
---

# Phase 7: Chat quality: reasoning mặc định + subagent params

## Overview
Sửa 3 thủ phạm "ngáo" đã định vị: (1) subagent hardcode `max_tokens 4096 / temperature 0.5`; (2) reasoning mặc định off + downgrade im lặng; (3) người dùng không thấy model/thinking nào đang chạy. Không đổi kiến trúc loop.

## Requirements
- Functional: subagent kế thừa effective config parent (max_tokens/temperature/reasoning) trừ khi definition ghi đè; config mới `agents.reasoning_default: auto|off|low|medium|high` (mặc định `auto` cho agent tạo mới); chat UI hiển thị model + thinking level effective mỗi run.
- Non-functional: agent cũ giữ nguyên config đã lưu (không đổi behaviour khách hàng hiện tại — chỉ default tạo mới); mọi downgrade reasoning phải thấy được (log warn có sẵn `resolver.go:186-200` + thêm event note vào run trace).

## Architecture
1. **Subagent params** (`internal/tools/subagent_exec.go:270-291`):
   - Bỏ literal `max_tokens: 4096, temperature: 0.5` (:288-289 — verify spot-check) → build request params từ effective config của parent agent (LoopConfig/AgentRecord: MaxTokens default 8192 `defaults.go:11`, Temperature, ReasoningConfig resolved).
   - Mở rộng `agents.subagents_config` definition fields (`internal/tools/subagent_config.go:53-107`): thêm `maxTokens`, `temperature`, `thinkingLevel` (optional; override parent khi set). JSONB nên không migration.
   - Adaptive cap delegation "low" (`adaptive_thinking.go:15-18`): giữ cho async fan-out, nới lên agent-level cho sync mode (sync = kết quả trả thẳng caller, cần chất lượng).
2. **Reasoning default** (`internal/store/agent_store.go:192-199,415-447`):
   - `ParseReasoningConfig` nil/empty → sentinel "inherit" thay vì "off"; `ResolveEffectiveReasoningConfig`: inherit → hỏi provider capability map (anthropic native: medium; openai-compat có model reasoning: low; không rõ: off) — capability map đặt trong providerresolve cạnh model registry.
   - Config `agents.reasoning_default` (config_load.go, default `auto`) áp cho agent tạo mới qua `internal/gateway/methods/agents_create.go:121-128` (nơi kế thừa DefaultProvider/DefaultModel).
   - Downgrade path: giữ hành vi fallback an toàn nhưng ghi rõ vào run trace (dòng "reasoning X → Y (provider không hỗ trợ)") — UI phase hiển thị luôn.
3. **Transparency UI** (audit: run-phase label KHÔNG nằm ở top bar — chat-top-bar.tsx chỉ 96 dòng, có spinner):
   - Sự kiện `llm.started/llm.completed` có sẵn (`pkg/protocol/events.go:204-205`) và payload đã có provider/model (`loop_pipeline_callbacks.go:627-631,640-645` — verify audit).
   - Chỉ indicator: `ui/web/src/components/chat/activity-indicator.tsx` (`getPhaseConfig` :42 — "single run-phase indicator", cuối thread cạnh composer) thêm dòng meta "model · thinking" của run hiện tại + note reasoning-downgrade; message meta line cuối run trong thread.
   - (Optional) mở rộng chat-top-bar thêm badge model — tách PR nhỏ nếu muốn.

## Related Code Files
- Modify: `internal/tools/subagent_exec.go:270-291`, `internal/tools/subagent_config.go` (definition fields), `internal/store/agent_store.go:192-199,415-447`, `internal/config/config_load.go` (reasoning_default) + `internal/config/defaults.go:8` (DefaultMaxTokens), `internal/gateway/methods/agents_create.go:121-128` (default áp), `internal/providerresolve/` (capability map), `internal/agent/adaptive_thinking.go:15-17,93-98` (sync nới cap), `pkg/protocol/events.go` (field nếu thiếu)
- Modify UI: `ui/web/src/components/chat/activity-indicator.tsx` (model·thinking line), `ui/web/src/pages/chat/hooks/use-chat-messages.ts` (capture llm.started meta), i18n ×5
- Tests: `internal/tools/subagent_exec_test.go` (params kế thừa), `internal/store/agent_store_reasoning_test.go`, providerresolve capability test

## Implementation Steps
1. Subagent params: đọc parent effective config → truyền vào request build; unit test assert request body (mock provider capture).
2. Definition override fields + test ưu tiên order: definition > parent > old-default.
3. Reasoning inherit + capability map + config reasoning_default; test bảng quyết định (agent mới/ cũ/ không set/ provider lạ).
4. Downgrade trace note + event field; UI hiển thị model·thinking + note.
5. Docs note trong phase 12 docs site ("tại sao trả lời ngáo — checklist").
6. Full check: `go fix/build/vet` 2 tags + `pnpm build` + test -race (skip bộ Windows pre-existing đã biết).

## Success Criteria
- [ ] Trace request subagent mang max_tokens/temperature của agent (không còn 4096/0.5 cố định)
- [ ] Agent mới trên provider anthropic có thinking medium mặc định (test); agent cũ sau upgrade giữ off như cũ
- [ ] UI mỗi run hiện đúng model + thinking thực tế; downgrade hiện chú thích
- [ ] Không tăng token chi phí cho agent cũ (test default không đổi behaviour hiện có)

## Risk Assessment
- `auto` reasoning tăng chi phí agent mới: open question #4 đã chốt phương án (agent mới only); ghi chú trong docs + dễ tắt.
- Capability map sai với model mới: map theo provider + model prefix, unknown → off (an toàn); registry forward-compat của providerresolve là chỗ đúng để nuôi map này.
