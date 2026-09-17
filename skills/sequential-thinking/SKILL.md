---
name: sequential-thinking
description: >-
  Step-by-step reasoning discipline with explicit revision and branching: keep
  a thought ledger, know when to backtrack, and avoid premature convergence on
  hard analysis. Use when a problem has many interdependent steps, confusing
  constraints, or an earlier conclusion that keeps feeling wrong. Keywords:
  chain of thought, backtrack, revise, reasoning steps. Dùng khi cần suy luận
  từng bước, suy nghĩ lại một kết luận, hoặc rẽ nhánh dòng suy nghĩ cho bài
  toán phức tạp.
license: MIT
version: 1
---

# Sequential Thinking

Make reasoning inspectable: number every thought, revise out loud, branch when
evidence splits, and converge only when the ledger is consistent.

## When to use
- Multi-step analysis where one wrong early assumption poisons the rest.
- The user asks to "think step by step" or the task keeps producing
  contradictory conclusions.
- Debugging causal chains, tricky math/logic, or architectural trade-offs.

## When NOT to use
- Simple lookups, single-file edits, or direct tool invocations.
- The answer is mechanical and a checklist suffices.

## Workflow
1. Write the question and the success criterion at the top of the ledger
   ("thought 0") so every later step can be checked against it.
2. Take ONE reasoning step per thought: state what you now believe, the
   evidence for it (file, line, doc, measurement), and what remains open.
3. After each step, verify it: does it contradict an earlier thought or the
   success criterion? If yes, do not push forward.
4. Revise explicitly: mark the old thought as superseded, write the new one,
   and say what evidence changed. Never silently rewrite conclusions.
5. Branch when two readings of the problem are both plausible: develop each
   branch for 2-3 thoughts, then keep the branch that survives evidence and
   record why the other was cut.
6. Backtrack when a downstream step invalidates an upstream premise; return
   to the earliest poisoned thought, not the last one.
7. Converge only when every open item is resolved or explicitly listed as an
   assumption. Then state the conclusion and the 2-3 load-bearing steps.

## Output
The conclusion, a compact ledger summary (steps kept, revisions made, branches
cut and why), and any remaining assumptions the user should confirm.

## Routing
- The conclusion is a candidate solution needing alternatives → `brainstorm`.
- The analysis exposes failure modes of a design → `predict`.
- The problem is finding a root cause in a live system → `debug`.

## Guardrails
- Never skip verification to reach a tidy conclusion; a consistent wrong
  ledger is worse than an honest open one.
- Cap branch exploration; if both branches still look equal, say what
  evidence would decide between them instead of guessing.
- Do not present a revision as if it were the original plan.
