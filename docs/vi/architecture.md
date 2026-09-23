# Kiến trúc

## Bức tranh tổng thể

GoClaw là một AI agent gateway multi-tenant được đóng gói trong **một binary
Go duy nhất** bao gồm:

- một **gateway WebSocket RPC + HTTP API** (frame: `req` / `res` / `event`),
- **agent runtime** — pipeline 8 giai đoạn, tool, memory, lập lịch,
- bộ kết nối kênh (Telegram, Discord, WhatsApp, Feishu/Lark, Zalo, ...),
- **web dashboard nhúng** (React SPA được chính binary đó phục vụ).

Triển khai chuẩn lưu mọi thứ trong **PostgreSQL 18 với pgvector** (SQL thuần
qua `database/sql` + pgx/v5, không ORM). Bản desktop biên dịch cùng runtime
đó với build tag `sqliteonly` trên một store SQLite nhúng — xem
[Desktop](../desktop).

| Tầng | Công nghệ |
|------|-----------|
| Ngôn ngữ | Go 1.26, Cobra CLI |
| Realtime | gorilla/websocket |
| Database | PostgreSQL 18 + pgvector; SQLite (`modernc.org/sqlite`) cho desktop |
| Web UI | React 19, Vite 6, TypeScript, Tailwind CSS 4, Radix UI, Zustand |
| Ứng dụng desktop | Wails v2 (`//go:build sqliteonly`) |
| Migration | golang-migrate (PG), schema tăng tiến nhúng sẵn (SQLite) |

## Pipeline 8 giai đoạn

Mỗi lượt chạy agent đi qua một pipeline gồm các giai đoạn pluggable, xây
trên interface `Stage` nhỏ gọn (`Execute(ctx, *RunState) error`, kèm
`Name()`). Pipeline có ba pha (`internal/pipeline/pipeline.go`):

```
Setup      [context]               runs once
Iteration  [prune, think, continuation gate, tools, observe, checkpoint]   runs per turn
Finalize   [finalize]              runs once after the loop
```

- **context** (setup) — phân giải workspace, nạp context file, dựng danh sách
  tool đã lọc và system prompt.
- **prune** — nén/cắt bớt history để vừa ngân sách token trước mỗi lượt.
- **think** — gọi LLM; có thể cho ra câu trả lời cuối cùng hoặc các lời gọi
  tool.
- **continuation gate** — chặn các model yếu kết thúc lượt chạy sớm (replies
  rỗng hoặc bị cắt sẽ nhận một lời nhắc có giới hạn thay vì dừng hẳn).
- **tools** — điều phối các lời gọi tool của lượt.
- **observe** — đưa kết quả tool trở lại vào hội thoại.
- **checkpoint** — đẩy các message pending vào session store mỗi vòng lặp và
  ghi checkpoint bền vững theo chu kỳ.
- **finalize** — kết thúc lượt chạy (hook tóm tắt chạy ở đây).

**Lượt chạy có thể tiếp tục:** vì trạng thái được checkpoint vào session
store, gateway bị lỗi hoặc khởi động lại sẽ tiếp tục lượt chạy từ vòng lặp đã
checkpoint thay vì bắt đầu lại từ đầu.

## Bộ nhớ 3 tầng

Bộ nhớ được xếp tầng với cơ chế nạp tăng tiến (L0 được tự động chèn vào
prompt, L1/L2 nạp theo yêu cầu):

| Tầng | Tên | Nội dung |
|------|-----|----------|
| L0 | Bộ nhớ làm việc | Tóm tắt ngắn gọn các session trước, tự chèn vào prompt trong ngân sách ~200 token |
| L1 | Bộ nhớ sự kiện (episodic) | Tóm tắt từng session, được tạo khi session kết thúc |
| L2 | Bộ nhớ ngữ nghĩa (semantic) | Thực thể và quan hệ của đồ thị tri thức |

Quá trình hợp nhất (consolidation) là **event-driven**: domain event chảy qua
**DomainEventBus** (`internal/eventbus` — event có kiểu, worker pool, dedup,
retry) vào các consolidation worker (`internal/consolidation`): tóm tắt
episodic, trích xuất đồ thị tri thức ngữ nghĩa, và một worker **Dreaming**
thăng cấp/chưng cất nội dung episodic trong nền.

## Tầng tri thức

- **Đồ thị tri thức** (`internal/knowledgegraph`) — thực thể và quan hệ do
  LLM trích xuất từ hội thoại, lưu trong PostgreSQL (pgvector cho tra cứu ngữ
  nghĩa) và có thể duyệt lúc truy vấn.
- **Knowledge Vault** (`internal/vault`) — sổ đăng ký tài liệu với
  `[[wikilinks]]` giữa các tài liệu, **tìm kiếm hybrid** kết hợp điểm
  full-text và vector, và đồng bộ filesystem để có thể sửa tài liệu Vault
  ngay trên đĩa.

## Điều phối (orchestration)

