---
name: decision-log
description: "Record and recall durable DECISIONS so future sessions stop re-litigating them: architecture picks, library/provider choices, schema changes, rejected alternatives, user preferences, and the reasoning that made each call. Writes short ADR-style entries to the workspace and retrieves them back through memory and vault search before any re-decision. Use when a meaningful choice is made ('we'll use pgx, not GORM'), when the user asks why something was chosen, or before reversing an earlier call ('didn't we decide against this?'). Triggers: log this decision, remember that we chose, why did we pick X, have we decided this before, 'ghi lại quyết định', 'vì sao chọn'. Do NOT use for project documentation maintenance (use the docs skill), for task/todo tracking (use mission), or for trivial choices (just decide)."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - decision
  - context
outputs:
  - adr_entry
  - recall_report
allowed-tools:
  - write_file
  - read_file
  - list_files
  - memory_search
  - vault_search
  - datetime
quality-gates:
  - recall_before_redeciding
  - entry_has_options_and_rationale
  - entry_dated_and_scoped
---

# Decision Log (ADRs the agent actually rereads)

A decision that is not retrievable will be re-argued from scratch in the
next session — by an agent that does not remember making it. This skill
keeps a lightweight decision log: short, structured entries written when a
meaningful call is made, and searched BEFORE any related call is made
again. The unit is the decision, not the document: one entry, one choice,
one page or less.

## When to record (and when not)

Record a decision when it has at least one of:
- **Cost to reverse** — schema, data model, provider choice, public API
  shape, file format.
- **A rejected alternative someone will re-propose** — the GORM-vs-pgx call,
  the "we don't add a cache here" call.
- **User preference with a reason** — "user wants raw SQL, no ORM";
  "commits in English, chat in Vietnamese".
- **A guess that later hardened into policy** — "we assumed single-tenant;
  write down that we assumed it."

Do NOT record: trivial picks (variable names, lunch), task progress (that
is the mission/journal of work, not decisions), or anything already obvious
from the code and its docs. If writing the entry would only restate the
diff, the docs skill owns it, not this log.

## Recall protocol (mandatory before re-deciding)

Before making any decision that touches a past choice:

1. `memory_search` for the topic — query in the SAME language the decision
   was recorded in (this matters for recall quality).
2. `vault_search` with `types="context,note"` to catch workspace entries
   memory may not have surfaced.
3. Found a prior decision?
   - Still valid → follow it, and say so ("keeping raw SQL per the decision
     from 2026-08-30").
   - Outdated → write a NEW entry that supersedes it and references it;
     never silently contradict the old record.
4. Nothing found and confidence is low → tell the user you checked and
   found nothing. Never fabricate a prior decision — an invented "we
   decided this before" is worse than no log at all.

## Entry format (write with write_file)

One markdown file per decision, in the workspace decision log directory
(default `decisions/`; follow the project's existing location if one
exists — `list_files` for `decisions/` or `adr/` first):

```markdown
# D-<seq>-<slug>: <decision in one imperative line>

- Date: <from datetime tool, ISO 8601 with local offset>
- Status: accepted | superseded by D-<seq> | reversed
- Scope: <repo/system/area this binds>

## Context
The problem and the constraints that forced a choice (2-5 sentences).

## Options
- **Chosen: <option>** — why: <the decisive reasons>
- <option 2> — rejected because: <reason>
- <option 3> — rejected because: <reason>

## Consequences
What gets easier, what gets harder, what we now must do/avoid.

## Revisit when
<the condition under which this decision is wrong — "if we go multi-tenant",
"if pgvector version drops X">
```

Rules for the entry itself:
- Get the date from the `datetime` tool — never guess "today".
- "Revisit when" is the most valuable field and the most skipped: write it.
  It is what lets a future session overturn this entry with confidence
  instead of guilt.
- Name concrete options. "Chose X because it was best" is a note, not a
  decision record; the rejections carry the reasoning.
- Supersede, never edit history: reversing a decision = new entry +
  `Status: superseded by D-<new>` edited into the old one, nothing else
  changed.

## Anti-patterns

- Deciding without recalling — the log exists precisely for the session
  that doesn't remember.
- Fabricating or "remembering" a decision memory_search did not return.
- Vague entries with no options, no date, or no scope — future-you cannot
  obey an entry it cannot situate.
- Logging trivia until the signal drowns — the log stays trustworthy by
  staying small.
- Rewriting an old entry to match today's opinion — decisions are
  append-only; reversal is a new entry.
- Duplicating what belongs in docs/README (how it works) instead of the
  decision log (why it is that way).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| memory_search returns nothing on a topic you know was decided | Try the other storage language (EN↔VI), then `vault_search types="context,note"`, then `list_files` the decisions directory directly |
| Two entries conflict | The later one wins if it explicitly supersedes; otherwise surface both to the user and reconcile with a superseding entry |
| User disputes a logged decision | Show the entry verbatim (date + rationale), then follow the user — and log the override as a new entry |
| No decisions directory yet | Create `decisions/` with the first entry; mention the location to the user once |
