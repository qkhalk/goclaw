---
name: google-adk-python
description: >-
  Build AI agents with the Google Agent Development Kit for Python: agent types, tools,
  multi-agent hierarchies, evaluation, and deployment to Vertex AI Agent Engine. Use when
  creating, testing, or reviewing Python agents on the Google ADK stack. Keywords: ADK,
  Google, Vertex AI, LlmAgent, tools, evaluation. Dùng khi xây dựng AI agent Python với
  Google ADK, Vertex AI Agent Engine.
license: MIT
version: 1
---

# Google ADK Python

Design, evaluate, and deploy Python agents using Google's Agent Development Kit — from a single `LlmAgent` with tools to multi-agent hierarchies running on Vertex AI Agent Engine.

## When to use
- Building an agent on the Google stack (Gemini models, Vertex AI)
- Composing multiple specialized agents into workflows (sequential, parallel, loop)
- Adding tools: function tools, built-in tools, or third-party/MCP tools
- Preparing an agent for evaluation and managed deployment

## When NOT to use
- Model-agnostic agent routing inside goclaw → `agentkit` and the repo's own agent loop
- Prompt writing quality for content → `copywriting`; API facts → `docs-seeker`
- Non-Python or non-Google stacks — verify the target runtime first

## Workflow
1. Pick the agent type: `LlmAgent` for model-driven reasoning; workflow agents (Sequential, Parallel, Loop) for deterministic control flow; custom `BaseAgent` only when neither fits.
2. Write the agent instruction as a system contract: role, rules, tool-use policy, output format; keep it versioned next to the agent code.
3. Define tools as typed Python functions with docstrings — the docstring is the tool description the model reads; return serializable values; keep side effects explicit.
4. Compose hierarchies: give each sub-agent a narrow job and its own instruction; pass state explicitly; avoid deep chains (prefer two levels until proven necessary).
5. Wire session state and memory: session state for in-conversation facts, long-term memory for cross-session recall; make PII handling explicit.
6. Evaluate before deploying: build an eval set of real tasks with expected tool calls and final answers; run the ADK eval tooling in CI; gate regressions on eval results.
7. Trace with callbacks (before/after model and tool calls) to inspect traffic; fix prompt or tool-description issues the traces reveal.
8. Deploy: container for Cloud Run, or register on Vertex AI Agent Engine for managed sessions; pin model versions; keep config in environment variables, not code.
9. Review: token budgets per step, max-iteration limits on loops, and a human handoff path for low-confidence outputs.

## Output
- A runnable ADK agent module (agent + tools + instruction file), an eval set with pass criteria, and a deployment note (target, env var names, model versions).

## Routing
- Exposing tools via the Model Context Protocol → `mcp-builder`
- Multi-agent orchestration concepts in goclaw → `agentkit`; workflow spec → `plan`
- Testing discipline and harnesses → `test`

## Guardrails
- Never hardcode API keys; read them from environment or a secret manager.
- Bound every loop agent with a max-iteration limit and a termination condition.
- Treat tool outputs as untrusted input to the model; validate before downstream actions.
- Ship with an eval set; an agent without evaluation is a demo, not a feature.
