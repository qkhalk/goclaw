---
phase: 5
title: "Mail digest automation + polish + docs"
status: completed
priority: P2
effort: "1d"
dependencies: ["phase-3"]
---

# Phase 5: Mail digest automation + polish + docs

## Overview

Skill `mail-digest` chạy theo lịch bằng cron sẵn có (không dựng subsystem mới), nút Test connection trên Cloud page, tài liệu setup GCP + bảo mật, sweep i18n + openapi + security review tổng.

## Requirements

- Functional:
  - **Digest = skill + cron, không phải code mới**: file `skills/mail-digest/SKILL.md` (dùng tool `mail_search`/`mail_read`/`mail_unsubscribe` phase 3) — user schedule qua cron sẵn có (`internal/cron` + agent chạy trên `LaneCron`). SKILL.md dạy agent: gom mail 24h qua (q=after:), nhóm theo sender, đếm newsletter/promo (List-Unsubscribe header), tóm tắt ngắn + liệt kê top sender đề xuất unsubscribe (chỉ đề xuất — KHÔNG tự execute), gửi báo cáo qua kênh đang chat.
  - Cloud page polish: nút **Test** cho account → `GET /v1/cloud/accounts/{id}/test` (gmail getProfile / rclone about tùy capability) → cập nhật status chip (endpoint thêm ở phase này, handler re dùng internal/cloud).
  - Nút **Reconnect** = Disconnect + Connect flow (UI only, không endpoint mới).
  - Docs `docs/30-cloud-accounts.md`: setup GCP OAuth client từng bước (create Web application client, redirect URI exact, consent screen; **cảnh báo testing-mode refresh token 7 ngày + 100 test users → publish-to-production unverified hoặc internal org app**), env `GOCLAW_CLOUD_GOOGLE_CLIENT_SECRET`, config `cloud.*`, install rclone (apt/brew/script), Docker full có sẵn, bảo mật (token AES-256-GCM, rcd loopback, không log token, consent unsubscribe), troubleshooting (`redirect_uri_mismatch`, 7-day expiry, 429).
  - OpenAPI spec hoàn chỉnh mọi `/v1/cloud/*`; CHANGELOG; README feature bullet.
  - Security review checklist (pass/fail ghi vào PR):
    - grep toàn repo: không có log token/secret (search `access_token`, `refresh_token` trong slog calls).
    - Callback: state HMAC + expiry + rate limit; 302 chỉ về path `/cloud` same-origin (không open redirect — không dùng param làm redirect target).
    - Store: mọi query WHERE tenant_id+user_id; DELETE ownership.
    - Tools: không delete vĩnh viễn mail; unsubscribe log warn; fetch trong workspace boundary.
    - Lite: endpoints 404/403, tools không đăng ký, UI ẩn.
- Non-functional: không thêm benchmark/load test (luật AGENTS.md); digest skill chạy như agent run thường (token tiêu thụ do user chủ động schedule).

## Architecture

Skill-digest leverage toàn bộ hạ tầng sẵn có (skills loader, cron service + lanes, mail tools) — zero backend mới ngoài endpoint `/test`. Đây là quyết định "simplest solution addressing the root need": anh muốn mail được dọn định kỳ — cron chạy agent với skill là đúng primitive đã có, không dựng worker riêng.

## Related Code Files

- Create: `skills/mail-digest/SKILL.md`
- Modify: `internal/http/cloud.go` (+ `GET /v1/cloud/accounts/{id}/test`), openapi spec
- Modify: `ui/web/src/pages/cloud/` (Test + Reconnect buttons wiring)
- Create: `docs/30-cloud-accounts.md`; modify `CHANGELOG.md`, `README.md`
- Modify (nếu thiếu key): `internal/i18n/keys.go` + 5 catalogs

## Implementation Steps

1. `/test` endpoint + UI wiring + tests (gmail path fake, storage path fake rc).
2. SKILL.md mail-digest + test thủ công schedule `every day 08:00` trên dev.
3. Docs + CHANGELOG + README + openapi cuối.
4. Security review checklist chạy + ghi kết quả vào PR description.
5. Full pass: `go fix`, build 2 mode, vet, `make test-critical`; manual E2E tổng trên server dev (connect 2 account → search/read/archive/unsubscribe → cloud_ls/fetch → digest schedule).

## Success Criteria

- [x] Schedule digest ngày 08:00 → agent chạy trên LaneCron, trả báo cáo nhóm theo sender + đề xuất unsubscribe (không tự unsubscribe).
- [x] Nút Test cập nhật status chip đúng (account sống → active; token chết → expired + message).
- [x] Docs: người mới tự setup được GCP client + connect thành công chỉ theo docs (anh test thử).
- [x] Security checklist tất cả pass, kết quả đính kèm PR.
- [x] Surface parity statement đầy đủ: gateway ✓, API contract ✓ (openapi), Web UI ✓, CLI/runtime — N/A vì không đổi lệnh CLI (state rõ trong PR).
- [x] Build/vet/test sạch 2 mode; release theo quy trình fork (build local Docker → scp, hoặc dispatch release-fork.yaml).

## Risk Assessment

- **Digest tốn token nếu mailbox lớn** — SKILL.md ghi giới hạn (top 20 sender, mail 24h, max_results 100); user tự chủ schedule frequency. Signal: user complaint cost → thêm config budget.
- **`/test` hit Gmail rate limit** nếu user bấm liên tục — response cache 30s phía handler + debounce UI.
- **Docs lỗi thời theo Google console UI** — screenshot-free docs (viết theo tên mục, không ảnh) để bền vững hơn.
- **Phase parity chung:** mọi phase commit theo checklist AGENTS.md (go fix, build 2 mode, vet, integration) — phase này là chỗ verify tổng lần cuối trước release.
