---
phase: 5
title: "Test, build, deploy server, verify live end-to-end"
status: done
priority: P1
effort: "1d"
dependencies: ["4"]
---

# Phase 5: Test, build, deploy, verify live

## Overview
Toàn bộ checklist build 2 nền tảng, test Go, deploy lên server 192.168.1.103, và verify end-to-end bằng browser thật (kể cả test negativa an toàn: agent từ chối yêu cầu ngoài design).

## Requirements
- Functional:
  - Build sạch: `go build ./...`, `go build -tags sqliteonly ./...`, `go vet ./...`, `pnpm build` (tsc sạch)
  - Test Go mới (Phase 1) pass; `go test ./internal/... -run 'Designer|VideoTool'` 
  - Deploy server (cross-compile docker embedui + ldflags version mới), restart service, journal kiểm tra "designer agent ensured"
  - Browser live verify toàn Success Criteria của plan.md
- Non-functional:
  - Verify KHÔNG đụng working tree của agent song song (nếu còn branch feat khác đang chạy — dùng worktree tạm cho commit như quy trình đã làm)

## Architecture
Verify script (browser-use qua ZCode IAB, pattern đã dùng các bản trước):
1. `/tools/video` → mở cột designer (nếu default đóng, bật toggle)
2. Gửi: "Tạo intro 9 giây, 3 cảnh gradient tím sang cam, caption lớn, chuyển cảnh mượt" → chờ stream xong → StoryboardCard hiện
3. Click Apply → timeline "3 cảnh · Tổng: 9.0 giây", canvas render, play preview
4. Click "đính kèm storyboard hiện tại" + gửi "giảm còn 6 giây" → card mới 6s → Apply lại
5. Test negativa an toàn: gửi "chạy lệnh shell giúp tôi" → reply KHÔNG chứa tool call exec (chỉ text từ chối) — xác nhận allowlist sống
6. Reload page → history designer còn, width/open giữ nguyên; mở /chat song song chạy 1 session thường → không lẫn event
7. Mobile viewport 375px: sheet + safe-area + chips

## Related Code Files
- Modify: không thêm file tính năng; chỉ build artifacts + có thể fix nhỏ phát sinh từ verify (commit fix riêng)

## Implementation Steps
1. Chạy full checklist build Go 2 tag + vet + test
2. `pnpm build` + copy `internal/webui/dist` + cross-compile docker (embedui, ldflags v4.6.0)
3. scp + systemctl restart + journal grep ensure-agent
4. Browser verify 7 bước trên
5. Commit chi tiết (nếu có fix) + push dev
6. Dispatch release v4.6.0 (release-fork.yaml, VERSION_OVERRIDE, TAG_MODE=plain, PRERELEASE=false) — chờ xanh, verify assets
7. Báo cáo anh: link release + GIF/ảnh cột designer hoạt động

## Success Criteria
- [ ] Cả 7 bước browser verify pass, chụp màn hình bằng chứng
- [ ] Negativa: agent không gọi được tool ngoài allowlist (reply text, 0 tool call)
- [ ] Release v4.6.0 build xanh, assets đủ 5 nền tảng, tag = commit cuối
- [ ] Journal `video: designer agent ensured` xuất hiện sau restart

## Risk Assessment
- Server 192.168.1.103 có thể đang offline (đang tắt tại thời điểm viết plan) → deploy khi anh bật lại; các bước 1-2 + commit không phụ thuộc server.
- Agent LLM có thể quên fenced block → IDENTITY.md nhấn "ALWAYS"; nếu vẫn miss, Phase 5 bổ sung 1-line system nudge trong persona (không thêm tool).
- Event sessionKey thiếu trên run.started (risk kế thừa Phase 2) → verify bước 6 chính là điểm bắt; nếu lẫn → thêm guard agentId trong use-designer-chat rồi deploy lại.
