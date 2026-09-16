---
name: research-prompt
description: >-
  Draft self-contained research briefs — context, questions, sources, success
  criteria, and deliverable format — so any agent or model can execute the
  research without asking follow-up questions. Use when delegating research or
  before launching an autonomous research run. Keywords: research brief,
  prompt specification, delegation, study design. Dùng khi cần viết bản tóm
  tắt yêu cầu nghiên cứu để giao cho agent khác thực hiện.
license: MIT
version: 1
---

# Research Prompt

A research brief is a contract: whoever receives it — a subagent via
`delegate`, another model, or a human — can execute it with zero additional
input and produce exactly the deliverable you pictured.

## When to use
- Before handing research work to a subagent or another session.
- The user describes an information need vaguely ("look into X for me").
- Repeated research tasks that should become a reusable template.

## When NOT to use
- You can answer directly from the current context or one lookup.
- The research will run in a loop with gap-checking → that executor is
  `autoresearch`; you are writing its input.
- The need is a single doc lookup → `docs-seeker`.

## Workflow
1. Clarify intent: what decision or artifact will this research serve? If the
   stated goal is ambiguous, ask once via `ask_options` with concrete
   interpretations.
2. Write context: background facts the executor cannot discover (project,
   stack, constraints, prior findings), each stated as a claim you verified.
3. Write research questions as answerable units: specific, bounded, and
   prioritized must-know vs nice-to-know. Convert vague questions ("is X
   good?") into decidable ones ("does X meet criteria A, B, C?").
4. Suggest sources and method: doc sites, GitHub issues, changelogs, vendor
   pages, version constraints, recency window, and what NOT to trust.
5. Define success criteria: what a complete answer contains, what makes a
   source acceptable, and how confidence should be expressed.
6. Define the deliverable format: structure, length, comparison-table shape,
   citation style, and the decision the output should enable.
7. Self-test the brief: pretend you are a stranger with no other context.
   Execute it mentally; fix any point where you would have to guess.

## Output
A single markdown brief with sections: Goal, Context, Questions (prioritized),
Sources and method, Success criteria, Deliverable format, Constraints.
Ready to paste into `delegate` or hand to `autoresearch`.

## Routing
- Executing the brief in an iterative loop → `autoresearch`.
- One-shot investigation by a helper agent → pass to `delegate` or `research`.
- The questions target library/API specifics → `docs-seeker`.

## Guardrails
- No unbounded questions ("everything about X"); every question needs an
  answerable shape.
- Include versions and dates in context; research without a time window
  returns stale answers.
- Never encode the desired conclusion into the brief; specify criteria, not
  verdicts.
