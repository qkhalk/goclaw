---
name: orchestrate
description: >-
  Coordination layer above team execution: sequence workstreams, enforce
  shared conventions, arbitrate parallel outputs, and resolve conflicts when
  parallel edits collide. Use when multi-agent work needs sequencing,
  contracts, or a tie-breaker. Keywords: orchestration, arbitration, merge
  conflicts, conventions, dieu phoi. Dùng khi cần điều phối nhiều agent, xử
  lý xung đột và thống nhất quy ước chung.
license: MIT
version: 1
---

# Orchestrate

Sit above a team of working agents: define sequencing and shared conventions before anyone starts, then arbitrate outputs, resolve collisions, and keep the combined result coherent.

## When to use
- Parallel workstreams share files, interfaces, or conventions
- A `team` run keeps producing conflicts or inconsistent style
- Outputs must pass one quality bar before integration
- Sequencing matters: some streams gate others

## When NOT to use
- Streams are fully independent — plain `team` is cheaper
- Single-agent work — no arbitration needed
- You also want to implement — conflict of duty; delegate the hands

## Workflow
1. Write the coordination contract before any agent starts: shared conventions (naming, file ownership map, interfaces), the dependency graph between streams, and the integration order.
2. Assign file ownership: every file has exactly one owning stream; everyone else reads or coordinates.
3. Sequence streams by the graph; launch only streams whose inputs already exist.
4. Freeze interface contracts (APIs, schemas, shared types) for the run; changing one requires re-briefing dependents.
5. Collect outputs at defined checkpoints rather than only at the very end.
6. Arbitrate: review each output against the contract; verdicts are accept / fix-with-notes / reject-with-reason.
7. Resolve collisions: when two streams edited the same area, the contract decides the winner (not who finished last); reconcile manually and record the ruling.
8. Update the conventions doc when a ruling generalizes; keep one living file for the run.
9. Integrate in the declared order, running cross-cutting verification (build, test, `review`) after each integration step.
10. Publish the run log: contract, rulings, and the final integrated state.

## Output
A coordination contract file, a rulings log (conflict -> decision -> rationale), and the integrated result with integration-order evidence (build/test runs after each merge step).

## Routing
- Setting up the streams themselves -> `team`
- One area keeps conflicting — serialize that area instead of arbitrating forever
- Architecture authority in this repo -> consult `architect` rulings
- Post-run reflection on process -> `retro`

## Guardrails
- Contracts before code: never let agents start on an unwritten convention set
- One arbiter: if you arbitrate, you do not implement in the same run
- Rulings are written, never verbal — unwritten rulings get re-litigated
- Freeze interfaces mid-run; churn there multiplies across every stream
- Timebox arbitration debates; a reversible decision now beats a perfect one late
