---
layout: home

hero:
  name: GoClaw
  text: Multi-Tenant AI Agent Platform
  tagline: Multi-agent AI gateway built in Go. 40+ LLM providers, 10 channels, multi-tenant PostgreSQL. Single binary, production-tested — agents that orchestrate and create slides, videos, and art for you.
  image:
    src: /goclaw-icon.svg
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
  # Feature icons: Lucide (ISC license)
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <rect width="8" height="8" x="3" y="3" rx="2" /> <path d="M7 11v4a2 2 0 0 0 2 2h4" /> <rect width="8" height="8" x="13" y="13" rx="2" /> </svg>'
    title: 8-Stage Agent Pipeline
    details: context → history → prompt → think → act → observe → memory → summarize. Pluggable stages, always-on execution, 4-mode prompt system with cache boundary optimization.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z" /> <path d="M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12" /> <path d="M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17" /> </svg>'
    title: 3-Tier Memory
    details: Working (conversation) → Episodic (session summaries) → Semantic (knowledge graph), with progressive loading and a Knowledge Vault of [[wikilinked]] documents.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12 22v-5" /> <path d="M15 8V2" /> <path d="M17 8a1 1 0 0 1 1 1v4a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V9a1 1 0 0 1 1-1z" /> <path d="M9 8V2" /> </svg>'
    title: 40+ LLM Providers
    details: "Anthropic, OpenAI, OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI, DashScope and any OpenAI-compatible endpoint — plus OAuth subscriptions: ChatGPT, Claude Pro/Max, GitHub Copilot."
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M2.992 16.342a2 2 0 0 1 .094 1.167l-1.065 3.29a1 1 0 0 0 1.236 1.168l3.413-.998a2 2 0 0 1 1.099.092 10 10 0 1 0-4.777-4.719" /> </svg>'
    title: 10 Channels
    details: Telegram (inline pickers, paged skills, localized commands), Discord, Slack, Facebook/Messenger, Zalo, Feishu/Lark, WhatsApp, Bitrix24, Pancake.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z" /> <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" /> <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" /> <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" /> <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" /> </svg>'
    title: Creative Studio
    details: PPTX Studio with real .pptx export, a timeline video editor with TTS, server-side video rendering via a standalone ffmpeg worker, and a client-side watermark remover.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M15.39 4.39a1 1 0 0 0 1.68-.474 2.5 2.5 0 1 1 3.014 3.015 1 1 0 0 0-.474 1.68l1.683 1.682a2.414 2.414 0 0 1 0 3.414L19.61 15.39a1 1 0 0 1-1.68-.474 2.5 2.5 0 1 0-3.014 3.015 1 1 0 0 1 .474 1.68l-1.683 1.682a2.414 2.414 0 0 1-3.414 0L8.61 19.61a1 1 0 0 0-1.68.474 2.5 2.5 0 1 1-3.014-3.015 1 1 0 0 0 .474-1.68l-1.683-1.682a2.414 2.414 0 0 1 0-3.414L4.39 8.61a1 1 0 0 1 1.68.474 2.5 2.5 0 1 0 3.014-3.015 1 1 0 0 1-.474-1.68l1.683-1.682a2.414 2.414 0 0 1 3.414 0z" /> </svg>'
    title: Skill Library + Market
    details: 110+ repo-native skills with BM25 + semantic hybrid search, and a skill market — install only what you need instead of the whole bundle.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /> <path d="M16 3.128a4 4 0 0 1 0 7.744" /> <path d="M22 21v-2a4 4 0 0 0-3-3.87" /> <circle cx="9" cy="7" r="4" /> </svg>'
    title: Agent Teams & Subagents
    details: Shared task boards, inter-agent delegation (sync/async), three orchestration modes, and spawnable subagents you can track, cancel, and archive from chat or Telegram.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" /> <path d="m9 12 2 2 4-4" /> </svg>'
    title: Production Security
    details: Multi-tenant PostgreSQL with RBAC, AES-256-GCM encrypted API keys, rate limiting, prompt injection detection, SSRF protection.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" > <path d="M15.914 4a1.5 1.5 0 00-2.474-1.561l-9 9A1.5 1.5 0 005.5 14h4.002a.5.5 0 01.471.666L8.086 20a1.5 1.5 0 002.475 1.56l9-9A1.5 1.5 0 0018.5 10h-3.997a.5.5 0 01-.472-.667z" /> </svg>'
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
