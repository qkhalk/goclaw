---
layout: home

hero:
  name: GoClaw
  text: Nền tảng AI Agent đa thuê
  tagline: Multi-agent AI gateway viết bằng Go. Hơn 40 nhà cung cấp LLM, 10 kênh nhắn tin, PostgreSQL đa thuê. Một binary duy nhất, đã kiểm chứng thực tế — agent tự điều phối và tự tạo slide, video, hình ảnh cho bạn.
  image:
    src: /goclaw-icon.svg
    alt: GoClaw
  actions:
    - theme: brand
      text: Bắt đầu
      link: /vi/getting-started/install
    - theme: alt
      text: Khởi động nhanh
      link: /vi/#khởi-động-nhanh
    - theme: alt
      text: GitHub
      link: https://github.com/qkhalk/goclaw

features:
  # Feature icons: Lucide (ISC license)
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <rect width="8" height="8" x="3" y="3" rx="2" /> <path d="M7 11v4a2 2 0 0 0 2 2h4" /> <rect width="8" height="8" x="13" y="13" rx="2" /> </svg>'
    title: Pipeline Agent 8 tầng
    details: context → history → prompt → think → act → observe → memory → summarize. Tầng cắm được (pluggable), luôn chạy, hệ thống prompt 4 chế độ tối ưu ranh giới cache.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z" /> <path d="M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12" /> <path d="M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17" /> </svg>'
    title: Bộ nhớ 3 tầng
    details: Working (hội thoại) → Episodic (tóm tắt phiên) → Semantic (knowledge graph), nạp tiến triển theo mức, kèm Knowledge Vault với tài liệu liên kết [[wikilink]].
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12 22v-5" /> <path d="M15 8V2" /> <path d="M17 8a1 1 0 0 1 1 1v4a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V9a1 1 0 0 1 1-1z" /> <path d="M9 8V2" /> </svg>'
    title: Hơn 40 nhà cung cấp LLM
    details: "Anthropic, OpenAI, OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI, DashScope và mọi endpoint tương thích OpenAI — cùng đăng ký OAuth: ChatGPT, Claude Pro/Max, GitHub Copilot."
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M2.992 16.342a2 2 0 0 1 .094 1.167l-1.065 3.29a1 1 0 0 0 1.236 1.168l3.413-.998a2 2 0 0 1 1.099.092 10 10 0 1 0-4.777-4.719" /> </svg>'
    title: 10 kênh nhắn tin
    details: Telegram (inline picker, skill phân trang, lệnh bản địa hóa), Discord, Slack, Facebook/Messenger, Zalo, Feishu/Lark, WhatsApp, Bitrix24, Pancake.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z" /> <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" /> <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" /> <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" /> <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" /> </svg>'
    title: Studio sáng tạo
    details: PPTX Studio xuất .pptx thật, trình chỉnh sửa video timeline kèm TTS, render video phía server bằng ffmpeg worker riêng, và công cụ xóa watermark chạy hoàn toàn trong trình duyệt.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M15.39 4.39a1 1 0 0 0 1.68-.474 2.5 2.5 0 1 1 3.014 3.015 1 1 0 0 0-.474 1.68l1.683 1.682a2.414 2.414 0 0 1 0 3.414L19.61 15.39a1 1 0 0 1-1.68-.474 2.5 2.5 0 1 0-3.014 3.015 1 1 0 0 1 .474 1.68l-1.683 1.682a2.414 2.414 0 0 1-3.414 0L8.61 19.61a1 1 0 0 0-1.68.474 2.5 2.5 0 1 1-3.014-3.015 1 1 0 0 0 .474-1.68l-1.683-1.682a2.414 2.414 0 0 1 0-3.414L4.39 8.61a1 1 0 0 1 1.68.474 2.5 2.5 0 1 0 3.014-3.015 1 1 0 0 1-.474-1.68l1.683-1.682a2.414 2.414 0 0 1 3.414 0z" /> </svg>'
    title: Thư viện Skill + Chợ Skill
    details: Hơn 110 skill sẵn có với tìm kiếm lai BM25 + ngữ nghĩa, và chợ skill — chỉ cài những gì bạn cần thay vì cả bộ.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /> <path d="M16 3.128a4 4 0 0 1 0 7.744" /> <path d="M22 21v-2a4 4 0 0 0-3-3.87" /> <circle cx="9" cy="7" r="4" /> </svg>'
    title: Team Agent & Subagent
    details: Bảng nhiệm vụ dùng chung, ủy quyền giữa các agent (đồng bộ/bất đồng bộ), ba chế độ orchestration, và subagent có thể theo dõi, hủy, lưu trữ ngay từ chat hoặc Telegram.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" /> <path d="m9 12 2 2 4-4" /> </svg>'
    title: Bảo mật production
    details: PostgreSQL đa thuê với RBAC, mã hóa API key AES-256-GCM, giới hạn tốc độ, phát hiện prompt injection, chống SSRF.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M15.914 4a1.5 1.5 0 00-2.474-1.561l-9 9A1.5 1.5 0 005.5 14h4.002a.5.5 0 01.471.666L8.086 20a1.5 1.5 0 002.475 1.56l9-9A1.5 1.5 0 0018.5 10h-3.997a.5.5 0 01-.472-.667z" /> </svg>'
    title: Một binary duy nhất
    details: Binary Go tĩnh ~25 MB, không cần Node.js runtime, khởi động dưới 1 giây, chạy được trên VPS 5 đô. Có phiên bản desktop dùng SQLite.
---

## Khởi động nhanh

### Binary (nhanh nhất để trải nghiệm)

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

Chạy wizard cấu hình ban đầu (tự chạy migration cơ sở dữ liệu) rồi khởi động gateway:

```bash
goclaw onboard
source .env.local && goclaw
```

Bảng điều khiển web có sẵn tại `http://localhost:18790`.

### Docker Compose

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw

# Sinh file .env với mật khẩu tự động
chmod +x prepare-env.sh && ./prepare-env.sh

# Thêm ít nhất một GOCLAW_*_API_KEY vào .env, rồi:
make up
```

`make up` tạo mạng Docker, build và khởi động toàn bộ dịch vụ, tự động chạy
migration. Kiểm tra sức khỏe:

```bash
curl http://localhost:18790/health
```

::: tip Bước tiếp theo
Đọc [Hướng dẫn cài đặt](/vi/getting-started/install) đầy đủ (desktop, build
từ source, các biến thể image Docker) và phần Cấu hình (tiếng Anh) về file
JSON5, env overlay và quản lý khóa bí mật.
:::
