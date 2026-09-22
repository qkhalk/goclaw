# Kế hoạch: MCP Catalog + GitHub auto-installer

> Phase 3 của `capability-catalog.md` — kích hoạt theo yêu cầu: folder MCP + nút "Tải & cài" tự lấy từ link GitHub.

## Đánh giá kiến trúc trước khi build: tool của mình nên ở tầng nào?

Phân tích 3 hướng (so trên thực tế server 512MB / đĩa 82% / 1 vCPU):

| | **Tầng 1: Studio module (client)** | **Tầng 2: Go-native (trong binary, flag-gated)** | **Tầng 3: MCP server (folder mcp/, cài từ GitHub)** |
|---|---|---|---|
| Code chạy ở đâu | Browser (lazy React chunk) | Trong binary goclaw / worker riêng (videoworker pattern) | Process node/python riêng (stdio) |
| "Cài đặt" | Bật cờ — tức thời (ĐÃ CÓ, PR #115) | Bật cờ — tức thời | Clone + deps + smoke test, 1-3 phút |
| RAM server | 0 | Chia heap với gateway (~10-30MB khi chạy) | 60-90MB/process khi chạy |
| Runtime cần trên server | Không | Không (static Go) | node/python bắt buộc |
| Cập nhật | Theo binary (nút self-update có sẵn) | Theo binary | Phải cài lại từng tool |
| Giới hạn | Không chạy được khi không có browser | Phải viết bằng Go | Đúng chuẩn MCP — agent ngoài cắm được, dùng được thư viện JS/Python |

**Kết luận phân tích:**
- Tool của mình **mặc định nên là tầng 1 hoặc 2** — nhẹ nhất, không runtime ngoài, self-update lo hết, "cài" tức thời như anh thấy studio tool
- **Tầng 3 (MCP folder) đáng build nhưng là đường phụ**, dùng khi: (a) tool cần thư viện chỉ có trong JS/Python (remotion, jimp, SDK bên thứ ba), (b) muốn phát hành tool độc lập nhịp với bản goclaw, (c) sau này mở chợ cho nguồn ngoài
- Tool sẵn có (render_video, tts...) đã lộ cho agent ngoài qua `/mcp/bridge` — không thuộc tầng nào ở đây

Bài học từ chính goclaw: pptx/watermark là tầng 1 (không tốn server), video render là tầng 2 (Go + ffmpeg worker riêng) — **chưa tool nào của mình thực sự cần tầng 3**. Vậy tầng 3 build để có cơ chế + mở rộng tương lai, catalog bắt đầu rỗng, không ép tool nào vào nó.

## Câu trả lời ngắn: ĐƯỢC

Flow: folder `mcp/` ở gốc repo goclaw chứa **MCP server do mình tự viết** (mỗi tool 1 subfolder + manifest) → push lên GitHub (tag release) → Store hiển thị card "Tải & cài" → backend clone đúng tag, sparse-checkout đúng subfolder → cài dependencies → **test chạy thật** (spawn + MCP initialize + list tools) → tự đăng ký vào registry MCP server sẵn có → agent gán tool vào là dùng được.

Toàn bộ hệ sinh thái tự chứa — không phụ thuộc repo ngoài.

Khác studio tool: install **không tức thời** (clone + cài deps mất ~1-3 phút) → có job nền + progress.

## Đã có sẵn (không phải xây lại)

- `internal/mcp/manager*.go` — MCP client manager: spawn stdio server (command/args/env), **lazy-connect** (chưa dùng không chạy), retry, pool → [manager_connect.go](../goclaw-ov/internal/mcp/manager_connect.go)
- `crud_server.go` + trang `/mcp` UI — đăng ký/quản lý MCP server thủ công đã hoạt động
- Server anh: git 2.47, node v20.20 + npm, python 3.13 + pip (đủ cả 2 runtime phổ biến nhất của MCP server)

## Thiết kế

### 1. Folder tool server: `mcp/` ở gốc repo

Mỗi tool = 1 subfolder với code server (node/python) + `manifest.json`:

```
mcp/
  image-utils/
    manifest.json     ← name, description, runtime, entry, ram_note, repo (mặc định: chính repo goclaw), ref (mặc định: tag release đang chạy)
    package.json / pyproject.toml
    src/index.js ...
```

- **Nguồn cài mặc định = chính repo `qkhalk/goclaw`**: installer `git clone --depth 1 --branch <tag> --filter=blob:none --sparse` + sparse-checkout đúng `mcp/<tool>/` — không phải lập repo riêng cho từng tool; tool nào muốn repo riêng thì manifest khai `repo` + `subdir` riêng
- Catalog hiển thị trong Store được **sinh từ các manifest** (script build đọc `mcp/*/manifest.json` → generate JSON vào `internal/mcp/catalog/` rồi go:embed) — thêm tool = tạo folder, không phải sửa 2 chỗ
- **Custom entry**: admin dán link GitHub bất kỳ qua dialog "Thêm từ GitHub" — cùng đường install pin + audit

Ví dụ manifest:

```json
{
  "name": "image-utils",
  "display_name": "Image Utils",
  "description": "Resize/convert/compress ảnh ngay trên server",
  "category": "media",
  "runtime": "node",
  "entry": "src/index.js",
  "ram_note": "~60MB khi chạy, lazy-start"
}
```

### 2. Installer backend — `internal/mcp/installer/`

Job nền có progress (poll `GET /v1/mcp/install/{job}`), các bước:

1. **Preflight**: runtime có sẵn (node ≥18 / python3)? RAM available ≥ 150MB? đủ chỗ đĩa? Trùng tên đang cài?
2. **Tải**: `git clone --depth 1 --branch <ref> --filter=blob:none --no-checkout` + `git sparse-checkout set mcp/<tool>` vào `<data>/mcp/<name>/` — chỉ chấp nhận host `github.com`; ghi lại **commit SHA** sau clone để audit
3. **Deps**: node → `npm ci --omit=dev --no-audit --no-fund`; python → `python3 -m venv .venv && pip install --no-cache-dir -e .` — xong **tự dọn cache** (server chỉ còn 1.8GB đĩa)
4. **Smoke test**: spawn entry + MCP `initialize` + `tools/list` rồi tắt — cài xong nghĩa là CHẠY THẬT được, không cài xong hỏng
5. **Đăng ký**: ghi vào registry `mcp_servers` sẵn có (command=node, args=[`<dir>/dist/index.js`], env tối thiểu — **không bao giờ** đưa gateway token/bí mật vào env server ngoài)

Uninstall: kill process nếu đang chạy → xoá dir → huỷ đăng ký (confirm 2 bước như Store).

### 3. Guard tài nguyên (server 512MB)

- stdio server chỉ spawn khi agent thật sự gọi tool (lazy-connect đã có) — thêm **idle reaper**: process không gọi trong 10 phút → tắt, lần sau tự khởi động lại
- Refuse install nếu RAM available < 150MB hoặc đĩa < 300MB
- Mỗi node MCP server ~60-90MB RAM khi chạy → khuyến cáo cài ≤ 2-3 cái trên server anh (hiển thị RAM note trên card)

### 4. Guard (kỹ thuật, không phải "chống virus")

Nguồn cài = official `modelcontextprotocol/servers` + repo của anh → tin được chủ repo. Các guard giữ lại đều rẻ và vì lý do kỹ thuật, không phải nghi ngờ tác giả:

- **Pin tag + ghi commit SHA** — reproducibility/audit/rollback (biết chính xác đang chạy commit nào); KHÔNG theo branch mặc định trừ khi admin chủ động bật
- **Allowlist host `github.com`** — chống dán nhầm URL / package giả mạo (typo-squat từng xảy ra thật trong hệ sinh thái MCP), không phải chặn nguồn
- **Env tối thiểu** cho process MCP server — không đưa gateway token/bí mật vào env server ngoài (giới hạn blast radius nếu dependency bị compromise)
- Dependencies phía sau (`node_modules`) là mặt tấn công thật duy nhất còn lại — chấp nhận rủi ro này vì nguồn chính thống; smoke test + SHA log giúp phát hiện sớm khi có anomaly

### 5. API + Store UI

- `GET /v1/mcp/catalog` | `POST /v1/mcp/install` | `GET /v1/mcp/install/{job}` (progress) | `DELETE /v1/mcp/installed/{name}` — admin-only
- Store page: section "MCP servers" — card có trạng thái (Chưa cài / Đang cài 43% + log cuối / Đã cài @`a1b2c3d`) + nút Tải & cài / Gỡ + dialog "Thêm từ GitHub" (admin-only, không banner cảnh báo — nguồn là repo của anh/official)
- Sau khi cài: hint dẫn sang `/mcp` gán tool vào agent
- i18n: key mới vào `store.json` × 5 locale (en/vi/zh/ko/ru)

### 6. DB

Bảng mới `mcp_installed_packages`: name (unique), repo, ref, commit_sha, runtime, install_dir, entry, status, installed_at, updated_at. Migration **PG + SQLite song song** (checklist dual-DB).

## Rủi ro nói trước

- Install chậm hơn studio tool (phải tải + build) — có progress nên rõ ràng
- Đĩa server 82% dùng rồi — installer dọn cache mỗi lần, mỗi server node ~30-150MB
- Repo GitHub chết/moving → smoke test fail → rollback sạch dir, báo lỗi rõ

## Tool đầu tiên — anh chọn

Catalog bắt đầu **rỗng** (hoặc chỉ chứa đúng những tool anh khai). Để E2E chuỗi install chạy thật trên server anh, em sẽ **tự viết 1 server mẫu minimal** trong `mcp/` (mình tự viết 100%, không dính project ngoài — vd `image-utils`: resize/convert ảnh qua sharp, hoặc đơn giản hơn nữa nếu anh muốn). Tool thật sau này: anh tạo folder + manifest + push là hiện trong Store.

Sau này thấy tool hay của nguồn ngoài muốn thêm vào catalog thì vẫn làm được qua manifest khai repo riêng — nhưng mặc định chợ tool = của mình.

## Triển khai theo phase

- **A**: catalog embed + API list + installer (preflight→clone→deps→smoke→register) + uninstall + migration 2 DB
- **B**: Store UI (cards, progress, custom-URL dialog) + i18n 5 locale
- **C**: idle reaper + RAM/disk guard + docs

Mỗi phase: `go build` 2 tag + test + deploy server anh + **E2E cài thật 1 server trên 192.168.1.103** (cài → agent gọi tool → gỡ).


---

## Trạng thái triển khai (2026-09-20) — HOÀN THÀNH

- **PR #116** merged vào dev (9 commits), release **v4.9.4** + tag (workflow + artifacts đầy đủ)
- Deploy server 192.168.1.103, schema 127, health 200
- **E2E pass toàn chuỗi trên server thật:**
  - Catalog hiện media-probe từ manifest nhúng ✓
  - Install job 3.6s: preflight → clone @tag v4.9.4 (commit 7022574) → skip deps (zero-dep) → smoke test thấy tool `probe` → đăng ký mcp_servers ✓
  - Files 24KB tại `/root/.goclaw/data/mcp/media-probe` ✓
  - Gọi tool THẬT: probe MP4 → duration + codec streams JSON ✓
  - Uninstall: file + server row + package row sạch ✓
- Code-review pass + fix: env scrubbing cho git/npm/pip, process-group kill, stuck-row repair (WithoutCancel + RecoverStaleInstalls), ValidateArgs token-aware (path chứa -e/-c/-r hết false-positive), uninstall race guard, ref charset, followRedirects=0
- Ghi chú vận hành: migrations trên server đọc từ `/opt/goclaw/migrations` trên đĩa — deploy tay phải copy file migration kèm theo (release tarball chuẩn đã có sẵn); env file có prefix `export` nên systemd không nhận GOCLAW_AUTO_UPGRADE (đã chạy migrate thủ công)
- **Phase C (idle reaper / RAM-disk pre-check) — hoãn** như đã thống nhất: tier 3 là đường phụ, server anh chạy master tenant; thêm khi có tool tier-3 thứ hai
