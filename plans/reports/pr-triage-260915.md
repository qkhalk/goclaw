# PR Triage Report — 2026-09-15

Scope: open PRs on `nextlevelbuilder/goclaw` (upstream, 125 open) and `qkhalk/goclaw` (user fork, 3 open).
Method: `gh pr list --json` (mergeable/mergeStateStatus/statusCheckRollup), re-query of 25 stale `UNKNOWN` mergeability values, `gh run view <id> --log-failed` for deep-dives. Read-only — no comments, reviews, or pushes.

## Headline counts

### upstream nextlevelbuilder/goclaw (125 open)

| Category | Count | Detail |
|---|---|---|
| merge-conflict | 94 primary (+11 secondary) | 105 PRs total are CONFLICTING; 83 from list query + 22 of 25 stale-`UNKNOWN` re-verified |
| failing-tests | 10 | `go` job FAILURE: 1442, 1012, 983, 955, 813, 537, 393, 386, 380, 343 — 8 of them also conflicted; CI logs for 9 of 10 are purged (HTTP 410, >90d retention) |
| review-blocked (CI green) | 6 | 1173, 1135, 1114, 1088, 1083, 1082 — MERGEABLE + BLOCKED with all checks SUCCESS; waiting on maintainer review, nothing broken |
| review-blocked (no checks recorded) | 6 | 1522, 1276, 1242, 1194, 1181, 1142 — MERGEABLE + BLOCKED with empty statusCheckRollup; blocked on required review, no CI recorded on head |
| draft | 5 | 1487, 1166 (clean+mergeable), 1079, 1033, 891 (also conflicted) |
| stale — mergeable, target moved | 2 | 414, 408: green CI, MERGEABLE, but target `main` while active development is on `dev` |
| stale — orphaned stack | 2 | 969 → 944: CLEAN but base branches (`feat/packages-update-phase2a-pip-npm`, `feat/packages-update-flow`) are themselves unmerged feature branches; root of stack has no open PR |

Verified total: 105 CONFLICTING / 20 MERGEABLE (13 BLOCKED, 4 CLEAN, 3 old-mergeable).

### qkhalk/goclaw fork (3 open, all target fork `dev`)

| Category | Count | Detail |
|---|---|---|
| failing-tests (PR-caused) + inherited lint | 1 | #79 — `TestBundledSkills_NoRegression/ship` (go) + inherited web lint |
| inherited lint only | 2 | #78, #80 — web job fails with errors that pre-exist on `dev` itself |

**Root cause on the fork: the `dev` branch's own CI is red.** Runs 34928274723 (2026-09-15) and 34888445968 (2026-09-14): `go=success, web=failure`. Three ESLint errors:

- `ui/web/src/pages/cloud/drive/drive-file-area.tsx:133:11` — `no-useless-assignment` (value assigned to `cmp` never used)
- `ui/web/src/pages/tools/video/components/canvas-player.tsx:97:11` — `@typescript-eslint/no-unused-expressions`
- `ui/web/src/pages/tools/video/video-tool-page.tsx:467:31` — `@typescript-eslint/no-unused-vars` (`_scenes` unused)

These are byte-identical in the failed `web` logs of PRs 78, 79, and 80 → one fix on `dev` (3 files, 3 errors; 6 more warnings are `--fix`-able) turns the web job green for all three PRs.

## Summary table — upstream nextlevelbuilder/goclaw

Age in days from createdAt to 2026-09-15. CI: check conclusions from statusCheckRollup (`none` = no checks ran / purged).

