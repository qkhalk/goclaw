---
title: "Phase 1: Repo scaffold + server skeleton + CI"
status: todo
priority: P1
effort: "2d"
dependencies: []
---

# Phase 1: Repo scaffold + server skeleton + CI

## Overview
Dựng repo `qkhalk/gotools` với server Go chạy được: config JSON5 + env overlay, auth bearer, SQLite migrations tự chạy, serve SPA placeholder, health endpoint, CI xanh.

## Requirements
- Functional: `gotools --addr :18890` chạy, `/health` trả `{status, version}`; request `/v1/*` không token → 401 JSON; token đúng → 200 placeholder; DB tự tạo tại `~/.gotools/gotools.db` + migrations áp dụng.
- Non-functional: CGO_ENABLED=0 build tĩnh linux; deps tối thiểu (modernc.org/sqlite + stdlib; KHÔNG gorilla, KHÔNG framework); mọi log JSON slog như goclaw.

## Architecture
```
qkhalk/gotools/
  go.mod                      module github.com/qkhalk/gotools (go 1.26)
  cmd/gotools/main.go         flags --addr --config --data-dir --version
  internal/config/config.go   JSON5 gotools.json + env GOTOOLS_* overlay
                              (port pattern goclaw config_load.go)
  internal/config/defaults.go Addr :18890, DataDir ~/.gotools
  internal/auth/middleware.go bearer constant-time (port internal/http/auth.go:382-460)
  internal/db/db.go           sql.Open sqlite + WAL + FK ON + migrate runner
  internal/db/migrations.go   //go:embed migrations/*.sql
  internal/webui/webui.go     //go:embed web/dist → serve SPA + SPA fallback
  internal/httpapi/router.go  mux: /health, /v1/* (placeholder), static
  internal/httpapi/sse.go     SSE writer helper (http.Flusher) — dùng phase 5/6
  internal/version/version.go Version var (-ldflags set khi release)
  migrations/0001_init.sql    DDL ở plan.md (toàn bộ 5 bảng — tạo 1 lần)
  web/                        (phase 2 điền; phase 1 chỉ index.html placeholder)
  deploy/                     (phase 8)
  scripts/check-provenance.sh fail nếu file trong web/src/port/ thiếu header
  .github/workflows/ci.yml    go vet+test+build (linux) + web placeholder
  .gitignore, README.md, LICENSE
```

**Config shape** (`gotools.json`, JSON5 cho comment):
```json5
{
  "server": { "addr": ":18890" },
  "auth":   { "token": "" },              // env GOTOOLS_TOKEN bắt buộc khi trống
  "secret": "",                           // env GOTOOLS_SECRET — AES key + HMAC (derive SHA-256)
  "data_dir": "~/.gotools",
  "worker": { "url": "http://127.0.0.1:18891", "token": "" },  // phase 6 dùng
  "log": { "level": "info" }
}
```
Env overlay: `GOTOOLS_SERVER_ADDR`, `GOTOOLS_TOKEN`, `GOTOOLS_SECRET`, `GOTOOLS_DATA_DIR`, `GOTOOLS_WORKER_URL`, `GOTOOLS_WORKER_TOKEN`.

## Related Code Files
- Create: toàn bộ cây ở Architecture
- Reference (port pattern, KHÔNG sửa): `internal/http/auth.go:382-460` (constant-time bearer), `internal/config/config_load.go` (JSON5 + env overlay), goclaw embedui (cmd/gateway.go serve embedded dist)

## Implementation Steps
1. GitHub repo `qkhalk/gotools` (private) + go mod init + cây thư mục + .gitignore + README skeleton.
2. internal/config: JSON5 parse (port loader thu gọn — chỉ cần unmarshal + env overlay, không cần full goclaw overlay) + validate (token/secret bắt buộc khi chạy serve).
3. internal/db: open + PRAGMA journal_mode=WAL, busy_timeout 5s; migrate runner đọc embed, ghi schema_migrations; test với t.TempDir().
4. internal/auth middleware + internal/httpapi router + /health + /v1/auth/check (đúng/sai token test).
5. internal/webui embed placeholder index.html "GoTools" + SPA fallback (`/#` route).
6. cmd/gotools main: flags + config + start graceful shutdown (SIGINT/TERM, ctx timeout 10s).
7. CI workflow: `go vet ./... && go test -race ./... && go build` + placeholder web job.
8. Verify: chạy local, curl /health 200, curl /v1/auth/check không token 401, DB file tạo đúng chỗ + migration applied.

## Success Criteria
- [ ] Binary chạy :18890, health 200 có version; 401 JSON chuẩn khi thiếu token
- [ ] DB + 5 bảng tạo tự động (sqlite3 .tables check), chạy lại idempotent
- [ ] `go test -race ./...` xanh; CGO_ENABLED=0 GOOS=linux build tĩnh thành công
- [ ] CI xanh trên push đầu tiên

## Risk Assessment
- modernc sqlite trên máy build Windows (dev) cần GOOS đúng: test chạy native windows OK (pure Go — không cần cgo như mattn).
- JSON5: dùng lib goclaw đang dùng (`github.com/tidwall/json5`? — verify go.mod goclaw lúc port; nếu lỉnh kỉnh thì chấp nhận JSON thuần + comment strip).
