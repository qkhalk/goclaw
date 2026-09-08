# Agent Evaluation Harness

Deterministic, store-level behavioral suites for GoClaw. Where a unit test
pins an implementation detail, an eval pins a user-visible behavior — "another
user's memory must never surface for me" — and reports a per-suite score so
regressions are visible at a glance.

## Run

```bash
# Start the eval database (same as integration tests) once:
docker run -d --name pgtest -p 5433:5432 -e POSTGRES_PASSWORD=test -e POSTGRES_DB=goclaw_test pgvector/pgvector:pg18

make eval          # run all suites, print score table
make eval-list     # dry-run: list suites and case counts
```

Or directly (DSN resolution: `--dsn` > `$TEST_DATABASE_URL` > localhost:5433):

```bash
go run . eval run
go run . eval run --suite memory-isolation --failures-only
```

Run from the repo root so `evals/` and `migrations/` resolve (or pass
`--dir`/`--migrations`). Migrations are applied automatically. The exit code
is non-zero when any case fails.

## Suites

| Suite | Driver | Pins |
|-------|--------|------|
| `memory-isolation` | memory | memory-fabric identity gates: user/agent/session/tenant isolation, global ring visibility, supersession, dedup, multi-value whitelists |
| `run-state-machine` | resume | durable run contracts: checkpoint survives status transitions, idempotent create, stale-run recovery, interrupted-timeline reconciliation |
| `tool-security` | security | shell deny patterns (pre-execution), unicode-obfuscation bypass, workspace path boundary |

## Anatomy of a case

```yaml
suite: memory-isolation
driver: memory
cases:
  - name: user-scope-no-cross-user-leak   # unique within the file
    severity: P0                           # P0 = blocking, P1 = warn
    seed:                                  # write memories first
      memories:
        - user: alice                      # logical name, made unique per run
          scope: user
          kind: preference
          content: "alice-secret: favorite language is Go"
    act:
      search: { user: bob }                # retrieve as another identity
    expect:
      not_contains: ["alice-secret"]       # the leak assertion
    positive:                              # control: the memory IS retrievable
      search: { user: alice }              # for its owner — else the negative
      contains: ["alice-secret"]           # passes vacuously
  ```

The `positive` control is what makes a negative assertion trustworthy: a case
can only pass "bob sees nothing" when "alice still sees it" also holds.

Memory identities (`alice`, `agent-a`, `sess-one`) are logical names — the
driver suffixes them per run, so repeated executions are hermetic and need no
manual cleanup.

## Adding cases

1. Add a case to the relevant YAML in `evals/<category>/` (or a new file —
   discovery is `evals/*/*.yaml`).
2. For a new behavior family, add a scenario/case shape in
   `internal/eval/driver_*.go`. Drivers run the real stores and the real tool
   implementations — never mocks; a mock here would only test itself.
3. `make eval-list` to validate the YAML, then `make eval`.

## Found bugs

The harness pays for itself — bugs caught so far, all in code paths that had
zero coverage before:

1. `SearchMemories` multi-value `Scopes`/`Kinds` whitelists AND-chained into a
   contradiction (always-empty result). Fixed with `IN (...)` in
   `internal/store/pg/memory_fabric.go` and
   `internal/store/sqlitestore/memory_fabric.go`; pinned by
   `scope-whitelist-multi-value`.
2. `SearchMemories` selected a computed `score` column (21 columns) but the
   shared memory scanner scans 20 — every retrieval errored with
   `expected 21 destination arguments`. Fixed by ordering on the SQL
   expression and computing `Score` in Go (`store.MemoryRecallScore`);
   pinned by every memory-isolation case.
3. `WriteMemory` dedup lookup computed the `kind` placeholder BEFORE appending
   the tenant filter, so `kind = $N` pointed at the tenant UUID — PostgreSQL
   failed every tenant-scoped write with `operator does not exist: text =
   uuid`, SQLite silently matched nothing (dedup/supersession never ran).
   Fixed in both stores; pinned by every memory-isolation seed write.
4. Deny-pattern gaps in `internal/tools/shell_deny_groups.go`:
   `wget -qO- … | bash` escaped the `-O -`-shaped pattern, and
   `chmod -R 777 /` escaped the flagless `chmod MODE /` pattern (the command
   then EXECUTED until timeout — the first eval run surfaced it). Fixed with
   flag-tolerant patterns; pinned by eval cases and
   `internal/tools/shell_deny_regression_test.go`.

The security driver asserts deny cases by the policy-deny message, not by any
error — a timeout is a FAIL, because it means the command actually ran.
