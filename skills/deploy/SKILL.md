---
name: deploy
description: >-
  Deploy services safely: build-migrate-release ordering, zero-downtime patterns, health
  checks, rollback plans, and config management for VMs, Docker, and serverless. Use when
  releasing any service to staging or production. Keywords: deploy, release, rollback,
  zero-downtime, health check, migration. Dùng khi triển khai, phát hành bản build,
  rollback, zero-downtime, kiểm tra health check.
license: MIT
version: 1
---

# Deploy

Run releases as repeatable operations: correct ordering of build, migrate, and release; zero-downtime cutover; verified health checks; and a rollback you have actually documented.

## When to use
- Shipping a service to staging or production (VM, Docker, or serverless)
- Designing the build → migrate → release pipeline order
- Planning zero-downtime cutover or a rollback procedure
- Managing environment config and secrets across deploy targets

## When NOT to use
- Pipeline/observability/incident tooling design → `devops`
- Schema migration content itself → `databases`
- App-framework build specifics → `web-frameworks`

## Workflow
1. Inventory the change: new env vars, new migrations, new endpoints, breaking API changes — each item changes the ordering below.
2. Fix the order: build the artifact first; run backward-compatible migrations (add columns/tables, expand-then-contract); only then switch traffic to the new release. Never ship a release that depends on a migration that has not run.
3. Keep migrations expand-compatible for zero downtime: two-phase deploys (expand now, contract after the old version is retired); no destructive schema steps in the automated path.
4. Pick the cutover pattern per target: VM/systemd → start the new instance, health-check, flip the reverse proxy; Docker/orchestrator → rolling update with readiness gates; serverless → versioned functions with traffic shifting.
5. Define health checks that mean "ready": readiness covers dependencies (DB reachable, migrations current), liveness covers only the process; wire both into the platform.
6. Prepare rollback before deploying: previous artifact retained, a documented one-command revert, and confirmation that the migration path stays compatible with the old version.
7. Manage config: environment-specific values via env/secret store; identical artifact across environments; nothing environment-specific baked into images.
8. Deploy to staging, run smoke checks (health plus one happy-path request per critical endpoint), then promote the same artifact; announce the release window for user-visible changes.
9. Watch the first minutes: error rate, latency, and logs after cutover; trigger the rollback at the pre-agreed threshold, not on gut feeling.

## Output
- A deploy run for the target with: ordering checklist, migration compatibility note, health-check wiring, rollback command, and a post-deploy verification log.

## Routing
- Pipeline, monitoring, alerting, incident process → `devops`
- Migration SQL design → `databases`; code-level pre-merge checks → `ship`
- This repo's Go services → follow repo release conventions, then this checklist

## Guardrails
- No destructive migration steps in the automated deploy path.
- Never store secrets in images, build logs, or environment files in git.
- Rollback must be documented against the current topology; untested rollbacks fail at 2am.
- One artifact promoted through environments; rebuilding per environment invites drift.
