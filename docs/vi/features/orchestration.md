# Điều phối & Teams

Agent GoClaw không chạy một mình: chúng có thể spawn subagent, ủy quyền nhiệm
vụ cho các agent được liên kết và làm việc trong team với bảng nhiệm vụ dùng
chung. Công việc nền được xử lý bằng cron job, và agent có thể tự tiến hóa
qua một vòng lặp tự tiến hóa có rào chắn (guardrail).

## Chế độ điều phối

Mỗi agent được xử lý về một chế độ điều phối khi bắt đầu chạy, theo ưu tiên
**team > delegate > spawn**:

| Chế độ | Tool giữa các agent khả dụng |
|------|------------------------------|
| `spawn` | chỉ `spawn` (tự nhân bản) |
| `delegate` | `spawn` + `delegate` (các agent liên kết) |
| `team` | `spawn` + `delegate` + `team_tasks` (đầy đủ tool team) |

Cách xử lý: thành viên của bất kỳ team nào nhận `team`; nếu không, agent có
liên kết ủy quyền outbound nhận `delegate`; nếu không thì `spawn`. Tool nằm
ngoài chế độ bị ẩn khỏi model, không chỉ là khuyến khích tránh.

## Subagent

Tool `spawn` tạo các agent con chạy nhiệm vụ trong môi trường biệt lập rồi
báo cáo lại:

- **Spawn template** — các định nghĩa tái sử dụng có tên được lưu trên agent
  (`subagents_config.definitions`), mỗi cái có thể ghi đè model, prompt và
  allowlist. Không có template thì agent con là bản tự nhân bản mặc định từ
  cấu hình hiệu lực của cha.
- **Roster** — `spawn` với một hành động roster liệt kê các nhiệm vụ con đang
  chạy và đã xong; nhiệm vụ đang chạy có thể **hủy** hoặc **điều hướng
  (steer)**, nhiệm vụ đã xong được **lưu trữ** (một cái hoặc tất cả).
- **Announce** — subagent hoàn thành công bố kết quả trở lại session cha qua
  message bus.
- **Giới hạn edition** — Lite trần số subagent đồng thời ở 2 và độ sâu ở 1;
  giới hạn đồng thời mặc định của bản standard là 20 (`subagents_config`).

Định nghĩa subagent được quản lý trên trang chi tiết agent trong web UI và
qua WS (`subagents.list`, `subagents.get`, `subagents.cancel`,
`subagents.archive`, `subagents.archive_completed`).

## Ủy quyền giữa các agent

`agent_links` là các cạnh liên kết tường minh giữa các agent:

| Trường | Giá trị |
|-------|--------|
| `direction` | `outbound`, `inbound`, `bidirectional` |
| `description` | Văn bản tự do được inject vào prompt của caller (giao gì cho ai) |
| `max_concurrent` | Trần đồng thời cho các lần ủy quyền qua liên kết |
| `status` | `active`, `disabled` |
| `team_id` | Đặt khi liên kết được một team tự tạo |

Tool `delegate` trao một nhiệm vụ cho agent liên kết theo hai chế độ:

- **Sync** — chặn caller cho đến khi đích trả lời; timeout cấu hình được theo
  từng lần gọi (mặc định 300 giây, trần cứng 600 giây).
- **Async** — fire-and-forget; đích công bố kết quả trở lại qua message bus
  khi xong.

Ủy quyền trao đổi file qua một delegation workspace biệt lập; kết quả đầu ra
đã xác thực được công bố trở lại dưới thư mục `.delegations/<delegation-id>/`
của caller.

Quản lý liên kết: WS `agents.links.list` / `create` / `update` / `delete`.

## Teams

Teams nhóm các agent với ngữ cảnh dùng chung và tool phối hợp:

- **CRUD + thành viên** — WS `teams.create`, `teams.update`, `teams.delete`,
  `teams.get`, `teams.list`, `teams.members.add`, `teams.members.remove`,
  cộng với `teams.scopes` và `teams.known_users`.
- **Team workspace dùng chung** — file hiển thị với mọi thành viên
  (`teams.workspace.list` / `read` / `delete`).
- **Bảng nhiệm vụ** — tạo, giao, nhận (claim), cập nhật tiến độ, review,
  phê duyệt, từ chối và bình luận nhiệm vụ (`teams.tasks.*`), với event trực
  tiếp (`teams.tasks.events`, `teams.events.list`). Phân loại công việc team
  tự động định tuyến các session phạm vi team.
