---
layout: home

hero:
  name: GoClaw
  text: Multi-Tenant AI Agent Platform
  tagline: Multi-agent AI gateway built in Go. 40+ LLM providers, 10 channels, multi-tenant PostgreSQL. Single binary, production-tested — agents that orchestrate and create slides, videos, and art for you.
  image:
    src: /logo.svg
    alt: GoClaw
  actions:
    - theme: brand
      text: Get Started
      link: /en/getting-started/install
    - theme: alt
      text: Quick Start
      link: /en/#quick-start
    - theme: alt
      text: GitHub
      link: https://github.com/qkhalk/goclaw

features:
  - icon: 🧠
    title: 8-Stage Agent Pipeline
    details: context → history → prompt → think → act → observe → memory → summarize. Pluggable stages, always-on execution, 4-mode prompt system with cache boundary optimization.
  - icon: 🗂️
    title: 3-Tier Memory
    details: Working (conversation) → Episodic (session summaries) → Semantic (knowledge graph), with progressive loading and a Knowledge Vault of [[wikilinked]] documents.
  - icon: 🔌
    title: 40+ LLM Providers
    details: "Anthropic, OpenAI, OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI, DashScope and any OpenAI-compatible endpoint — plus OAuth subscriptions: ChatGPT, Claude Pro/Max, GitHub Copilot."
  - icon: 💬
    title: 10 Channels
    details: Telegram (inline pickers, paged skills, localized commands), Discord, Slack, Facebook/Messenger, Zalo, Feishu/Lark, WhatsApp, Bitrix24, Pancake.
  - icon: 🎨
    title: Creative Studio
    details: PPTX Studio with real .pptx export, a timeline video editor with TTS, server-side video rendering via a standalone ffmpeg worker, and a client-side watermark remover.
  - icon: 🧩
    title: Skill Library + Market
    details: 110+ repo-native skills with BM25 + semantic hybrid search, and a skill market — install only what you need instead of the whole bundle.
  - icon: 👥
    title: Agent Teams & Subagents
    details: Shared task boards, inter-agent delegation (sync/async), three orchestration modes, and spawnable subagents you can track, cancel, and archive from chat or Telegram.
  - icon: 🔐
    title: Production Security
    details: Multi-tenant PostgreSQL with RBAC, AES-256-GCM encrypted API keys, rate limiting, prompt injection detection, SSRF protection.
  - icon: ⚡
    title: Single Binary
    details: ~25 MB static Go binary, no Node.js runtime, sub-second startup, runs on a $5 VPS. SQLite desktop edition available too.
---

## Quick Start

### Binary (recommended for a quick look)

```bash
# macOS / Linux / WSL
curl -fsSL https://github.com/qkhalk/goclaw/raw/dev/scripts/install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -c "irm https://github.com/qkhalk/goclaw/raw/dev/scripts/install.ps1 | iex"
```

Then run the onboarding wizard (it runs database migrations for you) and start the gateway:

```bash
goclaw onboard
source .env.local && goclaw
```

The web dashboard is served at `http://localhost:18790`.

### Docker Compose

```bash
git clone -b dev https://github.com/qkhalk/goclaw.git && cd goclaw

# Generate .env with auto-generated secrets
chmod +x prepare-env.sh && ./prepare-env.sh

# Add at least one GOCLAW_*_API_KEY to .env, then:
make up
```

`make up` creates the Docker network, builds and starts all services, and runs
database migrations automatically. Health check:

```bash
curl http://localhost:18790/health
```

::: tip Next steps
Read the full [Installation guide](/en/getting-started/install) for desktop,
source builds and Docker image variants, then
[Configuration](/en/getting-started/configuration) for the JSON5 config file,
env overlay and secrets handling.
:::
