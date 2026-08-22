# Wave 1 — Durable Runtime P0 + AgentKit Native P1

**Nguồn:** `GoClaw_Remaining_Work_Completion_Plan.md` (43-phase roadmap) đối chiếu 4 scout reports (`reports/scout-*.md`).
**Kết luận gap:** phần lớn primitive đã có (reliability pkg wired, checkpoint+resume+event store Phase 2, workflow/artifact/kit Phase 3, multi-agent Phase 5, hibernation/time-travel/mission Phase 6, enterprise Phase 7). Còn lại là **integration gaps** — liệt kê dưới đây, mỗi workstream sở hữu file riêng biệt (orchestration-protocol: không parallel edits cùng file).

## Quyết định user (2026-08-21)

1. **Scope Wave 1 = P0 + P1 chung lúc.**
2. **Completion Verifier: config-gated, mặc định advisory** (`reliability.completion_verifier.mode = "advisory" | "recover" | "hard"`).
3. **RunWithFailover: XOÁ** (failover.go, 0 production caller) — enrich `ModelFallbackProvider.runOrdered` thay thế.
4. **Auto-resume sau restart: config-gated, mặc định OFF** (`runtime.auto_resume_interrupted`).

## Workstreams — Stage 1 (P0, song song, disjoint files)

### WS-A Durable Core (phase persistence + snapshot writer + QUEUED + attempt)
Files (exclusive): `internal/store/run_timeline_store.go`, `internal/store/pg/run_timeline.go`, `internal/store/sqlitestore/run_timeline.go`, `internal/store/checkpoint_snapshot.go`, `internal/store/pg/checkpoint_snapshot.go`, `internal/store/sqlitestore/checkpoint_snapshot.go`, `migrations/000109_*`, `internal/upgrade/version.go` (→109), `internal/store/sqlitestore/schema.sql` + `schema.go` (→72), `internal/agent/run_record.go`.
Deliverables:
- A1 Phase-transition persistence: THINKING/WAITING_TOOL/WAITING_PROVIDER(new const)/VERIFYING ghi vào `agent_runs.status` qua runRecordUpdater hook; thêm `AgentRunStatusVerifying="verifying"`, `AgentRunStatusWaitingProvider="waiting_provider"` vào valid set.
- A2 Snapshot writer: gọi `AppendCheckpointSnapshot` trong cả 2 checkpointWriter closures (Run path loop_run.go + ResumeRun path); thread real seq vào `RunPausedPayload.CheckpointSeq` (bỏ hardcode 0). Extend snapshot status set khi cần.
- A3 QUEUED durable: scheduler enqueue tạo `agent_runs` row status `pending` sớm hơn (trước Loop.Run) hoặc mới `queued`; restart sweep nhận diện.
- A4 Attempt++: `UpdateRunStatus`/resume path tăng attempt (thay vì luôn 1); resume giữ attempt cũ +1.
- A5 Seq durability: DB-backed next-seq cho timeline recorder (SELECT MAX(seq)+1 per run khi reserveSeq miss) chống reset-to-1 overwrite sau restart.

### WS-B 429 / Failover Hygiene
Files (exclusive): `internal/providers/failover.go` (XOÁ), `internal/providers/model_fallback.go`, `internal/providers/cooldown.go`, `internal/providers/retry.go`, `internal/reliability/ratelimit.go`, tests tương ứng.
Deliverables:
- B1 Xoá `RunWithFailover` + `FailoverConfig` + helpers chỉ nó dùng; migrate test sang ModelFallbackProvider semantics.
- B2 cooldown.go:219 bridge chuyển tiếp parsed Retry-After (không pass 0).
- B3 runOrdered: record 429 cooldown với real Retry-After + observeSuccess/observeFailure per candidate attempt (health feedback loop đầy đủ).
- B4 RetryDo re-check coordinator/breaker giữa các attempts (abort sớm khi cooldown armed — tránh burn attempts vào armed cooldown).
- B5 Enforce maxPending trong RateLimitCoordinator.Wait (reject-fast khi quá tải thay vì xếp hàng vô hạn).

### WS-C Recovery Engine (classify → policy → action)
Files (exclusive): `internal/pipeline/recover*.go` (mới), `internal/pipeline/think_stage.go`, `internal/pipeline/deps.go`, `internal/reliability/errors.go` (chỉ bổ sung nếu thiếu), `internal/agent/loop_pipeline_adapter.go`, `internal/config/config.go` (reliability block additions), `cmd/gateway.go` (wiring).
Deliverables:
- C1 RecoveryPolicy table: error class → {retryable, max_attempts, backoff, fallback, strategy_switch} bám plan §6 (14 classes).
- C2 Wire vào ThinkStage failure paths hiện có (truncation retries, empty-reply nudges, context-overflow compaction): unified policy thay scattered counters; adaptive retry theo class (429 Retry-After aware, 5xx exp+jitter, timeout adaptive, empty→nudge, malformed→repair, verify→continuation NOT provider-retry).
- C3 Global recovery budget: max_retry_time/max_retry_count/max_recovery_cost config `reliability.recovery.*`.
- C4 ReliabilityError run-context threading: populate RunID/SessionKey/Stage/Attempt tại think-stage + tool-stage error sites (hiện chỉ loop_tools.go:259).
- C5 Weak-model semantic recovery: empty/"..."/invalid JSON/wrong tool/missing arg/repeat/premature-done → detect→classify→repair→revalidate→retry→alternate strategy→fallback model pipeline (tái dùng repairJSON/repairToolCallArgs/observeToolLoop hiện có, thêm policy engine bao ngoài).

