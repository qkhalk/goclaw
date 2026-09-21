---
phase: 1
title: "Backend: /v1/system/stats endpoint"
status: pending
priority: P2
effort: "~3-4h"
dependencies: []
---

# Phase 1: Backend /v1/system/stats

## Overview

Thêm endpoint HTTP `GET /v1/system/stats` trả về chỉ số phần cứng host (CPU, RAM, disk, load, uptime) + chỉ số process gateway (goroutines, heap, RSS). Không có handler hệ thống nào tồn tại trước đó (grep `gopsutil|runtime.NumCPU|VirtualMemory` trong `internal/`, `cmd/` = 0 kết quả) — đây là nền cho System card ở phase 3.

## Requirements

- Functional: viewer role đọc được (GET auto-floor qua `requireAuth("")`, `internal/http/auth.go:420-460`); trả JSON ổn định, không crash khi một sub-system lỗi (load avg trên Windows, disk path không tồn tại).
- Non-functional: giá trị CPU% thật (delta sampler), chi phí mỗi request < 50ms, không cấp phát rác đáng kể, compile được với cả PG và `sqliteonly` tags.

## Architecture

```
cmd/gateway_http_wiring.go (~dòng 148, cạnh SetUsageHandler)
  └─ d.server.SetSystemStatsHandler(httpapi.NewSystemStatsHandler(d.dataDir))
       └─ internal/gateway/server.go: SetSystemStatsHandler → s.handlers = append(...)  (pattern :682-691)
            └─ internal/gateway/server.go:250-254: h.RegisterRoutes(mux)
                 └─ mux.HandleFunc("GET /v1/system/stats", requireAuth("", h.handleStats))
```

- Package mới `internal/sysstats`: gói sampler thuần (không phụ thuộc http), giữ state giữa các lần gọi:
  - `Sampler` struct: `lastCPUPercent` qua `cpu.Percent(0, false)` (gopsutil — interval 0 = delta từ lần gọi trước, đúng cơ chế stateful), `mu sync.Mutex`.
  - `NewSampler()` gọi 1 lần `cpu.Percent(200*time.Millisecond, false)` lúc khởi tạo để lần đọc đầu đã có baseline.
  - `Snapshot(diskPath string) Stats` — trả toàn bộ payload.
- Gopsutil v4 imports: `cpu`, `mem`, `disk`, `load`, `host`; process metrics dùng `runtime.NumGoroutine()`, `runtime.ReadMemStats()` (heap), `os.Getpid()`; RSS qua `process.NewProcess(int32(pid))` + `p.MemoryInfo()` (skip khi lỗi).

## Response shape (API contract)

```json
{
  "cpu":      { "percent": 23.5, "cores": 8, "load1": 0.42, "load5": 0.38, "load15": 0.31 },
  "memory":   { "total": 34359738368, "used": 17179869184, "available": 17179869184, "used_percent": 50.0,
                "swap_total": 8589934592, "swap_used": 0 },
  "disk":     { "path": "/opt/goclaw/data", "total": 104857600000, "used": 52428800000, "used_percent": 50.0 },
  "host":     { "os": "linux", "platform": "ubuntu 24.04", "host_uptime_secs": 864000 },
  "proc":     { "pid": 1234, "goroutines": 42, "heap_alloc_bytes": 12345678, "rss_bytes": 52428800, "go_version": "go1.26.0" }
}
```

- `load1/5/15`, `swap_*`, `rss_bytes`, `platform` là **pointer/omitempty** — absent trên Windows hoặc khi lỗi; UI phải ẩn khi thiếu.
- `disk.path` = dataDir được wire vào handler (`d.dataDir`); nếu rỗng/không stat được → omit toàn object `disk`.

## Related Code Files

- Create: `internal/sysstats/sampler.go`, `internal/sysstats/sampler_test.go`
- Create: `internal/http/system_stats.go`
- Modify: `internal/gateway/server.go` (thêm field + `SetSystemStatsHandler`, cạnh `SetBrowseRelayHandler` :677)
- Modify: `cmd/gateway_http_wiring.go` (wire cạnh `SetUsageHandler` :148)
- Modify: `go.mod` / `go.sum` (`go get github.com/shirou/gopsutil/v4`)

## Implementation Steps

1. `go get github.com/shirou/gopsutil/v4` (MIT; tương thích Go 1.26).
2. Viết `internal/sysstats/sampler.go` theo Architecture trên. Mỗi sub-system bọc riêng: lỗi của `load`/`disk`/`process` không ảnh hưởng phần còn lại.
3. Viết `internal/http/system_stats.go`: `SystemStatsHandler{sampler, diskPath}`, `RegisterRoutes` với `requireAuth("", h.handleStats)` (copy pattern `internal/http/video.go:54-58`), `handleStats` gọi `sampler.Snapshot(h.diskPath)` + `writeJSON`.
4. Wire `internal/gateway/server.go` + `cmd/gateway_http_wiring.go` (dùng `d.dataDir` — biến đã tồn tại trong wiring descriptor, cùng nguồn với `NewBrandingAssetsHandler(d.dataDir)` ngay trên :147).
5. Test `internal/sysstats/sampler_test.go`: Snapshot trả % trong [0,100], cores > 0, total > 0; gọi 2 lần liên tiếp không panic (delta state); disk path không tồn tại → disk omitted, không error.
6. Chạy: `go build ./... && go build -tags sqliteonly ./... && go vet ./internal/sysstats/ ./internal/http/ && go test ./internal/sysstats/`.

## Success Criteria

- [x] `curl -H "Authorization: Bearer $TOKEN" http://localhost:18790/v1/system/stats` (trên server) trả 200 + JSON đúng shape; không token → 401.
- [x] CPU percent > 0 ở request thứ 2 trở đi.
- [x] Cả 2 build tags pass; `go test ./internal/sysstats/` xanh.

## Risk Assessment

- **gopsutil CGO?** Không — pure Go, không ảnh hưởng cross-compile `GOOS=linux`. Signal nếu vỡ: lỗi build `go build ./...` → fallback đọc `/proc` trực tiếp (Linux-only) + build tag windows shell-out, nhưng chỉ làm khi thật sự vỡ.
- **Disk path sai** (dataDir trống ở Lite): `disk.Usage("")` lỗi → omit; UI phase 3 đã xử lý missing disk.
