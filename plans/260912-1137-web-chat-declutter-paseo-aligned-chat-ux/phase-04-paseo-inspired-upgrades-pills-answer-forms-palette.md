---
phase: 4
title: "Paseo-inspired upgrades: pills, palette, (stretch) answer forms + @file"
status: pending
priority: P2
effort: "1d"
dependencies: [2]
---

# Phase 4: Paseo-inspired upgrades: pills, palette, (stretch) answer forms + @file

## Overview

Nâng cấp phần CÒN LẠI sau khi dọn (không thêm chrome mới): composer toolbar thành pill row gọn kiểu Paseo, command palette bỏ 4 lệnh trùng trang riêng. Hai mục stretch (answer-form cards, @file mention) chỉ kích hoạt khi điều kiện chốt.

## Requirements

- Functional (committed scope):
  - **Composer pills**: 3 select (Provider, Model, Thinking) trong `composer-toolbar.tsx` (`:71-156`) đổi thành pill row compact kiểu Paseo ("model pill + effort pill"): mỗi pill là badge-button mở dropdown; khi chưa override hiển thị giá trị mặc định mờ ("Agent default"); override localStorage (`goclaw.composer-override`) giữ nguyên logic — chỉ đổi presentation. Mobile: pills thu vào 1 pill "+" mở popover chứa đủ 3 chọn lựa.
  - **Palette prune**: bỏ 4 lệnh control `gc:status, gc:runs, gc:doctor, gc:approve` khỏi `GC_COMMANDS` (`command-palette.tsx:38-53`); giữ 10 lệnh prompt-style (plan/fix/cook/review/test/debug/docs/architect/uiux/mission) + skills entries (nguồn `skills.list` giữ nguyên).
- Functional (stretch — điều kiện kích hoạt, không phải scope mặc định):
  - **Answer-form cards**: khi agent trả lời chứa block "ask" có options (3 lựa chọn + Other), timeline render card nút bấm thay vì text. ĐIỀU KIỆN: plan Telegram `260912-1116-*` đã chốt format block ask-options → web parse cùng format. Nếu plan đó chưa làm: bỏ qua mục này, không tự chế format.
  - **@file mention**: gõ `@` trong composer → gợi ý file từ `workspace.files.list` (chỉ khi session có workspace đang chọn) → chèn path vào prompt. ĐIỀU KIỆN: anh xác nhận dùng workspace console đủ thường xuyên để xứng đáng; mặc định bỏ qua.
- Non-functional: không thêm dependency; pill ≥44px hit area (AGENTS.md touch target); `text-base md:text-sm` cho mọi control trong composer.

## Architecture

- `composer-toolbar.tsx` refactor presentation: giữ hooks (`useProviders`, `useProviderModels`) + localStorage override logic nguyên trạng; chỉ đổi UI shell — Pill component nhỏ nội bộ (button + Radix DropdownMenu, pattern đang có ở các page khác — tái dùng nếu dự án đã có component pill/dropdown chung, grep trước).
- Palette: xóa 4 entry trong mảng `GC_COMMANDS` + key i18n mô tả tương ứng không xóa (key shared có thể dùng chỗ khác — chỉ bỏ entry).
- Stretch answer-forms: parser trong `rich-content-parser.ts` + renderer `rich-content.tsx` thêm 1 block type "ask" — chỉ khi format đã chốt từ plan Telegram.

## Related Code Files

- Modify: `ui/web/src/components/chat/composer-toolbar.tsx`, `ui/web/src/components/chat/command-palette.tsx`, (stretch: `rich-content-parser.ts`, `rich-content.tsx`, `chat-input.tsx` cho @file)
- Create: không bắt buộc (pill có thể là subcomponent trong composer-toolbar)
- Delete: không

## Implementation Steps

1. Grep component pill/dropdown chung sẵn có trong `ui/web/src/components/ui/` — tái dùng nếu có.
2. Refactor `composer-toolbar.tsx` sang pill row (desktop) + "+" popover (mobile) — giữ nguyên behavior override (test: chọn provider/model xong reload còn giữ).
3. Prune palette 4 lệnh; i18n key mô tả pill mới (`chat.composer.*` × 5 locale).
4. `pnpm build` + kiểm tay: pill chọn provider/model/thinking, override persist, mobile popover, palette còn 10 lệnh + skills.
5. (Chỉ nếu điều kiện) answer-form / @file — làm riêng PR nhỏ sau khi xác nhận.

## Success Criteria

- [x] Composer 1 hàng pill gọn (không còn 3 select full-width chiếm dòng); hành vi override không đổi
- [x] Mobile: composer không tràn, pills thu gọn, hit area ≥44px
- [x] Palette không còn 4 lệnh control; gõ `/` vẫn đủ 10 lệnh prompt + skills
- [x] `pnpm build` pass; stretch items KHÔNG được làm nếu điều kiện chưa đạt

## Risk Assessment

- **Select → pill đổi control native sang custom**: mất accessibility nếu làm vội — dùng Radix DropdownMenu (đã là pattern dự án), giữ label + aria. Tín hiệu gãy: keyboard không mở được pill → fix aria/role trước khi merge.
- **Scope creep từ stretch items**: 2 mục stretch dễ bị "tiện tay làm luôn" — điều kiện kích hoạt là chốt chặn; cook không được tự mở rộng.
- **Độc lập Phase 3**: phase này chỉ phụ thuộc Phase 2 (top bar/console đã dọn), không cần backend Phase 3 — có thể chạy song song với Phase 3 nếu orchestrate nhiều agent (không đụng file nhau: P3 đụng backend + top bar mount point; P4 đụng composer + palette — file giao = `chat-top-bar.tsx` nếu context-meter mount cùng lúc pill... context-meter là P3 file, composer-toolbar là P4 → không giao).

## Kiểm tra tuyển chọn khi cook nhiều phase song song

Nếu chạy Phase 3 + Phase 4 song song: P3 sắp đụng `chat-top-bar.tsx` (mount context-meter) — P4 KHÔNG đụng file này (composer-toolbar + command-palette riêng). Quy ước ownership file theo orchestration-protocol: tránh parallel edits cùng file.
