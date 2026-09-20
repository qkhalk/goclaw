# MCP tool servers — the Tool Store catalog

Mỗi subfolder ở đây là **một MCP tool server cài được từ Tool Store**
(tier 3 — tải từ GitHub về, chạy như process riêng). Folder gốc chứa code
Go của catalog (`catalog.go` + tests), còn tool nằm trong subfolder.

## Thêm tool mới — 3 bước

```
goclaw mcp new <name>                      # sinh khung: manifest + server mẫu + smoke test
cd mcp/<name> && (sửa src/, node test/smoke.js)
git add mcp/<name> && git commit && git tag v<phiên-bản-mới> && git push --tags
```

Dynamic catalog tự phát hiện tool mới ở tag release mới nhất trong ~15
phút — **mọi bản cài hiện có thấy trong Store mà không cần nâng cấp goclaw**.

Yêu cầu folder (test `TestToolFoldersWellFormed` chặn trước khi release):

- `manifest.json` hợp lệ, `name` == tên folder
- `entry` tồn tại trên đĩa, `README.md` tồn tại
- Tool node phải **zero-dependency** (không `dependencies` trong
  `package.json`) — tool catalog giữ chuẩn không supply chain

## Manifest

```json
{
  "name": "media-probe",
  "display_name": "Media Probe",
  "description": "...",
  "category": "media",
  "runtime": "node",            // node | python
  "entry": "src/index.js",      // tương đối với folder tool
  "ram_note": "~60MB khi chạy, lazy-started"
}
```

- `repo` (tuỳ chọn): mặc định là chính repo goclaw — installer sparse-clone
  tag release và chỉ lấy đúng subfolder này. Khai repo github.com khác nếu
  tool muốn ship độc lập.
- `ref` (tuỳ chọn): mặc định là tag release đang chạy.

## Runtime

- **node**: entry chạy `node <entry>`; phải có `package.json`; server có
  thể gọi binary hệ thống (vd media-probe gọi `ffprobe`)
- **python**: entry chạy `python3 <entry>`; nếu có `requirements.txt`,
  installer cài vào `<tool>/vendor` và set `PYTHONPATH` khi spawn

Env của process server chỉ gồm `PATH` (+`PYTHONPATH`) — không có secret
của gateway.

## Install pipeline (tham khảo)

preflight → sparse clone (tag pin, github.com-only) → npm/pip deps nếu
khai báo → smoke test (spawn + MCP initialize + tools/list) → đăng ký vào
`mcp_servers`. Gỡ = xoá registration + files + row.
