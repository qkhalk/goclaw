---
title: "GoTools standalone repo: kiến trúc chi tiết và triển khai"
description: "Plan con chuyên sâu cho workstream A (thay thế phase 1-4 của plan cha 260922-0554): dựng repo riêng qkhalk/gotools — Go server + Vite SPA + SQLite + videoworker sidecar, 3 tool (watermark/pptx/video) + admin model/providers, deploy domain riêng."
status: pending
priority: P1
effort: "~12 ngày (8 phase)"
tags: [gotools, new-repo, standalone, go, vite, sqlite, sse, video, pptx, watermark]
created: "2026-09-22"
related: [260922-0554-goclaw-platform-expansion-studio-repo-rieng-subagents-ui-archive-skill-market-docs-site]
---

# GoTools standalone repo: kiến trúc chi tiết và triển khai

## Overview

GoTools (repo `qkhalk/gotools`) là sản phẩm độc lập tách từ phần Tools của goclaw: **1 binary Go** serve SPA + API + SSE, **1 sidecar videoworker** render ffmpeg, **SQLite** lưu trữ — chạy ở domain riêng, không cần goclaw. Plan này bung chi tiết kiến trúc của plan cha (thay thế phase 1–4 của `260922-0554-...`).

**Quyết định đã chốt từ validate session plan cha** (áp dụng nguyên): repo `qkhalk/gotools` display **GoTools**, backend Go + SPA; TTS KHÔNG port; standalone tự chứa không proxy goclaw; slim designer thay agent loop.

## Kiến trúc tổng thể

```
┌─ Domain riêng (tools.anh.vn — ví dụ) ─────────────────────────────┐
│  nginx/Caddy (TLS, SSE: proxy_buffering off)                      │
│    │                                                              │
│    ▼ :18890                                                       │
│  ┌───────────────────── gotools (1 binary) ─────────────────────┐ │
│  │ webui.go   go:embed web/dist (SPA React)                     │ │
│  │ auth.go    bearer token admin (constant-time)                │ │
│  │ /v1/providers*   CRUD + models + verify  ── AES-GCM key      │ │
│  │ /v1/designer/*   slim LLM chat (SSE fetch-stream)            │ │
│  │ /v1/video/jobs*  + dispatcher poll ──────┐                   │ │
│  │ /v1/files/videos/{id}.mp4?ft=HMAC       │                    │ │
│  │ SQLite ~/.gotools/gotools.db            │                    │ │
│  └─────────────────────────────────────────┼────────────────────┘ │
│                                            ▼ :18891               │
│  ┌──────────── gotools-videoworker (sidecar) ──────────────────┐  │
│  │ ffmpeg render storyboard → MP4 (port nguyên cmd/videoworker)│  │
│  │ + internal/vworker — self-contained, bearer token           │  │
│  └─────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────┘
```

**Tech stack:** Go 1.26 (net/http thuần, KHÔNG gorilla — SSE viết tay qua `http.Flusher`), `modernc.org/sqlite` (pure Go, CGO_ENABLED=0), Vite 6 + React 19 + TS + Tailwind 4 + zustand + react-query (mirror ui/web goclaw). Không PostgreSQL, không Redis, không WS — mọi realtime là SSE qua fetch-stream (có thể set `Authorization` header nên không cần ticket query-param như EventSource).

**Vì sao copy-port chứ không import:** Go cấm import package `internal/` cross-module (mọi code goclaw dùng chung nằm trong `goclaw/internal/`). Mọi file port gắn header PROVENANCE `// Ported from goclaw@<sha>: <path>` + script CI check.

## Data model (SQLite — đầy đủ, chạy một lần)

