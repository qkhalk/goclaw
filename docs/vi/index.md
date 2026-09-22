---
layout: home

hero:
  name: GoClaw
  text: Nền tảng AI Agent đa thuê
  tagline: Multi-agent AI gateway viết bằng Go. Hơn 40 nhà cung cấp LLM, 10 kênh nhắn tin, PostgreSQL đa thuê. Một binary duy nhất, đã kiểm chứng thực tế — agent tự điều phối và tự tạo slide, video, hình ảnh cho bạn.
  image:
    src: /goclaw-hero.svg
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
  - title: Pipeline Agent 8 tầng
    details: context → history → prompt → think → act → observe → memory → summarize. Tầng cắm được (pluggable), luôn chạy, hệ thống prompt 4 chế độ tối ưu ranh giới cache.
  - title: Bộ nhớ 3 tầng
    details: Working (hội thoại) → Episodic (tóm tắt phiên) → Semantic (knowledge graph), nạp tiến triển theo mức, kèm Knowledge Vault với tài liệu liên kết [[wikilink]].
  - title: Hơn 40 nhà cung cấp LLM
    details: "Anthropic, OpenAI, OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI, DashScope và mọi endpoint tương thích OpenAI — cùng đăng ký OAuth: ChatGPT, Claude Pro/Max, GitHub Copilot."
  - title: 10 kênh nhắn tin
    details: Telegram (inline picker, skill phân trang, lệnh bản địa hóa), Discord, Slack, Facebook/Messenger, Zalo, Feishu/Lark, WhatsApp, Bitrix24, Pancake.
  - title: Studio sáng tạo
    details: PPTX Studio xuất .pptx thật, trình chỉnh sửa video timeline kèm TTS, render video phía server bằng ffmpeg worker riêng, và công cụ xóa watermark chạy hoàn toàn trong trình duyệt.
  - title: Thư viện Skill + Chợ Skill
    details: Hơn 110 skill sẵn có với tìm kiếm lai BM25 + ngữ nghĩa, và chợ skill — chỉ cài những gì bạn cần thay vì cả bộ.
  - title: Team Agent & Subagent
    details: Bảng nhiệm vụ dùng chung, ủy quyền giữa các agent (đồng bộ/bất đồng bộ), ba chế độ orchestration, và subagent có thể theo dõi, hủy, lưu trữ ngay từ chat hoặc Telegram.
  - title: Bảo mật production
    details: PostgreSQL đa thuê với RBAC, mã hóa API key AES-256-GCM, giới hạn tốc độ, phát hiện prompt injection, chống SSRF.
  - title: Một binary duy nhất
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
