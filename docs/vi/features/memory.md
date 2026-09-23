# Bộ nhớ & Tri thức

Agent GoClaw ghi nhớ xuyên suốt các tin nhắn, session và nhiều tuần nhờ hệ
thống bộ nhớ 3 tầng, đồ thị tri thức và vault tài liệu. Cả ba tầng đều theo
từng agent, có phạm vi theo tenant và tra cứu được qua một đường truy vấn hợp
nhất.

## 3 tầng bộ nhớ

| Tầng | Tên | Nội dung | Vòng đời |
|------|------|----------|----------|
| L0 | Bộ nhớ làm việc | Lịch sử session hiện tại, với auto-compaction dựa trên token cho hội thoại dài | Session |
| L1 | Bộ nhớ sự kiện (episodic) | Tóm tắt theo từng session được tạo sau mỗi lần chạy | Mặc định 90 ngày (cấu hình được) |
| L2 | Bộ nhớ ngữ nghĩa (semantic) | Đồ thị tri thức: thực thể và quan hệ được LLM trích xuất | Dài hạn |

### L0 — bộ nhớ làm việc

- Lịch sử hội thoại nằm trong session; khi phình to vượt ngưỡng token, nó được
  **tự nén (auto-compaction)** (tóm tắt) để giữ các lần chạy nằm trong context
  window.
- Trước khi nén, một giai đoạn **memory flush** lưu nội dung quan trọng vào
  kho dài hạn, để không gì bị mất qua việc tóm tắt.
- Các ký ức dài hạn liên quan được **tự động inject** vào system prompt với
  ngân sách ~200 token (per-agent `memory_config.auto_inject_max_tokens`,
  bật theo mặc định, ngưỡng liên quan 0.3). Mỗi mục được inject là một đoạn
  tóm tắt ngắn; agent có thể kéo full ký ức bằng `memory_expand(id)`.
- Truy hồi là hybrid: điểm từ khóa BM25 pha trộn với độ tương đồng embedding
  (vector).

### L1 — bộ nhớ sự kiện (episodic)

- Sau mỗi lần chạy, episodic worker tóm tắt session thành một bản ghi episodic
  (được tạo từ domain event `session.completed`).
- Mỗi bản ghi còn có một **tóm tắt L0** ~50 token để tự inject nhanh.
- Tìm kiếm là hybrid: full-text search của PostgreSQL (`search_vector` đánh
  chỉ mục GIN) pha trộn với tìm kiếm vector theo độ tương đồng cosine trên
  episodic embeddings.
- Bản ghi hết hạn sau **90 ngày** theo mặc định — tinh chỉnh được theo agent
  qua `memory_config.episodic_ttl_days`.

### L2 — bộ nhớ ngữ nghĩa (semantic, đồ thị tri thức)

- Một LLM trích xuất **thực thể và quan hệ** từ các session vào đồ thị.
- Thực thể trùng lặp được phát hiện và có thể gộp lại (quét dedup với ứng
  viên xem xét được).
- Đồ thị hỗ trợ truy vấn duyệt (traversal), nên agent có thể trả lời các câu
  hỏi trải rộng qua nhiều session trong quá khứ.

## Consolidation theo event

Consolidation chạy trên [DomainEventBus](../architecture) — không polling:

```
session.completed → episodic summary (L1) → KG extraction (L2) + dreaming
```

**Dreaming worker** hợp nhất các tóm tắt episodic chưa được thăng hạng thành
tài liệu sự kiện dài hạn: khi **5 hoặc nhiều hơn** tóm tắt chưa thăng hạng
tích lũy cho một agent, một LLM tổng hợp các sự kiện bền vững từ chúng. Các
lần chạy được debounce theo agent (mặc định 10 phút) và cả hai giá trị đều
ghi đè được theo agent qua `memory_config.dreaming` (`threshold`,
`debounce_ms`, `enabled`).

## Knowledge Vault

Vault là một sổ đăng ký tài liệu nằm trên các kho lưu trữ bộ nhớ:

