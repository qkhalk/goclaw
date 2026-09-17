---
name: fan-out
description: "Use when one question decomposes into several independent investigations that a single agent would do slowly and shallowly: parallel research across sources/technologies/code areas, multi-angle comparisons, or competitive draft generation. Orchestrates GoClaw's real multi-agent tools — spawn for independent parallel subagents, jury for competitive same-task contenders with scoring, delegate for handing a slice to a linked specialist agent, wait for spacing while they run. Triggers: research these 5 options in parallel, compare approaches from multiple angles, ask several agents, run a contest on this task, 'tìm hiểu song song', 'so sánh nhiều phương án cùng lúc'. Do NOT use for a single quick lookup (do it yourself), for dependent sequential steps (spawn just adds latency), or for delegating one task to one known specialist (call delegate directly)."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - question
  - slice_count
outputs:
  - synthesis_report
allowed-tools:
  - spawn
  - delegate
  - jury
  - wait
  - web_search
  - web_fetch
quality-gates:
  - slices_independent_and_non_overlapping
  - all_results_collected_or_accounted
  - conflicts_surfaced_not_averaged
  - synthesis_names_each_contributor
---

# Fan-Out (parallel investigation & competitive drafts)

One brain, many arms: split a question into independent slices, run them in
parallel, then synthesize with conflicts shown — not averaged away. GoClaw
gives you three distinct fan-out primitives; this skill is about picking the
right one and briefing each arm well. Briefing quality decides the outcome:
a vague brief returns a vague page per subagent.

## Choose the pattern first

| Pattern | Tool | When |
|---------|------|------|
| **Parallel slices** | `spawn` | The question splits into DIFFERENT independent sub-questions (5 libraries → one subagent each; 3 code areas → one each) |
| **Competitive round** | `jury` | You need N independent attempts at the SAME task, scored against criteria; jury returns the verdict (approve/revise/reject) plus the winning output |
| **Specialist hand-off** | `delegate` | One slice belongs to a linked specialist agent (its context/tools beat a generic subagent); delegate the slice instead of spawning a duplicate |

Simple test: different tasks → `spawn`; same task, want the best of N →
`jury`; a slice with a known owner → `delegate`. Mix patterns in one fan-out
when the slices genuinely differ (3 spawns + 1 delegate is fine).

## Process

1. **Decompose (write it down first).** Split the question into 2–6 slices.
   Each slice must be:
   - **Independent** — answerable without another slice's result. If slice B
     needs slice A's output, that is a sequence, not a fan-out.
   - **Non-overlapping** — two slices reading the same docs is duplicated
     cost and an echo chamber. Cut the boundaries by source, by angle, or by
     candidate — not "one general, one detailed".
   - **Small enough to finish** — one slice = one focused brief, not a
   mission.
2. **Brief each arm.** A brief is a complete work order, because subagents
   do not see this conversation:
   - The exact question, with scope bounds and the user's constraints.
   - Where to look (sites, repos, code paths) and what counts as a source.
   - The required output: format, length, and "state what you could NOT
     verify" — a slice that returns unknowns honestly beats one that
     guesses.
   - Deadline spirit: prefer 3 verified facts over 10 impressions.
3. **Launch.** `spawn` each slice arm with its brief (or `delegate` the
   specialist slices). For a competitive round, hand `jury` the shared task
   plus explicit scoring criteria (correctness, coverage, cost, risk —
   whatever the user cares about; unweighted criteria get weighted by you,
   stated in the report).
4. **Wait and collect.** Arms run in the background; use `wait` for spacing
   when polling rather than busy-looping. Collect every result. An arm that
   failed or timed out is REPORTED as missing — never silently dropped and
   never replaced by your imagination.
5. **Cross-check before synthesizing.** Line the results up:
   - Agreement across independent arms → high confidence.
   - Conflict → do NOT average. Name the conflict, check the primary
     sources yourself (`web_fetch` the two contradicting pages), and show
     the user which claim rests on which evidence.
   - Every number/quote you carry into the synthesis must appear in some
     arm's result — synthesis adds structure, not new facts.
6. **Synthesize.** One report: answer first, then a table (slice × finding ×
   source), then conflicts & unknowns. Credit each arm ("web arm on pricing:
   ...", "jury winner, score 8/10 vs runner-up 6/10: ...") so the user can
   trace every claim to its arm. For a jury round, report the verdict, the
   winner, and WHY it won — then say if you disagree and why.

## Spawn vs jury anti-confusion

- `spawn` arms should never receive identical briefs — if you catch yourself
  copy-pasting a brief, you wanted `jury`.
- `jury` criteria must be decidable from the outputs alone ("cites primary
  sources: yes/no"), not vibes ("feels more complete").
- Do not re-run a `jury` round because you disliked the winner — re-run only
  with sharpened criteria, and say you did.

## Anti-patterns

- Fanning out a 30-second lookup — overhead exceeds the work; answer
  directly.
- Overlapping slices ("research React" + "research React ecosystem") —
  duplicated cost, correlated errors, false confidence from "two sources"
  that were one source.
- Briefs that reference "the above" or "as discussed" — subagents have no
  shared context; every brief must stand alone.
- Dropping a failed arm and presenting the synthesis as complete — say
  which slice is missing.
- Averaging conflicting numbers (pricing, dates, limits) — show the range
  and the sources; verify what you can.
- Synthesis that "improves" facts beyond what any arm reported.
- Spawning more arms than slices because more sounds thorough — 6 focused
  arms beat 12 vague ones, every time.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Arms return suspiciously similar text | Briefs overlapped or echoed one source; re-cut boundaries, re-run the echo slice with different sources |
| One arm never reports | Collect the rest, mark it missing, finish; do not block the answer on the slowest arm |
| Jury verdict contradicts strong evidence you hold | Present both: the verdict AND your evidence-backed dissent |
| Results too shallow across the board | The briefs were too thin — add required output format, minimum source count, and "list unknowns" |
