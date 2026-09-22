---
name: predict
description: >-
  Adversarial pre-implementation review: five expert personas (security, scale,
  UX, ops, cost) stress-test a plan or design and raise concrete failure
  scenarios before any code is written. Use when a design, migration, or plan
  is about to be committed. Keywords: red team, pre-mortem, risk review,
  failure modes. Dùng khi cần phản biện thiết kế, soi rủi ro, hoặc chạy
  pre-mortem trước khi triển khai.
license: MIT
version: 1
---

# Predict

Find the ways a plan will fail while it is still cheap to fix. Five personas
attack the design in turn; their scenarios become a ranked risk register with
mitigations.

## When to use
- A design, plan, or migration is ready and you want failure modes surfaced
  before implementation starts.
- The user asks "what could go wrong", "poke holes in this", or for a
  pre-mortem / red-team pass.
- A change touches security, money, data integrity, or many downstream users.

## When NOT to use
- Code already exists — that is a review, not a prediction → `review`.
- The task is writing tests for implemented behavior → `test`.
- The plan is a rough sketch that first needs idea generation → `brainstorm`.

## Workflow
1. Read the target artifact (`read_file`) and restate its intent, scope, and
   success criteria in 3 lines so personas attack the real thing.
2. For each persona, raise at least 2 concrete failure scenarios phrased as
   events, not vibes ("cache stampede at 10k req/s melts the DB", not
   "scaling may be hard"):
   - Security: abuse, injection, authz gaps, secret exposure, tenant leakage.
   - Scale: hot paths, N+1 queries, unbounded growth, backpressure.
   - UX: confusing states, error recovery, latency perception, mobile.
   - Ops: deploy risk, monitoring gaps, rollback, on-call burden.
   - Cost: egress, token spend, over-provisioning, noisy neighbors.
3. For each scenario note: likelihood (low/med/high), impact, earliest
   signal we would see.
4. Deduplicate overlapping scenarios across personas.
5. Rank by likelihood x impact; keep the top 5-10.
6. For each surviving risk write one concrete, cheap mitigation or a
   deliberate "accept" with rationale.

## Output
A risk register: table of rank, persona, scenario, likelihood, impact,
mitigation. Ends with the 3 risks most worth fixing before implementation.

## Routing
- Risks change the design itself → back to `architect` or `goclaw-kit`.
- Implementation exists and needs inspection → `review`.
- Scenarios should become test cases → `test`.

## Guardrails
- Every scenario must be falsifiable and tied to a named component or flow;
  reject generic warnings.
- Do not block progress: pair every top risk with a mitigation, not just fear.
- State explicitly which personas found nothing — silence is information.
