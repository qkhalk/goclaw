---
name: xia
description: >-
  Extract a feature from another repository and port it into the target
  codebase: study the upstream implementation, map concepts, plan the port,
  adapt to local idioms, and verify behavioral parity. Use when borrowing a
  proven feature. Keywords: port, extract feature, upstream, parity, chuyen
  giao tinh nang. Dùng khi cần lấy một tính năng từ repo khác về và chuyển
  vào dự án hiện tại một cách đúng chuẩn.
license: MIT
version: 1
---

# Xia

Bring one feature from an upstream repo into this codebase: understand the original deeply, map its concepts onto local idioms, port it as native code (never a transplant), and verify the behavior matches.

## When to use
- "Implement X like repo Y does" — with Y accessible (local clone or public)
- Modernizing a feature from a legacy project into the current stack
- Re-implementing an upstream change with deliberate local adaptations

## When NOT to use
- The feature exists as a maintained library — prefer adding the dependency
- Licensing forbids derivative work
- Only the idea is needed — copy the concept, not the code

## Workflow
1. Get the upstream source: clone it or pack the relevant slice (see `repomix`); identify the feature's file set and entry points.
2. Trace the upstream feature end-to-end: inputs, core algorithm, side effects, outputs, config surface, and its tests.
3. Build the concept map: for each upstream concept (types, functions, tables, events), find the local equivalent or decide to create one; write the mapping down.
4. Plan divergences by design: local store layer, error handling, logging, security, and i18n conventions differ — adapt, do not transplant.
5. Port plan: ordered file-level steps in the target repo, each with the test that proves its behavior; get review if the change is large.
6. Implement natively: write code as if for this repo (local patterns, naming, error style) — paste-and-patch is forbidden.
7. Port or write tests for each upstream behavior; encode every intentional divergence in a test so it cannot silently regress.
8. Verify parity: run local tests; where feasible, run the same fixture through both implementations and compare outputs.
9. Document: a short port note — upstream source (repo + commit), concept map, intentional divergences, parity evidence.

## Output
A native implementation with tests in the target repo, plus a port note: upstream reference (repo + commit), concept mapping table, intentional divergences with rationale, and parity test evidence.

## Routing
- Upstream repo is huge — pack the relevant slice first with `repomix`
- Target repo is this Go gateway -> implementation guidance via `go-claw-engineer`
- Large ports need phasing -> `plan` first, `cook` to implement, `test` to verify
- The port adds schema changes -> `databases`

## Guardrails
- Respect upstream licenses; record attribution in the port note
- Adapt, never transplant: local conventions win over upstream style
- Every intentional divergence is written down — silent divergence is a bug factory
- Extract the minimal feature core; do not port unused machinery
- Record the upstream commit hash for every studied file; future diffs depend on it
