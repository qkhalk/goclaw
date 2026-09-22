# Plan: Tool Store — cài/gỡ tool theo nhu cầu (pptx, video, MCP…), agent trong & ngoài đều dùng được

> Cập nhật: 2026-09-20 · Repo: qkhalk/goclaw · Yêu cầu của anh:
> 1. Vào một trang, thấy nút **Cài** từng phần (tạo PPTX, tạo Video…) → dùng → **Xóa** đi, server giữ gọn
> 2. Tool đã cài phải dùng được cho **agent ngoài goclaw** (Claude Code, Cursor…), không riêng agent trong goclaw
> 3. Nguyên tắc đứng sau: nặng để client / không để server sập; cẩn thận nguồn ngoài (virus)

## 0. Quyết định kiến trúc (tổng quát)

Một trang **"Tool Store"** duy nhất với 3 loại mục, cùng nút Cài/Gỡ:

| Loại mục | "Cài" nghĩa là | Code nằm đâu | RAM khi chưa cài |
|---|---|---|---|
| **tool** (pptx, video studio, tts-clone…) | bật module first-party per-tenant (đã có hạ tầng!) | đã compile trong binary | 0 — không process, không nav, không agent tool |
| **mcp** (phase sau) | ghi 1 dòng config MCP server vào DB | tải từ npm/binary **pin version** khi gọi lần đầu | 0 — process spawn lazy, idle 20' tự chết |
| **browser/worker** (phase sau) | tải model cho trình duyệt / khai báo sidecar | client hoặc máy khác | 0 trên server |

**Quan trọng & nói rõ:** với tool first-party, "Xóa" = tắt module (nav ẩn + API gate + agent tool bị gỡ khỏi surface + không process nào chạy) — binary không đổi. Đây là chọn đúng: module tải-được-từ-ngoài (plugin thật) mở cửa virus + phức tạp hóa, trong khi RAM server chỉ tiêu khi tool chạy thật — tắt module là RAM như chưa từng có.

## 1. Hiện trạng đã verify trong code (không làm lại)

| Mảnh | Bằng chứng (file:line) | Ý nghĩa |
|---|---|---|
| Bảng builtin tools có metadata đầy đủ | `internal/store/builtin_tool_store.go:12` `BuiltinToolDef{Name, DisplayName, Description, Category, Enabled, Settings, Requires, Metadata}` | Card Store không cần table/migration mới |
| **API cài/gỡ per-tenant đã có** | `internal/http/builtin_tools.go:47-54` — `GET /v1/tools/builtin`, `PUT/DELETE /v1/tools/builtin/{name}/tenant-config` (adminAuth) + audit event | Nút Cài/Gõ chỉ cần gọi API có sẵn |
| Override per-tenant | `internal/store/tenant_config_store.go:35` `BuiltinToolTenantConfigStore` (Set/ListDisabled/ListAll, tường minh tenantID) | Multi-tenant an toàn sẵn |
| Seed bảo tồn tuỳ biến | `cmd/gateway_builtin_tools.go:14` "Seed preserves user-customized enabled/settings" | Cài/gõ sống sót qua nâng cấp |
| Trang video đã gate theo enabled | `ui/web/src/pages/tools/video/video-tool-page.tsx:146,379` (`gateError` → EmptyState "video.not_enabled") | Pattern gate UI đã có |
| **Sidebar đang hardcode** | `ui/web/src/components/layout/sidebar.tsx:123-125` (list cố định TOOLS_VIDEO/TOOLS_PPTX) | Đây là chỗ phải sửa — nav theo dữ liệu |
| Gốc MCP-HTTP cho agent ngoài | `internal/mcp/bridge_server.go:134` (streamable-http, filter per-caller), `crud_server.go:101` (token auth pattern, mount `/api/mcp/`) | Nền cho Phase Hub |
| MCP servers DB + lazy + idle | `internal/store/mcp_store.go:132`, `internal/mcp/manager.go:431` (LoadForAgent), `pool.go:20-36` (IdleTTL 20m) | Cho loại mục `mcp` phase sau |

## 2. Thiết kế Phase 1 — Tool Store cho tool first-party

