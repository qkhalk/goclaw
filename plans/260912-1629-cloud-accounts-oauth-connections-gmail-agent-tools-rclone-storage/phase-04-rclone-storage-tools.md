---
phase: 4
title: "rclone storage layer + cloud tools"
status: completed
priority: P2
effort: "1.5d"
dependencies: ["phase-1"]
---

# Phase 4: rclone storage layer + cloud tools

## Overview

Chạy `rclone rcd` (rc HTTP API) làm process con loopback, inject remote Google Drive từ token đã lưu (phase 1), và expose agent tools đọc lưu trữ: `cloud_ls`, `cloud_stat`, `cloud_read`, `cloud_fetch`, `cloud_about`. Không import rclone làm library (research: ~40 backend dependencies + global state + không cam kết API ổn định).

## Requirements

- Functional:
  - `internal/cloud/storage/rcd.go` — supervisor:
    - Khởi động lazy (lần đầu tool storage chạy): `rclone rcd --rc-addr 127.0.0.1:<port> --rc-user <rand> --rc-pass <rand> --config <data-dir>/cloud/rclone.conf --rc-no-auth=false` — port chọn bằng cách listen tạm rồi release; credentials sinh bằng `crypto/rand` (16 byte mỗi cái).
    - Health check `core/ping` (hoặc `core/version`) trước khi dùng; chết → restart với backoff; shutdown gateway → kill process con.
    - Binary detect: config `cloud.rclone_path` (default `rclone` trong PATH); thiếu → tools trả lỗi rõ "rclone not installed — see docs/30-cloud-accounts.md", mail tools KHÔNG ảnh hưởng.
  - Remote injection: qua rc `config/create` (JSON body `{name: "goclaw-<accountID-prefix>", type: "drive", parameters: {token: <json>, scope..., root_folder_id...}}`) — không đụng INI tay. **Quan trọng (research):** rclone tự refresh token và ghi lại vào config file của nó → config file ở data-dir là nguồn sự thật chạy đime; `cloud_accounts` trong DB chỉ là bootstrap khi (re)connect. Reconnect = `config/delete` + `config/create` lại. Google rotate refresh token sẽ làm token trong DB cũ đi — flow hoạt động vì rclone giữ bản mới trong conf của nó; ghi nhận trong docs.
    - `drive.readonly` scope đã xin ở phase 1.
  - Tools (`internal/tools/cloud_storage.go`):
    - `cloud_accounts_storage` — list account có storage capability (tức provider google) — có thể gộp vào `mail_accounts` đổi tên `cloud_accounts`? **Quyết định:** tool phase 3 đổi tên thành `cloud_accounts` dùng chung (list mọi account + capability flags `{mail: true, storage: true}`) — tránh 2 tool trùng chức năng.
    - `cloud_ls(account?, path="/", max=100)` → `operations/list` (opt show-level?) → `[{name, is_dir, size, mod_time}]` (không recursive mặc định).
    - `cloud_stat(account?, path)` → `operations/stat`.
    - `cloud_read(account?, path, max_bytes cap 256KB)` → `operations/copyfile` remote → temp file → đọc → xóa temp (endpoint đã verify trong research; tránh `core/command` — shell-equivalent, cấm).
    - `cloud_fetch(account?, remote_path, dest_subdir="cloud/")` → copyfile vào **workspace** (agent dùng được với FS tools có sẵn); size cap 100MB config.
    - `cloud_about(account?)` → `operations/about` — quota used/total (research bonus polish).
  - Docker: runtime stage của `Dockerfile` (alpine, `FROM alpine:3.23` tại :70) dùng build-arg mechanism có sẵn — **audit-verify: "full" = `ENABLE_FULL_SKILLS=true`** (release-fork.yaml :232-236): cài rclone static binary (downloads.rclone.org, musl-compatible) trong nhánh `ENABLE_FULL_SKILLS=true`; các variant khác không — docs chỉ dẫn install.
  - Wiring/seed/systemprompt/i18n như phase 3 (category "cloud").
