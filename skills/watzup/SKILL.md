---
name: watzup
description: >-
  Project status digest: read git history, branch state, and workspace
  plans/notes, then produce a structured report — current state, recent work,
  in-flight items, next steps, warnings. Use for standups, stakeholder
  updates, or resuming after a break. Keywords: status report, digest,
  standup, changelog, bao cao, tinh hinh du an. Dùng khi cần tổng hợp báo cáo
  tình hình dự án từ git log và ghi chú trong workspace.
license: MIT
version: 1
---

# Watzup

Compile a structured status report from read-only evidence — git log and branches, plan and journal files in the workspace, open threads. Answer "where are we?" with facts, not opinions dressed as facts.

## When to use
- Standup-style updates or stakeholder asks like "what changed lately?"
- Resuming work after a break and needing to reconstruct state
- Before a handover, to capture current reality
- Auditing whether claimed work actually landed

## When NOT to use
- Deep process analysis over a period — that is `retro`
- Forward-looking planning — that is `goclaw-kit`
- Live incident watching — that is `monitor`

## Workflow
1. Confirm the time window (default: last 7 days) and the audience for the digest.
2. Gather git evidence with read-only `exec`: recent log (oneline with dates), branch list with last-commit dates, uncommitted status, ahead/behind vs the default branch.
3. Read workspace context: `list_files` over `plans/`, journals, TODO and notes files, then targeted `read_file` on what matters.
4. Cross-reference: which planned phases have commits; which journals promise work with no commits yet.
5. Classify items: shipped (merged/committed), in-flight (branch or WIP), stalled (old branch, no movement), planned (notes only).
6. Extract warnings: stale branches, conflicts with the default branch, blockers noted in journals, work silent for the whole window.
7. Write the digest in the Output structure; every claim cites evidence (commit hash or file path).
8. Deliver in chat and, if the workspace keeps a reports area, save a dated copy with `write_file`.

## Output
A digest with sections: Current state (branch, uncommitted work), Recent work (landed items, newest first), In-flight (owner and next step each), Next steps (prioritized), Warnings (stale or risky items). Every bullet carries an evidence link.

## Routing
- Findings imply process changes -> `retro`
- Next steps need a real plan -> `goclaw-kit`
- Handing work to another agent -> pair this digest with `handover`
- Repo-specific engineering context -> `goclaw` or `docs`

## Guardrails
- Strictly read-only: no commits, checkouts, or mutations except writing the dated report file
- Label uncertainty: "inferred" vs "confirmed by commit hash"
- No editorializing about people; report work, not blame
- Keep the digest under ~60 lines; link out for detail
- Respect the asked time window; do not pad with old news
