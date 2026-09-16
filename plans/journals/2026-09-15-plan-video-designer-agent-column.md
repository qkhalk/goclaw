---
title: "Plan: video designer agent column"
date: 2026-09-15
summary: "Ke hoach cot chat agent designer trong /tools/video: allowlist fail-closed, design skills, storyboard card apply timeline"
---

# Plan: video designer agent column

## What happened
 anh yeu cau them tinh nang: cot tuong tac voi agent designer trong trang tao video (/tools/video), agent chi design (khong dieu khien he thong), du design skills, UI theo ak:frontend-design. Su dung ak:plan + 2 Explore scout song song:
 - Scout 1 (chat UI): ChatThread/MessageBubble/ChatInput/useChatMessages/useChatSend deu reusable; WS global; session key client-side `agent:{key}:ws:direct:{convId}`; chat-side-pane la layout precedent (ResizeHandle + useUiStore width); zustand store keyed sessionKey -> 2 surface cong ton. Caveat: composer localStorage chung `goclaw.composer-override`, run.started co the thieu sessionKey.
 - Scout 2 (agent config): AgentData.ToolsConfig (tools_config JSONB) -> config.ToolPolicySpec{allow} enforcement 3 lop, lop cuoi fail-closed `makeAuthorizeToolCall` (loop_pipeline_callbacks.go:331-384). Team membership tu them write_file -> giu agent ngoai team. Skills: internal visibility + GrantToAgent -> SkillAllowList. render_video tool ton tai nhung KHONG dua vao allowlist (agent chi design, user tu export).

## Decision
 - Khong them WS method moi: reuse chat.send/chat.history/chat.abort + event "agent".
 - Allowlist designer: skill_search, use_skill, session_status. KHONG exec/write_file/render_video/delegate/cron.
 - Agent_key = "video-designer" (predefined), ensure idempotent o gateway start khi video enabled (cmd/gateway_video.go), khong migration DB.
 - Storyboard bridge: agent ket thuc reply bang fenced ```storyboard JSON; UI parse + validate, render StoryboardCard, Apply dung lai path loadJson() (timeline.replaceScenes + setMeta).
 - Column: desktop rail resize 320-560 (default 384) + collapse; mobile bottom-sheet full safe-area; suggestion chips empty-state.

## Next steps
 Plan 5 phase tai plans/260915-1243-video-designer-agent-column (ak plan validate OK, da set active):
 1. Backend ensure agent + skills builtin
 2. Chat column reuse chat blocks
 3. Storyboard bridge + card
 4. i18n 5 locale + mobile polish
 5. Build + deploy 192.168.1.103 + browser verify end-to-end (gom test negativa: agent tu choi yeu cau ngoai design) + release v4.6.0

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
