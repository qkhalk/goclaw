# Tools, Browser & MCP

Tool là đôi tay của agent: các tool tích hợp sẵn bao phủ filesystem, shell,
web, bộ nhớ, nhắn tin và cloud, được mở rộng bởi Tool Store, thực thi trong
Docker sandbox, tự động hóa browser phía server, MCP server bên ngoài và truy
cập CLI có credentials.

## Danh mục tool tích hợp sẵn

Danh mục được seed vào cơ sở dữ liệu và quản lý trong web UI
(**/builtin-tools**: bật/tắt theo tool, cấu hình theo tenant qua
`/v1/tools/builtin/{name}/tenant-config`).

| Danh mục | Tool |
|----------|-------|
| Filesystem | `read_file`, `write_file`, `edit`, `list_files` |
| Runtime | `exec` (yêu cầu phê duyệt), `wait`, `datetime`, `workstation_exec`, `claude_remote` |
| Web | `web_search`, `web_fetch` |
| Bộ nhớ | `memory_search`, `memory_get`, `memory_expand`, `knowledge_graph_search` |
| Vault | `vault_search`, `vault_read` |
| Media | `read_image`, `read_document`, `create_image`, `read_audio`, `read_video`, `create_video`, `create_audio`, `tts`, `stt` (phụ thuộc capability) |
| Browser | `browser` — tự động hóa phía server qua go-rod |
| Session | `sessions_list`, `session_status`, `sessions_history`, `sessions_send`, `plan` |
| Nhắn tin | `message`, `ask_options`, `send_file`, `create_forum_topic`, `list_group_members`, `zalo_list_groups` |
| Cloud | `cloud_accounts`, `mail_search`, `mail_read`, `mail_archive`, `mail_unsubscribe`, `cloud_ls`, `cloud_read`, `cloud_fetch`, `cloud_about`, `cloud_write`, `cloud_mkdir`, `cloud_copy`, `cloud_move`, `cloud_delete`, `cloud_share` |
| Định lịch | `cron`, `heartbeat` |
| Subagent | `spawn`, `delegate` (khả dụng tùy [chế độ điều phối](./orchestration#orchestration-modes)) |
| Skills | `skill_search`, `use_skill`, `publish_skill`, `skill_manage`, `mcp_tool_search` |
| Teams | `team_tasks` |
| Studio | `pptx_studio`, `video_studio`, `watermark_studio` — xem [Studio sáng tạo](./tools-studio) |

### Nạp trì hoãn

Các tool phổ biến nhất luôn nằm trong danh sách tool của model; các tool ít
phổ biến hơn bị **trì hoãn** và được khám phá theo nhu cầu qua meta-tool
`tool_search` dựa trên BM25, tool này nạp các tool khớp vào registry cho phần
còn lại của lần chạy. Điều này giữ kích thước prompt nhỏ gọn mà không hy sinh
phạm vi bao phủ.

### Chuỗi provider tìm kiếm web

`web_search` đi qua một chuỗi provider theo tenant — Brave, Tavily, Exa,
SearXNG, Parallel — với DuckDuckGo luôn ở cuối làm fallback miễn phí. Chuỗi
hoạt động theo kiểu ai thành công trước thắng; một lần gọi cũng có thể ép một
provider cụ thể để đối chiếu chéo giữa các engine.

## Chính sách tool và phê duyệt

- **Chính sách theo agent** (`tools_config`): bật/tắt từng tool; một
  `toolCallPrefix` ánh xạ kết quả model có tiền tố (ví dụ `proxy_exec`) về
  tên tool chuẩn.
- **Phê duyệt exec**: các lần chạy `exec` bị chặn bởi một chế độ bảo mật; khi
  một lệnh cần phê duyệt, nó vào hàng đợi mà admin xử lý bằng approve/deny.
  Grant có phạm vi (`once` theo mặc định, `session`, cho phép luôn luôn) và
  có thể kèm thời hạn. WS method: `exec.approval.list`, `.approve`, `.deny`,
  `.history`. UI: **/approvals**.

## Mở rộng bộ tool

Không có CRUD "custom tool" tại runtime — bộ tool được mở rộng qua các bề mặt
đã kiểm chứng thay vào đó:

- **Quản lý tool builtin**: bật/tắt bất kỳ tool builtin nào với cài đặt theo
  tool qua `GET/PUT /v1/tools/builtin/{name}`, toàn cục hoặc theo tenant
  (`tenant-config`). UI: **/builtin-tools**.
- **Gói Tool Store**: cài/gỡ các gói mang theo tool bổ sung
  (`POST /v1/packages/install`, `POST /v1/packages/uninstall`,
  `GET /v1/packages/runtimes`). UI: **/packages**.
- **Exec có credentials**: các CLI quen thuộc (`git`, `gh`, `psql`, `kubectl`,
  `aws`, `gcloud`, `terraform`) chạy qua credential adapter với credentials
  tạm thời và các mẫu deny regex chặn tham số nguy hiểm
  (`/v1/cli-credentials/*`).
- **Nhóm deny shell**: `shell_deny_groups` trên agent cộng với các mẫu deny
  toàn cục (`GET /v1/shell-deny-groups`) chặn các cấu trúc nguy hiểm trước
  khi `exec` chạy.
- **MCP server**: gắn MCP server bên ngoài để có thêm tool của chúng một cách
  động (xem [MCP bridge](#mcp-bridge) bên dưới).

## Sandbox

`exec` có thể chạy bên trong một Docker sandbox cấu hình theo agent
(`sandbox_config`):

| Trường | Giá trị (mặc định đứng trước) |
|-------|------------------------|
| `mode` | `off`, `non-main` (chỉ subagent/delegate), `all` |
| `image` | `goclaw-sandbox:bookworm-slim` |
| `workspace_access` | `rw`, `ro`, `none` |
| `scope` | `session`, `agent`, `shared` |
| `memory_mb` / `cpus` | 512 MB / 1.0 CPU |
| `network_enabled` | `false` theo mặc định |
| `read_only_root` | `true` theo mặc định |
| `setup_command` | Chạy một lần sau khi tạo container |

Container rảnh được dọn tự động (mặc định: rảnh 24 giờ, tuổi tối đa 7 ngày).

## Tự động hóa browser

Tool `browser` điều khiển một browser thật **phía server** qua
[go-rod](https://github.com/go-rod/rod) (`pkg/browser`) — điều hướng, bấm,
điền form và trích xuất, không cần browser cục bộ trên thiết bị client. Bề
mặt WebSocket: `browser.act`, `browser.snapshot`, `browser.screenshot` (cộng
với pairing, relay panel và session từ xa). Web chat có một panel browser
hiển thị snapshot trực tiếp được relay qua gateway.

## MCP bridge

Kết nối các [Model Context Protocol](https://modelcontextprotocol.io) server
bên ngoài qua `stdio`, `sse` hoặc `streamable-http`:

- Tool MCP đã đăng ký đi vào registry tool bình thường với tiền tố `mcp_` và
  đặt tên tùy chọn `{prefix}__{tool}` để tránh xung đột.
- **Grant theo agent và theo user** với danh sách allow/deny tool; user có
  thể yêu cầu quyền truy cập và admin xem xét
  (`POST /v1/mcp/requests/{id}/review`).
- `mcp_tool_search` tìm kiếm trên các server đã kết nối; quản lý đầy đủ dưới
  `/v1/mcp/*` và trang UI **/mcp**.
- GoClaw cũng có thể **để lộ chính tool của mình như một MCP server** cho
  bên ngoài tiêu dùng (bridge server `internal/mcp`), và
  `goclaw mcp new <name>` dựng khung một dự án tool-server mới.

## Exec có credentials

Với các workflow CLI đáng tin, credential adapter inject credentials tạm thời
vào các lần gọi `exec` để agent dùng tool đã xác thực mà không nhìn thấy
secret lâu dài:

- Preset: `git`, `psql`, `gh`, `kubectl`, `aws`, `gcloud`, `terraform`.
- Credentials có phạm vi theo user và theo agent, được xác thực với binary
  đích, và cấp tạm thời cho mỗi lần thực thi.
- Bề mặt HTTP: `/v1/cli-credentials` (preset, kiểm tra binary, credentials và
  grant user/agent theo tài khoản, kiểm tra kết nối).
- Web UI: tab **/packages → CLI credentials**.

## Bước tiếp theo

- Các trường chính sách tool theo agent: [Agent](./agents)
- Nơi các tool bộ nhớ truy vấn: [Bộ nhớ & Tri thức](./memory)
- Tool skill và mô hình truy cập của chúng: [Skills & Chợ skill](./skills)
