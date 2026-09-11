---
phase: 5
title: "Video input reliability"
status: completed
priority: P1
effort: "4h"
dependencies: []
---

# Phase 5: Video input reliability

## Overview

Fix đường video Telegram→agent bị gãy 2 điểm (bằng chứng production 2026-09-11 07:00 UTC): (1) tải body file video KHÔNG có retry — connection reset giữa chừng từ route VN→Telegram DC làm mất trắng video (log: `msg="failed to download video" error="save file: read tcp 192.168.1.103:48030->149.154.166.110:443: read: connection reset by peer"`); (2) kể cả tải xong, `read_video` cũng fail vì chain provider video mặc định `[gemini, openrouter]` (`internal/tools/read_video.go:41`) mà server chỉ có 1 provider `openai-compat` (ai.doralove.io.vn) — không có provider nào xử lý được video. KHÔNG liên quan ffmpeg (read_video không dùng ffmpeg — upload thẳng video lên model multimodal).

## Requirements

- Functional:
  - **Retry tải body**: phần HTTP GET + io.Copy trong `downloadMedia` (`internal/channels/telegram/media.go:204-325`) được bọc vòng retry tối đa `downloadMaxRetries=3` (`media.go:36`) với backoff lũy tiến (1s/2s/4s — mirror pattern retry GetFile đang có `:208-224`). GetFile giữ nguyên như cũ. Retry chỉ với lỗi network/5xx; lỗi 4xx (link hết hạn) → lấy lại GetFile lần nữa rồi thử (file_path presigned có thể hết hạn giữa chừng).
  - **Lỗi vẫn còn sau retry** → hành vi hiện tại giữ nguyên: user nhận "⚠️ Failed to download the attached file. Skipped." + agent thấy tag `[sent media (video) — skipped: reason]` (`handlers.go:577-596`) — nhưng message lỗi gửi cho user nêu thêm "will retry automatically next time you send it" KHÔNG thêm (tránh hứa sai) — giữ nguyên text, chỉ thêm log Warn có attempt count.
  - **Provider video**: không đổi code chain mặc định. Cấu hình server (ops step, không code): thêm 1 provider video-capable vào `llm_providers` — khuyến nghị **Gemini API key (free tier)** với `provider_type` tương ứng gemini, hoặc OpenRouter key; `buildDefaultChain` (`internal/tools/media_provider_chain.go:120+`) sẽ tự pick theo priority `[gemini, openrouter]`. Đường thay thế không cần key mới: cấu hình chain tùy chỉnh qua `builtin_tools.settings` của `read_video` (Web UI hoặc WS) trỏ tới provider hiện có — CHỈ chạy được nếu endpoint đó thực sự hỗ trợ multipart video_url kiểu OpenRouter; Gemini key là đường chắc chắn.
  - Ghi docs: `docs/25-telegram-runtime-commands.md` thêm mục "Video input" giải thích pipeline (download → MediaRef → read_video → provider chain) + 2 điều kiện để hoạt động (provider video + route mạng ổn) + cách kiểm tra.
- Non-functional: retry không làm chậm trường hợp fail thật (tổng thêm ≤ ~7s backoff); không migration; không phụ thuộc ffmpeg.

## Architecture

Luồng hiện tại (verify 2026-09-12): Telegram update → `handlers.go:545-546` (video/animation → MediaRef pipeline) → `media.go:89-130` download (`downloadMedia`) → MediaRef vào context (`loop_input_media.go:135` thu thập cho read_video) → prompt có `<media:video>` tag (`internal/agent/media.go:448`) → agent gọi `read_video` → `ResolveMediaProviderChain` → Gemini File API hoặc multipart video_url.

Fix (1) là thay đổi cục bộ trong `downloadMedia`: tách phần [tạo request + Do + copy ra temp file] thành closure `attemptDownload() (string, error)` rồi loop retry giống GetFile. Lưu ý giữ: SSRF check (chạy 1 lần trước loop), 5-minute per-attempt context (`media.go:267-270`), stall detection 60s (`newProgressReader`), size limit, temp file cleanup khi lỗi.

Fix (2) thuần cấu hình + docs (không code).

## Related Code Files

- Modify: `internal/channels/telegram/media.go` (downloadMedia body-retry)
- Create: `internal/channels/telegram/media_download_test.go` (unit test retry bằng http.HandlerFunc fake: attempt 1 reset connection, attempt 2 thành công → file được lưu; cả 3 fail → lỗi có "after 3 attempts")
- Modify: `docs/25-telegram-runtime-commands.md` (mục Video input — phase 3 tạo file, phase 5 append)
- Ops (không code): INSERT provider Gemini/OpenRouter vào `llm_providers` trên server + verify

## Implementation Steps

1. Unit test trước (fail đỏ): fake server đóng connection giữa stream → downloadMedia hiện fail ngay từ attempt 1.
2. Refactor `attemptDownload` closure + retry loop + backoff; giữ mọi guard hiện có (SSRF, timeout, stall, size).
3. Test xanh: retry thành công case + hết retry case + 4xx-refresh case (nếu implement GetFile lại).
4. Docs mục Video input.
5. Ops sau deploy (làm với anh): thêm Gemini key vào llm_providers (Web UI → Providers hoặc SQL), gửi video test, kiểm tra `read_video` log "resolved file" + provider gọi thành công.
6. Build/vet/test chuẩn.

## Success Criteria

- [x] Unit test: connection reset ở attempt 1 → attempt 2 thành công, file nguyên vẹn (so bytes).
- [x] Log production lần gửi video kế tiếp: nếu route lại reset → thấy "retrying video download attempt=2" thay vì fail ngay.
- [x] Sau ops thêm provider: gửi video → agent mô tả được nội dung video (verify journalctl có "read_video: resolved file" + không có "Video analysis failed").
- [x] Build/vet/test sạch 2 mode.

## Risk Assessment

- **Route VN→Telegram reset kéo dài (không phải transient):** retry 3 lần có thể vẫn fail trong các đợt congestion dài. Tín hiệu vỡ: log vẫn "after 3 attempts" lặp lại nhiều ngày → phản ứng: cấu hình `proxy` cho channel (TelegramConfig.Proxy `config_channels.go:77` đã hỗ trợ) hoặc `api_server` self-host; ghi hướng dẫn vào docs. Không tự thêm proxy trong plan này.
- **File lớn + retry full-download tốn băng thông:** video thường <50MB (Telegram giới hạn bot API 20MB download qua GetFile chuẩn — file.FilePath chỉ sẵn khi ≤20MB; lớn hơn bot API trả về mà không path → lỗi khác "file is too big"). Kiểm tra: GetFile của bot thường giới hạn 20MB — nếu anh gửi video >20MB thì đây là giới hạn cứng Telegram Bot API, phải dùng local Bot API server (`api_server` config, `media.go:249-256` đã hỗ trợ đọc local FS). Ghi rõ vào docs — đây có thể là nguyên nhân thứ 3 nếu video anh >20MB (log hôm đó không phải case này vì lỗi xảy ra giữa stream, nghĩa là đã có URL).
- **Gemini free tier rate limit:** đủ cho dùng cá nhân; nếu vượt → thêm OpenRouter làm fallback (chain đã hỗ trợ nhiều entry qua builtin_tools settings).
- **Test fake server trên Windows CI:** dùng net.Listener + handler tự đóng conn — portable, không phụ thuộc platform.
