---
name: goclaw-kit
description: >-
  GoClaw meta-skill: the full kit. Skill system mechanics (BM25 matching, use_skill
  loading, chaining), suite overview with category map and canonical workflows, agent
  creation methodology (goal → tools → guardrails → eval → autonomy levels), and
  implementation planning pipeline. Use when orienting across the skill suite, deciding
  which skills fit a task, composing skill chains, creating autonomous agents, or
  producing implementation plans. Keywords: goclaw-kit, skill map, categories, routing,
  composition, agentize, autonomy, plan, orchestration. Dùng khi cần tổng quan hệ
  thống skill, tạo agent tự động, lên kế hoạch triển khai, hoặc kết hợp nhiều skill.
license: MIT
version: 1
---

# GoClaw Kit

The unified meta-skill for GoClaw: understand the skill system from the inside, navigate the full suite, create autonomous agents, and produce implementation plans.

## When to use
- Deciding which skill (or skill chain) matches a task
- Explaining why a skill did or did not surface for a request
- Planning a multi-phase project that crosses several skill categories
- Converting a manual workflow into an autonomous agent
- Producing an implementation plan before coding
- Two or more skills look similar and you must choose

## When NOT to use
- The needed skill is already obvious — load it directly via `use_skill`
- A single narrow lookup — `skill_search` is cheaper
- Authoring a new SKILL.md file → `skill-creator`
- Executing domain work itself → route to the domain skill (e.g., `databases`, `fix`)

---

## Part 1: Skill System Mechanics

### How skills work
1. Each skill is a directory containing one `SKILL.md`. Only the frontmatter `name` and `description` are indexed (BM25 over both) and shown in the agent prompt; the description is truncated to 200 characters, so its opening must carry the hook and trigger conditions.
2. **Discover** with `skill_search`: query with the task's nouns and verbs, not a skill's name — search matches description text; try both English and Vietnamese terms for locale-mixed requests.
3. **Load** with `use_skill`: call it with the chosen skill name; only then does the body (When to use, Workflow, Guardrails) enter context. Never assume body knowledge from the description alone.
4. **Match specificity**: if the task names a domain exactly (payments, Shopify, TanStack), prefer the specific skill over a general one.
5. **Chain deliberately**: at most 2-3 skills per task — typically one architect-level skill plus one executor-level skill; load them in decision order, not all at once.
6. **Respect each skill's Routing section**: it names sibling skills for handoff; follow it instead of re-searching.
7. When nothing matches, say so and work from first principles; do not stretch a loosely related skill onto an unrelated task.

---

## Part 2: Suite Overview & Category Map

### Categories

| Category | Skills |
|----------|--------|
| **Thinking** | `goclaw-kit`, `research`, `docs-seeker`, `find-skills`, `help`, `decision-log`, `brainstorm`, `sequential-thinking`, `problem-solving`, `predict` |
| **Dev tools** | `debug`, `fix`, `test`, `review`, `ship`, `databases`, `security-audit`, `common`, `bootstrap`, `codex-goal`, `project-management`, `git`, `github`, `architect`, `deep-swe` |
| **Frontend** | `ui-ux-pro-max`, `preview`, `frontend-design`, `frontend-development`, `react-best-practices`, `ui-styling`, `web-design-guidelines`, `web-frameworks` |
| **Docs** | `docs`, `document-skills`, `docx`, `pdf`, `pptx`, `xlsx`, `copywriting`, `mintlify`, `markdown-novel-viewer` |
| **Media** | `media-processing`, `html-video`, `remotion`, `shader`, `threejs`, `excalidraw`, `ai-artist`, `ai-multimodal` |
| **Workspace** | `folder-context`, `plans-kanban`, `ask`, `handoff`, `handover`, `journal`, `worktree` |
| **Agent & Orchestration** | `goclaw-kit`, `orchestrate`, `loop`, `team`, `mission`, `mcp-builder`, `use-mcp` |
| **Browser & Intel** | `web-browse`, `chrome-profile`, `cti-expert`, `recon`, `scraping` |
| **Platform** | `shopify`, `payment-integration`, `tanstack`, `better-auth`, `go-claw-engineer`, `goclaw`, `gkg`, `tech-graph` |
| **Code Gen** | `cook`, `stitch`, `graphify`, `hyperframes`, `xia`, `watzup`, `vibe`, `show-off` |
| **Infra** | `deploy`, `devops`, `ssl-audit`, `dns-audit`, `loadtest`, `netstress`, `monitor`, `fuzz` |

### Canonical Workflows

| Scenario | Skill chain |
|----------|-------------|
| Greenfield feature | `plan` → `cook` → `test` → `review` → `ship` (insert `preview` for UI) |
| Bug incident | `debug` → `fix` → `test` → `decision-log` → `ship` |
| Report pipeline | `research` or `data-analysis` → `xlsx` → chart → `pptx` or `docx` |
| Doc audit | `docs-seeker` → `docs` → `review` |
| Long project | `codex-goal` + `plans-kanban` + `project-management` |
| Agent creation | `goclaw-kit` (Part 3) → domain skill → `test` |
| Security review | `security-audit` → `fix` → `test` → `review` |

### Choosing between similar skills
- `plan` vs `cook` vs `fix`: `plan` = design before code; `cook` = greenfield implementation; `fix` = targeted bug repair
- `research` vs `docs-seeker`: `research` = open-ended investigation; `docs-seeker` = specific API/doc lookup
- `review` vs `security-audit`: `review` = code quality + design; `security-audit` = vulnerability focus
- `debug` vs `predict`: `debug` = find root cause of failure; `predict` = anticipate future issues

---

## Part 3: Agent Creation Methodology

