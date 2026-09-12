---
phase: 2
title: "/skills phân trang + reply-to-run"
status: completed
priority: P1
effort: "4h"
dependencies: ["phase-1"]
---

# Phase 2: /skills phân trang + reply-to-run

## Overview

`/skills` chuyển sang danh sách nút bấm 10 skill/page (◀ ▶), bấm skill xem mô tả ngay trong message, và reply tin mô tả đó để chạy skill với yêu cầu của mình.

## Requirements

- Functional:
  - `/skills` → message "Skills (N) — page 1/k" + keyboard 10 nút skill (tên rút gọn ≤20 chars) + hàng điều hướng `◀ Prev | Page x/y | Next ▶` (ẩn nút vô nghĩa ở page đầu/cuối).
  - Bấm nút skill → edit message: tên + mô tả đầy đủ (không truncate 120) + deps nếu thiếu + hướng dẫn: "Reply tin nhắn này kèm yêu cầu để chạy <slug>".
  - **Reply-to-run**: user reply tin skill đó với nội dung X → inbound content = `/<slug> X` (dùng slash-command sẵn có → kích hoạt skill qua matching đã có từ 3.18.0, kể cả slug gạch ngang). Chỉ áp dụng khi reply tới message "skill detail" còn hạn (24h — lấy từ message date).
  - Giữ whitelist group/topic như list cũ (`resolveTopicConfig(...).skills`).
  - Text list cũ vẫn dùng được qua `/skills list` (tham số) cho ai thích copy.
- Non-functional: 1 message duy nhất được edit đi edit lại (không spam); callback ≤64 bytes: `sk:p:<n>` (page), `sk:s:<idx>` (skill index trong DANH SÁCH ĐẦY ĐỦ — map từ pendingSkillsCtx, vì slug dài không nhét nổi cùng page).

## Architecture

Command `/skills` (không tham số) gọi `handleSkillsPicker` (file `commands_skills_picker.go`): load `ListSkills` (filter whitelist) → render page 1 → gửi message + keyboard → lưu `pendingSkillsCtx[messageID] = {infos []skills.Info (slug only + desc), sessionScope, expire}`. Callback `sk:p:<n>` → edit message sang page n. Callback `sk:s:<idx>` → edit message thành detail view + keyboard back (`sk:p:<curPage>`).

**Reply-to-run hook** nằm ở `handlers.go` TRƯỚC `handleBotCommand` (vì commands không thấy reply context — call site `handlers.go:252` chạy trước enrich `:257`): đầu `handleMessage`, sau khi có `message.ReplyToMessage`, check `pendingSkillsCtx[reply.MessageID]` còn hạn → transform `content = "/<slug> " + content` rồi cho đi tiếp đường thường (mention gate vẫn áp ở group — đúng, vì chạy skill cần quyền chat). Lưu ý chỉ transform khi `reply.From.Username == bot` (đây là tin bot gửi) — `IsBotReply` pattern `context.go:133`.

## Related Code Files

- Create: `internal/channels/telegram/commands_skills_picker.go` (picker + callbacks + detail view)
- Create: `internal/channels/telegram/commands_skills_picker_test.go`
- Modify: `internal/channels/telegram/commands.go` (`/skills` case: no-arg → picker; `list` → text list cũ)
- Modify: `internal/channels/telegram/commands_tasks.go` (dispatch `"sk:"`)
- Modify: `internal/channels/telegram/handlers.go` (reply-to-run hook đầu handleMessage)
- Modify: `internal/channels/telegram/channel.go` (struct `pendingSkillsCtx sync.Map`)

## Implementation Steps

1. Picker render + pagination unit tests (10/page,Prev/Next ẩn hiện, ✅ whitelist filter).
2. Callback handlers page/detail + edit flow + pendingSkillsCtx TTL sweep (chung sweeper của Phase 1).
3. Reply-to-run hook + transform test (unit trên helper thuần: `transformReplyToSkillRun(content, reply, ctx) string`).
4. Integration smoke thủ công: reply tin skill → trace thấy skillFilter đúng slug.
5. Build/vet/test chuẩn.

## Success Criteria

- [ ] 25 skills → 3 pages, điều hướng edit đúng message, không tin mới.
- [ ] Bấm skill → detail đầy đủ; reply detail với "làm gọn code X" → run của agent có skillFilter=[slug] (trace/log verify).
- [ ] Reply một tin thường (không phải skill detail) KHÔNG transform (regression: nội dung nguyên vẹn).
- [ ] Whitelist group vẫn chặn skill ngoài list ở picker lẫn reply-run.
- [ ] Build/vet/test sạch 2 mode.

## Risk Assessment

- **Slug dài + 64 bytes callback**: giải bằng idx + ctx map (đã thiết kế); tin bot bị xóa → pendingSkillsCtx entry mồ côi — TTL sweep dọn.
- **Reply-to-run bypass mention gate group?** KHÔNG — transform chỉ đổi content, mention gate chạy sau như thường (đã đặt hook trước command nhưng gate mention ở `handlers.go:275+` sau đó).
- **Trùng tin nhắn bot khác** (bot gửi nhiều message có ID khác nhau) — map theo messageID chính xác tin detail, không nhầm.
