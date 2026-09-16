---
name: team
description: >-
  Multi-agent work distribution: split a large task into three or more
  independent workstreams, write precise per-agent briefs, run them with
  delegate or spawn, then merge and reconcile results. Use when a task is too
  big or too parallel for one agent. Keywords: multi-agent, delegation,
  parallel workstreams, sub-agent. Dùng khi cần chia việc lớn cho nhiều agent
  chạy song song rồi gộp kết quả.
license: MIT
version: 1
---

# Team

Turn one large task into at least three independent workstreams, brief each sub-agent precisely, run them concurrently via `spawn`/`delegate`, then merge and reconcile the outputs into one deliverable.

## When to use
- The task splits into genuinely independent parts (modules, screens, doc sections, test suites)
- Latency matters and the parts can run concurrently
- Different parts need different specialist skills

## When NOT to use
- Steps are tightly sequential (B consumes A's output)
- The task is small — delegation overhead exceeds the gain
- Streams would write the same files with no merge strategy — split differently or escalate to `orchestrate`

## Workflow
1. Decompose: list candidate workstreams; verify each has defined inputs, outputs, and a file set disjoint from the others (or an explicit merge strategy).
2. Keep integration for yourself: merging, reconciling, final QA are never delegated.
3. Write a brief per agent: goal, scope (exact paths in and out of bounds), constraints, acceptance criteria, output format, skills to use. One screen maximum, self-contained.
4. Choose the mechanism: `spawn` for independent sub-agent runs, `delegate` to hand a briefed task to a named agent or agent type.
5. Launch all workstreams; journal launch time, agent, and brief reference per stream.
6. While waiting, prepare the merge plan: where outputs combine, which conflicts are likely, what "done" means overall.
7. Collect results (`wait` for spawned runs) and verify each against its brief's acceptance criteria; re-brief once with specifics if a result misses.
8. Merge outputs, reconcile overlaps, then run cross-cutting checks (build, tests, `review`) on the combined result.
9. Close with a final report mapping each workstream to its outcome plus unresolved items.

## Output
One merged deliverable plus a run log: workstreams, briefs, agents used, per-stream result vs acceptance criteria, conflicts found and how they were reconciled.

## Routing
- Conflicts or sequencing start to dominate -> escalate coordination to `orchestrate`
- A stream is itself a full feature -> its brief may invoke `vibe` internally
- Stream work is repo plumbing -> brief agents to use `go-claw-engineer`
- The parent goal needs budget/checkpoint discipline -> `mission` or `goal-warmup`

## Guardrails
- Never give two agents write access to the same file set without a merge strategy
- Briefs must be self-contained — sub-agents cannot see your context
- Cap retries: one precise re-brief per stream, then accept and reconcile manually
- Track time and cost per stream; kill a stream that blows its budget and note it
- You own final quality: sub-agent output is input, not deliverable
