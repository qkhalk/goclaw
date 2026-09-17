---
name: markdown-novel-viewer
description: >-
  Structure and maintain long-form fiction in markdown: chapter files with
  front matter, a story bible, POV/tense consistency tracking, progress
  ledger, and reading-friendly formatting for serialized stories. Use when
  writing, organizing, or auditing a multi-chapter novel or serial kept as
  markdown. Keywords: novel writing, serialized fiction, story bible, chapter
  structure, POV tracking. Dùng khi cần tổ chức, viết hoặc kiểm tra tính nhất
  quán của tiểu thuyết dài nhiều chương dưới dạng markdown.
license: MIT
version: 1
---

# Markdown Novel Viewer

Keep a serialized novel as clean, ordered markdown that any reader or agent
can traverse chapter by chapter — with continuity tracking that survives
dozens of chapters.

## When to use
- Starting, reorganizing, or continuing a novel/serial stored as markdown.
- Checking a manuscript for POV, tense, naming, or timeline drift.
- Preparing chapters for serialized release with readable formatting.

## When NOT to use
- Short copywriting pieces (blurbs, marketing) → `copywriting`.
- Reference documentation → `docs`.

## File conventions
- `README.md`: logline, reading order, TOC linking every chapter file,
  status legend.
- `bible.md`: characters (names, aliases, arcs), locations, timeline, and
  global rules — POV person/tense per arc, naming and capitalization
  conventions, world rules that must not be violated.
- `chapters/NNN-slug.md`: zero-padded numbers so files sort correctly
  (`chapters/007-the-harbor.md`).
- Chapter front matter: `title`, `chapter`, `pov` (character + person +
  tense), `timeline` (in-story date), `status` (draft/revised/final), and
  `words`.
- `progress.md`: one row per chapter — status, word count, open continuity
  notes.

## Formatting rules for reading comfort
- Scene breaks as a centered marker: a blank line, `* * *`, blank line.
- Short paragraphs; dialogue on its own lines with consistent punctuation.
- Pure markdown — no raw HTML, no styling hacks — so any viewer renders it.
- Italics (`*word*`) for emphasis and direct thought; never all-caps.

## Workflow
1. If the project exists, read `README.md`, `bible.md`, and `progress.md`
   plus the last 1-2 chapters (`read_file`) to load continuity. If starting
   fresh, create the three scaffolding files first.
2. Draft or edit the chapter, honoring the POV and tense recorded in its
   front matter and the rules in `bible.md`.
3. Update front matter and `progress.md` (status, word count) in the same
   sitting.
4. Run a consistency pass on changed chapters: character names against
   `bible.md`, timeline against neighboring chapters, unresolved setups
   noted as open threads.
5. Refresh the README TOC if files were added, renamed, or renumbered.
6. For release, verify the chapter reads cleanly top-to-bottom in a plain
   markdown viewer (headings, breaks, no broken links).

## Output
Updated chapter files with complete front matter, a consistent story bible,
and a current progress ledger; for audits, a continuity findings list.

## Routing
- Prose quality, voice, and promotional text → `copywriting`.
- Repetitive continuity lookups across many files → `delegate`.
- Long-running writing sessions benefit from a work log → `journal`.

## Guardrails
- Never contradict `bible.md`; if the story must change the canon, update
  the bible in the same edit and note the retcon.
- One POV per scene unless a deliberate switch is marked with a scene break.
- Do not renumber existing chapters; additions go at the end or as
  explicitly marked interludes.
