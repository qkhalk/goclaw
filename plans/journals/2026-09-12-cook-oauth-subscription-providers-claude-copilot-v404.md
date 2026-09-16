---
title: "Cook: OAuth subscription providers (Claude + Copilot), v4.0.4"
date: 2026-09-12
---

# Cook: OAuth subscription providers (Claude + Copilot), v4.0.4

Cooked plans/260913-0241 end-to-end: claude_oauth (PKCE paste-back, AuthType=oauth middleware flip, anthropic-beta comma-merge, /v1 base normalization for restarts) and copilot_oauth (device flow with in-flight dedup, poller shutdown, save/logout race guard, SSRF allowlist fallback). Code review agent verdict CHANGES-REQUIRED with 2 blockers + 4 major + 5 minor — all fixed and re-verified (go build both tags, vet, oauth -race, web build). Deployed v4.0.4 to 192.168.1.103 (upload-verify-swap-restart; version + UI asset hash confirmed). Learned: fresh pnpm install inside Docker on a Windows bind mount produces a broken node_modules (missing type packages -> 608 phantom TS errors); fix is a named Linux volume at /web/node_modules (8s install, 27s build). 6 Ollama model-list test failures in internal/http are pre-existing env-dependent (verified failing on clean HEAD).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
