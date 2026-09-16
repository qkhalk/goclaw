---
name: advise
description: >-
  Structured advisory answers: restate the question, lay out options with
  trade-offs, give one recommendation with justification, then risks and
  follow-ups, all with calibrated confidence. Use when the user asks for
  guidance, a recommendation, or a second opinion on a decision. Keywords:
  recommendation, trade-offs, second opinion, advisory, should I. Dùng khi
  cần tư vấn, so sánh lựa chọn, xin ý kiến hoặc khuyến nghị cho một quyết
  định.
license: MIT
version: 1
---

# Advise

Answer advice requests as decisions, not essays: options on the table, one
picked, reasons shown, confidence calibrated.

## When to use
- "Should we...", "which is better...", "what would you recommend?"
- A second opinion is needed on a plan, purchase, library, or approach.
- The user is torn between named options and needs a tiebreaker.

## When NOT to use
- No options exist yet and ideas must be generated → `brainstorm`.
- The question is factual and lookup-able → answer directly or use
  `web_fetch` / `docs-seeker`.
- A full implementation plan is the real ask → `plan`.

## Workflow
1. Restate the question as a decision: "choosing A for situation B, given
   constraints C". If the real decision differs from the asked one, say so.
2. Gather the minimum facts to advise responsibly: read relevant files with
   `read_file`, check versions/pricing/docs with `web_fetch`, or recall
   prior decisions with `memory_search`. Do not advise from vibes.
3. Enumerate 2-4 realistic options, always including the strongest
   counter-candidate (often "do nothing" or "cheapest path").
4. Build the trade-off view: for each option list 2-4 pros/cons that matter
   to THIS user's constraints, not generic ones.
5. Recommend exactly one option and justify it in 2-3 sentences tied to
   their stated constraints and priorities.
6. List risks of the recommendation with early-warning signs, and 2-3
   follow-up actions that de-risk or validate it.
7. Calibrate confidence: state high/medium/low and what evidence would move
   it. Never fake certainty; never hide behind total uncertainty either.

## Output
A compact advisory: Decision restated, Options + trade-offs table,
Recommendation + justification, Risks + follow-ups, Confidence + what would
change it.

## Routing
- The decision needs to be archived with rationale → `decision-log`.
- The chosen option needs a design or plan → `design` or `plan`.
- The recommendation should be stress-tested → `predict`.

## Guardrails
- Recommend one option; a menu without a pick is not advice.
- Distinguish facts, estimates, and opinions in the response.
- If key facts are missing and cheap to get, fetch them before advising
  instead of hedging the answer.
