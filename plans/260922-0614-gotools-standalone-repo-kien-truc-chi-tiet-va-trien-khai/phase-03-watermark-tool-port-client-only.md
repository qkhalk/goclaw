---
title: "Phase 3: Watermark tool port (client-only)"
status: todo
priority: P1
effort: "0.5d"
dependencies: [2]
---

# Phase 3: Watermark tool port (client-only)

## Overview
Port nguyên trang Watermark — tool 100% chạy trong browser (thư viện `@pilio/gemini-watermark-remover` load model vào Cache Storage), server không dính dáng gì. Tool đầu tiên live của GoTools, xác nhận quy trình port hoạt động end-to-end.

## Requirements
- Functional: kéo-thả ảnh → chọn vùng watermark → xoá → tải ảnh kết quả; reveal mask xem trước vùng bị xoá.
- Non-functional: zero request tới `/v1/*` khi dùng tool (verify DevTools network); ảnh không rời máy người dùng (privacy selling point — ghi rõ UI).

## Architecture
- Copy nguyên vẹn vào `web/src/port/pages/tools/watermark/`: `watermark-tool-page.tsx` + `components/removal-reveal.tsx` (nguồn `ui/web/src/pages/tools/watermark/` — scout xác nhận chỉ dùng auth store cho local item id, không backend).
- Dep: `@pilio/gemini-watermark-remover` ^1.0.43 (mirror `ui/web/package.json:21`), lazy import dynamic route chunk (model ~MBs không block shell).
- Route `/tools/watermark` + nav active; bỏ StudioGate goclaw (không có builtin-tool system ở GoTools).
- i18n: copy namespace `toolbox` phần watermark (en/vi).

## Related Code Files
- Create: `web/src/port/pages/tools/watermark/**` (2 file)
- Reference: `ui/web/src/pages/tools/watermark/watermark-tool-page.tsx`, `ui/web/src/pages/tools/watermark/components/removal-reveal.tsx:72,254,265` (browser-only lib usage)

## Implementation Steps
1. Copy 2 file + PROVENANCE + sửa import `@/port/...` + bỏ phần import gate/store của goclaw không có ở GoTools.
2. Thêm dep + dynamic import route chunk; route + nav wire.
3. i18n keys watermark en/vi.
4. E2E tay: ảnh có watermark logo góc → select → remove → download PNG sạch; F12 network: 0 call API.
5. Test mobile: drop-zone touch + viewport nhỏ.

## Success Criteria
- [ ] Xoá watermark ảnh test thật sạch, tải về được
- [ ] 0 network call tới server trong lúc dùng tool
- [ ] Route lazy chunk — shell load không chậm thêm đáng kể (Lighthouse quick check)

## Risk Assessment
- Thư viện tải model từ CDN ngoài lần đầu (offline fail): UI hiện trạng thái tải model + lỗi rõ; cache Storage cho lần sau (behavior sẵn của lib — không sửa).
