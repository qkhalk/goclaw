# Skills & Chợ skill

Skills là các gói khả năng tái sử dụng — nghiên cứu, scraping, xử lý tài
liệu, phân tích dữ liệu, sản xuất media và hơn thế nữa — mỗi gói được mô tả
bởi một file `SKILL.md`. Agent tìm skill bằng tìm kiếm và gọi chúng như tool;
bạn quyết định skill nào được cài đặt và ai có thể dùng.

## Cách skill hoạt động

- Mỗi skill là một thư mục chứa `SKILL.md` (YAML frontmatter: name,
  description, version, category, dependencies) cùng các file hỗ trợ.
- Khám phá dùng **tìm kiếm BM25** trên index trong bộ nhớ (`internal/skills`);
  trên các edition có tìm kiếm vector, điểm được pha với độ tương đồng
  embedding.
- Thư mục skill đóng gói sẵn (`skills/`, ~120 skill gốc repo) được theo dõi
  bằng **fsnotify**: chỉnh sửa hot-reload mà không cần khởi động lại.
- Thư mục được phân giải qua **hệ thống phân cấp 5 tầng** (cấp system-bundled,
  managed, tenant, agent, user), nên cùng một slug có thể tồn tại ở các phạm
  vi khác nhau với thứ tự ưu tiên xác định.
- Skill được cấp theo từng agent hoặc từng user — cài toàn cục không tự động
  lộ một skill cho mọi agent.

Tool phía agent: `skill_search` (tìm), `use_skill` (thực thi),
`skill_manage` (tạo/patch/xóa từ hội thoại), `publish_skill`
(đăng ký một thư mục skill vào cơ sở dữ liệu hệ thống). Trong chat, tiền tố
`/gc:<skill>` chặn một tin nhắn và định tuyến nó như một lần gọi skill.

## Seeding: những gì được cài vào ngày đầu

`skills.seed_mode` (env `GOCLAW_SKILLS_SEED_MODE`) kiểm soát những gì một
**lượt cài mới** seed từ thư mục đóng gói sẵn:

| Chế độ | Hành vi |
|------|----------|
| `all` | Mọi skill đóng gói sẵn (mặc định, tương thích ngược) |
| `core` | Chỉ phần lõi thiết yếu cho runtime: `review`, `issue-to-plan`, `cook`, `fix`, `test`, `scout`, `journal`, `pptx`, `docx`, `pdf`, `xlsx`, `html-video`, `remotion` |
| `none` | Không gì cả — cài những gì bạn cần từ chợ skill |

Chuyển một triển khai hiện có sang `core` hoặc `none` không bao giờ gỡ cài
bất cứ thứ gì: nó chỉ ảnh hưởng đến lượt cài mới, và reconciler giữ các skill
đã cài tồn tại qua các lần nâng cấp.

## Chợ skill

Chợ skill biến bộ skill đóng gói sẵn thành danh mục chọn-cái-gì-cài-cái-đó.
Danh mục được dựng từ chính thư mục đóng gói — mọi thư mục con có `SKILL.md`
là một dòng, chú thích trạng thái cài đặt và khả năng cập nhật. Cài đặt là
bản sao thư mục cục bộ (không tải qua mạng) và hỗ trợ kit (ví dụ
`goclaw-kit`) để cài hàng loạt.

- **Web UI** — trang Skills, tab Market: duyệt theo danh mục, tìm kiếm,
  cài/gỡ/cập nhật với tiến trình trực tiếp.
- **CLI** — `goclaw skills market list` và `goclaw skills market install <slug>`.
- **HTTP** — `GET /v1/skills/market`, `POST /v1/skills/market/install`,
  `POST /v1/skills/market/update/{slug}`,
  `DELETE /v1/skills/market/installed/{slug}`.

::: warning Skill tùy chỉnh được bảo vệ
Gỡ cài qua chợ chỉ xóa skill có nguồn gốc hệ thống (cài từ kit đóng gói sẵn).
Skill tùy chỉnh bạn tạo ra không bao giờ bị xóa qua chợ.
:::

## Vòng đời dependency