### 2.1 Luồng Cài / Xóa (dùng API có sẵn)

```
[Đã cài ✓]──bấm Xóa──► DELETE/PUT tenant-config {enabled:false}
                        ├─ nav "Video Studio" biến mất (sidebar động)
                        ├─ agent mất tool render_video (ListEnabled lọc)
                        └─ vào tay /tools/video → EmptyState (đã có)

[Chưa cài]──bấm Cài───► PUT /v1/tools/builtin/video_studio/tenant-config {enabled:true}
                        ├─ nav hiện "Video Studio"
                        ├─ agent được cấp tool; worker render按 demand
                        └─ RAM: 0 cho tới lần render đầu tiên
```

### 2.2 Việc phải làm mới

**Backend (nhỏ):**
- B1. Rà seed `builtinToolSeedData()` (`cmd/gateway_builtin_tools.go:15`): đảm bảo có def cho **mỗi trang tool** (video_studio, pptx_studio, watermark, tts_clone…) nếu còn thiếu — seed preserve nên thêm def mới an toàn. Gắn vào `Metadata` JSON: `nav_key`, `icon`, `ram_note` (cột có sẵn, không migration)
- B2. Endpoint list cho Store: mở rộng `GET /v1/tools/builtin` trả kèm tenant-override đã resolve (handler hiện đã enrich tenant config — verify `handleList` builtin_tools.go:170) + field `installed` per tenant
- B3. Xác nhận chuỗi gate: disable tool ⇒ agent tool thực sự rời surface (kiểm `ListEnabled` consumers + tools registry; nếu trang tool và agent tool là 2 def khác nhau thì map rõ)

**Frontend (chủ yếu):**
- F1. Trang mới **Store** (`ui/web/src/pages/store/`): grid card theo category, mỗi card = icon + tên + mô tả + badge RAM + nút **Cài/Xóa** (confirm khi Xóa đang dùng), trạng thái loading/error chuẩn; mobile: `grid-cols-1 sm:grid-cols-2 lg:grid-cols-3`, touch ≥44px
- F2. **Sidebar động**: thay list cố định `sidebar.tsx:123-125` bằng query `useBuiltinTools` (react-query có thể đã có hook — verify) lọc theo `installed` + map `Metadata.nav_key`
- F3. Route guard: vào tay route tool khi chưa cài → EmptyState + nút "Cài ngay" (tận dụng gate sẵn của video page, chuẩn hoá cho pptx/watermark)
- F4. i18n 5 locale: `store.*` (title, categories, install, uninstall, installed, confirm_uninstall, ram_note, requires_missing…)

### 2.3 UI Store (mock)

```
┌─ Store ────────────────────────────────────────────────────────┐
│  [Tất cả] [Media] [Văn phòng] [Giọng nói] [MCP (sau)]          │
│                                                                 │
│  ┌───────────────┐ ┌───────────────┐ ┌───────────────┐         │
│  │ 🎬 Video      │ │ 📊 PPTX       │ │ 🎙 Clone      │         │
│  │ Studio        │ │ Studio        │ │ giọng (TTS)   │         │
│  │ Render MP4    │ │ Slide kiểu    │ │ Thu âm → giọng│         │
│  │ 9:16/16:9     │ │ PowerPoint    │ │ trong browser│         │
│  │               │ │               │ │               │         │
│  │ ~RAM khi chạy │ │ nhẹ           │ │ model tải về  │         │
│  │ [✓ Đã cài]    │ │ [+ Cài]       │ │ browser       │         │
│  │   (Xóa)       │ │               │ │ [+ Cài]       │         │
│  └───────────────┘ └───────────────┘ └───────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

## 3. Phase 2 — Hub: agent NGOÀI goclaw dùng tool đã cài

Goclaw đã chạy MCP streamable-http (bridge + CRUD server có token auth). Delta:
- H1. Mỗi mục Store có toggle **"Cho agent ngoài"** (opt-in): tool first-party đã cài + toggle ⇒ expose qua bridge MCP với tiền tố `goclaw__<tool>`
- H2. Auth client ngoài: API key scope `mcp` (API key store có sẵn), middleware theo pattern `crud_server.go:101` — không dùng gateway admin token
- H3. Bridge tool filter (`bridge_server.go` `newBridgeToolFilter`) thêm nguồn "module có cờ expose" — không mở mặc định
- H4. UI: khối copy 1 dòng cho client ngoài: `claude mcp add goclaw --transport http http://<host>:18790/api/mcp --header "Authorization: Bearer <key>"` + JSON cho Cursor/Cline
- H5. E2E từ máy ngoài: add MCP vào Claude Code → gọi tool → thu hồi key → bị chặn