- **Lite gating** — `TeamActionPolicy` chặn các hành động phá hủy/điều phối
  (`comment`, `review`, `approve`, `reject`, `attach`, `ask_user`) trong bản
  Lite; agent được bảo chỉ chuyển tiếp blocker qua comment ở chế độ đầy đủ
  (full mode).

## Vòng multi-agent

Ngoài ủy quyền một-một, GoClaw hỗ trợ các vòng multi-agent có cấu trúc:

- **Team formation động** (`multiagent.formation`) — lập một hội đồng agent
  cho một nhiệm vụ.
- **Jury** (`multiagent.jury`) — nhiều agent trả lời độc lập và một phán quyết
  được tổng hợp; lịch sử phán quyết tra cứu được.
- **Đàm phán** (`multiagent.negotiate`) — các agent lặp dần đến đồng thuận với
  trạng thái đàm phán hiển thị.

Vòng thi hành được dẫn dắt bởi các tool jury/negotiate trong agent loop; các
RPC method bộc lộ lịch sử formation, phán quyết và trạng thái. Tổng hợp kết
quả dùng `BatchQueue[T]` trong `internal/orchestration`.

## Cron & nhiệm vụ

Công việc định lịch chạy qua cron job và cây nhiệm vụ:

- **Loại lịch** — `at` (mốc thời gian chạy một lần), `every` (khoảng cách cố
  định) và `cron` (biểu thức 5 trường [gronx](https://github.com/adhocore/gronx)).
- **Cô lập** — job thực thi qua runner `cronexec` (nhóm process riêng) và các
  lần chạy agent được xếp trên **cron lane** riêng của scheduler, tách khỏi
  lane main và subagent.
- **Retry** — lần thực thi thất bại được thử lại với backoff.
- **Cây nhiệm vụ** — nhiệm vụ cha/con nhẹ nhàng để tạo cấu trúc, và bảng nhiệm
  vụ team (ở trên) cho workflow đầy đủ.

Cron job được quản lý trong web UI (/cron) và qua WS (`cron.create`,
`cron.list`, `cron.get`, `cron.delete`, ...).

## Tự tiến hóa

Tự tiến hóa là vòng lặp metrics → đề xuất → áp dụng với bước admin xem xét ở
giai đoạn áp dụng:

1. **Thu thập metrics** — mỗi lần chạy ghi lại việc dùng tool và các metrics
   truy hồi theo từng agent.
2. **Phân tích đề xuất** — một engine định kỳ đánh giá tổng hợp 7 ngày với ba
   quy tắc:

   | Quy tắc | Điều kiện kích hoạt |
   |------|---------|
   | Mức dùng truy hồi thấp | Tỷ lệ sử dụng < 20% trên 50+ truy vấn với một nguồn |
   | Tool thất bại | Tỷ lệ thành công < 10% trên 20+ lần gọi với một tool |
   | Mẫu lặp lại | Một tool duy nhất > 100 lần gọi thành công/tuần |

3. **Xem xét** — đề xuất ở trạng thái `pending`; admin phê duyệt, từ chối hoặc
   hoàn tác. Rào chắn giới hạn những gì có thể áp dụng và yêu cầu dữ liệu
   metrics gần đây trước bất kỳ thay đổi nào.

Hai cờ theo từng agent mở rộng vòng lặp:

- `self_evolve` — agent có thể tự viết lại `SOUL.md` của mình trong phạm vi
  rào chắn.
- `skill_evolve` — thêm gợi ý tạo skill khi các workflow lặp lại chỉ ra một
  skill còn thiếu.

Endpoint: `GET /v1/agents/{id}/evolution/metrics`,
`GET /v1/agents/{id}/evolution/suggestions`,
`PATCH /v1/agents/{id}/evolution/suggestions/{suggestionId}`.
Tiến hóa theo từng skill có CLI riêng — xem
[Skills](./skills#per-skill-evolution). Web UI thể hiện điều này qua
**tab Evolution** trên trang chi tiết agent.

## Bước tiếp theo

- Loại agent và cấu hình: [Agent](./agents)
- Bộ nhớ mà ủy quyền và team dùng chung: [Bộ nhớ & Tri thức](./memory)
- Chi tiết định lịch: [WebSocket RPC](../api/websocket) và [HTTP API](../api/http)
