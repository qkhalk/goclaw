---
name: gkg
description: >-
  Build and query a semantic code knowledge graph: extract symbols and relations, store
  and query them, and answer architectural questions that grep cannot. Use when questions
  span many files, callers, or dependencies. Pairs with the knowledge_graph_search tool.
  Keywords: knowledge graph, code graph, architecture, symbols. Dùng khi cần phân tích
  kiến trúc code, quan hệ giữa các module, đồ thị mã nguồn.
license: MIT
version: 1
---

# GKG

Answer architectural questions with a semantic knowledge graph of the codebase — symbols (functions, types, services) and their relations (calls, imports, owns, reads/writes) — beyond what grep and file reads reveal.

## When to use
- "Who calls this?", "What breaks if we change X?", "What depends on module Y?"
- Mapping a subsystem before a refactor or a port
- Finding hidden coupling (shared state, cross-module writes) across many files
- Onboarding summaries: how data flows from endpoint to store

## When NOT to use
- Finding literal strings or one known symbol → `exec` with grep/ripgrep is faster
- Deep runtime behavior (perf, races) → `debug`, `loadtest`
- Document (not code) knowledge graphs → `graphify`

## Workflow
1. Scope the graph: pick the subsystem or entry points in question; extract a whole-repo graph only for repeated architectural queries, otherwise scope to the modules involved.
2. Extract symbols deterministically: run the language's tooling (compiler index, LSP, or tree-sitter-based extractors) via `exec` to list functions, types, and their file:line locations — do not hand-transcribe.
3. Extract relations from evidence: imports/uses from source, call sites from the index, ownership (methods on a type), and data flow (this store is written by these handlers). Record each edge with a file:line citation.
4. Persist the graph where the agent can query it: in goclaw, store nodes/edges so `knowledge_graph_search` can match them; otherwise emit a queryable artifact (JSON/SQL/GraphML) next to the code.
5. Query in the direction of the question: impact analysis walks outgoing edges (what this touches); root-cause analysis walks incoming edges (what touches this); a two-hop cap covers most real architectures.
6. Answer with citations: every "who calls what" claim must carry a file:line from an edge or be marked inferred; re-grep to confirm before asserting load-bearing claims.
7. Detect smells with graph patterns: cycles between packages, god nodes with high in-degree, layering violations (store code imported by UI), and orphaned symbols worth deleting.
8. Maintain incrementally: after changes, re-extract only touched files and their edges; stamp the extraction date so stale graphs are not trusted blindly.

## Output
- A cited answer or map: nodes and relations with file:line evidence, impact/dependency lists for the question asked, and graph smells found (cycles, hotspots, orphans).

## Routing
- Live debugging of runtime behavior → `debug`; performance → `loadtest`
- Document/notes knowledge graphs → `graphify`
- Repo-wide exploration for a feature plan → `architect`, `plan`

## Guardrails
- Every edge needs evidence (file:line); inferred relations must be labeled inferred.
- Static graphs miss reflection, codegen, and dynamic dispatch — flag those blind spots.
- Re-verify load-bearing claims with grep before editing; graphs go stale.
- Extraction is read-only: never modify code as a side effect of building the graph.
