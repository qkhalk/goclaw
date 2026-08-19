# Phase 7 — Enterprise (AgentKit Deep Integration Plan)

**Status:** IN PROGRESS — Wave 1 (Phases 1–2) SHIPPED
**Created:** 2026-08-19
**Branch:** dev
**Scope decision:** User chose "Toàn diện 8 khối" (all 8 Enterprise blocks).

## Wave 1 shipped (PR #22 → merge `53fe8f20`)

- **Phase 1** (approval persistence + audit completeness) — W1 + W2 merged 2026-08-19, PR #22, merge `53fe8f20`; CI 3/3 green.
  - Approval: PG `000103_approval_requests` + SQLite patch 66; `approval_store.go` interface, `pg/approval_store.go`, `sqlitestore/approval.go` + unit tests; `ExecApprovalManager` in-memory fast-path + best-effort persistence; notif `EventExecApprovalReq/Res` broadcast via msgBus; approver RBAC (tenant scope + role) + idempotent resolve; history via `GET /v1/approvals`.
  - Audit: login/logout + failure events on WS connect / HTTP auth; WS audit now carries `tenant_id`; SQLite audit persistence wired; `GET /v1/activity/export` (CSV/JSONL).
- **Phase 2** (cost governance + observability) — W1 + W2 merged 2026-08-19, same PR, `53fe8f20`.
  - Cost: SQLite `UsageCapStore` (patch 67) + pricing tables; `GET /v1/usage-caps/overview` budget window API + web hook; `warn_at_percent` threshold alerts (`000104`); WS usage pagination SQL-backed.
  - Observability: Prometheus `/metrics` (telemetry.prometheus_enabled + port); SLO tracker wired into 5s flush + burn-rate webhook; `goclaw.llm.cost_usd` span attr in OTel exporter; `go build -tags otel ./...` + vet in CI.
- Schema baseline after Wave 1: PG `RequiredSchemaVersion = 104`; SQLite `SchemaVersion = 67`.
- Verify: Docker `go build ./...` + `-tags sqliteonly ./...` + `-tags otel ./...` + `go vet ./...` pass; PR #22 CI 3/3.
- Wave 2 (Phases 3–4) NOT yet dispatched — requires migrations `000105–000108` + SQLite patches 68–71 per migration table.

## Context

Vision §105 Phase 7 (last phase). 4 scout reports in `reports/`:
- `scout-rbac-tenant-policies.md` — RBAC + tenant policies: 4 fixed roles exist (fail-closed); NO fine-grained/custom roles, NO per-tenant policies (quota/provider allowlist/resource limits/suspension).
- `scout-approval-audit.md` — Approval: exec-only, in-memory, NO `approval_requests` table, notif events declared never broadcast. Audit: `activity_logs` solid (PG+SQLite, tenant_id), ~120 sites; login not audited, WS lacks tenant_id, desktop drops audit, no retention/system UI.
- `scout-cost-observability.md` — Cost governance + observability largely BUILT (usage caps reserve/reconcile, pricing catalog, OTel); gaps: SQLite/desktop parity, no Prometheus, SLO alert unwired, dual cost paths, no budget-threshold alerts, OTel not CI-built.
- `scout-skill-signed.md` — Skill registry DB-backed exists (versions, grants, publish self-serve); NO approval/curation, NO package format/signing (only SHA-256 integrity).

## Confirmed schema baseline

- PG latest migration: `migrations/000102_missions.up.sql`, `RequiredSchemaVersion = 102`
- SQLite: `schema.go` `SchemaVersion = 65` + `schema.sql`

## Phases

