---
title: "Journal 2026-09-15: Bỏ Tools Hub + audit agent loop /dev + test ổn định tool"
date: 2026-09-15
---

# 2026-09-15 — Bỏ "Trung tâm công cụ", audit vòng lặp agent /dev, test tool trên server

## Bỏ Tools Hub (deployed + verified)
- Nhánh `fix/remove-tools-hub` (từ `deploy/260915-integration`) trong worktree `C:/Users/DORA/Downloads/.goclaw-deploy-wt`. 13 files, -121 dòng: xoá `tools-hub-page.tsx`, `/tools` → `<Navigate to TOOLS_VIDEO>`, bỏ SidebarItem toolsHub + icon Wrench, bỏ key `nav.toolsHub` (5 locales) + block `hub` toolbox.json (5 locales). Commit chưa push.
- **Bẫy deploy:** build nhúng UI qua `//go:embed all:dist` trong `internal/webui/dist/` (KHÔNG phải `ui/web/dist`) — lần build đầu quên sync nên server vẫn serve bundle cũ. Fix: `cp -r ui/web/dist internal/webui/dist` rồi build lại.
- Deploy `v4.5.0-integration.20260916-nohub` → gateway active, serve `index-BnTUR9EA.js`, verify bằng browser: `/tools` redirect về `/tools/video`, sidebar hết mục "Trung tâm công cụ".

## Test ổn định tool trên server (chat API, agent fox-spirit)
- exec (`echo/uname/date`), write_file, read_file: tất cả pass.
- spawn mode=sync: subagent tính 17*23+191=582, kết quả về đúng parent loop — pass.
- skill_search "design": tìm thấy 2 skill (databases, architect — BM25 khớp lỏng nhưng cơ chế hoạt động).
- API shape: POST /v1/chat/completions, header `X-GoClaw-User-Id` bắt buộc, `model` phải là `agent:<agent_key>` (extractAgentID, auth.go:59-77).

## Audit agent loop /dev (Explore agent, citation đầy đủ trong transcript)
- Loop think→act→observe THẬT: 30 iterations mặc định (config.DefaultMaxIterations), supervisor caps 60 LLM calls/30 phút/6 tool-fail liên tiếp, loop detector.
- Spawn subagent THẬT: async/sync/wait, batch song song ≤4 tool calls, cap 20 con, depth 1, 5 con/agent; kết quả tự announce về session cha (subagent_exec.go:19-148).
- **Thiếu first-class:** không có plan/todo tool; completion verifier chỉ advisory (recover/hard phải bật); ContinuationGate mặc định TẮT; dev mode CHỈ Telegram (/dev on, session prefs `chat_mode=dev`), web UI chưa có toggle; dev mode thuần prompt (plan-first, ask_options, verify-before-concluding) không enforce.
- Skill: agent TỰ CHỌN được (inline catalog ≤60 skills, else skill_search BM25 + use_skill); không auto-inject theo relevance.

## Next (đề xuất)
- Bật completion verifier mode=recover + cân nhắc plan/todo tool + dev-mode toggle cho web chat.
- Push nhánh `fix/remove-tools-hub` khi anh duyệt.
