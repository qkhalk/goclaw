---
name: databases
description: "Work with PostgreSQL (and MongoDB) safely from the agent: inspect schemas, write and validate queries, read EXPLAIN plans, design indexes, review migrations. Use whenever the user asks to query/analyze data in a database, design or change a schema, fix a slow query, understand a table's structure, write SQL/Mongo aggregation, plan an index, or review a migration — 'truy vấn giúp anh', 'viết câu SQL', 'chậm quá xem giúp', query the database, schema design, slow query, index, migration review. Triggers: SQL, PostgreSQL, Mongo, query, truy vấn, bảng, schema, index, EXPLAIN, migration. Read-only by default: any write (INSERT/UPDATE/DELETE/DDL) requires explicit user confirmation of the exact statement."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - database_task
outputs:
  - query_or_schema
allowed-tools:
  - exec
  - filesystem
quality-gates:
  - read_only_default
  - writes_confirmed
  - plans_checked
deps:
  - system:psql
---

# Databases (PostgreSQL-first, safety-first)

Query, inspect, and optimize databases through the host's SQL client. Read
freely; write only with explicit confirmation.

## Connection

Discover how to connect before querying — never guess credentials:
```bash
# ask the user for a DSN, or check env the app already exposes:
env | grep -iE 'DATABASE_URL|POSTGRES' | sed 's/:[^:@]*@/:***@/'
```
Then: `psql "$DSN" -c '\conninfo'` to verify. Flag exposed credentials back to
the user if found in plaintext env files.

## Read operations (default mode)

```bash
psql "$DSN" -c '\d+ table_name'                                  # schema + sizes + indexes
psql "$DSN" -P pager=off -c 'SELECT ... LIMIT 50;'               # always LIMIT ad-hoc reads
psql "$DSN" -c "EXPLAIN (ANALYZE, BUFFERS) SELECT ...;"          # plans with real timing
```

- **Always LIMIT** exploratory reads; aggregate server-side instead of
  shipping rows to the chat.
- **EXPLAIN before optimizing:** a slow query gets `EXPLAIN (ANALYZE, BUFFERS)`
  first — read the plan for Seq Scan on large tables, row-estimate vs actual
  mismatches (>10x ⇒ stale statistics → `ANALYZE table`), and sorts spilling
  to disk. One fix per iteration, re-plan after.

## Index guidance

- Index the join/filter/ORDER BY columns that the plan shows as bottlenecks;
  composite order = equality columns first, range last.
- Verify usage after creation (`EXPLAIN` again or `pg_stat_user_indexes`).
- Every index costs write throughput — say the trade-off when proposing.

## Writes (confirmation mandatory)

1. Draft the exact statement.
2. Show the user: the statement, affected-row estimate (`SELECT count(*)`
   with the same WHERE), and rollback plan.
3. Execute **only** after explicit approval. For bulk changes wrap in a
   transaction and verify the affected-row count against the estimate before
   `COMMIT`; for anything riskier, dump affected rows first
   (`CREATE TABLE backup_t AS SELECT ... WHERE ...`).
4. Schema changes (DDL): state the lock impact (`ALTER TABLE ... ADD COLUMN`
   with default locks big tables — prefer `ALTER ... ADD COLUMN ... DEFAULT`
   carefully / `CREATE INDEX CONCURRENTLY` for live tables).

## Schema design basics

- Normalize first, denormalize with a stated read-pattern reason.
- Every table: primary key, `created_at`/`updated_at`, FKs with explicit
  `ON DELETE` behavior, NOT NULL by default (NULL only when meaningfully
  absent).
- Name things in snake_case plurals (`user_sessions`); boolean columns read
  as predicates (`is_active`, `has_paid`).

## MongoDB (optional path)

If `mongosh` is available: same discipline — inspect via
`db.collection.findOne()` + `db.collection.getIndexes()`, `.explain("executionStats")`
before optimizing (`COLLSCAN` bad, index bounds check), aggregation stages
ordered `$match` → `$sort` → `$group`. Writes need the same explicit
confirmation as SQL.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `psql: command not found` | `apt install postgresql-client` (small, mention it) |
| password authentication failed | Wrong DSN/port — print `\conninfo`-style attempt with secrets masked, ask user |
| Query returns too much for chat | Aggregate/limit server-side; export big results to a workspace CSV instead of pasting |
| Plan estimates wildly off | `ANALYZE <table>`; if still off, check for correlated columns |
| Lock waits during DDL | Check `pg_stat_activity` for blockers; schedule `CONCURRENTLY` variants off-peak |
| Accidental destructive request ("xóa hết đi") | Confirm scope precisely (which rows, which table) and show count before doing anything |