| # | Phase | Blocks | Depends on |
|---|---|---|---|
| 1 | ~~Approval queue persistence + Audit completeness~~ ✅ shipped (`53fe8f20`) | approval (persist/notif/RBAC/history) + audit (login, tenant_id, SQLite parity, retention, export) | none |
| 2 | ~~Cost governance gaps + Observability gaps~~ ✅ shipped (`53fe8f20`) | SQLite UsageCapStore, budget overview, threshold alerts, session budget (optional), Prometheus, wire SLO alert, cost-in-OTel, OTel CI build | none |
| 3 | Skill registry approval/curation + Signed packages | skills.status lifecycle (draft→published), approve/reject methods, package format, ed25519 signing + `publisher_keys` trust anchor | none — W2 pending (not dispatched) |
| 4 | Tenant policies + RBAC fine-grained | tenant_policies table (quota/provider-allowlist/limits/suspension), custom roles + role_permissions, per-tenant permission resolution | none — W2 pending (not dispatched) |

Execution: dispatch phase groups in waves (01 then 02 then 03 then 04 per controller review). Controller owns Docker build/test + commit + PR + CI follow + merge to dev. Workstreams own their code + CI.

## Acceptance criteria (all phases)

- [x] Every new table/column: PG migration + SQLite `schema.go` patch + `schema.sql` + version bumps (PG `RequiredSchemaVersion`, SQLite `SchemaVersion`) — verified Phase 1/2: PG `000103/000104`, SQLite patches 66/67, versions 104/67.
- [x] Every new user-facing string: `internal/i18n/keys.go` + `catalog_{en,vi,zh}.go` + web UI locale JSON (en/vi/zh) if UI-visible — verified Phase 1/2 (i18n catalog diffs present in PR #22).
- [x] All user input parameterized (`$N` PG / `?` SQLite). NO concatenation — verified via code review + store tests.
- [x] NOT NULL columns never receive SQL NULL from store layer (audit nil helpers per `go-pro-max` skill) — verified Phase 1/2.
- [x] Tenant-scoped reads/writes fail-closed: `WHERE 1=0` when tenant absent; `WHERE tenant_id = $N` on writes — verified by approval/auth/activity isolation tests.
- [x] `go build ./...` + `go build -tags sqliteonly ./...` + `go vet ./...` pass (controller, in Docker) — verified Wave 1; Wave 2 re-runs on its own merge.
- [x] New store methods covered by unit tests (narrowest first); integration tests where public contract changed — Phase 1/2 store + method tests present.
- [x] Each phase report in `reports/` with Status protocol; plan ticked on merge — Phase 1/2 reports added in PR #22; plan ticked here.
- [x] i18n key ordering: key + 3 catalogs added BEFORE handler code (runtime crash guard) — Phase 1/2 keys landed with catalogs in same commit.
- [x] Reverse-mapping check: every new WS method lands in correct `isReadMethod`/`isWriteMethod`/`isAdminMethod` slice — PR #22 updated router slices + tests.

Wave 2 (Phases 3–4) re-verifies these ACs on its own merge before ticking.

## Migration number assignments (avoid collision)

| Phase | PG migration | SQLite patch |
|---|---|---|
| 1 W1 approval | `000103_approval_requests` | 66 (`approval_requests`) |
| 1 W2 audit | none (activity_logs exists) | none |
| 2 W1 cost | `000104_usage_cap_warn` (warn_at_percent col) | 67 (usage cap tables) |
| 2 W2 observability | none (code-only) | none |
| 3 W1 skill review | `000105_skill_review` | 68 (skill review cols) |
| 3 W2 signed | `000106_publisher_keys` (keys + signature cols) | 69 (publisher keys + signature) |
| 4 W1 tenant policies | `000107_tenant_policies` | 70 (tenant_policies) |
| 4 W2 RBAC | `000108_roles_permissions` | 71 (roles + role_permissions) |

Controller assigns final numbers if workstreams land out of order. Every workstream MUST bump `RequiredSchemaVersion` (PG) and `SchemaVersion` (SQLite) to match its own migration(s).

## Rollback

- All migrations additive-only (new tables/columns, no destructive change to existing rows).
- Down migrations provided for PG where table add; SQLite patches sequential (bump-down not supported — journal notes only).
- If a phase breaks CI: revert the phase's PR (previous verified commit), not silent partial.