### WS-D Watchdog + Reconciliation Worker
Files (exclusive): `internal/agent/watchdog.go` (mới), `internal/agent/watchdog_test.go`, `cmd/gateway_heartbeat.go`, `cmd/gateway_managed.go` (chỉ phần startup wiring watchdog), `internal/store/pg/run_timeline.go` + sqlitestore counterpart (CHỈ thêm reconcile queries — coordinate với WS-A qua contracts trước).
⚠️ Coordination: WS-A sở hữu run_timeline files; WS-D chỉ ĐỌC interface + thêm method mới theo contract WS-A đặt ra (ReconcileStuckRuns). Merge order: WS-A trước WS-D trong cùng train nếu chạm cùng file.
Deliverables:
- D1 Run-level watchdog: no-event-for-N-s, same-tool-repeat, same-output-repeat, budget-increasing-no-artifact → classify slow/stalled/looping/recovering-stuck.
- D2 Recovery ladder: checkpoint→nudge→strategy switch→model fallback→safe fail.
- D3 Reconciliation worker qu định kỳ: RUNNING-no-heartbeat, WAITING_PROVIDER expired, WAITING_TOOL expired, RECOVERING stuck, checkpoint-no-worker → mở rộng RecoverStaleRuns semantics (không mark-failed mù quáng; pause/resume-first).
- D4 Auto-resume post-startup: config-gated OFF (`runtime.auto_resume_interrupted`); ON → drive makeRunResumer cho paused runs sau RecoverInterruptedRuns.

## Workstreams — Stage 2 (P1, dispatch sau khi Stage 1 merge)

### WS-E Completion Verifier Terminal Gate
Files: `internal/agent/completion_verifier.go`, `internal/agent/loop_run.go` (:294 branch + :401 resume path), `internal/agent/loop_pipeline_adapter.go` (verify call site), `internal/agent/run_timeline_recorder.go` (timelineKindForEvent verifying case), protocol events verification.*, i18n keys + 3 catalogs, config mode advisory|recover|hard.
Behavior: mode=hard → chỉ verifier-pass được COMPLETED; mode=recover → fail-first-pass flip ContinueAfterFinal (ContinuationGate mechanism) một lần rồi fail nếu vẫn incomplete; mode=advisory (default) → ghi như hiện tại.

### WS-F Quality Gates Engine + Runtime Tool Permissions
Files: `internal/qualitygate/` (mới: types, evaluator, builtin gates test/build/lint/file/artifact/schema/approval), `internal/skills/loader.go` (chỉ đọc Info.QualityGates/AllowedTools), `internal/agent/loop_pipeline_callbacks.go` (makeAuthorizeToolCall intersect skill AllowedTools), skill metadata → executable gates. `/gc:` commands control-plane (status/runs/doctor/resume/retry/approve) qua extended CommandDispatcher non-skill kind + canned replies.

### WS-G Skill Dependency Resolver + Kit Lockfile
Files: `internal/skills/deps.go` (mới: resolve missing/conflict/cycle), `internal/skills/kit_manager.go` (install/update/rollback + version pin), `.goclaw/kit.lock` project lockfile, `/gc:kit` subcommands.

## Cross-cutting rules

- **Contracts commit đầu tiên** (controller làm ngay, trước dispatch): status consts mới + config structs — mọi WS build trên đó.
- **Dual-DB lockstep**: PG migration + RequiredSchemaVersion bump; SQLite schema.sql + schema.go patch + SchemaVersion bump. Luôn cả hai.
- **i18n**: key mới → keys.go + catalog_en/vi/zh trước handler code.
- **Verification local**: KHÔNG build/test local ngoài unit test nhẹ (go vet/test package-scoped nếu máy có Go); gate chính = GitHub CI trên PR (go + web + release-versioning). Integration tests chạy trong CI.
- **Mỗi workstream agent**: implement → unit test scope hẹp → tự review diff → commit chi tiết tiếng Anh (email qk08082009@gmail.com đã set) → push branch → mở PR → TỰ THEO DÕI CI của PR mình đến green, fix nếu đỏ.
- **Controller**: review diff từng PR, merge tuần tự theo dependency (contracts → A → B/C/D song song tuỳ file-conflict → E/F/G), resolve conflict nếu có.
- **Surface parity**: backend-only trừ khi WS-F thêm `/gc:` commands (cần protocol/methods docs update); UI/web không trong scope Wave 1 trừ reconnect replay (deferred như Phase 2).

## Acceptance criteria Wave 1

- [ ] Contracts landed (statuses + config) trước dispatch.
- [ ] WS-A: phase transitions persist qua restart; snapshots written mỗi durable checkpoint; attempt tăng đúng; seq không reset-overwrite.
- [ ] WS-B: 429 burst → shared cooldown honoured mid-call; Retry-After forwarded toàn bộ đường; RunWithFailover xoá sạch (go doc providers không còn symbol); health feedback từ runOrdered.
- [ ] WS-C: recovery policy table + budget enforce; think/tool failures đi qua unified classifier với run-context đầy đủ.
- [ ] WS-D: watchdog classify stalled/looping; reconciliation worker chạy định kỳ; auto-resume OFF mặc định, ON hoạt động.
- [ ] WS-E: verifier mode=hard chặn COMPLETED sai; recover thử tiếp 1 nhịp; advisory mặc định không đổi behavior.
- [ ] WS-F: quality gates khai báo trong SKILL.md thực thi được (ít nhất test/build/file types); skill allowed-tools enforce runtime tại authorize path.
- [ ] WS-G: kit lockfile pin + verify + rollback; dependency resolver bắt cycle/conflict.
- [ ] Mỗi PR: CI 3/3 green (go, web, release-versioning) trước merge.
- [ ] Regression: go build ./..., go build -tags sqliteonly ./..., go vet ./... xanh trong CI; integration suite xanh.