## 4. Phase 3 — Loại mục MCP trong Store (tool cộng thêm)

- Catalog JSON embed (`internal/catalog/`), curate + **pin version tuyệt đối**, schema test cấm `latest`
- Cài = ghi `MCPServerData` qua `MCPServerStore` có sẵn, `Settings.catalog_id` đánh dấu (không migration); process lazy + idle-evict có sẵn (`pool.go`)
- Endpoint `GET /v1/catalog`, `POST /v1/catalog/install|uninstall` theo pattern `packages.go`
- Card MCP xuất hiện trong cùng trang Store, badge "Local/Remote"

## 5. Phase 4 — Vòng đời & loại browser/worker

- Expose IdleTTL ra config + UI trạng thái running/idle; **RAM budget guard** trước khi spawn stdio MCP (đọc MemAvailable, dưới ngưỡng từ chối)
- Loại `browser`: mục trỏ model artifact GitHub Releases của repo anh → tải thẳng vào Cache Storage (khai thác luôn voice-clone ONNX + set `VOICE_CLONE_MODEL_BASE_URL`)
- Loại `worker`: khai báo sidecar kiểu videoworker (binary + port, chạy máy khác được)

## 6. Task list Phase 1 (thứ tự)

| # | Task | File |
|---|---|---|
| S1 | Scoping: map trang tool ↔ def builtin ↔ agent tool; ds def thiếu; verify hook react-query builtin tools & `handleList` enrich | `cmd/gateway_builtin_tools.go`, `internal/http/builtin_tools.go:170`, `ui/web/src/pages/builtin-tools/` |
| S2 | Bổ sung def thiếu + Metadata (nav_key/icon/ram_note) trong seed | `cmd/gateway_builtin_tools.go` |
| S3 | API list Store (installed per tenant) — mở rộng handler nếu thiếu | `internal/http/builtin_tools.go` |
| S4 | Trang Store UI + hook + confirm Xóa | `ui/web/src/pages/store/` (mới) |
| S5 | Sidebar động theo installed | `ui/web/src/components/layout/sidebar.tsx:123` |
| S6 | Route guard chuẩn hoá cho mọi trang tool | `pages/tools/*` |
| S7 | i18n 5 locale `store.*` | `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/` |
| S8 | Test: go build 2 edition + vet + unit; pnpm build; E2E live: Xóa Video → nav mất + agent thiếu tool + render bị chặn; Cài lại → mọi thứ về | server 192.168.1.103 |

## 7. Surface parity

- **Gateway server**: có (seed defs, list API enrich, gate chain verify) — không migration DB
- **API contract**: chỉ mở rộng response có sẵn (`installed`), không break
- **Web UI**: có (Store mới, sidebar động, guard, i18n×5)
- **CLI/runtime**: N/A — quản lý qua UI/HTTP

## 8. Rủi ro & giảm thiểu

| Rủi ro | Giảm thiểu |
|---|---|
| "Xóa tool" hiểu lầm là gỡ file | Card ghi rõ: tắt module — dữ liệu storyboard/deck trong workspace GIỮ NGUYÊN, cài lại là có lại |
| Disable mà agent vẫn thấy tool (gate hở) | S8 có bước E2E xuyên: disable → gọi tool phải bị chặn; nếu hở thì fix tại ListEnabled consumer |
| Sidebar động chậm (query chưa xong) | react-query staleTime + fallback giữ list hiện tại, không nhấp nháy |
| Anh lỡ xóa tool đang có job render chạy dở | Chỉ chặn job MỚI; job đang render chạy xong, UI ghi chú |
