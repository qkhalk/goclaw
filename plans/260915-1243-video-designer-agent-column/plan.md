---
title: "Video designer agent column"
description: "Cột chat agent designer trong trang /tools/video: người dùng trò chuyện với agent chỉ chuyên thiết kế video (storyboard, cảnh, caption, chuyển cảnh, màu), agent trả storyboard JSON được render thành thẻ xem trước và áp thẳng vào timeline. Agent bị khóa tool theo allowlist fail-closed, không điều khiển hệ thống."
status: pending
priority: P1
effort: "~4-6 ngày (5 phase)"
tags: [video, agent, chat, tools-gating, skills, web-ui]
created: 2026-09-15
---

# Video designer agent column

## Overview

Thêm vào trang Video Editor (`/tools/video`) một **cột hội thoại với agent designer** (kiểu chat panel của /chat nhưng nhúng trong trang công cụ). Người dùng mô tả video muốn làm ("intro 9s gradient tím, caption lớn, chuyển cảnh mượt") — agent designer trả lời kèm **storyboard JSON hợp lệ contract v1**, UI render thành **Storyboard Card** (thẻ xem trước) với nút **"Áp dụng vào timeline"**: timeline, canvas preview, mọi control hiện có nhận storyboard ngay mà không cần copy JSON thủ công.

Ràng buộc cứng từ anh:
1. Agent **chỉ là người design** — KHÔNG được điều khiển hệ thống, không exec, không ghi file, không quản trị server.
2. Agent phải có **đủ skill cho design** (thiết kế storyboard, màu, typography, motion).
3. Giao diện theo `ak:frontend-design` — product UI, preserve-mode hệ shadcn hiện có.

## Kiến trúc đã chọn (từ 2 scout report, file:line verified)

- **Không có WS method mới.** Toàn bộ hội thoại chạy trên `chat.send` / `chat.history` / `chat.abort` / `chat.session.status` hiện có (`pkg/protocol/methods.go:14-18`), stream qua event `"agent"` (`pkg/protocol/events.go:5`). Session key sinh client-side: `agent:video-designer:ws:direct:{convId}` — server materialize session ở `chat.send` đầu tiên (`internal/gateway/methods/chat.go:263-283`), user binding + tenant scope tự động từ connect frame.
- **Agent designer là predefined agent** `agent_key = "video-designer"`, được ensure lúc gateway start (khi video enabled). Tool restriction qua `AgentData.ToolsConfig` (JSONB `tools_config`, `internal/store/agent_store.go:65`) với `{"allow":[...]}` — enforcement 3 lớp: prompt filter (`internal/tools/policy.go:240-242`), loop wiring (`internal/agent/loop_tool_filter.go:69`), và **execution gate fail-closed** (`internal/agent/loop_pipeline_callbacks.go:331-384` trả "tool not allowed by policy"). Allowlist: `skill_search`, `use_skill`, `session_status` — KHÔNG có `exec`, `write_file`, `render_video`, `delegate`, `cron`. Agent **không tham gia team** (team membership tự thêm `write_file` qua `agentToolPolicyWithWorkspace`, `internal/agent/resolver_helpers.go:179-192`).
- **Skill design**: 2 SKILL.md builtin (`video-storyboard-design`, `video-color-motion`), visibility `internal` + grant riêng cho agent → `SkillAllowList` lọc đúng skill design (`internal/agent/resolver.go:440-452`). Agent dùng `use_skill` để nạp.
- **UI column** reuse toàn bộ building blocks chat: `ChatThread`, `MessageBubble`, `ChatInput`, `ActiveRunZone` + hooks `useChatMessages(sessionKey, agentId)` / `useChatSend` (chỉ chat-page dùng hôm nay, hooks nhận sessionKey là argument thuần). Zustand chat store keyed theo sessionKey → 2 surface cộng tồn (`ui/web/src/stores/use-chat-messages-store.ts:51`). Layout clone pattern `chat-side-pane.tsx` + `ResizeHandle` + width persist trong `useUiStore`.
- **Storyboard bridge**: agent kết thúc reply bằng fenced block ` ```storyboard {json} ` ` — UI parse từ message hoàn tất + stream text, validate bằng chính rules client đang có, render StoryboardCard; nút Apply gọi `timeline.replaceScenes(scenes)` + `setMeta` (đúng path `loadJson()` đang dùng trong `video-tool-page.tsx`).

## Mục tiêu Cross-Surface Parity

