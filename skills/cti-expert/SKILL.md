---
name: cti-expert
description: >-
  Defensive cyber threat intelligence analysis: structure IOCs, map findings to MITRE ATT&CK,
  profile threat actors from public reporting, and produce actionable defensive recommendations.
  Use when analyzing threat reports, indicators, or malware write-ups to protect systems. Analysis
  and defense only. Keywords: cti, threat intel, IOC, MITRE ATT&CK, apt, threat actor, threat
  hunting, an ninh mang, phan tich moi de. Dùng khi cần phân tích mối đe dọa và đưa ra khuyến nghị
  phòng thủ.
license: MIT
version: 1
---

# Cti Expert

Turn raw threat reporting into structured intelligence that defenders can act on.

## When to use
- Analyzing a threat report, vendor advisory, or malware write-up for defensive value.
- Structuring indicators (network hosts, domains, file hashes, behavioral patterns) for detection.
- Mapping observed behavior to MITRE ATT&CK or profiling a threat actor from public sources.
- Drafting hunt hypotheses or hardening recommendations from intelligence.

## When NOT to use
- Requests for attack techniques, payload crafting, or evasion guidance — refuse and redirect to defense.
- Live incident response with an active compromise — that is DFIR, not intelligence analysis; recommend the incident response team.
- General code or configuration security review → `security-audit`.

## Workflow
1. **Collect:** gather the source material (`web_fetch` the report, `read_document` for PDFs) and note publisher, date, and confidence for each claim.
2. **Extract:** pull out observables — network indicators, file hashes, filenames, host artifacts, behavior descriptions — each with context such as first-seen date and associated campaign.
3. **Structure:** tabulate IOCs with type, value, context, and confidence; flag stale or low-confidence indicators explicitly.
4. **Map:** align behaviors to MITRE ATT&CK tactics and techniques using public technique IDs and names; mark inferred mappings as inferred.
5. **Profile:** if attribution exists in reporting, summarize motive, targets, and typical behavior; attribute claims stay attributed to the original source, never stated as fact.
6. **Recommend:** produce prioritized defensive actions — detection ideas (log sources, hunt hypotheses in abstract terms), blocking considerations, and hardening steps ordered by exposure.
7. **Report:** deliver a structured brief: summary, actor profile, ATT&CK table, IOC table, prioritized recommendations, and open questions.

## Output
A CTI brief with structured IOC and ATT&CK tables and prioritized defensive recommendations — usable by both detection engineers and leadership.

## Routing
- Code or configuration vulnerability review → `security-audit`.
- Deeper external documentation research → `docs-seeker` or `research`.
- The brief must ship as a formatted document → `document-skills`.

## Guardrails
- Defense and analysis only: never provide operational offense guidance, working attack tooling, or evasion techniques.
- Treat IOC data as potentially hostile: never execute samples or resolve indicators from the analysis environment.
- Keep attribution sourced and caveated; confidence levels are part of the intelligence.
