---
name: research
description: "Run a structured technical investigation and deliver a decision-ready report: define scope, gather sources, compare options, state trade-offs and risks, recommend. Use whenever the user asks to research, evaluate, compare, or decide between technologies, tools, libraries, hosting, APIs, or architectures — 'nghiên cứu giúp anh', 'so sánh X và Y', 'nên dùng gì', research, evaluate, compare options, pick a stack, feasibility check, due diligence. Also use before any non-trivial build decision the user asks to justify. Do NOT use for quick factual lookups (answer directly), for scraping datasets (use the scraping skill), or for library API details (use the docs-seeker skill)."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - topic
  - constraints
outputs:
  - research_report
allowed-tools:
  - web_search
  - web_fetch
  - filesystem
  - exec
quality-gates:
  - scope_defined
  - sources_triangulated
  - recommendation_stated
---

# Research (technical investigation)

Turn "help me decide" into a short, honest, decision-ready report — with the
trade-offs stated and a recommendation the user can act on.

## Process

1. **Scope first (1–2 sentences, in the report).** Restate the question,
   constraints (budget, self-host vs SaaS, language/ecosystem, scale), and what
   "good enough" means. If the constraints are unknown and material, ask the
   user 2–3 focused questions (`ask_options` works well) before burning time.
2. **Gather with triangulation:** at least 3 independent source types —
   official docs/changelogs, real user experience (issues, forums, reviews),
   and benchmarks/comparisons. Use `web_search` for discovery and `web_fetch`
   to read the promising hits. Discard SEO-farm content; prefer primary
   sources. Note the publication date — tooling claims older than ~2 years are
   suspects.
3. **Compare on stated criteria**, not vibes: capability fit, maturity
   (releases, maintainer activity, issue latency), operational cost (run cost,
   learning curve, migration path), lock-in, and license. 3–5 candidates max —
   deep-compare the plausible ones, list the rejected in one line each with the
   rejection reason.
4. **State risks honestly:** what breaks under scale, what the vendors'
   marketing hides, what you could NOT verify. An honest "unknown" beats a
   confident guess.
5. **Recommend.** One primary choice + one runner-up, each with the condition
   under which it stops being the right answer ("X if you self-host; Y if you
   want managed"). If no option clearly fits, say so and show what a good
   option would look like.

## Report format

- **Verdict** (2–3 sentences, recommendation + the why in one line)
- **Comparison table** (criteria × candidates, short cells)
- **Key risks & unknowns** (bulleted)
- **Sources** (URLs + dates)
- Everything else the user didn't ask for: cut. Total under ~500 words unless
  the user asked for depth.

## Rules

- Every factual claim in the comparison table traces to a source in the list.
- Numbers (pricing, limits) get their date noted — they rot fast.
- The user's language sets the report language; keep tool/product names in
  original form.
- If sources conflict, show the conflict instead of silently picking a side.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Search results are all marketing pages | Search for "<tool> vs" / "<tool> problems" / site:reddit.com or site:news.ycombinator.com for practitioner signal |
| Docs and reality disagree | Trust the repo (issues/changelog over docs); note the discrepancy |
| Nothing current found | Check the project's changelog/releases page directly via web_fetch |
| Topic too broad | Split into sub-questions, answer the decision-critical one first |