Skill có thể khai báo dependency gói; GoClaw quản lý vòng đời của chúng:

- **Scanner** — soi các file của skill và tạo một dependency manifest
  (pip, npm, apk và gói hệ thống) kèm phát hiện runtime.
- **Checker** — báo cáo dependency nào đã thỏa mãn trên host.
- **Installer** — cài các gói còn thiếu theo từng hệ sinh thái.

CLI: `goclaw skills deps scan|check|install <skill>`.

Hỗ trợ theo edition: installer cho pip/npm/apk có trên bản Standard. Bản
desktop Lite không có installer pip/npm/apk (`SupportsPipNpm: false`,
`SupportsApk: false`) và không có tìm kiếm vector — tìm kiếm skill của nó chỉ
dùng BM25/FTS.

## Kiểm soát truy cập

Quyền truy cập được xử lý từ một chế độ truy cập cộng với các grant tường
minh:

- **Access mode** theo từng skill chi phối mặc định (ví dụ open so với
  chỉ-grant).
- **Grant theo agent và theo user** ghi đè mặc định, có phiên bản để các lần
  nâng cấp có thể cấp lại.
- **Quyền truy cập hiệu lực** là kết quả hợp nhất cho một cặp agent + user
  cụ thể.

CLI:

```bash
goclaw skills access get <skill>       # show mode and grants
goclaw skills access set <skill> ...   # set access mode
goclaw skills access effective <skill> # inspect effective access
goclaw skills grant agent <skill> <agent-id>
goclaw skills revoke user <skill> <user-id>
```

## HTTP API

| Endpoint | Mục đích |
|----------|---------|
| `GET/POST /v1/skills`, `GET/PUT/DELETE /v1/skills/{id}` | CRUD |
| `POST /v1/skills/upload` | Upload một kho lưu trữ skill |
| `GET/POST /v1/skills/export`, `POST /v1/skills/import` | Import/export |
| `/v1/skills/{id}/grants/agents`, `/v1/skills/{id}/grants/users` | Quản lý grant |
| `POST /v1/skills/{id}/toggle` | Bật/tắt |
| `GET /v1/skills/{id}/versions` | Lịch sử phiên bản |
| `/v1/skills/{id}/dependencies(/scan|/check|/install)`, `/v1/skills/install-deps`, `/v1/skills/rescan-deps` | Vòng đời dependency |
| `GET /v1/skills/runtimes` | Runtime đã phát hiện |
| `GET/PUT /v1/skills/{id}/tenant-config` | Cấu hình theo tenant |

Xem [HTTP API](../api/http) để biết xác thực và cấu trúc request chung.

## Tiến hóa theo từng skill

Skill tham gia tự tiến hóa: việc sử dụng được đo theo từng skill và hệ thống
có thể đề xuất cải thiện.

```bash
goclaw skills evolve status <skill>     # show evolution settings
goclaw skills evolve enable <skill>     # enable evolution
goclaw skills evolve disable <skill>    # disable evolution
goclaw skills evolve mode <skill> suggest_only|auto_analyze
goclaw skills metrics <skill>           # usage metrics (calls, success rate)
goclaw skills activity <skill>          # recent evolution activity
goclaw skills suggestions list <skill>  # list improvement suggestions
goclaw skills suggestions approve <skill> <suggestion-id>
goclaw skills suggestions reject <skill> <suggestion-id>
goclaw skills suggestions apply <skill> <suggestion-id>
```

Tương đương HTTP nằm dưới `/v1/skills/{id}/evolution`,
`/v1/skills/{id}/metrics` và `/v1/skills/{id}/activity`. Tiến hóa cấp agent
(metrics, đề xuất, tự viết lại SOUL.md) được đề cập trong
[Điều phối](./orchestration#self-evolution).

## Web UI

Trang **Skills** kết hợp thư viện (skill đã cài, grant, phiên bản, file) với
tab **Market** (danh mục, cài/cập nhật/gỡ). Các agent designer của Studio
như PPTX designer dùng các skill thiết kế đóng gói sẵn — xem
[Studio sáng tạo](./tools-studio).
