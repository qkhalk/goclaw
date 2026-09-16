---
name: ak
description: >-
  AgentKit skill-suite overview: the map of the territory. Category map (dev-tools, frontend,
  thinking, docs, media), canonical end-to-end workflows, and how skills compose into pipelines.
  Use when you need orientation across the whole suite, are planning multi-phase work, or must
  choose between several similar skills. Keywords: agentkit, skill map, categories, routing
  table, suite overview, ban do skill, tong quan. Dùng khi cần nhìn tổng thể bộ skill và cách chúng kết hợp.
license: MIT
version: 1
---

# Ak

The routing table and mental model for the full skill suite — which skills exist, how they group, and how they compose.

## When to use
- Planning a multi-phase project that crosses several skill categories.
- Two or more skills look similar and you must choose (e.g. `plan` vs `cook` vs `fix`).
- The user asks how the suite is organized or what a category contains.

## When NOT to use
- The needed skill is already obvious — load it directly.
- A single narrow lookup — `skill_search` is cheaper.

## Workflow
1. Orient with the category map: **thinking** (`plan`, `decision-log`, `research`, `docs-seeker`, `find-skills`, `help`); **dev tools** (`debug`, `fix`, `test`, `review`, `ship`, `databases`, `security-audit`, `common`, `bootstrap`, `codex-goal`, `project-management`); **frontend** (`ui-ux-pro-max`, `preview`); **docs** (`docs`, `document-skills`, `docx`, `pdf`, `pptx`, `xlsx`, `ocr`, `copywriting`); **media** (`media-processing` plus direct tools `create_image`, `tts`, `render_video`); **workspace** (`folder-context`, `plans-kanban`, `workspace-organizing`, `ask`); **browser and intel** (`web-browse`, `chrome-profile`, `cti-expert`).
2. Match the task to a canonical composition: greenfield feature = `plan` → `cook` → `test` → `review` → `ship`, with `preview` inserted for UI work; incident = `debug` → `fix` → `test` → `decision-log` → `ship`; report pipeline = `research` or `data-analysis` → `xlsx` → chart → `pptx` or `docx`; doc audit = `docs-seeker` → `docs` → `review`; long project = `codex-goal` plus `plans-kanban` plus `project-management`.
3. Choose the smallest chain that reaches done; drop stages whose outputs already exist.
4. Load the first skill via `use_skill`, execute its workflow, then hand off to the next slug in the chain.
5. Re-check the map when scope changes mid-project; a phase promotion may need a different skill than planned.

## Output
A recommended ordered chain of skill slugs for the task, or a category overview when asked — then immediate load of the first skill via `use_skill`.

## Routing
- Request-level menu instead of suite map → `help`.
- Keyword-level discovery → `find-skills`.
- Authoring a missing skill → `skill-creator`.

## Guardrails
- Treat this map as orientation, not gospel — re-verify slugs with `skill_search` before promising a chain to the user.
- Do not load more than the current phase requires; compose lazily, one skill at a time.