- **Gateway server**: ensure-agent startup hook, tools_config allowlist, skills builtin + grants. KHÔNG migration DB (cột `tools_config`, bảng `skill_agent_grants` đã có).
- **API contract**: không thêm method/event mới — reuse `chat.*` + event `agent`. Surface parity: `pkg/protocol` N/A because không đổi wire contract.
- **Web UI**: cột designer trong `/tools/video`, StoryboardCard, mobile bottom-sheet, i18n 5 locale.
- **CLI/runtime package**: N/A because tính năng hoàn toàn nằm trong gateway + web UI, không surface CLI nào tương tác.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Cột chat designer nhúng trong /tools/video: chat 2 chiều, stream, abort, history — reuse component chat | P1 |
| 2 | Agent `video-designer` bị khóa tool allowlist fail-closed (design-only, 0 tool hệ thống) | P1 |
| 3 | Agent có skill design (storyboard, màu, motion) scoped riêng qua grants | P1 |
| 4 | StoryboardCard: agent trả JSON → thẻ xem trước → Apply vào timeline 1 click | P1 |
| 5 | Mobile: designer là bottom-sheet full-screen có safe-area; resize + collapse trên desktop | P2 |
| 6 | i18n đầy đủ en/vi/zh/ko/ru, tuân thủ mobile UI/UX rules + frontend-design craft rules | P2 |

## Phases

| # | Phase | Status | Priority |
|---|-------|--------|----------|
| 1 | [Phase 1: Designer agent backend — ensure agent + tool allowlist + design skills](./phase-01-designer-agent.md) | Pending | P1 |
| 2 | [Phase 2: Chat column trong Video Editor — layout, session, reuse chat blocks](./phase-02-chat-column.md) | Pending | P1 |
| 3 | [Phase 3: Storyboard bridge — parse ```storyboard block, StoryboardCard, Apply vào timeline](./phase-03-storyboard-bridge.md) | Pending | P1 |
| 4 | [Phase 4: i18n 5 locale + design polish + mobile bottom-sheet](./phase-04-i18n-polish.md) | Pending | P2 |
| 5 | [Phase 5: Test, build, deploy server, verify live end-to-end](./phase-05-verify-release.md) | Pending | P1 |

## Success Criteria

- [ ] Nhập "làm intro 9 giây gradient tím 3 cảnh có caption" vào cột designer → agent stream trả lời, cuối reply có block ` ```storyboard ` hợp lệ
- [ ] StoryboardCard hiện đúng: tỉ lệ canvas, số cảnh, tổng thời lượng (tabular-nums), nút Apply
- [ ] Click Apply → timeline 3 cảnh + canvas preview cập nhật tức thì; tổng duration đúng
- [ ] Agent KHÔNG có tool nào ngoài allowlist: kiểm tra `agents.list` hoặc debug — LLM không nhận schema tool khác; thử yêu cầu "chạy lệnh shell" → agent từ chối/không có tool
- [ ] Cột designer resize được (width persist qua reload), collapse/expand, không phá layout chat page chính
- [ ] Mobile 375px: không horizontal scroll, composer ≥16px font, touch target ≥44px, safe-area bottom
- [ ] 2 surface chat (chat page + designer column) chạy song song không cản trở nhau (session key khác nhau)
- [ ] i18n: 0 key thô hiển thị (đủ 5 locale), 0 em-dash trong copy mới
- [ ] Build: `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...`, `tsc` + `pnpm build` sạch

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Composer localStorage chung `"goclaw.composer-override"` (chat-input.tsx:26) — 2 composer đánh nhau | Phase 2: thêm prop `storageKey` cho ChatInput (default giữ nguyên key cũ); designer column truyền key riêng |
| Event `run.started` không mang sessionKey sẽ lọt filter mọi instance useChatMessages (use-chat-messages.ts:236,252) | Designer agent key riêng `video-designer` + verify payload luôn có sessionKey trước khi ship; nếu thiếu → thêm guard `event.agentId` ở hook mới, không sửa hook chung |
| LLM trả storyboard sai contract (thiếu source, duration lệch) | UI validate bằng cùng rules client (hasValidationErrors + JSON parse), card hiện lỗi cụ thể; prompt IDENTITY.md nhấn contract v1 + mặc định `type: "color"` khi không có ảnh |
| Agent tự ý gọi render/job | Allowlist không chứa `render_video` — execution gate fail-closed chặn; test negativa trong Phase 5 |
| useChatMessages kéo thêm teams.tasks + team.leader.processing (noise) | Chấp nhận (scout: harmless); designer agent không thuộc team nên không có data |
| Bootstrap ensure-agent chạy trên tenant nào | Chạy 1 lần ở master scope lúc start (giống seeder hiện có); agent là predefined shared, không phải per-tenant |

## Non-Goals

- KHÔNG cho agent gọi render video / tạo job — người dùng tự bấm export (đúng yêu cầu "chỉ design").
- KHÔNG làm agent marketplace hay nhiều agent chọn lựa trong column (1 agent cố định).
- KHÔNG đổi wire protocol hay thêm WS method.
- KHÔNG đụng desktop UI (`ui/desktop/`) — lite edition không có video tool.

## Conventions

- Agent identity theo `docs/agent-identity-conventions.md`: DB/UUID nội bộ, `agent_key = "video-designer"` cho mọi path/prompt/log.
- Mobile UI/UX rules + frontend-design craft rules áp dụng cho mọi component mới (liệt kê cụ thể trong Phase 4).
- Commit chi tiết theo phase, giống quy trình QA trước đó.

<!-- slug: video-designer-agent-column -->
