---
phase: 3
title: "Rich /status + verbosity + docs page"
status: completed
priority: P1
effort: "1d"
dependencies: ["phase-1", "phase-2"]
---

# Phase 3: Rich /status + verbosity + docs page

## Overview

Thay `/status` stub 3 dòng hardcode (`commands.go:200-205`) bằng status đầy đủ kiểu OpenClaw nhưng 100% từ dữ liệu GoClaw thật: version, uptime gateway + system, agent + model, session + updated, cost + tokens, context usage + compactions, think + mode + queue, link docs nội bộ. Kèm `/status full|short` toggle verbosity per-chat (DM default full, group default short). Tạo docs page `docs/25-telegram-runtime-commands.md` làm đích của link.

## Requirements

- Functional:
  - `/status` → output theo verbosity mặc định của chat (DM: full; group: short).
  - `/status full` / `/status short` → đổi default per-chat (persist `tg_status_verbosity` metadata — pattern Phase 1) + render ngay theo mức mới.
  - Full output (mỗi dòng + nguồn):
    - `🦊 GoClaw <version>` — `cmd.Version` inject qua provider.
    - `⏱️ Uptime: gateway <Xd Yh> · system <Xd Yh>` — `Server.StartedAt()` (`server.go:1030`); system từ `/proc/uptime` (Linux-only, không có thì bỏ phần system).
    - `🤖 Agent: <agentKey> · 🧠 Model: <provider>/<model>` — agentKey channel đã biết; model/provider từ SessionData (`Model`, `Provider`) hoặc agentStore fallback khi session chưa có.
    - `🧵 Session: <sessionKey rút gọn> · updated <humanized>` — SessionData.Updated + sessionKey (channel tự tính — helper Phase 1).
    - `💵 Cost (session): $<sum> · 🔢 Tokens: <in>/<out>` — cost: TỔNG `traces.total_cost` theo session_key (method mới, xem Architecture); tokens: `sessions.input_tokens/output_tokens` (`SessionData`).
    - `📚 Context: <last_prompt_tokens|?>/<context_window> (<%>) · 🧹 Compactions: <n>` — `SessionData.LastPromptTokens` (`session_store.go:52-54`), `ContextWindow`, `CompactionCount`.
    - `⚙️ Think: <hiện trạng> · Mode: <dev|normal> · 🪢 Queue: <lane> <active>/<concurrency> active, <pending> pending` — think/mode từ metadata (Phase 1+2) + agent default; queue từ `Scheduler.LaneStats()` (lane "main").
    - `📖 Docs: https://github.com/qkhalk/goclaw/blob/dev/docs/25-telegram-runtime-commands.md`
  - Short output: 4 dòng đầu (version, uptime, agent+model, session updated).
  - Session chưa tồn tại → các dòng session/cost/context thay bằng "No session yet".
  - HTML-escape toàn bộ giá trị động (sessionKey chứa `:` an toàn nhưng agent name/model do user đặt); chunk qua `sendHTML` (`send.go:424`).
- Non-functional: handler < 1s (0 LLM call, chỉ store/index reads); không migration (traces column đã có); không thêm config file field.

## Architecture

**StatusProvider** — channel không biết đến gateway/scheduler/traces; inject 1 interface (file `commands_status.go`):

```go
type StatusProvider interface {
    Gateway(ctx context.Context) StatusGatewayInfo   // Version, StartedAt, SystemUptime, MainLane LaneStat
    Session(ctx context.Context, sessionKey string) (StatusSessionInfo, bool)
    SessionCost(ctx context.Context, sessionKey string) (float64, bool)
}
// StatusSessionInfo: Model, Provider, Updated, InputTokens, OutputTokens,
//                    LastPromptTokens, ContextWindow, CompactionCount
```

Concrete impl `cmd/gateway_status_provider.go` (package cmd) capture: `Version` (biến package), `server *gateway.Server` (tạo `cmd/gateway.go:646`), `sched *scheduler.Scheduler` (`cmd/gateway.go:1175`), session store, trace store. Wire qua Option `WithStatusProvider` trong `FactoryWithStoresAndAudio` (2 caller như Phase 1).

**Session cost** — method mới trên `store.TracingStore` (interface TỒN TẠI sẵn — audit verify: `internal/store/tracing_store.go:164-196`, đã có các method cost-aggregation `GetMonthlyAgentCost`/`GetCostSummary`; impls `pg/tracing.go` + `sqlitestore/tracing.go`):
- `SessionTotalCost(ctx, sessionKey string) (float64, bool)` — `SELECT COALESCE(SUM(total_cost),0), COUNT(*) FROM traces WHERE session_key=$1` (dùng index `idx_traces_session` `migrations/000001:364`).
- Thêm method vào interface + CẢ HAI impl (PG + SQLite — luật dual-DB). Runtime truy cập traces qua `tracing.Collector` (tạo `cmd/gateway.go:371`); provider capture store TracingStore trực tiếp từ setup stores cùng chỗ.

**Wiring (R8 — 2 đường như Phase 1):** Option `WithStatusProvider` gắn cả `FactoryWithStoresAndAudio` (`factory.go:62-77` + caller `cmd/gateway.go:1049`) lẫn direct `telegram.New(...)` (`cmd/gateway_channels_setup.go:52`).

