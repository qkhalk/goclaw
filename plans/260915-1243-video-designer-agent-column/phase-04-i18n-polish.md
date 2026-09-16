---
phase: 4
title: "i18n 5 locale + design polish + mobile bottom-sheet"
status: done
priority: P2
effort: "1d"
dependencies: ["3"]
---

# Phase 4: i18n + design polish + mobile

## Overview
Đủ chuỗi i18n 5 locale cho cột designer + StoryboardCard; pass frontend-design lần cuối (craft rules đếm được); mobile bottom-sheet hoàn chỉnh theo Mobile UI/UX Rules của repo.

## Requirements
- Functional:
  - Namespace `toolbox.json` group `video.designer.*` cho mọi chuỗi mới; đủ en/vi/zh/ko/ru trước khi viết handler/UI dùng key (key missing = runtime crash)
  - Suggestion chips empty-state: 3 gợi ý mẫu ("Tạo intro gradient 9 giây", "Video quảng cáo 15 giây 4 cảnh", "Sửa storyboard hiện tại ngắn gọn hơn") — click gửi ngay
- Non-functional (frontend-design self-review gate — kiểm đếm được):
  - 0 em-dash trong copy mới; không dùng từ cấm (Elevate/Seamless/…)
  - 1 depth strategy (border hairline như hệ hiện có), 1 radius system (rounded-lg/md hiện có), không thêm shadow mới ngoài shadow-sm chuẩn shadcn
  - Accent primary chỉ dùng: nút Apply, status dot running, chip tỉ lệ — không tô cả cột
  - `tabular-nums` cho mọi con số (duration, scene count, timestamp)
  - Touch targets ≥44px (nút close/new-chat/attach đều min-h-11 min-w-11 mobile); composer input `text-base md:text-sm`; empty-state suggestion chips ≥44px cao
  - Bottom-sheet mobile: `max-sm:inset-0`, slide-up animation, `overscroll-behavior: contain` cho message list, composer trên `env(safe-area-inset-bottom)`, cột không gây horizontal scroll 375px
  - Focus ring default shadcn giữ nguyên (đủ ≥3:1); không `transition: all`

## Architecture
- Key list (đặt tên trước, viết catalog trước UI — theo Plan Verification Rule 15):
  - `video.designer.title`, `.description`, `.new_chat`, `.close`, `.attach_current`, `.attached`, `.empty_hint`, `.suggest_1/2/3`
  - `video.designer.card.apply`, `.card.applied`, `.card.invalid`, `.card.scenes`, `.card.duration`, `.card.view_json`, `.card.apply_error`
  - `video.designer.status.idle/.running`, `.error.chat`
- Viết 5 catalog JSON một commit riêng với script python như quy trình QA trước (giữ thứ tự key alphabet trong group)

## Related Code Files
- Modify: `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/toolbox.json`
- Modify: `ui/web/src/pages/tools/video/components/designer-column.tsx`, `storyboard-card.tsx` (dùng key + polish class)
- Read-only reference: Mobile UI/UX Rules trong AGENTS.md, `ui/web/src/components/ui/sheet.tsx`

## Implementation Steps
1. Thêm key vào 5 catalog (script python, verify JSON parse + 0 key thiếu giữa các locale)
2. Gắn key vào components; xoá mọi hardcode string
3. Pass polish: rà craft rules checklist ở trên (grep em-dash trong file mới = 0)
4. Mobile verify 375px DevTools: không h-scroll, sheet đủ cao 80dvh, composer không bị bàn phím che (giả lập keyboard overlap)
5. Desktop verify ≥1280px: rail + editor cạnh nhau thoáng, editor vẫn đủ chỗ canvas (min 720px)

## Success Criteria
- [ ] 5 locale đủ key, không có key thô hiển thị khi chuyển ngôn ngữ
- [ ] Checklist frontend-design pass (0 em-dash, tabular-nums, touch target, focus)
- [ ] 375px không horizontal scroll; sheet safe-area; chips ≥44px
- [ ] Suggestion chips gửi được message

## Risk Assessment
- Thiếu key 1 locale → runtime hiển thị key thô: script verify so sánh key-set 5 file trước khi commit.
- Bottom-sheet + bàn phím ảo iOS: dùng pattern `useVirtualKeyboard` nếu đã có trong chat input hiện có — chỉ áp var `--keyboard-height` (AGENTS.md Mobile rules); nếu thiếu thì safe-area + `interactive-widget=resizes-content` đã có từ app.
