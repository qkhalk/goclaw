---
name: agentkit
description: >-
  Meta-skill for the goclaw skill system: how skills are structured, matched by BM25,
  injected into the prompt, and loaded via use_skill — plus choosing and chaining skills
  for a task. Use when deciding which skills fit a task or how to compose them. Keywords:
  skills, SKILL.md, meta, routing, composition. Dùng khi tìm skill phù hợp, kết hợp nhiều
  skill, hoặc hiểu cơ chế nạp skill của goclaw.
license: MIT
version: 1
---

# AgentKit

Understand goclaw's skill system from the inside: what gets indexed, what the model sees, when bodies load, and how to pick and chain skills deliberately instead of guessing.

## When to use
- Deciding which skill (or skill chain) matches a task
- Explaining why a skill did or did not surface for a request
- Improving how `skill_search` and `use_skill` are used together
- Reviewing whether a task should be split across skills or one skill suffices

## When NOT to use
- Authoring a new SKILL.md file → `skill-creator`
- Executing the domain work itself → route to the domain skill (e.g., `databases`)
- Persistent knowledge storage → `vault_search`, `memory_search`

## Workflow
1. Know the mechanics: each skill is a directory containing one SKILL.md. Only the frontmatter `name` and `description` are indexed (BM25 over both) and shown in the agent prompt; the description is truncated to 200 characters there, so its opening must carry the hook and trigger conditions.
2. Discover with `skill_search`: query with the task's nouns and verbs, not a skill's name — search matches description text; try both English and Vietnamese terms for locale-mixed requests.
3. Load with `use_skill`: call it with the chosen skill name; only then does the body (When to use, Workflow, Guardrails) enter context. Never assume body knowledge from the description alone.
4. Match specificity: if the task names a domain exactly (payments, Shopify, TanStack), prefer the specific skill over a general one; general skills shape the approach, specific ones provide the playbook.
5. Chain deliberately: at most 2-3 skills per task — typically one architect-level skill (e.g., `backend-development`) plus one executor-level skill (e.g., `fix`, `test`); load them in decision order, not all at once.
6. Respect each skill's Routing section: it names sibling skills for handoff; follow it instead of re-searching when routing already answers the question.
7. When nothing matches, say so and work from first principles; do not stretch a loosely related skill's workflow onto an unrelated task.
8. Feed lessons back: if a skill's guidance was wrong for a case, note the mismatch in the final report so it can be corrected via `skill-creator` conventions.

## Output
- A short skill plan — which skills were selected and why, the order to load them, and what each contributes — followed by the work executed under their guidance.

## Routing
- Creating or editing SKILL.md files → `skill-creator`
- Decomposing a large task before choosing skills → `plan` or `architect`
- Web/API facts needed while executing → `docs-seeker` or `research`

## Guardrails
- Never invent a skill name; only load skills that `skill_search` or the prompt listing actually shows.
- Do not load many skill bodies speculatively — each one spends context; load at decision points.
- Skills are guidance, not truth: verify file paths, tool names, and API facts against the live repo or docs.
- Keep the final answer in the task's language and contract, not the skill's vocabulary.