Các agent phối hợp với nhau qua một số cơ chế:

- **Subagent** — một agent sinh ra các task con (tool `spawn`, được theo dõi
  trong `subagent_tasks`) chạy trên pipeline riêng với tham số model kế thừa.
- **Delegation (ủy quyền)** — tool `delegate` giao việc cho agent khác qua
  các cạnh quyền **`agent_links`**, đồng bộ hoặc bất đồng bộ, kết quả trả về
  dạng artifact.
- **Team** — các agent được nhóm dưới một team lead với bảng dùng chung và
  vai trò thành viên (bản Standard; bản Lite giới hạn các thao tác team).
- **Jury / negotiate** — các tool ra quyết định đa agent: nhiều agent cùng
  góp ý và một phán quyết được tổng hợp (`internal/tools/jury_tool.go`,
  `internal/tools/negotiate_tool.go`, `internal/orchestration`).

## Tự tiến hóa

Vòng lặp tự tiến hóa chạy qua ba giai đoạn tăng tiến
(`internal/agent/suggestion_engine.go`, `evolution_guardrails.go`):

1. **Thu thập metrics** — metrics tool theo từng agent được tổng hợp trong
   một cửa sổ trượt 7 ngày.
2. **Phân tích gợi ý** — các quy tắc trên metrics đó sinh ra gợi ý khả thi
   (ví dụ tool lỗi lặp lại, điều chỉnh prompt).
3. **Áp dụng/hoàn tác có guardrail bảo vệ** — gợi ý được áp dụng sau
   guardrail và có thể hoàn tác; không gì thay đổi agent một cách âm thầm.

## Scheduler

Độ tương được tổ chức thành **bốn lane** (`internal/scheduler/lanes.go`) để
công việc nền không bao giờ làm đói chat tương tác:

| Lane | Độ tương mặc định | Ghi đè qua env |
|------|-------------------|----------------|
| `main` | 30 | `GOCLAW_LANE_MAIN` |
| `subagent` | 50 | `GOCLAW_LANE_SUBAGENT` |
| `team` | 100 | `GOCLAW_LANE_TEAM` |
| `cron` | 30 | `GOCLAW_LANE_CRON` |

Session xếp hàng theo session key trong lane của mình, giữ đúng thứ tự từng
session.

## Các bản edition

`internal/edition` phân tách tính năng giữa hai preset:

- **Standard** — server PostgreSQL, đầy đủ tính năng: đồ thị tri thức, RBAC,
  multi-tenancy, kênh, tìm kiếm vector, trình cài phụ thuộc.
- **Lite** — preset desktop: 5 agent, 1 team / 5 thành viên, chỉ tìm kiếm
  FTS, không đồ thị tri thức, không RBAC. Danh sách đầy đủ trong
  [Desktop](../desktop).

`GET /v1/edition` (không cần xác thực) báo bản edition đang hoạt động để UI
thích ứng.

## Các yếu tố xuyên suốt

- **Tầng store** — các store dựa trên interface (`store.SessionStore`,
  `store.AgentStore`, ...) được cài đặt hai lần (PostgreSQL và SQLite) sau
  một pattern Dialect dùng chung. Context tenant/user/agent/locale được
  truyền tường minh qua các context helper (`store.WithTenantID(ctx)`,
  `store.WithUserID(ctx)`, ...); vai trò admin không bao giờ được coi là một
  kiểm tra tenant đủ đứng riêng.
- **Giao thức WebSocket** — request đầu tiên trên một kết nối phải là
  `connect` (auth + locale); sau đó các frame `req` gọi RPC method và server
  đẩy các frame `event` (delta streaming, event vòng đời). Mọi tham số là
  camelCase, phản chiếu tag `json` của Go. Xem [HTTP API](../api/http) cho
  bề mặt REST.
- **Provider** — Anthropic, tương thích OpenAI, DashScope, Vertex AI, Codex
  CLI và nhiều hơn nữa, nằm sau một interface adapter duy nhất với model
  registry tương thích tiến; API key được mã hóa AES-256-GCM trong bảng
  `llm_providers`.
- **Bảo mật** — rate limiting, input guard chỉ phát hiện, CORS, shell deny
  pattern, chống SSRF, ngăn path traversal, và Docker sandbox cho code không
  tin cậy. Event bảo mật log dưới dạng `slog.Warn("security.*")`.
- **Bản địa hóa** — catalog backend với `i18n.T(locale, key, args...)`
  (en/vi/zh); web UI có sẵn en, vi, zh, ko, ru. Locale được truyền qua tham
  số `connect` của WS hoặc header HTTP `Accept-Language`.

## Bước tiếp theo

- [Agents](../features/agents) — loại agent, context file, subagent
- [Skills](../features/skills) — nạp SKILL.md và chợ skill
- [Kênh Telegram](../channels/telegram) — kênh hoàn thiện nhất
- [Xử lý sự cố](../troubleshooting) — chẩn đoán vấn đề routing và reasoning
