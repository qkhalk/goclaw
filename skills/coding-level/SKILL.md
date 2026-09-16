---
name: coding-level
description: >-
  Assess the engineering level of a codebase or contribution: quality signals,
  complexity hotspots, and test maturity, scored into a level 1-5 rating with a
  growth roadmap. Use when evaluating code quality, onboarding onto an
  unfamiliar repo, or deciding where refactoring effort pays off most.
  Keywords: code quality audit, maturity model, tech debt, refactoring
  priorities. Dùng khi cần đánh giá chất lượng code, mức độ trưởng thành của
  codebase, hoặc lên lộ trình cải thiện.
license: MIT
version: 1
---

# Coding Level

Grade a codebase or a contribution on observable signals, not taste, and turn
the grade into a short, ranked growth roadmap.

## When to use
- Before joining, inheriting, or bidding on work in an unfamiliar repo.
- A contribution needs an objective quality assessment.
- The team argues about quality and needs a shared scale and evidence.

## When NOT to use
- Reviewing one diff for correctness against its own intent → `review`.
- Checking behavior against a spec — that is `test` territory.
- Estimating effort or scheduling work → `goclaw-kit`.

## Level scale
1. Script: works on the happy path, no tests, monolithic, ad-hoc naming.
2. Structured: clear modules and naming, some error handling, few tests.
3. Tested: meaningful test coverage on core paths, consistent style,
   documented interfaces, failures handled deliberately.
4. Hardened: observability (logs, metrics, traces), graceful degradation,
   performance-aware hot paths, CI enforced.
5. Productized: versioned APIs, migration discipline, security reviews,
   docs and onboarding that strangers can follow.

## Workflow
1. Survey structure with `list_files`, then read entry points and the 3-5
   largest or most-changed files with `read_file`. Note the layout logic.
2. Sample quality signals: naming consistency, error handling on I/O paths,
   duplication, dead code, magic numbers, comment honesty.
3. Find complexity hotspots: deeply nested conditionals, functions over
   ~80 lines, god files, tangled imports, copy-pasted blocks.
4. Grade test maturity: presence, what they assert (behavior vs implementation),
   flakiness signs, coverage of failure paths, not just happy paths.
5. Score each dimension 1-5 with at least two cited examples per score.
6. Produce the overall level (median, not best dimension) and a growth
   roadmap: the top 3 improvements ranked by payoff-to-effort, each with a
   concrete first step.

## Output
Level report: per-dimension table (dimension, score, evidence), overall level,
top-3 growth roadmap with first steps.

## Routing
- Fixing the top hotspot → `fix` for the concrete change, `test` to lock it in.
- Formal inspection of a specific diff → `review`.
- Documenting findings for the team → `docs`; recording the decision to
  invest → `decision-log`.

## Guardrails
- Every score needs evidence (file and line references); no vibes-based
  grades.
- Style differences are not defects unless they raise maintenance cost.
- Do not recommend rewrites by default; the roadmap should start from the
  highest-payoff incremental step.