**Verbosity** — lưu `tg_status_verbosity` = "full"|"short" qua SessionPrefsStore (Phase 1); default: DM full, group short (chatType có trong handler qua `message.Chat.Type` — handleBotCommand đã có ngữ cảnh; verify biến chatType truyền được vào handler theo pattern `setThread`).

**Docs page** — `docs/25-telegram-runtime-commands.md`: mục lệnh (/status, /thinking, /dev + bảng verbosity/thinking levels hợp lệ), bảng nguồn dữ liệu từng dòng status, hướng dẫn menu_skills, ghi chú authorization-gate cho skills kiểm thử. Số 25 kế tiếp series 00–24 (verify lúc tạo không trùng). Link trong status dùng URL GitHub repo `qkhalk/goclaw` branch `dev` (repo thật của anh — fork upstream nextlevelbuilder).

**Thay thế openclaw link:** grep toàn repo xác nhận KHÔNG có link `docs.openclaw.ai` nào (scout đã khẳng định 0 occurrence) — không cần xóa gì; link docs mới là của riêng plan này. Ghi kết quả grep vào Verify Log dưới.

## Related Code Files

- Create: `internal/channels/telegram/commands_status.go` (interface StatusProvider, WithStatusProvider, handleStatusCommand, renderStatusFull/Short, humanizeUptime)
- Create: `internal/channels/telegram/commands_status_test.go`
- Create: `cmd/gateway_status_provider.go` (+ test nhỏ nếu dễ fake; tối thiểu build)
- Create: `docs/25-telegram-runtime-commands.md`
- Modify: `internal/channels/telegram/commands.go` (case `/status` `:200-205` → gọi handler; `/help`)
- Modify: `internal/store/tracing_store.go` (+`SessionTotalCost` vào `TracingStore` :164-196) + `internal/store/pg/tracing.go` + `internal/store/sqlitestore/tracing.go` (impls)
- Modify: `internal/channels/telegram/factory.go` + `cmd/gateway.go:1049` + `cmd/gateway_channels_setup.go:52` (wire provider cả 2 đường)
- Modify: `docs/15-core-skills-system.md` — KHÔNG đụng ở phase này (Phase 4)

## Implementation Steps

1. Trace store `SessionTotalCost` (PG + SQLite + interface) + unit/integration test (seed 2 traces cùng session_key + 1 trace khác key → SUM đúng).
2. `commands_status.go`: interface + Option + handler (đọc verbosity metadata → render mức tương ứng; args full/short → persist + render) + humanize helpers.
3. Provider concrete trong cmd/ + wire 2 caller.
4. Case `/status` đổi sang handler; `/help` cập nhật mô tả.
5. Docs page `docs/25-telegram-runtime-commands.md` + URL khớp literal trong render (test assert URL đúng path).
6. Tests: render full (đủ 8 dòng, escape), render short (4 dòng), no-session, verbosity persist, group default short, LaneStat format, /proc/uptime parse (fake file content qua hàm thuần `parseProcUptime(string) (time.Duration, bool)` — không đọc file thật trong test).
7. Build/vet/test chuẩn + `go test ./internal/store/...` cho method cost mới.

## Success Criteria

- [x] DM gõ `/status` thấy full 8 dòng; group thấy short; `/status full` ở group persist (tin sau vẫn full).
- [x] Cost khớp SUM traces của session đó (verify tay SQL trên DB dev: `SELECT SUM(total_cost) FROM traces WHERE session_key='<key>'`).
- [x] Context % đúng toán (LastPromptTokens có thể 0 lúc đầu → hiển thị `?/200k`).
- [x] Gateway restart giữa phiên: verbosity + các toggle Phase 1-2 còn nguyên.
- [x] Docs page tồn tại ở URL literal; CI docs link check nếu có (không thì bỏ qua).
- [x] Build/vet/test sạch 2 mode; không migration.

## Risk Assessment

- **Method cost mới chậm với session nhiều traces:** SUM quét index `idx_traces_session` theo key — số traces/session hàng trăm là thường; nếu sau này chậm thì thêm cột tổng hợp trên sessions (out of scope, ghi docs). Tín hiệu vỡ: /status > 1s trên session lâu năm → phản ứng: cache kết quả 30s trong provider.
- **/proc/uptime không đọc được (non-Linux dev machine trên Windows)** — hàm parse thuần + test fake; runtime omit dòng system khi lỗi. Trên server Linux sẽ có.
- **SessionData.LastPromptTokens semantics** — nó là "calibrated current usage" (`session_store.go:52-54`, PR gần đây) — nếu chưa từng chạy run thì 0 → hiển thị `?`. Không over-promise.
- **Cost NUMERIC precision** — SUM trả decimal; format `$%.4f` đủ dùng (giá trị GoClaw hiện nhỏ); không làm rounding tài chính.
- **StatusProvider test trong cmd/** — package cmd test nặng; giữ provider mỏng (chỉ closure + struct assembly), logic render + format nằm hết trong telegram package (test được bằng fake provider).
