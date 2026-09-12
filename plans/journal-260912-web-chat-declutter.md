# Web /chat declutter — Paseo-aligned chat UX — Planning Journal

Date: 2026-09-12 · Repo: goclaw-mod (qkhalk/goclaw) · Plan: `plans/260912-1137-web-chat-declutter-paseo-aligned-chat-ux`

## Outcome

Plan created + validated (0/4 phases, pending anh review). Không implement code.

## What was investigated

1. **Code audit** (Explore agent, very thorough): toàn bộ element trên `/chat` — 3 cột desktop (global nav + chat sidebar + right panel), team task render **3 nơi** (TaskPanel auto-open + TeamActivityPanel inline + chip top bar), **2 agent picker** + **3 fetch `/v1/agents`**, run-phase text **2 nơi**, workspace console chiếm 4/8 top-bar control với `workspaceId` không liên kết session, palette chỉ chèn text, ~7 cụm string hardcode tiếng Anh giữa UI 5-locale. Full inventory: `reports/audit-chat-page.md`.
2. **Live verify** (browser, 192.168.1.103:18790/chat @3.18.0): top bar thực tế có "Không có workspace" + 3 nút console (1 disable) + "Ready" tiếng Anh; agent prompt grid giữa màn. Khớp audit.
3. **Paseo research** (getpaseo/paseo — control plane for coding agents, ~17k⭐): triết lý "workspaces, not chats"; task tracker = pill trên composer (v0.5.0); context meter được khen; những gì để NGOÀI chat view (settings/schedules/history/terminal) — GoClaw đã có page riêng tương đương, chỉ cần bỏ bản sao trong chat. Full: `reports/research-paseo.md`.

## Red-team catch (quan trọng)

Audit đầu tiên kết luận **"context badge là UI chết"** do đọc nhầm struct `:130-133` của `sessions.go` (đó là params của `handlePatch`). Verify lại: handler `sessions.list` (`:72-76`) trả `SessionInfoRich` nguyên trạng — `estimatedTokens/contextWindow/compactionCount` ĐẦY ĐỦ cả PG lẫn SQLite (PG SQL `sessions_list.go:175-181`, SQLite có test). Pipeline ghi `last_prompt_tokens` (`loop_history_sanitize.go:433`), compaction ghi `last_compaction_at` (`loop_pipeline_callbacks.go:916`). Badge không hiện trên live vì chưa chọn session. → Phase 3 re-scope từ "làm backend trả data" thành "nâng cấp UX badge (popover + mobile + live-update)". Bài học: trace wire path trước khi kết luận field "không bao giờ đến".

## Key decisions (D1-D6, chờ anh duyệt)

D1 console thu 1 popover (không xóa) · D2 team task → 1 pill trên composer · D3 giữ sidebar selector, bỏ grid · D4 giữ + nâng cấp badge · D5 run-phase chỉ ở ActivityIndicator dưới thread · D6 bỏ 4 lệnh control khỏi palette.

## Notes

- Red-team agent spawn bị usage limit (reset 12:50) → verify pass tự chạy bằng grep/read trực tiếp. 18/18 claim đã check.
- Dự án **không có** Radix Popover wrapper — mọi dropdown chat là custom `createPortal` (plan đã sửa assumption).
- Phase 3 + Phase 4 chạy song song được (không đụng file: P3 = top bar/context-meter + backend verify; P4 = composer-toolbar + palette).
- Stretch items Phase 4 (answer-forms, @file) có điều kiện kích hoạt — không làm nếu plan Telegram `260912-1116-*` chưa chốt format ask-options.

## Ship note (cập nhật sau cook + deploy)

- PR #65 merged vào dev (c79d6982), CI xanh (go/web/release-versioning + dev CI run 34685049414).
- Deploy 192.168.1.103: binary `0.1.0-chatdeclutter.1` (embedui), server phục vụ `index-D6q52es3.js` khớp build local, service active.
- Live verify: top bar chỉ còn "Bảng điều khiển"; menu mở đúng (workspace list + Tệp/Jobs & Tasks/Terminal disabled khi chưa chọn); empty-state CTA "Chọn agent" thay grid; "Không có workspace"/"Ready"/3 nút console đã biến mất khỏi chrome thường trực.
- Phase 4 composer pills: bỏ — dev đã có sẵn pill toolbar; chỉ palette prune.
- Sau review-fix: xóa workspace-picker.tsx mồ côi; 4 commits trên PR.