- **Tài liệu** với metadata, danh mục và nội dung, được đăng ký từ upload hoặc
  **đồng bộ filesystem** (rescan phát hiện file mới/thay đổi).
- **Indexing tăng dần** — nội dung được hash bằng SHA-256, nên tài liệu không
  thay đổi không bao giờ bị xử lý lại.
- **[[wikilinks]]** giữa các tài liệu, được phân giải và duyệt được theo cả
  hai chiều.
- **LLM enrichment worker** — phân loại, tóm tắt và tự liên kết tài liệu trong
  nền (có endpoint status và stop).
- **Chế độ xem đồ thị** trên cấu trúc liên kết.

### Tìm kiếm hợp nhất

Tìm kiếm Vault pha trộn ba nguồn với trọng số cố định:

| Nguồn | Trọng số |
|--------|--------|
| Tài liệu Vault | 40% |
| Tóm tắt episodic | 30% |
| Thực thể KG | 30% |

## Bề mặt API

Các WebSocket method:

| Method | Mục đích |
|--------|---------|
| `memory.write` | Ghi một ký ức |
| `memory.get` | Lấy ký ức theo ID |
| `memory.search` | Tìm kiếm hybrid |
| `memory.supersede` | Thay thế ký ức bằng phiên bản mới hơn |
| `memory.archive` | Lưu trữ một ký ức |

HTTP endpoint (xem [HTTP API](../api/http) để biết xác thực và cấu trúc
request):

| Endpoint | Mục đích |
|----------|---------|
| `GET /v1/agents/{id}/episodic` | Liệt kê tóm tắt episodic |
| `POST /v1/agents/{id}/episodic/search` | Tìm kiếm bộ nhớ episodic |
| `GET /v1/agents/{id}/kg/entities` | Liệt kê thực thể KG |
| `GET /v1/agents/{id}/kg/graph` | Lấy đồ thị |
| `POST /v1/agents/{id}/kg/traverse` | Duyệt từ một thực thể |
| `POST /v1/agents/{id}/kg/extract` | Kích hoạt trích xuất |
| `GET /v1/agents/{id}/kg/stats` | Thống kê đồ thị |
| `POST /v1/agents/{id}/kg/merge` | Gộp thực thể trùng lặp |
| `GET /v1/agents/{id}/kg/dedup` · `POST .../kg/dedup/scan` · `POST .../kg/dedup/dismiss` | Quy trình dedup |
| `/v1/vault/documents` · `/links` · `/search` · `/graph` · `/tree` · `/upload` · `/rescan` | Sổ đăng ký Vault, wikilinks, tìm kiếm, đồ thị, rescan FS, upload |
| `GET /v1/vault/enrichment/status` · `POST /v1/vault/enrichment/stop` | Điều khiển enrichment worker |

Các biến thể phạm vi theo agent nằm dưới `/v1/agents/{id}/memory/...` và
`/v1/agents/{id}/vault/...`; `/v1/memory/documents` liệt kê xuyên các agent.

## Tool cho agent

Agent dùng các tool tích hợp sẵn này (xem [Tools](./tools)):

- `memory_search` — tìm kiếm hybrid trên các ký ức
- `memory_get` / `memory_expand` — lấy một ký ức hoặc mở rộng tóm tắt đã inject
- `knowledge_graph_search` — truy vấn đồ thị thực thể
- `vault_search` / `vault_read` — tìm kiếm và đọc tài liệu vault

## Web UI

- **/memory** — trình duyệt episodic và tài liệu bộ nhớ dài hạn.
- **/knowledge-graph** — đồ thị thực thể/quan hệ, trích xuất thủ công và xem
  xét dedup.
- **/vault** — tài liệu, wikilinks, chế độ xem đồ thị và tìm kiếm.

::: warning Bản Lite
Bản desktop Lite tắt đồ thị tri thức và tìm kiếm vector (`KGEnabled: false`,
`VectorSearch: false`). Bộ nhớ episodic vẫn khả dụng với tìm kiếm từ vựng
(không vector) trên SQLite.
:::
