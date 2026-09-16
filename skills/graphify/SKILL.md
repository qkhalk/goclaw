---
name: graphify
description: >-
  Turn documents and notes into a knowledge graph: extract entities and relations, link
  them wikilink-style, update incrementally, and query for recall and synthesis. Use when
  notes, docs, or transcripts should become connected, queryable knowledge. Keywords:
  knowledge graph, entities, relations, wikilinks, vault. Dùng khi chuyển tài liệu, ghi
  chú thành knowledge graph, liên kết thực thể, truy vấn tổng hợp kiến thức.
license: MIT
version: 1
---

# Graphify

Convert documents and notes into a navigable knowledge graph: entities, typed relations, and wikilink-style connections — built incrementally so it stays queryable as knowledge grows.

## When to use
- A corpus of notes, docs, or transcripts should become cross-linked, queryable knowledge
- Questions like "how do these concepts relate?" or "summarize everything about X"
- Merging new documents into an existing graph without duplicate entities
- Preparing connected context for retrieval (`vault_search`, `memory_search`)

## When NOT to use
- Code symbols and relations → `gkg` (code-specific extraction)
- One-off document summarization → just `read_document` / `research`
- Collecting web material → `research` first, then graphify the distilled notes

## Workflow
1. Define the domain schema lightly: 5-15 entity types (person, project, decision, incident, product) and relation types (relates-to, caused, supersedes, owned-by); let types emerge from the corpus, then fix them.
2. Extract per document: list entities with normalized names (one canonical form; keep aliases), then relations as subject → typed predicate → object, each with a source citation (doc + section).
3. Resolve identity before inserting: search existing entities (`vault_search`, `memory_search`, or the graph store) for the same real-world thing; merge into the canonical node and add the alias instead of creating a near-duplicate.
4. Link with wikilinks where prose benefits: write `[[Entity]]` references inside notes so the text stays human-readable and the graph stays derivable; prefer many small notes over one monolith.
5. Write synthesis nodes for clusters: after ingesting a batch, create hub notes (per project, per incident) that summarize and link participants — these make recall fast.
6. Update incrementally: new documents only add nodes/edges and bump timestamps on touched entities; superseded facts get a `supersedes` edge rather than a silent edit, preserving history.
7. Query by traversal, not just keyword: start from the asked entity, walk one to two hops, and synthesize from the subgraph; cite the underlying documents for every claim.
8. Keep it healthy: run periodic passes to merge duplicates found by alias matching, prune empty nodes, and flag contradictions (relations that cannot both hold) for human review.

## Output
- A populated graph (entities with types and aliases, typed relations with citations), hub/synthesis notes, and answers to target queries with document-level citations.

## Routing
- Code knowledge graphs → `gkg`
- Storage and search tools → `vault_search` / `vault_read` / `memory_search` / `knowledge_graph_search`
- Collecting the raw material first → `research`, `scraping`, `docs-seeker`

## Guardrails
- Every relation cites its source document; uncited edges are guesses — mark or drop them.
- Merge entities conservatively; a wrong merge corrupts downstream answers.
- Do not ingest confidential documents into shared stores without checking access scope.
- Contradictions are data: surface them, never silently overwrite the older fact.
