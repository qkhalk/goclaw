# Memory & Knowledge

GoClaw agents remember across messages, sessions and weeks through a 3-tier
memory system, a knowledge graph and a document vault. All tiers are
per-agent, tenant-scoped and searchable through one unified query path.

## The 3 tiers

| Tier | Name | Contents | Lifetime |
|------|------|----------|----------|
| L0 | Working | Current session history, with token-based auto-compaction for long conversations | Session |
| L1 | Episodic | Per-session summaries created after each run | Default 90 days (configurable) |
| L2 | Semantic | Knowledge graph: entities and relations extracted by an LLM | Long-term |

### L0 — working memory

- The conversation history lives in the session; when it grows past token
  thresholds it is **auto-compacted** (summarized) to keep runs inside the
  context window.
- Before compaction, a **memory flush** stage persists important content to
  long-term storage, so nothing is lost to summarization.
- Relevant long-term memories are **auto-injected** into the system prompt
  with a ~200 token budget (per-agent `memory_config.auto_inject_max_tokens`,
  default on, relevance threshold 0.3). Each injected entry is a short
  abstract; the agent can pull the full memory with `memory_expand(id)`.
- Retrieval is hybrid: BM25 keyword scoring blended with embedding (vector)
  similarity.

### L1 — episodic memory

- After every run, the episodic worker summarizes the session into an
  episodic entry (created from the `session.completed` domain event).
- Each entry also gets a ~50 token **L0 abstract** for fast auto-injection.
- Search is hybrid: PostgreSQL full-text search (GIN-indexed
  `search_vector`) blended with cosine-similarity vector search over
  episodic embeddings.
- Entries expire after **90 days** by default — tunable per agent via
  `memory_config.episodic_ttl_days`.

### L2 — semantic memory (knowledge graph)

- An LLM extracts **entities and relations** from sessions into a graph.
- Duplicate entities are detected and can be merged (dedup scan with
  reviewable candidates).
- The graph supports traversal queries, so an agent can answer questions
  that span many past sessions.

## Event-driven consolidation

Consolidation runs on the [DomainEventBus](../architecture) — no polling:

```
session.completed → episodic summary (L1) → KG extraction (L2) + dreaming
```

The **dreaming worker** consolidates unpromoted episodic summaries into
long-term fact documents: once **5 or more** unpromoted summaries accumulate
for an agent, an LLM synthesizes durable facts from them. Runs are debounced
per agent (default 10 minutes) and both values are overridable per agent via
`memory_config.dreaming` (`threshold`, `debounce_ms`, `enabled`).

## Knowledge Vault

The vault is a document registry above the memory stores:

- **Documents** with metadata, categories and content, registered from uploads
  or a **filesystem sync** (rescan detects new/changed files).
- **Incremental indexing** — content is hashed with SHA-256, so unchanged
  documents are never re-processed.
- **[[wikilinks]]** between documents, resolved and traversable in both
  directions.
- **LLM enrichment worker** — classifies, summarizes and auto-links documents
  in the background (status and stop endpoints available).
- **Graph view** over the link structure.

### Unified search

Vault search blends three sources with fixed weights:

| Source | Weight |
|--------|--------|
| Vault documents | 40% |
| Episodic summaries | 30% |
| KG entities | 30% |

## API surfaces

WebSocket methods:

| Method | Purpose |
|--------|---------|
| `memory.write` | Write a memory |
| `memory.get` | Fetch a memory by ID |
| `memory.search` | Hybrid search |
| `memory.supersede` | Replace a memory with a newer version |
| `memory.archive` | Archive a memory |

HTTP endpoints (see [HTTP API](../api/http) for auth and request shape):

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/agents/{id}/episodic` | List episodic summaries |
| `POST /v1/agents/{id}/episodic/search` | Search episodic memory |
| `GET /v1/agents/{id}/kg/entities` | List KG entities |
| `GET /v1/agents/{id}/kg/graph` | Fetch the graph |
| `POST /v1/agents/{id}/kg/traverse` | Traverse from an entity |
| `POST /v1/agents/{id}/kg/extract` | Trigger extraction |
| `GET /v1/agents/{id}/kg/stats` | Graph statistics |
| `POST /v1/agents/{id}/kg/merge` | Merge duplicate entities |
| `GET /v1/agents/{id}/kg/dedup` · `POST .../kg/dedup/scan` · `POST .../kg/dedup/dismiss` | Deduplication workflow |
| `/v1/vault/documents` · `/links` · `/search` · `/graph` · `/tree` · `/upload` · `/rescan` | Vault registry, wikilinks, search, graph, FS rescan, upload |
| `GET /v1/vault/enrichment/status` · `POST /v1/vault/enrichment/stop` | Enrichment worker control |

Per-agent scoped variants exist under `/v1/agents/{id}/memory/...` and
`/v1/agents/{id}/vault/...`; `/v1/memory/documents` lists across agents.

## Agent tools

Agents use these built-in tools (see [Tools](./tools)):

- `memory_search` — hybrid search over memories
- `memory_get` / `memory_expand` — fetch a memory or expand an injected abstract
- `knowledge_graph_search` — query the entity graph
- `vault_search` / `vault_read` — search and read vault documents

## Web UI

- **/memory** — episodic browser and long-term memory documents.
- **/knowledge-graph** — entities/relations graph, manual extraction and
  dedup review.
- **/vault** — documents, wikilinks, graph view and search.

::: warning Lite edition
The desktop Lite edition disables the knowledge graph and vector search
(`KGEnabled: false`, `VectorSearch: false`). Episodic memory stays available
with lexical (non-vector) search on SQLite.
:::
