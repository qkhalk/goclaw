# WebSocket RPC

API chính của gateway là JSON RPC over WebSocket tại `ws://host:18790/ws`
(cổng gateway, mặc định `18790`). Dashboard HTTP và CLI đều nói giao thức
này. Các wire type nằm trong `pkg/protocol` của repo GoClaw.

## Frame

Mỗi tin nhắn là một frame JSON thuộc một trong ba kiểu:

| Kiểu | Chiều | Mục đích |
|------|-----------|---------|
| `req` | client → server | Gọi một method; mang `id`, `method`, `params` |
| `res` | server → client | Trả lời một `req` theo `id` (thành công hoặc lỗi) |
| `event` | server → client | Push chủ động (streaming delta, vòng đời, presence) |

**Request đầu tiên trên một kết nối phải là `connect`** — bất kỳ method nào
khác bị từ chối cho đến khi handshake hoàn tất.

```json
{"type":"req","id":"1","method":"connect","params":{"token":"...","locale":"en"}}
```

## Xác thực connect

`connect` hỗ trợ nhiều đường xác thực (đánh giá theo thứ tự):

| Đường | Credentials | Kết quả |
|------|-------------|--------|
| Gateway token | `token` khớp gateway token đã cấu hình (so sánh constant-time) | Vai trò owner/admin; owner có thể thu hẹp phạm vi bằng `tenant_id` |
| API key | `token` là API key hợp lệ có tiền tố `goclaw_` | Vai trò suy ra từ các scope của key; tenant lấy từ key |
| Không cấu hình token | gateway chạy không có token (cho phép fallback tường minh) | Vai trò operator, tenant master |
| Trình duyệt đã pair kết nối lại | `sender_id` của một lần pair đã phê duyệt trước đó | Vai trò operator từ tư cách thành viên tenant |
| Luồng mã pairing | không token, không pairing | Trả về `pending_pairing` với `pairing_code` 8 ký tự; mã được phê duyệt ngoài băng bằng `goclaw pairing approve` (cũng có `list`, `revoke`) |

Mọi trường hợp khác bị từ chối (fail-closed).

`connect` cũng nhận một tham số `locale` (en, vi, zh) tồn tại suốt kết nối và
bản địa hóa lỗi cùng prompt.

## Các nhóm method

Khoảng 200 method được đăng ký. `pkg/protocol/methods.go` là danh sách chính
thức. Điểm nổi bật:

| Nhóm | Method |
|--------|---------|
| Chat | `chat.send`, `chat.history`, `chat.abort`, `chat.inject` — với event streaming (chunk, gọi/kết quả tool, vòng đời run) |
| Agent | `agents.list/create/update/delete`, `agents.files.*` |
| Session | `sessions.list/preview/patch/delete/reset/compact/branch/archive/restore` |
| Liên kết agent | `agents.links.list/create/update/delete` (đồ thị ủy quyền) |
| Kênh | `channels.instances.*`, `channels.list/status/toggle` |
| Bộ nhớ | `memory.write/get/search/supersede/archive` |
| Teams | `teams.*` bao gồm `teams.tasks.*` (create/assign/approve/reject/comments/events) |
| Nhiệm vụ | `tasks.tree/create/updateStatus` |
| Cron | `cron.list/create/update/delete/toggle/status/run/runs` |
| Subagent | `subagents.list/get/archive/archive_completed/cancel` |
| Skills | `skills.list/get/update/approve/reject` |
| Hooks | `hooks.list/create/update/delete/toggle/test/history` |
| Cấu hình | `config.get/apply/patch/schema/defaults` |
| Runs | `runs.get/list/events/resume`, `runs.checkpoints.list`, `runs.replay` |
| Browser | `browser.act/snapshot/screenshot` (+ biến thể panel/remote) |
| Heartbeat | `heartbeat.get/set/toggle/test/logs`, `heartbeat.checklist.*`, `heartbeat.targets` |
| Usage | `usage.get`, `usage.summary` |
| Log | `logs.tail` |
| Node & thiết bị | `nodes.*`, `device.pair.*`, `node.hello/heartbeat/bye` |
| Workstation | `workstations.*` bao gồm permissions và activity |
| Multi-agent | `multiagent.formation`, `multiagent.jury`, `multiagent.negotiate` |
| Terminal | `terminal.create/list/attach/input/resize/close` — web PTY |
| Backup | `backup.schedule.get/set/run` |

## Event

Server push các frame `event` từ nhiều nhóm (`pkg/protocol/events.go`):

- **Vòng đời agent run** — `run.started` / `run.completed` / `run.failed` /
  `run.cancelled`, `tool.call` / `tool.result`, `llm.started` /
  `llm.completed`, cộng với streaming delta `chunk` / `thinking` của chat
- **Presence & sức khỏe** — `presence`, `health`, `heartbeat`
- **Ủy quyền** — `delegation.started/progress/completed/failed/...`
- **Nhiệm vụ team** — `team.task.created/claimed/completed/commented/...`,
  `team.message.sent`
- **Kênh & pairing** — `whatsapp.qr.code/done`,
  `zalo.personal.qr.code/done`, `device.pair.requested/resolved`,
  `node.pair.requested/resolved`
- **Session** — `session.updated`
- **Phê duyệt** — `exec.approval.requested/resolved`

## Quy ước

- **Param viết camelCase** — khớp tag `json:"..."` của struct Go
  (`teamId`, `taskId`, `sessionKey`)
- **Giới hạn tốc độ** — trần RPM tùy chọn theo từng kết nối
  (`gateway.rate_limit_rpm` trong config; tắt theo mặc định)
- **RBAC** — mọi method được kiểm tra vai trò (`admin` / `operator` /
  `viewer`); method chưa phân loại fail closed
- Response tham chiếu `id` của request; lỗi mang một mã lỗi giao thức và một
  thông điệp đã bản địa hóa

## Ví dụ

```json
{"type":"req","id":"2","method":"chat.send","params":{"agentId":"...","message":"hi"}}
{"type":"event","event":"chunk","payload":{"...":"streamed delta"}}
{"type":"res","id":"2","payload":{"sessionKey":"..."}}
```
