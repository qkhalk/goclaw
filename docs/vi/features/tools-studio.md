# Tool sáng tạo (Studio)

GoClaw đi kèm các ứng dụng sáng tạo chạy nơi chúng phù hợp nhất — chỉnh sửa
tương tác trong browser, render nặng trên server. Chúng nằm trong mục
**Tools** của web dashboard, và cũng đang được tách thành ứng dụng **GoTools**
độc lập mà bạn có thể triển khai mà không cần GoClaw.

## PPTX Studio (`/tools/pptx`)

Xây bộ slide bằng cách trò chuyện với **`pptx-designer`** — một agent
chỉ-thiết kế bị giới hạn trong allowlist tool tối thiểu. Slide render trực
tiếp trong browser khi bạn hội thoại, và xuất ra thành **file `.pptx` thật**
(không phải ảnh chụp).

Designer được hỗ trợ bởi hai skill đóng gói trong binary: `pptx-deck-design`
(cấu trúc bộ slide và mạch truyện) và `pptx-visual-style` (tính nhất quán
thị giác). Nó cố ý không chạy agent loop đầy đủ — một lần completion streaming
duy nhất biến ý định của bạn thành các khối slide mà editor hiểu được.

## Trình chỉnh sửa video (`/tools/video`)

Chỉnh sửa canvas + timeline với:

- Lời thoại TTS theo từng cảnh (giọng Edge TTS, ví dụ `vi-VN-HoaiMyNeural`)
- Hiệu ứng chuyển cảnh vào và transform/filter theo từng cảnh
- Quản lý render job với tiến trình

Các chỉnh sửa tương tác chạy **phía client**; render cuối đi qua pipeline phía
server (bên dưới).

### Pipeline video phía server

Render cuối được thực thi bởi `videoworker` — một binary ffmpeg worker độc
lập, triển khai tách khỏi gateway:

```
agent tool render_video ──► videoworker HTTP API (127.0.0.1:18791)
                                  │  storyboard JSON → ffmpeg
                                  ▼
                          rendered .mp4 + WS progress events
```

Agent kích hoạt render trực tiếp bằng tool `render_video`; vòng đời job
(queued → running → done/failed, hủy, xem trước, xóa) được bộc lộ qua HTTP
và WebSocket event. Hướng dẫn triển khai đầy đủ (systemd unit, giới hạn tài
nguyên, font, lời thoại):
[Tự host → Video worker sidecar](/vi/self-hosting#video-worker-sidecar).

## Watermark Remover (`/tools/watermark`)

Gỡ watermark ảnh và video hoàn toàn **phía client** bằng phép unblend nghịch
đảo alpha đã hiệu chỉnh. Đoạn video được phục hồi từng khung theo thời gian
thực trong lúc quay — **không gì rời khỏi browser**; không có bất kỳ
round-trip server nào.

## GoTools — studio độc lập

Các ứng dụng studio đang được tách vào
**[qkhalk/gotools](https://github.com/qkhalk/gotools)** (GoTools): một Go
server + SPA tự trọn vẹn với cơ sở dữ liệu SQLite riêng, triển khai được ở
domain riêng **mà không cần GoClaw gateway**.

- Cùng ba tool đó — watermark remover, PPTX studio, video studio (với sidecar
  videoworker riêng)
- Một **admin panel cho model/provider**: đăng ký API key, thêm model, xác
  minh bằng một lần gọi thử
- Một binary mỗi domain — không phụ thuộc PostgreSQL, kênh hay agent runtime
  của GoClaw

Mục Tools tích hợp sẵn của GoClaw vẫn tiếp tục tồn tại cho người dùng gateway;
GoTools dành cho các team chỉ muốn bộ sáng tạo.
