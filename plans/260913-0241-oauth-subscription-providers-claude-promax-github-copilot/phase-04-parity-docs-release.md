---
phase: 4
title: "Parity, docs, release"
status: completed
priority: P2
effort: "0.5d"
dependencies: [3]
---

# Phase 4: Parity, docs, release

## Overview

Close cross-surface parity, document, and ship a release the same way as the previous rounds (detailed commits → versioned binary → deploy).

## Requirements

- CLI parity: `goclaw auth status|logout [provider]` (`cmd/auth.go:37,65`) must work for claude/copilot provider names (it currently hits the `/v1/auth/chatgpt/...` legacy routes with an optional provider arg — generalize the path per provider type).
- Surface parity statement (required by AGENTS.md): gateway ✓ (phases 1-2), API contract ✓ (new routes documented), Web UI ✓ (phase 3), CLI ✓ (this phase), desktop/lite: OAuth providers are Standard+Lite (no edition gate — same as `chatgpt_oauth`; the SQLite `config_secrets` store already exists), Docker N/A (no image change).

## Implementation Steps

1. `cmd/auth.go` (status at :37, logout at :65, `resolveOAuthProviderArg` at 76-85): today the CLI always calls `/v1/auth/chatgpt/<name>/...` (the server-side legacy alias is `/v1/auth/openai/...`, oauth.go:70-74). Generalize: resolve the row's provider type via the existing `GET /v1/providers` list the web UI uses, then pick the chatgpt|claude|copilot path; default to chatgpt when no provider arg (current behavior preserved).
2. Docs: `docs/31-provider-oauth-subscriptions.md` (flows, security model, override envs, troubleshooting: expired subscription, Copilot client restrictions, Anthropic rotation escape hatch). Update `README.md` provider blurb + `CHANGELOG.md` Unreleased.
3. i18n audit: `grep` for new user-facing backend strings — add to `internal/i18n/keys.go` + 5 backend catalogs (`catalog_en/vi/zh/ko/ru.go`) if handler errors are localized (follow the chatgpt handler precedent).
4. Full checklist: `go fix`, `go build` (both tags), `go vet`, `go test -race` oauth+http, web `pnpm build` — ALL GREEN after red-team fixes.
5. Detailed commits (one per logical unit), push `origin/dev`, build `v4.0.4` (`-tags embedui`, worktree at HEAD), deploy to 192.168.1.103 with upload-verify-swap-restart-verify, confirm `version=v4.0.4` + served asset hash matches the fresh build.

## Todo

- [x] Steps 1-5

## Success Criteria

- [x] `goclaw auth status claude-pro` / `github-copilot` report authenticated state against the deployed gateway
- [x] Docs + CHANGELOG merged in the same commit series; server runs v4.0.4 with the new UI

## Risk Assessment

- **CLI path mapping guesses the wrong provider type for legacy aliases.** Mitigation: default to chatgpt when no provider arg is given (current behavior preserved).
