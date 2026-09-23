# Ứng dụng desktop (GoClaw Lite)

GoClaw Desktop (bản **Lite**) là ứng dụng desktop native để chạy agent cục
bộ. Đây là ứng dụng một cửa sổ làm bằng [Wails v2](https://wails.io) bọc
cùng binary gateway như bản server, biên dịch với build tag `sqliteonly`:

- **Store SQLite nhúng** — không PostgreSQL, không Docker.
- **Web UI nhúng** — dashboard React được biên dịch vào binary và hiển thị
  ngay trong cửa sổ ứng dụng.
- Một tiến trình, một thư mục dữ liệu, không cần hạ tầng nào.

## Cài đặt

```bash
# macOS
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install-lite.ps1 | iex"
```

Bản release được publish trên GitHub Releases với tag `lite-v*` cho macOS
(arm64/amd64) và Windows (amd64).

## Hành vi khi chạy

| Khía cạnh | Giá trị |
|-----------|---------|
| Địa chỉ lắng nghe | Chỉ `127.0.0.1` (localhost, không exposure ra mạng) |
| Cổng | `18790` (ghi đè bằng `GOCLAW_PORT`) |
| Thư mục dữ liệu | `~/.goclaw/data/` (database SQLite `goclaw.db`, config) |
| Workspace | `~/.goclaw/workspace/` (file agent, workspace team) |

Không có bước `goclaw onboard` trong ứng dụng desktop: khóa mã hóa và gateway
token được tạo tự động khi mở lần đầu.

## Secrets

Secrets được lưu trong **keyring của hệ điều hành** (`go-keyring`) với phương
án dự phòng là file tại `~/.goclaw/secrets/`. Cơ chế này bao gồm khóa mã hóa
và gateway token; API key của provider được mã hóa khi lưu trữ y như bản
server.

## Tự cập nhật

Ứng dụng kiểm tra GitHub Releases để tìm tag `lite-v*` mới hơn:

- một lần khi khởi động, và
- mỗi 6 giờ trong lúc chạy.

Khi có bản cập nhật, một update banner hiện ra trong ứng dụng; áp dụng sẽ tải
bản build mới, thay binary và khởi động lại ứng dụng. Không có updater daemon
riêng.

Phiên bản ứng dụng lấy từ `cmd.Version`, được tiêm vào qua `-ldflags` lúc
build. Edition đang chạy được gateway expose tại `GET /v1/edition` (không cần
xác thực), UI dùng thông tin này để thích ứng khả dụng tính năng.

## Giới hạn bản Lite

Giới hạn được thực thi bởi `internal/edition/edition.go` (`edition.Lite`):

| Năng lực | Lite | Standard |
|----------|------|----------|
| Agent | 5 | Không giới hạn |
| Team | 1 (5 thành viên) | Không giới hạn |
| Kênh | 1 Telegram + 1 Discord | Mọi loại kênh được hỗ trợ |
| Subagent chạy đồng thời | 2 | Không giới hạn |
| Độ sâu delegation | 1 | Không giới hạn |
| Tìm kiếm bộ nhớ | Chỉ full-text (FTS) | FTS + ngữ nghĩa pgvector |
| Đồ thị tri thức | Không có | Có |
| RBAC / multi-tenancy | Không có | Có |
| Trình cài phụ thuộc pip/npm/apk | Không có | Có |
| Tài khoản cloud (kết nối OAuth cloud) | Không có | Có |

## Build từ mã nguồn

```bash
# Dev mode with hot reload
cd ui/desktop && wails dev -tags sqliteonly

# Production builds (from the repo root)
make desktop-build VERSION=0.1.0   # .app (macOS) or .exe (Windows)
make desktop-dmg VERSION=0.1.0     # .dmg installer (macOS only)
```

## Reset dữ liệu

**Settings → About → Reset Database** xóa `goclaw.db` (gồm cả các file `-wal`
và `-shm`) khỏi thư mục dữ liệu và khởi động lại ứng dụng với một database
mới tinh. File trong workspace không bị xóa. Nếu ứng dụng có hành vi lạ sau
một lần nâng cấp thất bại, đây là cách nhanh nhất để trở về trạng thái sạch —
xem [Xử lý sự cố](../troubleshooting).

## Bước tiếp theo

- [Cài đặt](./getting-started/install) — các bản server và Docker
- [Cấu hình](./getting-started/configuration) — file config và biến môi trường
- [Xử lý sự cố](./troubleshooting) — các vấn đề thường gặp
