---
title: "Plan: OAuth subscription providers (Claude Pro/Max + Copilot)"
date: 2026-09-12
---

# Plan: OAuth subscription providers (Claude Pro/Max + Copilot)

Planned claude_oauth + copilot_oauth provider types following the chatgpt_oauth pattern (plans/260913-0241). Verified endpoints from OpenClaw source (Copilot device flow Iv1.b507a08c87ecfe98, copilot_internal/user, sends original GitHub token — no v2/token) and public Claude OAuth implementations (claude.ai/oauth/authorize PKCE, platform.claude.com/v1/oauth/token, anthropic-beta oauth-2025-04-20, console paste-back). Red-team audit caught 3 required fixes folded into the plan: AnthropicProvider.middlewareConfig() hardcodes AuthType api_key so service_tier/fast-mode injection must be gated by a TokenSource-aware flip; anthropic-beta header must comma-merge with interleaved-thinking; NoEmbeddingTypes/NO_EMBEDDING_TYPES need both new types. Same-day context: v4.0.3 deployed (combobox auto-open fix + 6 API-key providers NVIDIA/StepFun/Venice/Baseten/Chutes/HuggingFace).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