```sql
-- migrations/0001_init.sql (chạy tự động khi khởi động)
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);

CREATE TABLE llm_providers (
  id TEXT PRIMARY KEY,                -- uuid
  name TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  provider_type TEXT NOT NULL DEFAULT 'openai_compat',  -- openai_compat | anthropic
  api_base TEXT NOT NULL,
  api_key_enc BLOB NOT NULL,          -- AES-256-GCM(secret, key)
  default_model TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

CREATE TABLE designer_sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  conv_id TEXT NOT NULL,
  kind TEXT NOT NULL,                 -- pptx | video
  role TEXT NOT NULL,                 -- user | assistant
  content TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX idx_designer_sessions_conv ON designer_sessions(conv_id, id);

CREATE TABLE video_jobs (
  id TEXT PRIMARY KEY,                -- uuid
  storyboard TEXT NOT NULL,           -- JSON đã validate
  status TEXT NOT NULL DEFAULT 'queued', -- queued|running|completed|failed|cancelled
  worker_job_id TEXT NOT NULL DEFAULT '',
  output_name TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
-- keys: designer.pptx.provider/model, designer.video.provider/model
```

## API contract (toàn bộ)

| Method | Path | Auth | Notes |
|---|---|---|---|
| GET | /health | — | 200 + version |
| POST | /v1/auth/check | bearer | verify token (login form gọi) |
| GET/POST | /v1/providers | bearer | list (key masked) / create |
| PUT/DELETE | /v1/providers/{id} | bearer | update / delete |
| GET | /v1/providers/{id}/models | bearer | fetch /models, fallback catalog tĩnh |
| POST | /v1/providers/{id}/verify | bearer | 1 call nhỏ → {ok, latency_ms, model_reply} |
| GET/PUT | /v1/settings | bearer | designer defaults |
| POST | /v1/designer/chat | bearer | SSE fetch-stream: `{convId, kind, message}` → events `delta`/`done`/`error` |
| GET | /v1/designer/sessions?kind= | bearer | list convId + title + updated_at |
| DELETE | /v1/designer/sessions/{convId} | bearer | xoá history |
| POST/GET | /v1/video/jobs | bearer | submit (storyboard JSON validate) / list |
| GET/DELETE | /v1/video/jobs/{id} | bearer | status / cancel (proxy worker) |
| GET | /v1/video/jobs/events | bearer | SSE fetch-stream: event `video.job.updated` |
| GET | /v1/files/videos/{name}?ft= | ft token | download MP4 — HMAC sign (port pattern `SignFileToken` `internal/http/video.go:55-66`) |

## Port manifest (nguồn goclaw → đích gotools — chi tiết ở từng phase)

