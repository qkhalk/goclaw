# Skill & Chợ Skill

Skill là các gói năng lực dùng lại được — research, scraping, OCR,
phân tích dữ liệu, xử lý media, viết content, cơ sở dữ liệu... — mỗi skill
được mô tả bởi một file `SKILL.md` kèm metadata frontmatter. Agent tìm và
dùng skill qua tìm kiếm lai; bạn quyết định skill nào được cài.

## Skill hoạt động thế nào

- Mỗi skill là một thư mục chứa `SKILL.md` (tên, mô tả, phiên bản, danh mục
  trong frontmatter) cùng các file hỗ trợ.
- Agent khám phá skill bằng `skill_search` — **tìm kiếm lai BM25 + ngữ
  nghĩa** — và thực thi bằng `use_skill`.
- Skill được cấp theo agent: cài chung, cấp có chọn lọc.
- Hơn 110 skill gốc repo đi kèm dự án, được nhúng vào binary.

## Seed: cái gì được cài ngay từ đầu

Khóa cấu hình `skills.seed_mode` (env: `GOCLAW_SKILLS_SEED_MODE`) quyết định
một **lần cài mới** seed những gì:

| Chế độ | Hành vi |
|--------|---------|
| `all` | Cả bộ skill — mặc định cũ, tương thích ngược hoàn toàn |
| `core` | Chỉ ~10 skill thiết yếu lúc runtime (review, plan, search, phụ thuộc designer của studio) |
| `none` | Không seed gì — muốn gì cài từ chợ |

Chuyển server đang chạy sang `core` **không gỡ** skill đã seed — reconciler
giữ nguyên các skill đã cài qua mỗi lần nâng cấp; chế độ này chỉ ảnh hưởng
lần cài mới. Skill không muốn dùng thì gỡ thủ công qua chợ.

## Chợ Skill

Chợ biến bộ skill bundle thành danh mục **chọn-gì-cài-đó**, có trong web UI
(tab **Skills → Market**) và qua HTTP.

- **Duyệt theo danh mục** — các thẻ có icon, tên, mô tả, phiên bản; tìm kiếm
  client-side và chip lọc danh mục trên toàn bộ danh mục.
- **Cài / Gỡ / Cập nhật** — lệnh cài chạy ngầm dưới dạng job có thanh tiến
  trình và log trực tiếp (chọn nhiều skill sẽ xếp vào một job hàng loạt).
  Quá trình cài copy từ **thư mục bundle trên đĩa — không tải qua mạng**.
- **Huy hiệu "Đã cài vX"** và dấu "có bản mới" hiển thị trạng thái rõ ràng;
  sau khi cài có thể chuyển thẳng sang cấp skill cho agent.
- **Thẻ Kit** — "cài cả bộ" vẫn là một cú click cho ai muốn hành vi cũ.
- **Chỉ admin** — cài và gỡ yêu cầu quyền admin; viewer chỉ xem được.

::: warning Skill tự viết được bảo vệ
Gỡ qua chợ chỉ áp dụng cho skill do chợ cài (nguồn system). Skill tự viết
không bao giờ bị gỡ qua chợ.
:::

### API Chợ Skill

| Endpoint | Chức năng |
|----------|-----------|
| `GET /v1/skills/market` | Danh sách danh mục (tên, mô tả, danh mục, phiên bản, `installed`, số cấp) |
| `POST /v1/skills/market/install` | `{ slugs: [...], grantAgentIds? }` — job cài ngầm |
| `POST /v1/skills/market/update/{slug}` | Cập nhật một skill đã cài |
| `DELETE /v1/skills/market/installed/{slug}` | Gỡ (chỉ skill do chợ quản lý) |

CLI phản chiếu bằng `goclaw skills market list|install`. Xem
[HTTP API (EN)](/en/api/http) về xác thực và hình thức request chung.

## Skill bundle cho studio

Hai skill đi kèm bundle dành riêng cho công cụ sáng tạo:
`pptx-deck-design` và `pptx-visual-style` vận hành agent thiết kế
`pptx-designer` đứng sau [PPTX Studio (EN)](/en/features/tools-studio).
