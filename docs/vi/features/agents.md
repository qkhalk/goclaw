# Agent

Agent là đơn vị thực thi cốt lõi của GoClaw: một trợ lý vận hành bằng LLM với
danh tính, provider, model, chế độ prompt, context file, tool và ngân sách
riêng. Agent trò chuyện qua [WebSocket RPC](../api/websocket),
[HTTP](../api/http) và [các kênh nhắn tin](../channels/telegram), đồng thời có
thể giao việc cho các agent khác — xem [Điều phối & Teams](./orchestration).

## Các loại agent

| Loại | Mô hình ngữ cảnh | Phù hợp nhất cho |
|------|---------------|----------|
| `open` | Ngữ cảnh riêng tư theo từng user (7 context file được seed cho mỗi user ở lần chat đầu) | Trợ lý cá nhân — mỗi user trò chuyện với bản thể "của riêng họ" |
| `predefined` | Ngữ cảnh dùng chung cấp agent cộng với một lớp `USER.md` mỏng theo từng user | Agent xây dựng theo mục đích riêng (nghiên cứu, thiết kế) dùng chung cho nhiều user, có vòng đời summon và bản thể theo từng user |

Context file nằm trong hai bảng được định tuyến bởi một
`ContextFileInterceptor`: `agent_context_files` (dùng chung, cấp agent) và
`user_context_files` (theo từng user). Các bootstrap template được seed tự động
theo loại agent và, với agent `open`, theo từng user.

### Danh tính kép

Mỗi agent có hai định danh:

- **UUID** — dùng cho quan hệ cơ sở dữ liệu, khóa ngoại và event.
- **agent_key** — slug dễ đọc dùng cho log, đường dẫn workspace, session key
  và UI.

