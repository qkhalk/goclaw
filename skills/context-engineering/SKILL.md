---
name: context-engineering
description: >-
  Manage an LLM's context deliberately: decide what to include, summarize, or
  drop; order items by priority; prefer retrieval over stuffing; budget tokens;
  and evaluate whether the context actually enables the task. Use when
  assembling prompts or agent contexts, or when long sessions degrade quality.
  Keywords: prompt context, token budget, context window, retrieval, memory.
  Dùng khi cần chọn lọc và sắp xếp ngữ cảnh cho LLM, tiết kiệm token, hoặc
  cải thiện chất lượng prompt dài.
license: MIT
version: 1
---

# Context Engineering

Context is a budget, not a landfill. Every token must earn its place by
changing what the model would otherwise do.

## When to use
- Building a prompt, system message, or agent context by hand.
- A long session shows drift: forgotten constraints, repeated questions,
  instructions losing to chit-chat.
- Preparing delegated work (e.g. via `delegate`) that must carry exactly the
  right background.

## When NOT to use
- The task fits in a short prompt with no curation needed.
- Writing new feature code — this shapes inputs to models, not code.

## Workflow
1. Inventory: list everything you could include — task spec, constraints,
   code excerpts, history, retrieved docs, tool results, examples.
2. Budget: estimate the token cost of each item and set a ceiling well under
   the model's window; leave room for the response and tool outputs.
3. Classify each item: LOAD-BEARING (changes the answer if removed),
   HELPFUL (reduces error risk), NOISE (feels relevant, is not).
4. Compress: summarize NOISE and long history into a few factual lines;
   replace big code dumps with the relevant function signatures and line
   ranges; prefer retrieval via `memory_search`, `vault_search`, or
   `skill_search` over pasting entire corpora.
5. Order by priority: role and hard constraints first, task spec next,
   load-bearing context, then helpful background, then history. Most
   important content goes to the top and the very bottom.
6. Resolve instruction conflicts with an explicit hierarchy: system rules
   beat task instructions beat stylistic preferences; state precedence
   instead of hoping the model infers it.
7. Evaluate: ask "could a fresh agent answer correctly from this context
   alone?" Test by re-reading with fresh eyes or probing a subagent; fix
   gaps and repeat once.

## Output
The curated context (or the edits to an existing one) plus a one-paragraph
rationale: what was dropped, what was summarized, and the estimated budget.

## Routing
- Packaging the context as a research or task assignment → `research-prompt`.
- Retrieval sources need curating first → `vault_search`, `memory_search`.
- Structural design of the surrounding agent → `architect`.

## Guardrails
- Never drop hard constraints (security, tenant scope, non-negotiables) to
  save tokens.
- Summaries must preserve facts, numbers, and decisions verbatim; a smooth
  but lossy summary is worse than a terse accurate list.
- Bigger context is not better quality; measure whether changes improve task
  outcomes, not fullness.
