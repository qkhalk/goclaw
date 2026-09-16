---
name: mermaidjs-v11
description: >-
  Write Mermaid v11 diagrams inline in markdown: flowchart, sequence, class,
  state, ER, gantt, pie, mindmap, and timeline — with syntax gotchas and
  layout hints. Use when a diagram should live inside docs, READMEs, PR
  descriptions, or chat as plain text. Keywords: mermaid diagram, flowchart,
  sequence diagram, erd, gantt. Dùng khi cần vẽ sơ đồ mermaid nhúng trong
  markdown, tài liệu hoặc mô tả PR.
license: MIT
version: 1
---

# Mermaid v11

Diagrams-as-code that render inline in markdown. Pick the smallest diagram
type that answers the reader's question, then write syntax that survives
renderers.

## When to use
- Documentation, README, PR/issue descriptions, or design docs need a
  diagram that lives as plain text next to the code.
- Architecture flows, state machines, data models, or timelines.

## When NOT to use
- An editable whiteboard file is the deliverable → `excalidraw`.
- Pixel-perfect custom layouts or brand-styled slides are required.

## Choosing the type
- Flow/decision logic → `flowchart TD` (hierarchies) or `LR` (pipelines).
- Interactions over time between actors → `sequenceDiagram`.
- Object lifecycle → `stateDiagram-v2`; data model → `erDiagram`; class
  structure → `classDiagram`; schedules → `gantt`; proportions → `pie`;
  idea branches → `mindmap`; milestone history → `timeline`.

## Syntax gotchas (v11)
- Wrap node labels containing parentheses, brackets, or colons in quotes:
  `A["parse (v2)"]`.
- `end` is reserved in flowcharts — a lowercase node id spelled `end` breaks
  rendering; capitalize it or quote the label.
- Subgraph titles with special characters need quoting; `direction` inside
  a subgraph overrides the parent only in flowcharts.
- Sequence diagrams: use `->>` for calls with replies expected, `-->>` for
  dashed returns, `autonumber` for ordered steps, `activate`/`deactivate`
  or `+`/`-` for lifespans, `alt/else/end` and `loop/end` for blocks.
- ER cardinalities use crow's-foot tokens (`||--o{`); attributes take a
  type then name, with `PK`/`FK` markers after.
- Gantt needs `dateFormat` and `axisFormat`; sequence tasks with
  `after taskId` instead of hardcoding dates.
- Mindmap and timeline are indentation-based — no arrow syntax there.

## Workflow
1. Identify the single question the diagram answers; drop everything that
   does not serve it.
2. Draft with ≤15 nodes; if larger, split into subgraphs or two diagrams.
3. Apply the gotchas above; keep styles minimal (theme variables, not
   per-node styling) so the diagram renders everywhere.
4. Emit inside a fenced code block marked `mermaid`, preceded by one
   sentence of orientation for readers whose client does not render it.
5. If a renderer is available (editor preview or `exec`-run local CLI),
   render once and fix reported syntax errors before shipping.

## Output
A fenced `mermaid` code block that renders cleanly, with a one-line caption
stating what the reader should take away.

## Routing
- Editable whiteboard/hand-drawn file deliverable → `excalidraw`.
- Diagram belongs inside a larger document effort → `docs` or `design`.
- System-level proposal around the diagram → `architect`.

## Guardrails
- One idea per diagram; a diagram answering two questions answers neither.
- Test-render when possible; broken mermaid blocks ship as raw text.
- Do not encode sensitive names (internal hosts, secret project names) into
  diagrams destined for public docs.
