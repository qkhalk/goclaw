---
name: devops
description: >-
  Design DevOps practices: CI/CD pipeline structure, containerization, observability with
  logs, metrics, and traces, alerting rules, infrastructure-as-code basics, and incident
  response runbooks. Use when setting up pipelines, monitoring, or incident process.
  Keywords: CI/CD, Docker, observability, alerting, IaC, runbook. Dùng khi setup CI/CD,
  Docker, monitoring, cảnh báo, quy trình xử lý sự cố.
license: MIT
version: 1
---

# DevOps

Stand up the engineering practices around a service: a CI/CD pipeline with real gates, containers, observability that answers questions, alerting that pages only when needed, and incident runbooks.

## When to use
- Designing or fixing a CI/CD pipeline and its quality gates
- Adding observability: structured logs, metrics, traces, dashboards
- Writing alert rules that fire for real incidents, not noise
- Containerizing a service or starting infrastructure-as-code
- Preparing or improving incident response

## When NOT to use
- The mechanics of a specific release/rollback → `deploy`
- Load or traffic stress scenarios → `loadtest`, `netstress`
- Security posture review → `security-audit`

## Workflow
1. Map the delivery path: commit → build → test → package → deploy → verify; decide which gates block (build, unit, lint, integration) and which only warn.
2. Keep CI fast and cached: cache dependencies and artifacts, run unit tests before integration tests, block only on meaningful coverage — a slow pipeline gets bypassed.
3. Containerize deliberately: small base images, multi-stage builds, non-root user, pinned versions, one process per container; pass config via env, never bake secrets into layers.
4. Add observability in the order it pays back: structured logs with correlation IDs → red metrics (rate, errors, duration) per endpoint → traces across the request path; one dashboard per service that answers "is it healthy".
5. Write alerts from user pain: page on symptoms (error rate, latency, saturation vs SLO) not causes (CPU spikes); every alert links a runbook; delete alerts nobody acted on.
6. Adopt IaC incrementally: declare compute, DNS, and queues in code with review-by-PR; keep state remote and locked; no console changes that drift from code.
7. Prepare incident response: severity levels, on-call rotation, a comms template, and per-service runbooks (symptom → checks → mitigation → escalation); rehearse once with a game day.
8. Run post-incident reviews blamelessly: timeline, root cause, and action items with owners; track the actions like bugs, not suggestions.

## Output
- Pipeline definition, container/IaC changes, a metrics-and-alert plan (metric → threshold → severity → runbook link), and an incident runbook skeleton per service.

## Routing
- Executing a specific release → `deploy`
- Monitoring a live service in this repo → `monitor`
- Performance/load investigation → `loadtest`, `netstress`; security → `security-audit`

## Guardrails
- Never put secrets in pipeline logs or container layers.
- Alerts must link to a runbook; unactionable alerts train people to ignore pages.
- Do not change infrastructure by hand where IaC exists; drift is an incident seed.
- Keep blast radius small: staged rollouts and feature flags beat heroic fixes.
