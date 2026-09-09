---
name: loadtest
description: Use when asked to measure how much traffic a website or HTTP API can handle before launch (capacity planning, pre-production load testing, finding the breaking point, soak testing). Drives HTTP Layer-7 load with wrk/hey, ramps until the latency/error knee, correlates with server-side saturation, and delivers a go-live capacity report with SLO verdicts. Only for targets the requester owns or is authorized to load test.
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - target
  - profile
outputs:
  - capacity_report
allowed-tools:
  - filesystem
  - shell
  - search
  - exec
quality-gates:
  - authorization_confirmed
  - slo_defined
  - capacity_report_generated
deps:
  - system:wrk
  - system:hey
  - system:curl
---

# Load Test (HTTP, Layer 7)

Capacity testing for an HTTP(S) service before it goes to production: find
how many requests per second it sustains, where latency degrades, and what
breaks first — with numbers, not vibes.

## Authorization gate (mandatory, first)

Load testing can take a service down. Before generating ANY load, confirm:

1. **Ownership or written authorization** for every target host. If the
   target is shared infrastructure (a SaaS you use, a hosting provider's
   shared tier, any system the requester does not control), do not load test
   it — the acceptable alternative is a read-only single-user benchmark of
   your own client, or testing a clone in your own environment.
2. **Environment** — staging/pre-production preferred. Production load tests
   only with explicit confirmation that degraded availability during the
   window is acceptable, and ideally off-peak.
3. **Ceilings agreed up front**: max concurrent clients, max duration per
   step, and the abort conditions below. Write them down before starting.
4. **Stop conditions** (abort the run immediately, no exceptions):
   - HTTP error rate ≥ 5% sustained for 10s,
   - p95 latency ≥ 4× the SLO target,
   - target becomes unreachable (connection refused/timeout on the health
     check),
   - anyone from the requester side says stop.

## Define the test before running it

Record, and get the requester to confirm or default sensibly:

- **Target endpoint(s)**: full URLs, method, headers, body. Include one
  cheap endpoint (health/static) and at least one representative real
  workload endpoint (the page/API most users hit).
- **SLO**: e.g. p95 < 300ms and error rate < 0.1% — without an SLO a load
  test has no verdict.
- **Profiles** to run (in this order):
  1. `smoke` — 1 client, 15s. Proves the endpoint works and warms caches.
  2. `baseline` — 10 concurrent, 60s. Records unloaded latency.
  3. `ramp` — step up concurrency (10→25→50→100→200→400), 60s per step,
     10s cooldown between steps. Finds the knee.
  4. `soak` — 70% of the knee's concurrency for 30–60 min. Catches leaks,
     GC pressure, connection-pool exhaustion. Only if the requester wants it.

## Tooling

Pick the first available; they measure the same things:

```sh
# wrk — best latency detail (Lua body/headers via -s if needed)
wrk -t4 -c50 -d60s --latency https://<host>/api/endpoint

# hey — simpler, good error breakdown (ab fallback: ab -n 20000 -c 100 <url>)
hey -z 60s -c 50 https://<host>/api/endpoint

# health check used by stop conditions (run in parallel from another shell)
while true; do curl -s -o /dev/null -w '%{http_code} %{time_total}\n' \
  https://<host>/healthz; sleep 2; done
```

Record per step: RPS achieved, p50/p95/p99 latency, error count by class
(connect/read/HTTP status), socket errors. wrk prints a latency distribution;
hey prints a status-code histogram — capture both.

## Correlate with server side

Numbers without server telemetry guess at the bottleneck. While the ramp
runs, sample on the server (or ask the requester to watch their dashboard):

- CPU % (user vs sys), load average — `top -bn1 | head -15`
- Memory / swap — `free -m`
- Connection counts — `ss -s` and `ss -tn state established '( dport = :443 )' | wc -l`
- For Go services: pprof endpoint if exposed; for Postgres: `pg_stat_activity`
  connection count vs `max_connections`.

Note which resource saturated at the knee — that is the bottleneck sentence
of the report ("DB connections exhausted at 320 rps, CPU still 40%").

## Report

Write `loadtest-report.md`:

- Verdict table: endpoint × profile → peak sustainable RPS @ SLO, knee point,
  bottleneck resource, SLO pass/fail.
- Ramp curve as text: per-step line `c=<concurrency> rps=<n> p95=<ms> err=<%>`.
- Soak section (if run): latency drift over time, memory growth, errors.
- Reproduction: exact commands per profile.
- Recommendations: the 2–3 highest-leverage fixes (e.g. raise DB pool +
  add p95-based autoscaling trigger at 70% of knee).

## Rules

- Never skip the smoke profile — a broken endpoint under load wastes the
  whole window and can look like a DDoS in the target's logs.
- Honor stop conditions mechanically; a "failed" run that found the knee is
  a successful test.
- One variable at a time: don't change concurrency and duration and endpoint
  between steps of the same ramp.
- Clean up: ensure all generators exited (`pgrep wrk hey ab` empty) before
  declaring the run finished.