| PR | Title | Author | Branch → Base | Age | State | Mergeable/Status | CI checks | Failure category | Action |
|---|---|---|---|---|---|---|---|---|---|
| 1548 | fix(anthropic): stack overflow guard + adaptive thinking format + Fable … | JFernandoAmorim2005 | pr/claude-anthropic-model-support → dev | 13d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | needs-rebase |
| 1539 | feat: add OrcaRouter as a named LLM provider | bangla24bdrang-lab | feat/orcarouter-provider → dev | 17d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | needs-rebase |
| 1538 | feat(webhooks): accept inbound media on POST /v1/webhooks/llm (sync mode… | justintruong29 | feat/webhook-inbound-media-upstream → dev | 19d | OPEN | CONFLICTING/DIRTY | release-versioning:SUCCESS,go:SUCCESS,web:SUCCESS | merge-conflict | needs-rebase |
| 1522 | Update MiniMax music generation handling | octo-patch | octo/20260823-music-generation-recvsf4VGQpexP → dev | 23d | OPEN | MERGEABLE/BLOCKED | none | review-blocked (no checks) | approve-fixable (maintainer review) |
| 1503 | fix(slack): gate require_mention on the event's own text, not injected t… | alex-coolfy | fix/slack-mention-gate-thread-parent-context → main | 41d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | needs-rebase |
| 1495 | feat(team-work): one-call LLM routing classifier with native DAG orchest… | nguyenha935 | feat/team-work-native-orchestration → dev | 44d | OPEN | CONFLICTING/DIRTY | release-versioning:SUCCESS,go:SUCCESS,web:SUCCESS | merge-conflict | needs-rebase |
| 1487 | fix(memory): enforce auto-inject budget and correct search schema | longlonggo | agent/fix-memory-tool-contracts → dev | 47d | OPEN | MERGEABLE/CLEAN | none | draft | needs-author |
| 1452 | feat(mcp): expose MCP server management as goclaw_mcp_servers_* tools | bclermont | feat/mcp-servers-crud-tools → dev | 57d | OPEN | CONFLICTING/DIRTY | release-versioning:SUCCESS,go:SUCCESS,web:SUCCESS | merge-conflict | needs-rebase |
| 1442 | fix(migrations): auto-create required extensions, prevent dirty state | bclermont | fix/migration-extension-prerequisite → dev | 61d | OPEN | MERGEABLE/BLOCKED | release-versioning:SUCCESS,go:FAILURE,web:SUCCESS | failing-tests | needs-author (fix tests) |
| 1429 | feat(dingtalk): DingTalk channel (Stream mode + AI Card streaming) | connermo | feat/dingtalk-channel → dev | 64d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | needs-rebase |
| 1276 | feat(discord): follow threads after first mention | Hitesh-Sisara | feat/discord-thread-follow → dev | 83d | OPEN | MERGEABLE/BLOCKED | none | review-blocked (no checks) | approve-fixable (maintainer review) |
| 1252 | Feat/aiclaw prompt mode | ThanhDang-Vn | feat/aiclaw-prompt-mode → dev | 85d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | needs-rebase |
| 1242 | feat: Mattermost channel handler (DM mode) | msaidf | feat/mattermost-channel → dev | 88d | OPEN | MERGEABLE/BLOCKED | none | review-blocked (no checks) | approve-fixable (maintainer review) |
| 1224 | fix(hooks): improve agent-scoped hook error messaging | bclermont | fix/agent-scope-hook-tenant-id-clean → dev | 93d | OPEN | CONFLICTING/DIRTY | release-versioning:SUCCESS,go:SUCCESS,web:SUCCESS | merge-conflict | needs-rebase |
| 1194 | Update goclaw version | trwng-thdat | update-goclaw-version → dev | 99d | OPEN | MERGEABLE/BLOCKED | none | review-blocked (no checks) | approve-fixable (maintainer review) |
| 1181 | Render deploy | ojusave | render-deploy → dev | 108d | OPEN | MERGEABLE/BLOCKED | none | review-blocked (no checks) | approve-fixable (maintainer review) |
| 1173 | fix(ui+http): timezone validator + system-configs 404 noise | vanducng | upstream-fix/timezone-validator-sysconfig-404 → dev | 113d | OPEN | MERGEABLE/BLOCKED | release-versioning:SUCCESS,go:SUCCESS,web:SUCCESS | review-blocked (CI green) | approve-fixable (maintainer review) |
| 1168 | fix(browser): resolve context canceled errors on Windows | egany | fix/browser-tool-windows-context-handling → main | 115d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | needs-rebase |
| 1166 | feat(media): pluggable storage backend + S3 implementation | ilyaseverin | feat/media-s3-backend → main | 117d | OPEN | MERGEABLE/CLEAN | none | draft | needs-author |
| 1146 | fix(exec): allow skills-store and user allow_paths as working_dir | codebit0 | fix/exec-workingdir-allow-paths → dev | 125d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1142 | feat(channels): implement channels.toggle RPC method | zhihan | feat/channels-toggle → dev | 126d | OPEN | MERGEABLE/BLOCKED | none | review-blocked (no checks) | approve-fixable (maintainer review) |
| 1135 | feat(prepare-compose): compose file picker | keithy | feature/prepare-compose-script → main | 128d | OPEN | MERGEABLE/BLOCKED | go:SUCCESS,web:SUCCESS | review-blocked (CI green) | approve-fixable (maintainer review) |
| 1129 | fix(cron): stamp + replay creator sender/role across cron fires | mozaa-solana | fix/cron-creator-sender-propagation → dev | 130d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1123 | fix(providers): increase HTTP connection pool for embedding provider | ronaldtangg | fix/embedding-http-pool → dev | 130d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1117 | feat(ui): add Compact button to session detail page | nguyennguyenit | feat/ui-session-compact-button → main | 130d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1116 | fix(agent): suppress empty final content instead of sending literal "...… | nguyennguyenit | fix/agent-empty-final-content-no-ellipsis → main | 131d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1115 | fix(mcp): exact-match dangerous flags, stop -c substring false positives | nguyennguyenit | fix/mcp-arg-validation-substring-1027 → dev | 131d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1114 | feat(config): add GOCLAW_CRON_JOB_TIMEOUT env var override | nguyennguyenit | fix/cron-job-timeout-env → dev | 131d | OPEN | MERGEABLE/BLOCKED | go:SUCCESS,web:SUCCESS | review-blocked (CI green) | approve-fixable (maintainer review) |
| 1110 | feat(pkg/tool): add GlobalToolFactoryRegistry for external tool regis… | 249313652 | feature/pr-documentation → main | 131d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1109 | feat(channels): add Max Messenger channel | HumanGoClaude | feat/max-channel → dev | 131d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1105 | fix(perms): propagate sender/role through MCP bridge & delegate (#915) | mozaa-solana | fix/perms-bridge-delegate-915 → dev | 132d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1104 | fix(skills): instruct agent to use exact absolute path for skill files | buidackim | fix/skill-path-prompt → dev | 132d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1101 | fix(agent): add Thinking field to synthetic post-summary assistant messa… | hoakhongmau98 | fix/deepseek-synthetic-thinking → dev | 133d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1093 | New providers | azharkov78 | dev → dev | 134d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1088 | fix(teams): propagate task result in team task completion event | khanhtranhd | hotfix/team-task-result-propagation → dev | 134d | OPEN | MERGEABLE/BLOCKED | go:SUCCESS,web:SUCCESS | review-blocked (CI green) | approve-fixable (maintainer review) |
| 1084 | feat(i18n): cascade locale resolution for cron + system-triggered runs | codebit0 | feat/cron-locale → dev | 135d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1083 | fix(cli): add missing X-GoClaw-User-Id header to gateway client | codebit0 | fix/cli-user-id-header → dev | 135d | OPEN | MERGEABLE/BLOCKED | go:SUCCESS,web:SUCCESS | review-blocked (CI green) | approve-fixable (maintainer review) |
| 1082 | fix(ops): bot token masking + episodic timeout 120s + path_escape log le… | codebit0 | fix/ops-hardening → dev | 135d | OPEN | MERGEABLE/BLOCKED | go:SUCCESS,web:SUCCESS | review-blocked (CI green) | approve-fixable (maintainer review) |
| 1079 | fix(http,ws): RBAC for providers read + master predefined context files … | vanducng | fix/rbac-providers-master-context-1075-1054 → dev | 135d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | draft | needs-author |
| 1072 | feat(acp): full Gemini CLI integration — tool exposure + spec compliance… | codebit0 | fix/acp-tool-exposure → dev | 137d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1068 | feat: Add full Russian localization support | cubtok | dev → dev | 138d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1065 | fix(whatsapp): scope whatsmeow device per channel instance | kamushadenes | fix/whatsapp-per-channel-device → dev | 139d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1063 | fix(providers): resolve HTTP 400 DeepSeek reasoning passback error durin… | nagaame | dev → dev | 139d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1061 | feat(channels): Bitrix24 channel core + UI + per-user MCP (split 3/3 fro… | tech-synity | feat/bitrix24-channel-core → dev | 139d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1060 | feat(store): bitrix_portals store + migration 000058 + CLI (split 2/3 fr… | tech-synity | feat/bitrix24-store-migration → dev | 139d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1059 | chore(infra): build/deps/MCP/whitelist fixes (split 1/3 from #1057) | tech-synity | feat/infra-fixes → dev | 139d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1048 | feat(channels/zalo): OA OAuth + webhook transport (#966) | vanducng | feat/zalo-oa-webhook-966-clean → dev | 141d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 1045 | feat(hooks): surface script reason in synthetic block messages | kamushadenes | kamushadenes/hook-reason-in-tool-message → dev | 141d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1044 | perf(providers): cache conversation history on Anthropic requests | kamushadenes | kamushadenes/cache-messages-rolling → dev | 141d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1033 | feat(exec): detect credentialed CLIs in shell chains + allow_chain_exec … | kamushadenes | fix/credentialed-exec-chain-detection → dev | 143d | OPEN | CONFLICTING/DIRTY | none | draft | needs-author |
| 1032 | fix(cron): always reset session before cron runs — stateless flag invert… | kamushadenes | fix/cron-stateless-session-reset → dev | 143d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1024 | feat(discord): real-time voice-channel transcription | tarrencev | upstream-voice-recv → dev | 144d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1013 | feat(channels): configurable table rendering for Telegram and WhatsApp | srinis76 | feat/table-mode-channels → dev | 145d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 1012 | feat: configurable MCP stdio command allowlist | reski-rukmantiyo | feat/register-mcp-allowlist → dev | 145d | OPEN | CONFLICTING/DIRTY | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 991 | feat(discord): register slash commands + interaction reply path | tarrencev | feat/discord-slash-commands → dev | 146d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 988 | feat(tools): add send_discord_embed tool for rich embeds | tarrencev | feat/discord-embeds → dev | 146d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 987 | feat(channels): suppress_placeholder toggle for Discord and Slack | tarrencev | feat/suppress-thinking-placeholder → dev | 146d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 983 | fix(memory): truncate embeddings locally | badgerbees | fix/vllm-embedding-dimensions → dev | 146d | OPEN | CONFLICTING/DIRTY | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 981 | feat(webhooks): HTTP webhooks to trigger agents with HMAC auth + durable… | mrgoonie | feat/webhook-agent-triggering → dev | 146d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 980 | feat(packages): unify Packages & CLI Credentials + per-grant env overrid… | mrgoonie | feat/packages-cli-credentials-unified-ui → dev | 147d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 976 | feat(tools): add create_discord_thread tool | tarrencev | feat/create-discord-thread → dev | 147d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 971 | fix(slack,background): deliver media through placeholder path + auto-res… | Vo-Linh | fix/slack-file-upload-and-background-model-fallback → dev | 148d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 969 | feat(packages): Phase 2b — apk update flow + pkg-helper v2 protocol (#90… | mrgoonie | feat/packages-update-phase2b-apk-pkghelper → feat/packages-update-phase2a-pip-npm | 148d | OPEN | MERGEABLE/CLEAN | none | stale (orphaned/non-dev base) | close-as-stale or retarget |
| 965 | feat(discord): expose channel_id in inbound prefix | badgerbees | feat/discord-channel-id-prefix → dev | 148d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 955 | feat(tools): add read-only GitHub REST tool for agents  | badgerbees | feat/github-read-tool → dev | 149d | OPEN | CONFLICTING/DIRTY | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 953 | feat(channels) WeChat intergration porting from ts official lib | himulawang | feat/wechat-channel → dev | 149d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 946 | ci: native arm64 runners + DRY docker release workflows via reusable wor… | vanducng | ci/docker-multiarch-dry → dev | 151d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 944 | feat(packages): Phase 2a — pip + npm update flow (#900) | mrgoonie | feat/packages-update-phase2a-pip-npm → feat/packages-update-flow | 151d | OPEN | MERGEABLE/CLEAN | none | stale (orphaned/non-dev base) | close-as-stale or retarget |
| 943 | feat(workstation): Remote Workstation Runtime — SSH exec + security + au… | mrgoonie | feat/remote-workstation-runtime → dev | 151d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 908 | feat(providers): add Google Cloud Vertex AI provider (#576) | mrgoonie | feat/vertex-ai-provider → dev | 153d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 899 | feat(vertex): add ADC-only native Vertex provider support | wavewiser | feature/vertex-provider-support → main | 154d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 892 | feat(providers): add Qwen Code CLI provider | ntheanh201 | feat/qwen-code-cli → dev | 154d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 891 | fix(browser): add SSRF policy and navigation guards | badgerbees | browser-ssrf-policy → dev | 154d | OPEN | CONFLICTING/DIRTY | none | draft | needs-author |
| 857 | fix(heartbeat): keep stable phase scheduling | badgerbees | fix/heartbeat-stable-phase → dev | 155d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 817 | feat(channels): add Microsoft Teams channel integration | olbboy | feat/teams-v3 → dev | 158d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 813 | feat: WhatsApp group agent overrides, pipeline block reply fix, and grou… | reski-rukmantiyo | dev-311-whataspp → dev | 158d | OPEN | CONFLICTING/DIRTY | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 805 | feat: add delete-all chats action to web sidebar | TheSethRose | feat/delete-all-chats → dev | 158d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 781 | feat: WhatsApp groups management with auto-discovery and per-group agent… | reski-rukmantiyo | dev-origin-whatsapp-lagi → dev | 159d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 780 | feat(exec): add packageAutoApprove for agent package installs | konamgil | feat/package-auto-approve → dev | 159d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 778 | feat(browser): multi-engine browser automation with pool, stealth, proxy… | nhokboo | feat/webbrowser-control → dev | 159d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 777 | Add GitHub Copilot OAuth provider | TheSethRose | feat/github-copilot-oauth-provider → dev | 159d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 769 | feat(whatsapp): add text-based command menu | reski-rukmantiyo | feat/whatsapp-command-menu → dev | 159d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 765 | feat(i18n): add Russian language support | AlexZander85 | feat/russian-localization → dev | 159d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 751 | feat: automatic task de-duplication on create | reski-rukmantiyo | feat/task-auto-dedup → dev | 160d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 750 | refactor: optimize team-dispatched agent context and restrict roster vis… | reski-rukmantiyo | refactor/team-agent-context → dev | 160d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 715 | feat(line): LINE Messaging API channel adapter | stanleykao72 | feat/line-channel → main | 162d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 712 | feat(cron): enforce unique active cron job names per agent+user | nguyennguyenit | feat/cron-unique-active-name → main | 162d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 682 | feat(providers): add Cursor CLI provider | chinhtran-dev | feat/cursor-cli-provider → dev | 164d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 653 | ci: publish Docker images with dev tag on push to dev branch | vanducng | feat/652-docker-dev-tag → dev | 165d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 639 | Add GoClaw Hub marketplace integration | bnqtoan | feat/hub-marketplace-integration → main | 166d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 612 | feat(tracing): integrate langsmith-go for LLM request tracing | kevinle128 | feature/add-langsmith → main | 167d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 607 | feat(skills): add Skill Hub — GitHub registry + CLI install/remove/searc… | lukebaze | feat/skill-hub → main | 168d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 592 | feat(team): multi-bot Discord delivery, memory team scoping, vision fall… | kokorolx | feat/team-discord-fixes → main | 168d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 570 | fix(summoner): preserve user-set agent name after summon completes | teexiii | dev/tam → main | 169d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 556 | chore(docker): add uv package manager to Python installs | SkyTik | chore/add-uv-to-dockerfile → main | 169d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 553 | fix(sessions): use UPSERT in Save() to persist first-run cron sessions | duhd-vnpay | fix/session-save-upsert → main | 169d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 551 | feat: project-as-a-channel — per-project MCP environment isolation | duhd-vnpay | feat/project-as-a-channel → main | 169d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 538 | feat(providers): upgrade MiniMax to dedicated provider with M2.7 default | octo-patch | feature/upgrade-minimax-provider → main | 170d | OPEN | CONFLICTING/DIRTY | none | merge-conflict | close-as-stale |
| 537 | feat(channels): add channel_groups directory for group name resolution | Luvu182 | feat/channel-groups-directory → main | 170d | OPEN | CONFLICTING/DIRTY | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 533 | feat(skills): install skills from GitHub URL | Luvu182 | feat/skill-install-url → main | 170d | OPEN | CONFLICTING/DIRTY | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 530 | feat(docker): integrate Claude CLI into container build | ThuyTran07 | feat/claude-cli-docker → main | 170d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 528 | fix(slack): Thread announce routing for top-level channel mentions | ronaldtangg | fix/slack-thread → main | 170d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 485 | fix: nginx use DNS resolvers (needed for podman) | keithy | feature/podman-setup-script → main | 173d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 466 | fix(setup): generate unique agent key to prevent duplicate conflict | hoangthuc701 | fix/setup-duplicate-agent-key → main | 174d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 461 | feat(memory): add Voyage AI embedding provider | luongndcoder | feat/voyage-embedding-provider → main | 174d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 430 | feat: add Mattermost channel support in web dashboard | ducconit | feat/mattermost-integration → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 417 | perf(agent): optimize prefix cache efficiency and harden inbound securit… | badgerbees | perf/prefix-cache-optimization → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 415 | feat(agent): support thinking tag promotion from assistant response text | badgerbees | feat/thinking-tag-promotion → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 414 | perf(agent): improve accuracy of context pruning by including Thinking a… | badgerbees | perf/accurate-context-pruning → main | 175d | OPEN | MERGEABLE/UNKNOWN | go:SUCCESS,web:SUCCESS | stale (target moved, CI green) | close-as-stale or retarget to dev |
| 412 | fix(feishu): ensure slash commands work in group chats by stripping bot … | badgerbees | fix/feishu-group-slash-commands → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 408 | fix(agent): auto-recover from model context window overflow errors | badgerbees | fix/context-overflow-recovery → main | 175d | OPEN | MERGEABLE/UNKNOWN | go:SUCCESS,web:SUCCESS | stale (target moved, CI green) | close-as-stale or retarget to dev |
| 406 | perf(agent): move context maintenance (memory flush & summarization) to … | badgerbees | perf/non-blocking-compaction → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 403 | fix(zalo): improve inbound identity and resolve image-drop field mismatc… | badgerbees | fix/zalo-inbound-identity → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 402 | feat(xai): add grok catalog update and GA models | badgerbees | feat/xai-grok-catalog-update → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 401 | feat(telegram): add security warning for open-pairing mode | badgerbees | fix/telegram-pairing-security-warning → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 397 | fix: error "unsupported image format: (supported: jpg, png, gif, webp, b… | chungtran4078 | fix/unsupported-image-format → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:SUCCESS,web:SUCCESS | merge-conflict | close-as-stale |
| 393 | fix(telegram): clear draft after materializing DM stream | badgerbees | fix/telegram-duplicate-reply → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 386 | fix(providers): register Ollama provider in-memory and fix double /v1 | hoangthuc701 | fix/ollama-provider-registration → main | 175d | OPEN | CONFLICTING/DIRTY(recheck) | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 380 | feat(i18n): add Japanese (ja) language support | hoangthuc701 | feat/i18n-japanese-pr → main | 176d | OPEN | MERGEABLE/UNKNOWN | go:FAILURE,web:SUCCESS | failing-tests | needs-author + rebase (logs purged) |
| 343 | feat(providers): support Anthropic OAuth setup tokens | kevinle128 | feature/anthropic-token-base → main | 177d | OPEN | CONFLICTING/DIRTY(recheck) | go:FAILURE,web:SUCCESS | failing-tests + merge-conflict | needs-author + rebase (logs purged) |
| 316 | feat: add project-scoped MCP isolation and Projects management | duhd-vnpay | feat/mcp-scope-per-project → main | 178d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 315 | feat: add Party Mode — multi-persona collaborative discussion engine | duhd-vnpay | feat/party-mode → main | 178d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 314 | fix(agent): sanitize runID in uniquifyToolCallIDs for Anthropic compatib… | duhd-vnpay | fix/anthropic-tool-id-sanitize → main | 178d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 196 | feat(channels): add Google Chat channel with Pub/Sub pull and Cards V2 | duhd-vnpay | feat/google-chat-channel → main | 184d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |
| 148 | feat(googlechat): Google Chat channel integration | tuntran | feat/googlechat-channel → main | 188d | OPEN | CONFLICTING/DIRTY(recheck) | none | merge-conflict | close-as-stale |

## Summary table — qkhalk/goclaw (fork)

| PR | Title | Author | Branch → Base | Age | Draft | Mergeable | CI | Failure category | Action |
|---|---|---|---|---|---|---|---|---|---|
| 80 | feat(exec): per-call timeout_seconds + preserve partial output on kill | qkhalk | feat/exec-timeout-resilience → dev | 0d | no | MERGEABLE / UNSTABLE | go PASS, **web FAIL** | inherited lint (not PR-caused) | approve-fixable — fix dev lint, then mergeable |
| 79 | feat(chat): client browser panel, web_browse tool, resizable tabbed layout | qkhalk | feat/client-browser-panel → dev | 0d | no | MERGEABLE / UNSTABLE | **go FAIL, web FAIL** | failing-tests (PR-caused) + inherited lint | needs-author — fix skills regression test, plus dev lint |
| 78 | feat(ask-options): redesign UI with confirm step, expand, one-per-row | qkhalk | feat/ask-options-redesign → dev | 1d | no | MERGEABLE / UNSTABLE | go PASS, **web FAIL** | inherited lint (not PR-caused) | approve-fixable — fix dev lint, then mergeable |

### Which fork PRs block `dev`

All three PRs base on the fork's `dev`. They do not technically block each other (independent heads), but:

1. **`dev` itself is red** (web lint) — every PR merged into dev keeps failing CI until the 3 lint errors are fixed. This is the single highest-leverage fix.
2. **#79 is the only PR with a self-caused failure** (`internal/skills` test). Its branch is also the largest UI change (resizable tabbed layout); merging #78 (ask-options UI) first is likely to conflict with #79 in the chat UI area — expect one rebase between the two.
3. #80 (exec timeout, backend-only) overlaps neither — safe to merge first once dev lint is green.

## Deep-dives (top 5, with log evidence)

### 1. fork #79 — feat/client-browser-panel (qkhalk/goclaw, run 34937449088, 2026-09-15)

- **go FAILURE** (job 104278357316, 10m21s):
  - `--- FAIL: TestBundledSkills_NoRegression (0.11s)`
  - `--- FAIL: TestBundledSkills_NoRegression/ship (0.00s)`
  - `FAIL github.com/nextlevelbuilder/goclaw/internal/skills 23.009s` → exit code 1
  - Interpretation: the branch changes bundled-skill content (web_browse/browser panel ships or touches the bundled `ship` skill), tripping the no-regression guard over bundled skills.
- **web FAILURE** (job 104278357351, 33s): the 3 inherited dev lint errors (see above) — same file/line/rule set as the dev branch run; not caused by this PR's diff.
- Action: needs-author. Fix the bundled-skill regression (or update the no-regression fixture), and fix dev lint separately.

### 2. fork #80 — feat/exec-timeout-resilience (run 34883301715, 2026-09-14)

- **go PASS** (10m6s), release-versioning PASS.
- **web FAILURE** (34s): exactly the 3 dev-branch lint errors (`drive-file-area.tsx:133:11 no-useless-assignment`, `canvas-player.tsx:97:11 no-unused-expressions`, `video-tool-page.tsx:447:31 no-unused-vars '_scenes'`). Line drift on the last one (447 vs 467) is from the PR's own file context, not a different error.
- Action: approve-fixable. No PR-side defect; unblock by fixing lint on `dev`.

### 3. fork #78 — feat/ask-options-redesign (statusCheckRollup)

- **go PASS**, release-versioning PASS, **web FAILURE** (~33s, same duration signature and UNSTABLE state as #80/#79).
- Not log-dived (5-PR cap) but the rollup plus identical failure duration and shared base `dev` make inherited lint the cause with high confidence.
- Action: approve-fixable after dev lint fix.

### 4. upstream #1442 — fix(migrations): auto-create required extensions (run 29438786446, 2026-07-18)

- MERGEABLE, BLOCKED only by the failing `go` job. Author bclermont, opened 2026-07-15 (62d old).
- **go FAILURE** (job 88068532579, 6m25s): 20+ PostgreSQL store tests fail, each in ~0.08–0.15s — a setup/schema failure, not assertion failures. Failing tests include:
  - `TestPGAgentStoreList_NilTenant_FailsClosed`, `TestPGAgentStoreSyncsMonthlyBudgetUsageCap`
  - `TestPGHookStore_CRUD`, `TestPGHookStore_TenantIsolation`, `TestPGHookStore_ResolveForEvent`, `TestPGHookStore_WriteExecution`, `TestPGHookStore_CacheInvalidatedOnWrite`, `TestPGHookStore_CreateHonorsFixedID`, `TestPGHookStore_BuiltinReadOnly`
  - `TestPGRunTimelineStoreAppendAndListBySeq`, `TestPGRunTimelineStoreTenantScope`
  - `TestExportSkillsSelectionIncludesExplicitSystemAndTenantCustom`, `TestExportSkillsDefaultRemainsCustomOnly`
  - `TestPGSnapshotStoreBackfillSnapshotCosts`, `TestPGTracingStoreBackfillLLMCosts`, `TestPGTracingStoreReconcileTraceUsageAggregatesIncludesToolUsageTokens`
  - `TestPGUsageCapStoreReserveUsageIdempotent`, `TestPGUsageCapStoreRejectsCrossTenantRefs`, `TestPGUsageCapStoreUpsertPricingCatalogSkipsInvalidEntries`
- Interpretation: consistent with the PR's own thesis — it changes migration prerequisite handling (auto-create extensions, prevent dirty state), and its migration edits break test-DB bootstrap so every PG store suite dies at schema setup. This is the highest-value upstream fix candidate: recent, mergeable otherwise, well-scoped failure.
- Action: needs-author (fix tests) — then approve-fixable.

### 5. upstream failing-`go` cluster with purged logs — #983, #1012 (+ #955, #813, #537, #393, #386, #380, #343)

- `gh run view 24720823622 --log-failed` (#983) and `24814875789` (#1012) both return `HTTP 410` — April runs, logs beyond GitHub's retention. Same applies to the other April go-failures.
- What remains knowable: the `go` check conclusion is FAILURE on each PR's latest head SHA; #983, #1012, #955, #813, #537, #393, #386, #343 are also CONFLICTING (146–150 days old, target `dev` or `main` which moved far). #380 (167d) is MERGEABLE but its failing run's evidence is gone.
- Action: close-as-stale (or request rebase + re-run if the maintainer values the feature). Re-running CI requires a head push (rebase), which regenerates logs anyway.

## Recommended actions — rollup

| Action | Upstream PRs | Rationale |
|---|---|---|
| approve-fixable (needs maintainer review) | 1173, 1135, 1114, 1088, 1083, 1082 (CI green); 1522, 1276, 1242, 1194, 1181, 1142 (no checks recorded) | mergeable, only blocked on review; the no-checks six should get a head push or maintainer check-run to regenerate CI |
| needs-author (fix tests) | 1442 | mergeable; PG store test suite fails on schema setup (evidence above) |
| needs-rebase | conflicted PRs newer than ~2026-05-18: 1548, 1539, 1538, 1503, 1495, 1452, 1429, 1252, 1224, 1168, 1146, 1129, 1123, 1117, 1116, 1115, 1110, 1109, 1105, 1104, 1101, 1093, 1084, 1072, 1068, 1065, 1063, 1061, 1060, 1059, 1048, 1045, 1044, 1032, 1024, 1013, 991, 988, 987, 981, 980, 976, 971, 965, 955*, 953, 946, 943, 908, 899, 892, 857, 817, 813*, 805, 781, 780, 778, 777, 769, 765, 751, 750, 715, 712, 682, 653, 639, 612, 607, 592, 570, 556, 553, 551, 538, 537*, 533 | (*) also failing `go` — rebase first, then fix whatever still fails |
| close-as-stale | older-than-~120d conflicted PRs incl. the purged-log failing cluster: 1012, 983, 955, 813, 537, 393, 386, 343, 530, 528, 485, 466, 461, 430, 417, 415, 412, 408, 406, 403, 402, 401, 397, 316, 315, 314, 196, 148 | 5–6 months old, conflicted, CI evidence expired |
| needs-author (draft) | 1487, 1166 (clean, mergeable — just finish them), 1079, 1033, 891 (draft + conflicted) | drafts never leave the queue without author action |
| close-as-stale or retarget | 414, 408 (green + mergeable but target `main`), 969, 944 (orphaned stack) | active development is on `dev`; stacks' root PR absent |

## Caveats

- The 25 `UNKNOWN` mergeability values from the bulk list query were stale; all 25 were re-queried individually (22 CONFLICTING, 3 MERGEABLE). The table reflects the re-queried values.
- Fork PRs 80/78 showed `UNKNOWN` in the bulk query; individual views resolved both to MERGEABLE/UNSTABLE.
- "CI green" for very old PRs means the checks on their last pushed head; branches may have been deleted from forks (does not change the reported state).
- Log deep-dive capped at 5 PRs as instructed; #78's web cause was inferred from rollup + identical duration signature, not from logs.
