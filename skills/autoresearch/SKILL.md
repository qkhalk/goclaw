---
name: autoresearch
description: >-
  Autonomous research loop: plan, gather, synthesize, gap-check, and iterate
  under an explicit stopping rule until a decision-ready memo with citations
  emerges. Use for multi-source questions that need breadth and convergence,
  such as build-vs-buy, library selection, or market scans. Keywords:
  deep research, literature scan, compare options, cited memo. Dùng khi cần
  nghiên cứu tự động nhiều nguồn, tổng hợp và hội tụ thành bản ghi nhớ có
  trích dẫn để ra quyết định.
license: MIT
version: 1
---

# Autoresearch

Run research as a controlled loop, not an open-ended crawl: every iteration
either answers a question or proves a gap, and the loop must end in a
recommendation someone can act on.

## When to use
- Multi-source questions where the first search result is not enough.
- Comparisons that need evidence: tools, vendors, algorithms, pricing.
- The user asks for a researched memo, report, or "figure out the best X".

## When NOT to use
- One doc page answers it → `docs-seeker` or a single `web_fetch`.
- You only need to write the assignment, not run it → `research-prompt`.
- Purely local codebase questions → `memory_search`, `vault_search`, or
  reading the repo directly.

## Workflow
1. Plan: restate the decision to be made; list 3-7 research questions;
   name candidate source types; set the stopping rule up front (max 3-4
   iterations, or stop when no question has a promising new source).
2. Gather: for each question, fetch sources with `web_fetch` / `web_browse`,
   and local context with `vault_search` or `memory_search`. Record notes as
   claim + source URL + date. One claim, one citation.
3. Synthesize: group notes by question; write a 2-4 sentence answer per
   question; note conflicts between sources explicitly.
4. Gap-check: which questions are unanswered, answered only by weak sources,
   or contradictory? If any remain and the stopping rule allows, iterate with
   targeted queries for those gaps only.
5. Stop when the rule triggers or a new iteration would re-fetch known
   material. Never end mid-loop silently — say which rule fired.
6. Write the memo: recommendation first, then evidence per question, then
   confidence level (high/medium/low) with the reason, then open questions.

## Output
A decision-ready memo: Recommendation, Key evidence (with citations), Why not
the alternatives, Confidence + rationale, Open questions. Every factual claim
carries a source link.

## Routing
- Drafting the assignment before this loop → `research-prompt`.
- The decision needs formal recording → `decision-log`.
- Implementation plan follows the decision → `plan` or `architect`.

## Guardrails
- Never state a claim without a source or an explicit "unverified" label.
- Prefer primary sources (official docs, changelogs, code) over blog
  summaries; note source dates and discard stale ones.
- Respect the stopping rule; report residual gaps instead of hiding them.