| Nhóm | Nguồn (ui/web/src/) | Ghi chú |
|---|---|---|
| UI kit | components/ui/* (badge, button, input, label, progress, select, sheet, switch, tabs, textarea), components/shared/* | copy nguyên |
| Transport | api/http-client.ts (thêm getBaseUrl), stores use-auth/use-toast/use-ui | sửa nhẹ |
| Chat stack | pages/chat/hooks/use-chat-send.ts, components/chat/chat-input.tsx, message-bubble.tsx, active-run-zone.tsx, types/chat.ts | chỉ cho designer column |
| Tools | pages/tools/watermark/**, pages/tools/pptx/**, pages/tools/video/** | phase 3/5/7 |
| Providers admin | pages/providers/** (page, form, hooks) | phase 4, bỏ oauth/pool |
| Login | pages/login/** | phase 2 |
| i18n | i18n framework + locales | **chỉ en + vi** (sản phẩm mới, bỏ ko/ru/zh) |
| Backend worker | cmd/videoworker + internal/vworker | phase 6 — port nguyên (self-contained đã verify: chỉ import stdlib + vworker/contract) |
| Prompt/skill | internal/pptx/designer_agent.go, internal/video/designer_agent.go, bundled-skills/{pptx-deck-design,pptx-visual-style,video-storyboard-design,video-color-motion}/SKILL.md | phase 5/7 — embed qua go:embed assets/skills/ |

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Repo gotools build 1 binary + 1 worker binary, CI xanh, deploy domain riêng | P1 |
| 2 | Watermark + PPTX Studio + Video Studio hoạt động end-to-end không cần goclaw | P1 |
| 3 | Admin providers/model: thêm key, verify, chọn default cho từng designer | P1 |
| 4 | Slim designer chat SSE chất lượng đủ dùng (parse blocks chuẩn) | P1 |

## Phases

| # | Phase | Status | Dep |
|---|-------|--------|-----|
| 1 | [Repo scaffold + server skeleton + CI](./phase-01-start.md) | Pending | — |
| 2 | [Web shell: port kit + login + nav + i18n](./phase-02-web-shell-port-kit-login-nav-i18n.md) | Pending | 1 |
| 3 | [Watermark tool port (client-only)](./phase-03-watermark-tool-port-client-only.md) | Pending | 2 |
| 4 | [Admin providers: crypto + CRUD API + UI + designer defaults](./phase-04-admin-providers-crypto-crud-api-ui-designer-defaults.md) | Pending | 2 |
| 5 | [Designer engine: prompt assembly + SSE chat + PPTX studio](./phase-05-designer-engine-prompt-assembly-sse-chat-pptx-studio.md) | Pending | 4 |
| 6 | [Videoworker port + video jobs API + SSE events](./phase-06-videoworker-port-video-jobs-api-sse-events.md) | Pending | 1 |
| 7 | [Video studio UI port + E2E render](./phase-07-video-studio-ui-port-e2e-render.md) | Pending | 5, 6 |
| 8 | [Deployment pack + domain + release v1.0.0](./phase-08-deployment-pack-domain-release-v100.md) | Pending | 3, 7 |

## Cross-plan relations

- **Plan cha** `260922-0554-...`: phase 1–4 của plan cha **được thay thế bằng plan này** (chi tiết hoá). Cook workstream A từ plan này, không cook phase 1–4 plan cha. Workstream B/C/D/E (phase 5–12 plan cha) không đổi.
- `260915-1243-video-designer-agent-column`, `260914-tools-redesign-clouds-fixes` (done): nguồn lịch sử của các trang tool được port — không giao nhau khi sửa (repo khác).

## Success Criteria

- [ ] `https://<domain-riêng>` : 3 tool + admin dùng được; goclaw trên cùng server không bị ảnh hưởng (port tách 18890/18891)
- [ ] E2E: watermark xoá sạch ảnh test; PPTX "5 slide giới thiệu X" → export .pptx mở PowerPoint được; video 3 cảnh + narration vi → MP4 tải về chơi được
- [ ] Provider thật verify OK; đổi default model designer → phản ánh ở call tiếp theo; api_key trong DB là ciphertext, UI masked
- [ ] Cancel video job dừng thật; SSE update <2s; abort designer stream không leak goroutine (`go test -race`)
- [ ] Release v1.0.0 tag → CI ra 2 binary linux-amd64 + tarball deploy

## Risk Assessment

| Rủi ro | Mitigation |
|---|---|
| Diverge port vs goclaw theo thời gian | PROVENANCE header + `scripts/check-provenance` chặn CI + port/MANIFEST.md ghi map nguồn |
| Slim designer yếu hơn agent loop | Skill prompt inline đầy đủ (chúng vốn prompt thuần); nếu vẫn kém → mở "connect GoClaw mode" (out of scope, note phase 5) |
| SSE qua reverse proxy bị buffer | server gửi `X-Accel-Buffering: no` + snippet nginx `proxy_buffering off` trong deploy |
| ffmpeg/font thiếu trên host | deploy/README liệt kê gói (ffmpeg, fonts-noto) + Docker image đầy đủ ready-to-run |
| SQLite lock dưới tải nhẹ | WAL mode + 1 write connection pool; tải của tool studio đơn user là nhỏ |

## Open Questions

1. Domain thật cho GoTools là gì? (plan dùng placeholder `tools.example.com` — anh điền khi deploy phase 8)
2. Có cần Docker image chính thức ở v1.0.0 hay chỉ binary + systemd? (plan làm cả hai — Docker là optional artifact)

<!-- slug: gotools-standalone-repo-kien-truc-chi-tiet-va-trien-khai -->
