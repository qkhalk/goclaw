---
name: journal
description: >-
  Technical journaling for agents and humans: append-only daily entries with
  date, done, learned, decided, and next — kept in the workspace as plain
  markdown so future sessions can search it. Use when finishing a meaningful
  work session, tracking long-running work, or needing continuity across
  sessions. Keywords: work log, daily log, changelog diary, session notes.
  Dùng khi cần ghi nhật ký công việc hằng ngày, lưu lại những gì đã làm, đã
  học, đã quyết định và việc tiếp theo.
license: MIT
version: 1
---

# Journal

An append-only record of what happened, what was learned, what was decided,
and what comes next — written so a future session can reconstruct context in
one `memory_search` or file read.

## When to use
- At the end of a meaningful work session (feature finished, bug found,
  decision made, dead end hit).
- Work spans multiple sessions and continuity matters.
- The user says "log this", "note it down", or asks what was done recently.

## When NOT to use
- The insight is durable project knowledge, not a dated event → docs or
  `decision-log`.
- Trivial actions with no learned or decided content.
- Storing structured data that belongs in a task tracker or DB.

## Conventions
- Location: `journal/` in the workspace, one file per month:
  `journal/2026-09.md`. If the workspace already has a journal convention,
  follow it instead.
- One heading per day: `## 2026-09-15`. Append; never rewrite or delete old
  entries. Corrections go in a new entry that references the old one.
- Entries are plain markdown with stable keywords (Done, Learned, Decided,
  Next) so both humans and `exec`-based search can find things later.

## Workflow
1. Get the current date with `datetime`.
2. Open this month's file with `read_file` (create via `write_file` if
   missing); check today's heading exists.
3. Append an entry with exactly these fields:
   - **Done**: 1-3 lines of what actually changed (files, commands, PRs,
     outcomes — with concrete names).
   - **Learned**: facts, gotchas, or root causes worth remembering.
   - **Decided**: decisions made and the one-line why (link to the ADR or
     conversation if one exists).
   - **Next**: the single most useful continuation point for the next
     session.
4. Keep it under ~15 lines; terseness beats completeness.
5. When asked "what happened with X", search the journal with `exec` grep or
   `memory_search` and summarize with entry dates.

## Output
An appended journal entry in `journal/YYYY-MM.md` under the day's heading;
for queries, a dated summary of relevant entries.

## Routing
- A decision needs a formal durable record → `decision-log`.
- Learned content generalizes into reference material → `docs`.
- Next items are concrete upcoming work → `plan`.

## Guardrails
- Never edit or remove past entries; append-only is the whole point.
- No secrets (keys, tokens, passwords) in journal entries, ever.
- One entry per meaningful event; a journal that records everything records
  nothing.
