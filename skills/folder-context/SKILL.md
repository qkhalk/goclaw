---
name: folder-context
description: >-
  Workspace folder context discipline: per-folder conventions (plans/, docs/, src/), what belongs
  where, and keeping context files small and current so agents and humans navigate fast. Use when
  deciding where a new file goes, cleaning workspace clutter, or writing orientation docs.
  Keywords: folder structure, workspace layout, conventions, file placement, context files, cau
  truc thu muc, to chuc workspace. Dùng khi cần quyết định file nằm ở đâu và giữ workspace gọn gàng.
license: MIT
version: 1
---

# Folder Context

Keep every workspace folder purposeful so the next agent (or human) finds the right file in seconds.

## When to use
- Creating any new file and unsure where it belongs.
- A folder has become a dumping ground or its files are stale.
- Writing or refreshing per-folder orientation notes (README, conventions).

## When NOT to use
- Inside system directories or dependency trees (node_modules, vendor dirs) — leave them alone.
- Renaming or moving files referenced by code without updating references — that is a refactor for `fix`.

## Workflow
1. **One folder, one purpose:** `src/` ships, `tests/` verifies, `docs/` explains, `plans/` decides, `scripts/` automates. If a file serves two purposes, it goes where the dominant purpose lives.
2. **Plans and decisions** live in `plans/` as dated, phase-split files; status lives inside the file, not in separate tracker spreadsheets (see `plans-kanban`).
3. **Generated output never mixes with source:** build artifacts and exports go to ignored directories (`dist/`, `out/`, `tmp/`).
4. **Context files stay small:** a folder's README or notes file should be under ~100 lines — purpose, key files, how to run. Link; do not inline.
5. **Currency over completeness:** when behavior changes, update the folder notes in the same change. A wrong note is worse than no note.
6. **Names are metadata:** date prefixes `YYYY-MM-DD` for time-sensitive docs, lowercase-hyphen slugs, no spaces in new filenames.
7. **Stale files:** propose deletions or archive moves to the user rather than silently destroying anything.

## Output
Files in their canonical locations with current, minimal context notes — a workspace where listing the root directory explains itself.

## Routing
- The plans/ folder as a workflow board → `plans-kanban`.
- Whole-repository reorganization or cleanup → `workspace-organizing`.
- New project from scratch → `bootstrap`.

## Guardrails
- Never move or rename files without searching for references first.
- One context file per folder, kept current — never one per file.
