---
name: help
description: >-
  Self-describing help index for the skill suite: how to read skill descriptions, and request
  patterns mapped to suggested skill chains for bug fixes, features, docs, design, and data work.
  Use when the user asks "what can you do", wants an overview of capabilities, or needs a starting
  point for a common workflow. Keywords: help, capabilities, overview, what can you do, skill
  index, workflow menu, huong dan, nang luc. Dùng khi người dùng hỏi bạn làm được gì hoặc cần gợi ý luồng công việc.
license: MIT
version: 1
---

# Help

A menu of what the skill suite can do, and which skills to combine for common request patterns.

## When to use
- The user asks what you can do, asks for help, or wants a capability overview.
- A request is vague and you need a suggested workflow before committing.
- Onboarding a user to the agent's skills for the first time.

## When NOT to use
- The request already matches a known skill — go straight there (use `find-skills` if unsure).
- You need deep detail about one skill — load that skill directly instead.

## Reading a skill description
Each indexed description answers three questions: what the skill does, when it triggers ("Use when ..."), and a short Vietnamese trigger sentence at the end. The first ~200 characters carry the essentials; the full body loads only on demand via `use_skill`, keeping prompts small.

## Request patterns to suggested chains
- Bug fix flow: `debug` (reproduce and hypothesize) → `fix` (patch) → `test` (regression test) → `ship` (commit or PR).
- Feature flow: `plan` (spec and phases) → `cook` (implement) → `test` → `review` → `ship`.
- Doc flow: `research` or `docs-seeker` (gather facts) → `docs` (write) → `docx`/`pdf` office formats if requested.
- Design flow: `ui-ux-pro-max` (design decisions) → `preview` (visual verification) → `review`.
- Data flow: `databases` (query) → `data-analysis` (insight) → `xlsx`/`pptx` (deliverable).
- Security flow: `security-audit` (findings) → `fix` → `test` → `decision-log` (record rationale).

## Workflow
1. Classify the request into one of the patterns above, or answer the overview question with the chain list.
2. State the recommended chain as ordered skill slugs in one line.
3. Load the first skill of the chain with `use_skill` and begin; announce the hand-off.

## Output
A short capability summary or a recommended chain (ordered skill slugs) for the user's request, followed by immediate start of the first skill in that chain.

## Routing
- Vague single-task request → `find-skills` for keyword-level search.
- Big-picture category map or suite composition → `ak`.
- A genuinely missing capability → `skill-creator`.

## Guardrails
- Keep the answer a scannable list, not an exhaustive dump; offer to start the flow.
- Do not invent skill names; only reference skills confirmed via `skill_search` or this index.