- Non-functional:
  - rcd **chỉ** loopback 127.0.0.1 + basic-auth ngẫu nhiên mỗi lần start; không proxy rc qua gateway HTTP; không expose trong docker-compose ports.
  - Timeout mọi rc call 60s; async job (`_async=true`) không dùng v1 (sync đủ cho list/stat/copy nhỏ).
  - File content từ cloud_read/cloud_fetch coi như untrusted (không auto-execute); cloud_fetch vẫn nằm trong workspace boundary sẵn có.

## Architecture

Tool → resolve account → ensure rcd running → ensure remote exists (rc `config/listremotes` check, không thì `config/create` từ token DB) → rc POST (basic-auth) → JSON về ForLLM compact. Lifecycle: supervisor owned bởi cloud manager (cmd/gateway.go wiring, edition-gated), Close() khi gateway shutdown.

## Related Code Files

- Create: `internal/cloud/storage/{rcd.go,rc_client.go,remotes.go}` (+ tests httptest fake rc)
- Create: `internal/tools/cloud_storage.go` (+ test)
- Modify: `internal/tools/cloud_mail.go` — rename `mail_accounts` → `cloud_accounts` (capability flags)
- Modify: `cmd/gateway.go` (supervisor lifecycle), `cmd/gateway_tools_wiring.go`, `cmd/gateway_builtin_tools.go`, `internal/agent/systemprompt.go`
- Modify: `internal/config/config_cloud.go` (rclone_path, fetch_size_cap, rc timeout)
- Modify: `Dockerfile` (full variant: install rclone) — verify multi-stage target hiện tại
- Modify: `internal/i18n/keys.go` + 5 catalogs

## Implementation Steps

1. rc client + httptest fake (auth header, config/create|listremotes body shapes, operations/list|stat|about shape) + tests.
2. Supervisor: port pick, credentials, health check, restart backoff, shutdown; test với fake binary (sh script imitating rcd trên Windows CI? — dùng go test build helper binary pattern `os.Executable` trick hoặc skip-if-no-rclone guard).
3. Remote injection + reconnect path + test với fake rc.
4. 5 tools + validation + ForLLM format + tests (missing binary error, caps).
5. Rename mail_accounts → cloud_accounts + capability flags (sửa phase 3 wiring).
6. Dockerfile full + docs snippet; manual E2E: cloud_ls Drive thật, cloud_fetch 1 file PDF vào workspace rồi agent đọc.

## Success Criteria

- [x] `cloud_ls`/`cloud_stat`/`cloud_read`/`cloud_fetch`/`cloud_about` hoạt động với Drive thật trên server dev; output compact không phá context.
- [x] Thiếu rclone binary: mọi tool storage trả lỗi hướng dẫn rõ; mail tools + Cloud page vẫn hoạt động bình thường (isolation).
- [x] rcd chỉ nghe 127.0.0.1; từ máy khác không kết nối được port (verify ss/netstat).
- [x] Reconnect account: remote cũ bị thay token mới (config/delete + create) — list lại đúng.
- [x] Gateway shutdown: process rcd không mồ côi (verify ps sau stop).
- [x] Build/vet/test sạch 2 mode (test supervisor skip gracefully khi không có rclone trên CI).

## Risk Assessment

- **rc = shell-equivalent** (research, rclone docs): loopback + random basic-auth + không expose + không bao giờ gọi `core/command` từ code. Nếu cần hard hơn: unix socket (research chưa verify rclone hỗ trợ cho rc) — kiểm tra ở bước implement, nếu hỗ trợ thì ưu tiên.
- **rclone tự refresh token làm DB stale** — chấp nhận thiết kế (conf file = runtime truth); reconnect rewrite. Signal vỡ: sau khi rclone refresh, DB token cũ → chỉ hỏng khi rclone conf bị xóa; recovery: nút Reconnect trong UI (phase 5 có thể thêm nếu cần — hiện Disconnect+Connect đủ).
- **Drive phức tạp** (shared drives, shortcuts): v1 mount root personal drive; shared drive support = backlog sau khi có user demand.
- **CI không có rclone** — supervisor test dùng fake binary + build tag skip; tools test chỉ dùng fake rc client.
- **File lớn cloud_fetch** — cap 100MB + trả lỗi hướng dẫn agent dùng cloud_read streaming? v1: chỉ cap + error.
