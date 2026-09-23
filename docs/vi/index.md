---
layout: home

hero:
  name: GoClaw
  text: Nền tảng AI agent multi-tenant
  tagline: AI gateway đa agent viết bằng Go. Hơn 40 LLM provider, 10 kênh, PostgreSQL multi-tenant. Single binary, đã được kiểm chứng trong thực tế — các agent tự điều phối và tạo slide, video, tranh ảnh thay bạn.
  image:
    src: /goclaw-icon.svg
    alt: GoClaw
  actions:
    - theme: brand
      text: Bắt đầu
      link: /vi/getting-started/install
    - theme: alt
      text: Bắt đầu nhanh
      link: /vi/#bắt-đầu-nhanh
    - theme: alt
      text: GitHub
      link: https://github.com/qkhalk/goclaw

features:
  # Feature icons: Lucide (ISC license)
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <rect width="8" height="8" x="3" y="3" rx="2" /> <path d="M7 11v4a2 2 0 0 0 2 2h4" /> <rect width="8" height="8" x="13" y="13" rx="2" /> </svg>'
    title: Agent pipeline 8 giai đoạn
    details: context → history → prompt → think → act → observe → memory → summarize. Các giai đoạn pluggable, luôn chạy trên mọi lượt thực thi, hệ thống prompt 4 chế độ với tối ưu biên cache.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z" /> <path d="M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12" /> <path d="M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17" /> </svg>'
    title: Bộ nhớ 3 tầng
    details: Bộ nhớ làm việc (hội thoại) → Bộ nhớ sự kiện (episodic, tóm tắt session) → Bộ nhớ ngữ nghĩa (semantic, đồ thị tri thức), với cơ chế nạp tăng tiến và một Knowledge Vault gồm các tài liệu liên kết [[wikilink]].
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12 22v-5" /> <path d="M15 8V2" /> <path d="M17 8a1 1 0 0 1 1 1v4a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V9a1 1 0 0 1 1-1z" /> <path d="M9 8V2" /> </svg>'
    title: Hơn 40 LLM provider
    details: "Anthropic, OpenAI, OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI, DashScope và bất kỳ endpoint tương thích OpenAI nào — cùng các gói đăng ký OAuth: ChatGPT, Claude Pro/Max, GitHub Copilot."
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M2.992 16.342a2 2 0 0 1 .094 1.167l-1.065 3.29a1 1 0 0 0 1.236 1.168l3.413-.998a2 2 0 0 1 1.099.092 10 10 0 1 0-4.777-4.719" /> </svg>'
    title: 10 kênh
    details: Telegram (inline picker, skill phân trang, lệnh đa ngôn ngữ), Discord, Slack, Facebook/Messenger, Zalo (OA & Personal), Feishu/Lark, WhatsApp, Bitrix24, Pancake.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z" /> <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" /> <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" /> <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" /> <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" /> </svg>'
    title: Xưởng sáng tạo
    details: PPTX Studio xuất file .pptx thật, trình chỉnh sửa video timeline kèm TTS, render video phía server qua một ffmpeg worker độc lập, và công cụ xóa watermark phía client.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M15.39 4.39a1 1 0 0 0 1.68-.474 2.5 2.5 0 1 1 3.014 3.015 1 1 0 0 0-.474 1.68l1.683 1.682a2.414 2.414 0 0 1 0 3.414L19.61 15.39a1 1 0 0 1-1.68-.474 2.5 2.5 0 1 0-3.014 3.015 1 1 0 0 1 .474 1.68l-1.683 1.682a2.414 2.414 0 0 1-3.414 0L8.61 19.61a1 1 0 0 0-1.68.474 2.5 2.5 0 1 1-3.014-3.015 1 1 0 0 0 .474-1.68l-1.683-1.682a2.414 2.414 0 0 1 0-3.414L4.39 8.61a1 1 0 0 1 1.68.474 2.5 2.5 0 1 0 3.014-3.015 1 1 0 0 1-.474-1.68l1.683-1.682a2.414 2.414 0 0 1 3.414 0z" /> </svg>'
    title: Thư viện skill + chợ skill
    details: ~120 skill có sẵn trong repo với tìm kiếm hybrid BM25 + ngữ nghĩa, vòng đời phụ thuộc (pip/npm/apk), và một chợ skill — chỉ cài những gì bạn cần thay vì cả bộ.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /> <path d="M16 3.128a4 4 0 0 1 0 7.744" /> <path d="M22 21v-2a4 4 0 0 0-3-3.87" /> <circle cx="9" cy="7" r="4" /> </svg>'
    title: Nhóm agent & điều phối
    details: Bảng task dùng chung, ủy quyền giữa các agent (sync/async), ba chế độ điều phối, các vòng jury/đàm phán, và tự tiến hóa có guardrail bảo vệ — tất cả theo dõi được ngay trong chat hoặc Telegram.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" /> <path d="m9 12 2 2 4-4" /> </svg>'
    title: Bảo mật cấp production
    details: PostgreSQL multi-tenant với RBAC, API key mã hóa AES-256-GCM, rate limiting, phát hiện prompt injection, chống SSRF.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M15.914 4a1.5 1.5 0 00-2.474-1.561l-9 9A1.5 1.5 0 005.5 14h4.002a.5.5 0 01.471.666L8.086 20a1.5 1.5 0 002.475 1.56l9-9A1.5 1.5 0 0018.5 10h-3.997a.5.5 0 01-.472-.667z" /> </svg>'
    title: Single binary
    details: Binary Go tĩnh ~25 MB, không cần Node.js runtime, khởi động dưới một giây, chạy tốt trên VPS $5. Cả bản desktop SQLite cũng có sẵn.
---

## Bắt đầu nhanh

### Binary (khuyên dùng khi muốn xem nhanh)

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

Sau đó chạy wizard onboarding (tự chạy database migration giúp bạn) và khởi động gateway:

```bash
goclaw onboard
source .env.local && goclaw
```

Web dashboard được phục vụ tại `http://localhost:18790`.

### Docker Compose

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw

# Generate .env with auto-generated secrets
chmod +x prepare-env.sh && ./prepare-env.sh

# Add at least one GOCLAW_*_API_KEY to .env, then:
make up
```

`make up` tạo Docker network, build và khởi động toàn bộ service, đồng thời tự chạy database migration. Kiểm tra sức khỏe:

```bash
curl http://localhost:18790/health
```

::: tip Các bước tiếp theo
Đọc [hướng dẫn Cài đặt](/vi/getting-started/install) đầy đủ cho bản desktop, build từ mã nguồn và các biến thể Docker image, rồi xem [Cấu hình](/vi/getting-started/configuration) về file config JSON5, env overlay và cách xử lý secrets.
:::
