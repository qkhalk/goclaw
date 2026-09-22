---
title: "Phase 4: Studio: admin quản lí model/providers"
status: todo
priority: P2
effort: "1.5d"
dependencies: [1]
---

# Phase 4: Studio: admin quản lí model/providers

## Overview
Trang admin của Studio: CRUD provider (api base/key/type), duyệt model catalog, test verify, chọn model mặc định cho từng designer (video/pptx). Port slice `pages/providers` + handlers `/v1/providers` của goclaw, đơn hoá về single-user SQLite.

## Requirements
- Functional: thêm/sửa/xoá provider (openai_compat + anthropic); load danh sách model từ `/models` (fallback catalog tĩnh); nút Test (verify 1 call nhỏ); chọn default model cho designer video + pptx; API key mã hoá-at-rest.
- Non-functional: AES-256-GCM cho api_key (port `internal/crypto` slice), key master từ env `STUDIO_SECRET`; KHÔNG multi-tenant (bỏ tenant_id — server single admin).

## Architecture
- DB bảng `llm_providers` (port `migrations/000001_init_schema.up.sql:25-36` bỏ tenant) + bảng `settings` (key/value: `designer.video.model`, `designer.pptx.model`, `designer.*.provider`).
- API port slice `internal/http/providers.go:164-185`: `GET/POST/PUT/DELETE /v1/providers`, `GET /v1/providers/{id}/models` (port catalog fallback `provider_models_catalog.go`), `POST /v1/providers/{id}/verify` (1 call "ping" nhỏ đếm latency). Bỏ pool/codex/oauth sections (không cần cho designer).
- LLM caller cho designer: port phần provider cần thiết — HTTP client call `/chat/completions` + `/messages` (anthropic) stream; dùng lại chung cho verify. Đây là slice nhỏ, viết gọn hơn port nguyên `internal/providers` (chỉ 2 loại provider, 1 use case).
- UI port slice `pages/providers/`: providers-page, provider-form-dialog, provider-standard-form-fields, hooks use-providers/use-provider-models/use-provider-verify (`pages/providers/hooks/*`). Settings mặc định designer: card "Designer defaults" (2 select provider+model).
- Migration tự chạy khi khởi động (embed schema + version).

## Related Code Files
- Create (repo mới): `server/providers/` (handlers + llm client + verify), `server/store/providers.go`, `server/crypto/` (AES-GCM port), `web/src/port/pages/providers/**` (page + dialog + fields + hooks), `web/src/pages/settings/` (designer defaults card)
- Reference: `internal/http/providers.go:164-185`, `internal/http/provider_models_catalog.go`, `ui/web/src/pages/providers/**`, `internal/crypto/` (AES-256-GCM), `migrations/000001_init_schema.up.sql:25-36`

## Implementation Steps
1. Port crypto slice + providers store (SQLite) + handlers CRUD.
2. Viết LLM caller (openai-compat + anthropic, stream SSE parse) — dùng chung cho designer completion (phase 2 tạm hardcode 1 provider; phase 4 thay bằng chọn từ settings).
3. Port UI providers page + hooks; verify button hiện latency + model trả lời.
4. Designer defaults settings (2 select) + wire vào slim endpoint phase 2 & 3.
5. Test: provider sai key → verify fail rõ lỗi; designer đổi model → response theo model mới.
6. Security pass: api_key không bao giờ trả raw về UI (chỉ masked `sk-...abc`); log scrub key.

## Success Criteria
- [ ] Thêm provider OpenAI-compat thật + verify OK (latency hiển thị)
- [ ] Model list load từ API có fallback catalog khi provider không hỗ trợ /models
- [ ] Đổi default model designer → chat tiếp theo dùng model đó (verify qua response/meta)
- [ ] api_key trong DB là ciphertext (mở sqlite check), UI chỉ thấy masked

## Risk Assessment
- Anthropic stream format khác openai: caller viết 2 path riêng có test bằng mock SSE fixture; không cố tổng quát hoá.
- Quên scrub key trong log: review checklist + test unit assert log không chứa key pattern.
