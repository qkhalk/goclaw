---
name: docs-seeker
description: "Find and verify official documentation for libraries, frameworks, and APIs before writing code against them: locate docs fast (llms.txt convention, official sites, GitHub), confirm the exact API/behavior for the version in use, quote with source URLs. Use whenever about to call a library/API the agent hasn't verified in this conversation, when the user asks 'API này dùng thế nào', 'check docs', how do I use <library>, what's the signature of <function>, does <lib> support <feature>, or when generated code needs version-correct API verification. Triggers: docs, documentation, tài liệu, API reference, changelog, llms.txt. Do NOT use for broad technology comparison (use the research skill)."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - library
  - question
outputs:
  - verified_answer
allowed-tools:
  - web_search
  - web_fetch
  - exec
quality-gates:
  - official_source_used
  - version_matched
---

# Docs Seeker (official documentation lookup)

Answer "how does X actually work in the version we're running?" from official
sources, with URLs — not from training-data memory that may be stale.

## Lookup order (fastest first)

1. **llms.txt convention:** many projects publish LLM-readable docs at
   `<docs-origin>/llms.txt` (an index) and `<docs-origin>/llms-full.txt`
   (everything). Try `web_fetch https://<library-docs-domain>/llms.txt` first —
   one fetch often answers the whole question.
2. **Official docs via search:** `web_search "<library> <exact feature>
   site:official-domain"`; open the official page, not aggregators.
3. **GitHub as source of truth:** README, `docs/` folder, CHANGELOG, and the
   type signatures in source (`web_fetch` on raw.githubusercontent.com paths).
   When docs and code disagree, the code wins — cite the file.
4. **Installed version check (local):** pin the answer to reality —
   ```bash
   pip3 show <pkg> | head -2        # python
   npm ls <pkg>                     # node
   go list -m <module>              # go
   ```

## Answer format

- **Direct answer** — the signature/behavior/limit, in one block.
- **Minimal verified example** (5–15 lines) using the exact API as documented
  for the detected version.
- **Source line** — URL(s), and the version the answer applies to. No source,
  no answer: if official confirmation can't be found, say "not found in
  official docs — here's the closest evidence and its date".

## Rules

- Match the running version, not latest: an answer for v3 is wrong for a user
  on v2 — check first, state it explicitly.
- Never invent flags/endpoints. "I couldn't verify" is a valid outcome; offer
  the experiment that would settle it (a 3-line repro).
- Deprecations matter: if the found page marks it deprecated, say so and give
  the replacement.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| No llms.txt at the docs domain | Some projects host it under docs.<domain> or <domain>/docs — try variants, then fall back to search |
| Search returns tutorial farms | Add `site:` restriction or append the exact error/symbol name |
| Version mismatch suspected | Read the repo's CHANGELOG for the boundary version; cite both sides of the change |
| Docs behind JS-only SPA | web_fetch may get empty HTML — try the GitHub-rendered docs path or llms.txt instead |
