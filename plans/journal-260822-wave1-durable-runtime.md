# Wave 1 — Durable Runtime (P0) + AgentKit Native (P1) — Session Journal

Date: 2026-08-21 → 2026-08-22 · Repo: goclaw-mod (qkhalk/goclaw) · Controller: ox-alpha

## Outcome

Wave 1 shipped complete: 7 PRs merged to `dev`, all CI-gated (go + web + release-versioning), zero local-build gating.

| PR | Workstream | Content |
|---|---|---|
| #24 | Contracts | `AgentRunStatus{Thinking,WaitingTool,WaitingProvider,Verifying}`, `CompletionVerifierConfig` (advisory/recover/hard), `RecoveryConfig`, `RuntimeConfig.AutoResumeInterrupted` + `EffectiveVerifierMode()` |
| #25 | WS-B | Deleted `RunWithFailover`; shared failover types restored in `model_fallback.go`; `RecordFailureRetryAfter` bridge; `runOrdered` observe/record429 with real Retry-After; `RetryDoFor` mid-loop admission check |
| #26 | WS-C | `pipeline/recover.go` RecoveryPolicy table + RecoveryEngine; ThinkStage wiring (preserves 3/2 defaults); gateway budget wiring via `SetRecovery` |
| #27 | WS-D | Run-level watchdog (`watchdog.go`: stalled/looping/recovering-stuck/slow verdicts, 5-rung ladder, nil-safe), watchdog tests (10 cases), D3 pause-preferred reconciliation in heartbeat sweep, D4 opt-in auto-resume bounded to 2 |
| #28 | WS-E | Verifier terminal gate: hard ⇒ failed + localized reason + verification.failed event; recover ⇒ one-shot ContinueAfterFinal flip; advisory byte-identical; resume-path parity; timeline `verifying` reachable; `SetCompletionVerifier` plumbing |
| #29 | WS-F | `internal/qualitygate` engine (test/build/lint/file/artifact/schema/approval, fail-closed registry); skill allowed-tools enforcement at authorize gate (group expansion, legacy aliases, deferred-MCP bypass blocked); `/gc:status|runs|doctor|approve` control-plane kinds via CommandDispatcher2 canned replies |
| #30 | WS-G | `skills/deps.go` resolver (closure, missing-chain, conflicts, cycles, ==/>= pins); `.goclaw/kit.lock` byte-stable load/save + drift verify; KitManager InstallFrom/Update/Rollback |

Docs commit `0a5c2dbd` on dev: roadmap ticks (12 P0 + 4 P1 + 4 P2 items) + plan/scout artifacts.

## What worked

- **Worktree-per-agent** (round 2 fix): exclusive file ownership per workstream eliminated the round-1 checkout-clobber disaster.
- **Commit+push-per-deliverable**: survived 6 agent deaths (5 LLM-stream failures, 1 zombie). Every takeover resumed from remote, never from IRC claims.
- **CI-as-only-gate**: no local build/test policy held; all 7 PRs went green through GitHub Actions.
- **Interface assertions for cross-WS needs**: `pausedRunLister` / `CommandDispatcher2` optional interfaces let workstreams compile against dev without editing shared files.

## What hurt

- **Zombie agents lie by omission**: WS-D claimed "committing NOW" three times over ~70 min while its worktree stayed clean. Fix that worked: ultimatum + verify disk state (not chat claims) + takeover threat produced the push in minutes.
- **LLM stream failures killed 3 WS-G and 2 WS-F agents mid-write**, always during long silent file generation. Mitigation for attempt N+1: whole-file writes only (no multi-hunk edit patches — those corrupted files twice), small-step dispatch (one file per agent), push after every green build. WSGLock5's "types → append → append → test → push" cadence was the first to survive.
- **Edit-tool boundary echoes**: two of my own edits auto-repaired badly (truncated function tails). Always re-read the touched region after a warning.
- **Leftover dirty files in main checkout** from round 1 (WS-C sessionKey edits) were stale duplicates of merged code — stashed then dropped after confirming content already on dev.

## Reusable decisions

- Verifier modes: advisory default = zero behavior change (asserted in test); recover flips `Observe.ContinueAfterFinal` with its own one-shot marker so it composes with ContinuationGateFired.
- Pause-before-fail: stale runs holding checkpoints transition to `paused` before `RecoverStaleRuns` terminal-fails them — resume capability survives crashes.
- Skill allowed-tools: narrow at execution gate, keep provider flag on the un-narrowed policy set; lazy MCP activation re-checked against skill scope.