Tham số WebSocket chấp nhận cả hai dạng; các API trả về tham chiếu agent đều
bao gồm cả hai. Xem [Agent identity conventions](https://github.com/qkhalk/goclaw/blob/main/docs/agent-identity-conventions.md)
trong repo để biết quy tắc đầy đủ.

## Cấu hình theo từng agent

Mỗi agent mang cấu hình runtime riêng, chỉnh sửa được trên trang chi tiết
agent trong web UI hoặc qua `agents.update` / `PUT /v1/agents/{id}`:

| Trường | Mục đích |
|-------|---------|
| `provider` / `model` | Định tuyến LLM mặc định (từ các provider đã đăng ký trong `llm_providers`) |
| `context_window` | Ghi đè context window cho model |
| `max_tool_iterations` | Trần số vòng think→act mỗi lần chạy |
| `workspace` + `restrict_to_workspace` | Thư mục làm việc và giới hạn filesystem |
| `budget_monthly_cents` | Ngân sách token/chi phí hàng tháng |
| `temperature`, `thinking_level` | Sampling và mức suy luận cơ bản |
| `tools_config` | Chính sách tool — bật/tắt từng tool, allow/deny theo agent |
| `sandbox_config` | Docker sandbox cho `exec` (xem [Tools](./tools#sandbox)) |
| `subagents_config` | Spawn template và giới hạn subagent (xem [Điều phối](./orchestration)) |
| `memory_config` | Tự động inject bộ nhớ, TTL episodic, Dreaming (xem [Bộ nhớ & Tri thức](./memory)) |
| `compaction_config` | Hành vi nén lịch sử session |
| `context_pruning` | Chiến lược cắt tỉa ngữ cảnh |
| `reasoning_config` | Mức suy luận, bao gồm adaptive effort |
| `model_fallback` | Chuỗi fallback khi model chính gặp lỗi |
| `shell_deny_groups` | Các mẫu deny lệnh shell bổ sung |
| `kg_dedup_config` | Tinh chỉnh dedup đồ thị tri thức |
| `self_evolve`, `skill_evolve` | Cờ tự tiến hóa (xem [Điều phối](./orchestration#self-evolution)) |

## Các chế độ prompt

Mỗi agent có một chế độ prompt kiểm soát lượng system prompt được lắp ghép mỗi
lần chạy:

| Chế độ | Nội dung |
|------|---------|
| `full` | Tất cả các phần — agent trò chuyện chính |
| `task` | Gọn nhưng đủ mạnh — các lần chạy tự động |
| `minimal` | Giảm bớt các phần — kiểm tra định kỳ |
| `none` | Chỉ dòng danh tính |

Chế độ hiệu lực được xử lý theo từng lần chạy với thứ tự ưu tiên
**runtime override > auto-detect > cấu hình agent > mặc định (`full`)**.
Auto-detect giới hạn các lần chạy headless: session heartbeat chạy tối đa
`minimal`, session subagent và cron tối đa `task`. Cấu hình agent chặt chẽ hơn
luôn thắng chế độ auto-detect lỏng hơn.

## Context file

Hành vi agent được định hình bởi các context file markdown được seed từ
`internal/bootstrap/templates/`:

- `AGENTS.md` — hướng dẫn vận hành và môi trường
- `IDENTITY.md` — tên, tính cách, emoji
- `SOUL.md` — giá trị, giọng điệu và mô hình bản thân tiến hóa
- `TOOLS.md` — ghi chú sử dụng tool
- `USER.md` — hồ sơ theo từng user (lớp theo user cho cả hai loại agent)
- `BOOTSTRAP.md` — checklist onboarding lần đầu

Tất cả file đều chỉnh được từ web UI: **Agents → chi tiết agent → Files**
(hỗ trợ bởi các WebSocket method `agents.files.list` / `agents.files.get` /
`agents.files.set`).

## Heartbeat

Agent có thể chạy các lần kiểm tra định kỳ dựa trên file checklist
`HEARTBEAT.md`:

- Trình lập lịch heartbeat kích hoạt theo khoảng thời gian đã cấu hình, tùy
  chọn giới hạn trong **khung giờ hoạt động** (giờ bắt đầu/kết thúc
  `active_hours` theo từng heartbeat).
- Lần chạy thực thi trên lane riêng của cron scheduler, nên các lần kiểm tra
  nền không bao giờ tranh chấp với chat tương tác.
- Nếu không có gì cần chú ý, agent trả lời chứa `HEARTBEAT_OK` và việc gửi bị
  chặn lại — không tin nhắn nào được gửi tới user.

## Quản lý agent

Các bề mặt quản lý vòng đời agent:

- **WebSocket** — `agents.list`, `agents.create`, `agents.update`,
  `agents.delete`, cộng với `agents.files.*` cho context file và
  `agents.links.*` cho các cạnh ủy quyền (delegation).
- **HTTP** — `GET/POST /v1/agents`, `PUT/DELETE /v1/agents/{id}`,
  `POST /v1/agents/{id}/resummon`, `GET /v1/agents/{id}/instances` và nhiều
  hơn nữa.
- **Web UI** — trang Agents bao gồm tạo agent, cài đặt theo agent, file,
  định nghĩa subagent và tab Evolution.
- **Import/export** — kho lưu trữ agent đầy đủ:
  `GET /v1/agents/{id}/export` (với `/export/preview` và download token)
  và `POST /v1/agents/import` / `POST /v1/agents/{id}/import` để khôi phục
  hoặc gộp. Import yêu cầu vai trò admin.

::: tip
Agent `predefined` có vòng đời summon: một summoner khởi tạo ngữ cảnh của
agent từ mô tả, và các bản thể theo từng user có thể được kiểm tra và triệu
hồi lại (`/resummon`) khi định nghĩa dùng chung thay đổi.
:::

## Bước tiếp theo

- Spawn subagent, ủy quyền công việc và lập team: [Điều phối & Teams](./orchestration)
- Cấp độ bộ nhớ, đồ thị tri thức và vault: [Bộ nhớ & Tri thức](./memory)
- Cấp khả năng: [Skills & Chợ skill](./skills)
- Tool tích hợp sẵn, sandbox, browser và MCP: [Tools, Browser & MCP](./tools)
