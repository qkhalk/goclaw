---
title: "Phase 4: Admin providers: crypto + CRUD API + UI + designer defaults"
status: todo
priority: P1
effort: "1.5d"
dependencies: [2]
---

# Phase 4: Admin providers: crypto + CRUD API + UI + designer defaults

## Overview
Trang admin "Quản lí model" của GoTools: CRUD provider (openai-compat + anthropic), duyệt model catalog, verify thật, chọn model mặc định cho designer video/pptx. API key mã hoá AES-256-GCM at-rest.

## Requirements
- Functional: đầy đủ endpoint bảng providers trong plan.md; verify gọi thật 1 completion nhỏ ("ping" → đếm latency, hiện vài từ phản hồi); model list từ `/models` fallback catalog tĩnh; settings designer defaults GET/PUT.
- Non-functional: api_key không bao giờ trả raw (response chỉ `masked_key: "sk-...abc4"`); log scrub key; test unit assert ciphertext ≠ plaintext.

## Architecture
- `internal/crypto/` — port slice AES-256-GCM từ goclaw `internal/crypto/`: key = SHA-256(config.Secret) (HKCK đơn giản hoá: `sha256(secret)` làm key 32 bytes); Seal/Open helper + test.
- `internal/providers/` (mới, gọn — KHÔNG port nguyên internal/providers goclaw):
  ```go
  type Caller interface {
    ChatStream(ctx, req ChatRequest) (<-chan Delta, error)   // SSE parse phía đọc
    ListModels(ctx) ([]string, error)
  }
  // 2 impl: openaiCompatCaller (POST {base}/chat/completions, stream=true),
  //         anthropicCaller  (POST {base}/v1/messages, stream SSE)
  // Verify: ChatStream với max_tokens 16, prompt "Say OK" → đo time + first token
  ```
  Đọc SSE parse: viết mini scanner (tham khảo tinh thần `internal/providers/sse_reader.go` — chỉ lấy event/data lines).
- `internal/httpapi/providers.go`: handlers CRUD + models + verify; store `internal/store/providers.go` (sqlx không dùng — database/sql thuần cho nhẹ).
- Catalog fallback: `internal/providers/catalog.go` map provider_type → list model phổ biến (port danh sách từ goclaw `internal/http/provider_models_catalog.go` tinh giản).
- UI port slice vào `web/src/port/pages/providers/`: providers-page, provider-form-dialog (bỏ oauth/cli/pool sections — GoTools không cần), hooks use-providers/use-provider-models/use-provider-verify (đổi endpoint cùng path giữ nguyên).
- Designer defaults card: 2 hàng (PPTX designer, Video designer) × 2 select (provider + model) → PUT /v1/settings keys `designer.pptx.*`, `designer.video.*`.

## Related Code Files
- Create: `internal/crypto/`, `internal/providers/`, `internal/store/providers.go`, `internal/httpapi/providers.go` + settings handler, `web/src/port/pages/providers/**`, `web/src/app/pages/admin/providers-page.tsx` (compose)
- Reference: `internal/crypto/` goclaw (AES-GCM), `internal/http/providers.go:164-185` (endpoint shapes), `internal/http/provider_models_catalog.go` (fallback list), `ui/web/src/pages/providers/**` (UI slice), `ui/web/src/pages/providers/hooks/use-providers.ts:19-65`

## Implementation Steps
1. internal/crypto port + test (roundtrip, wrong key fail).
2. Store + migrations đã có bảng (phase 1) → CRUD store + unit test temp DB.
3. Caller 2 impl + SSE mini scanner + mock httptest SSE fixture test (openai + anthropic format).
4. Handlers + router wire + verify endpoint (timeout 15s).
5. Catalog fallback.
6. UI: port page + dialog + hooks; defaults card; i18n admin namespace en/vi.
7. Security pass: response mask, log scrub (test unit grep log output), 401 chuẩn khi sai token.
8. E2E tay: thêm provider thật (OpenAI-compat endpoint anh đang dùng) → verify OK → chọn default.

## Success Criteria
- [ ] CRUD + verify hoạt động với provider thật (latency hiện)
- [ ] Model list load; khi provider không hỗ trợ /models → fallback catalog
- [ ] api_key ciphertext trong DB; UI masked; log không lộ key (test)
- [ ] Defaults lưu + GET trả đúng; đổi default phản ánh phase 5 dùng ngay

## Risk Assessment
- Anthropic SSE format (event: content_block_delta) khác openai (data: choices[...]): 2 parser riêng + fixture test — không tổng quát hoá sớm.
- Provider anh dùng là OpenAI-compat gateway (như 9router/oneapi): api_base có thể đã có hậu tố /v1 — form field ghi chú rõ "gốc, không /v1" + tự normalize bỏ /v1 cuối nếu có.