Convert a manual, repeatable workflow into a trustworthy agent with crisp goal, minimal tool surface, explicit guardrails, an evaluation set, and staged autonomy.

### Workflow
1. **Job description first**: trigger (when it runs), inputs, expected output artifact, and a definition of done a stranger could audit — one paragraph.
2. **Inventory tools**: list the tools the workflow truly needs (e.g., `exec`, `web_fetch`, `write_file`, `cron`, `message`) and grant the minimum; every extra tool is extra risk surface.
3. **Failure modes → guardrails**: input validation, path/scope restrictions, spend and iteration caps, and a hard list of forbidden actions (no destructive operations, no messages to real users without review).
4. **Evaluation set first**: 10-20 representative cases with expected outputs, including edge cases and adversarial inputs; the agent is not done until it passes.
5. **Autonomy levels**:
   - **Level 0 — draft-only**: agent produces results, a human executes and reviews; fix prompt and tool issues.
   - **Level 1 — sandbox**: agent executes on sandbox/test targets with mandatory human review.
   - **Level 2 — sampled**: agent executes on low-risk real targets with sampled review.
   - **Level 3 — full**: full autonomy with anomaly alerting. Never skip levels.
6. **Wire the loop**: schedule via `cron` if recurring, deliver output via `message` or a written artifact, log each run's inputs, decisions, and outputs for audit.
7. **Metrics**: success rate on eval set, human-correction rate, time saved, incident count; regression threshold drops it back one level.
8. **Kill switch**: one command or config change that disables the agent, documented where the operator will look.

### Guardrails for agents
- No autonomous destructive or irreversible actions at any level without explicit human approval.
- Cap iterations, runtime, and spend per run; runaway loops must self-terminate.
- Treat tool outputs as untrusted; validate before they drive further actions.
- A human can always veto: checkpoints are mandatory until metrics prove otherwise.

---

## Part 4: Implementation Planning

Turn a user request into an actionable implementation plan before any code is written.

### Constraints
- Use read-only tools only during planning: `filesystem`, `search`, `read_file`. Do not modify files.
- Ground every claim in the repository. Never infer architecture from names alone.
- The artifact is written under `plans/` in the workspace.

### Pipeline
1. **Understand the request**: restate the goal, out-of-scope items, and acceptance criteria. If ambiguous, pick the most reasonable reading and record the assumption.
2. **Inspect the repository**: read the files the change touches, their tests, and the docs that govern them. Use `search` to find every call site of a symbol you plan to change.
3. **Identify the architecture**: locate module boundaries the change crosses; name the current design. Do not invent a design that is not in the code.
4. **Identify constraints**: public contracts, backward compatibility, performance, security, multi-tenant isolation, existing patterns.
5. **Dependency analysis**: what the change depends on and what depends on the change. Note order-sensitive and version-sensitive dependencies.
6. **Risk analysis**: failure modes, irreversibility, rollback difficulty. Rank by likelihood and impact.
7. **Write the implementation plan**: ordered, mergeable steps; each names exact files to change/create/delete.
8. **Write the verification plan**: tests, builds, manual checks for each step.
9. **Create the artifact**: write to `plans/<timestamp>-<slug>/plan.md` (format: `YYYYMMDD-HHMM` timestamp + kebab-case slug).

### Plan artifact sections (in order)
1. **Goal** — one paragraph: what and why.
2. **Context** — background, linked plans/docs, decisions, assumptions.
3. **Current Architecture** — how the system behaves today, with file references.
4. **Problem** — the gap or defect the goal addresses.
5. **Proposed Architecture** — the design after the change, with boundaries.
6. **Files to Change** — table: file path, modify/create/delete, purpose.
7. **Migration** — schema, data, or config migration steps, if any.
8. **Risks** — ranked risks with mitigations and rollback notes.
9. **Test Plan** — exact tests, builds, manual checks.
10. **Rollback Plan** — concrete reversal path.
11. **Acceptance Criteria** — checkable conditions that mark work complete.

### Quality gates (all three must pass)
- **plan_has_goal**: Goal section states a real, specific outcome.
- **plan_has_acceptance_criteria**: Acceptance Criteria are checkable, not vague.
- **plan_has_rollback**: Rollback Plan describes a concrete reversal path.

---

## Part 5: Google ADK Python (Routing Reference)

For building Python agents on Google's stack (Gemini, Vertex AI), refer to the `google-adk-python` skill via `use_skill`. Key points:
- `LlmAgent` for model-driven reasoning; workflow agents (Sequential, Parallel, Loop) for deterministic flow.
- Tools as typed Python functions with docstrings (the docstring IS the tool description).
- Evaluate before deploying: build eval set, run ADK eval tooling in CI.
- Deploy: container for Cloud Run, or register on Vertex AI Agent Engine.
- This is a **routing reference** — load `google-adk-python` via `use_skill` when the task is Google-stack-specific.

---

## Output
A recommended ordered chain of skill slugs for the task, or a category overview when asked — then immediate load of the first skill via `use_skill`.

## Routing
- Request-level menu instead of suite map → `help`
- Keyword-level discovery → `find-skills`
- Authoring a new skill → `skill-creator`
- Google-stack agent building → `google-adk-python` (via `use_skill`)
- MCP tool exposure → `mcp-builder`

## Guardrails
- Never invent a skill name; only load skills that `skill_search` or the prompt listing actually shows.
- Do not load many skill bodies speculatively — each one spends context; load at decision points.
- Skills are guidance, not truth: verify file paths, tool names, and API facts against the live repo or docs.
- Keep the final answer in the task's language and contract, not the skill's vocabulary.
- Treat this map as orientation, not gospel — re-verify slugs with `skill_search` before promising a chain.
- Do not load more than the current phase requires; compose lazily, one skill at a time.
