---
name: find-skills
description: >-
  Discover the right skill for any task using skill_search: craft keyword queries, read relevance
  scores, and chain multiple skills into a workflow. Use when unsure which skill applies, when a
  task needs a capability you have not loaded, or when composing multi-skill pipelines. Keywords:
  discover, find skill, search skills, which skill, capability lookup, route task, skill chain,
  tim skill, ky nang phu hop. Dùng khi không chắc skill nào phù hợp với công việc hiện tại.
license: MIT
version: 1
---

# Find Skills

Locate the best-matching skill for a task by searching the skill index with `skill_search`, then load it with `use_skill`.

## When to use
- The task maps to a capability but no skill is obviously loaded.
- The user says "is there a skill for X" or names a domain (PDF, debugging, UI) without picking one.
- You suspect a multi-skill chain (e.g. research, then plan, then implement, then test) and need the exact slugs.
- A previously chosen skill failed and you need an alternative approach.

## When NOT to use
- You already know the exact skill slug and it is loaded — just use it.
- The task is simple tool work (one `exec` or `read_file`) with no reusable method behind it.

## Workflow
1. Extract 3-6 core keywords from the task: the domain (pdf, database, ui), the action (debug, plan, convert), and any artifacts mentioned (chart, report).
2. Call `skill_search` with the keyword string. If results are weak, retry once with synonyms or a different abstraction level ("presentation" becomes "slides pptx"; "slow query" becomes "database performance index").
3. Read the returned `description` fields; match trigger conditions to the task, not just name similarity. A skill named "ship" is about releasing, not boats.
4. Pick the single closest skill. If two overlap, prefer the more specific one and check its Routing section for precedence.
5. Load it with `use_skill`, then follow its workflow. If its body names a prerequisite skill, load that first.
6. For multi-phase tasks, note the chain explicitly (e.g. `research` then `plan` then `cook` then `test`) and keep it in your progress notes so resumable work stays consistent.

## Output
A loaded skill plus a one-line statement of why it was chosen; for chains, the ordered list of skill slugs with the current position marked.

## Routing
- No result scores well → fall back to `help` for a request-pattern index, or `ak` for the full category map.
- The chosen skill underperforms and a genuinely reusable new skill is justified → `skill-creator`.

## Guardrails
- Never fabricate a skill slug; only use slugs returned by `skill_search` or listed in `help` / `ak`.
- Load only the skills the task needs; each loaded body costs context.
- Prefer domain skills over generic ones — a specific skill beats a broad one for quality.
