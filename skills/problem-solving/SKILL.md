---
name: problem-solving
description: >-
  Six systematic problem-solving techniques — first principles, inversion,
  5-whys, abstraction ladder, analogy, constraint relaxation — with a mapping
  from problem type to technique. Use when a problem is stuck, recurring, or
  the obvious approach keeps failing. Keywords: first principles, 5 whys,
  inversion, root cause, abstraction. Dùng khi cần giải quyết vấn đề khó, tìm
  nguyên nhân gốc, hoặc cách tiếp cận thông thường đang bế tắc.
license: MIT
version: 1
---

# Problem Solving

Pick the technique that matches the problem type, apply it with discipline,
then cross-check the answer with one different technique.

## When to use
- The same bug or failure keeps coming back despite fixes.
- Every proposed solution feels like a patch over a symptom.
- The problem statement is vague, or all obvious options are blocked.

## When NOT to use
- The cause is visible and the fix is a one-line change.
- The task is execution, not diagnosis.

## The six techniques and when each fits
1. First principles — for stale assumptions. Decompose the problem into
   verified facts ("what do we actually know, and how?"), rebuild the
   solution from those facts alone.
2. Inversion — for avoiding failure. Ask "how would I guarantee this fails?",
   list those failure paths, then treat each as a thing to prevent.
3. 5-whys — for recurring incidents. Ask "why" iteratively until the cause is
   a process or design flaw, not a person or a typo. Stop when another "why"
   would leave the system's scope.
4. Abstraction ladder — for wrong altitude. If drowning in detail, climb up:
   "what broader goal does this serve?" If the statement is vague, climb
   down: "what concrete instance shows the pain?"
5. Analogy — for novel domains. Find a mature domain that solved a structurally
   identical problem; copy the shape of its solution, not its surface.
6. Constraint relaxation — for "impossible" situations. Drop one constraint
   (budget, compatibility, deadline) hypothetically; if a good solution
   appears, that constraint is the real problem to negotiate.

## Workflow
1. Write the problem as a one-sentence gap: current state vs desired state.
2. Classify it: recurring incident, stale assumption, all-options-blocked,
   wrong-altitude, or novel-domain. Use the mapping above to pick one
   technique.
3. Apply the chosen technique in writing, step by step, citing evidence
   gathered via `read_file`, `exec`, or `web_fetch`.
4. Formulate the candidate solution, then stress-check it with one different
   technique (usually inversion).
5. State the smallest experiment or inspection that would confirm the cause
   before a full fix is built.

## Output
Root-cause or solution statement, the technique used and why it fit, the
reasoning trail, and the confirming next check.

## Routing
- Root cause found in running code → `debug` for the fix.
- Many solution candidates now exist → `brainstorm` to compare, `goclaw-kit` to
  sequence the work.
- The chosen design needs adversarial checking → `predict`.

## Guardrails
- Stop 5-whys at the system boundary; do not blame individuals.
- One technique applied thoroughly beats all six applied shallowly.
- Verify claimed facts before building on them; an unverified "fact" at the
  foundation poisons first-principles work.
