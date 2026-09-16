---
name: tech-graph
description: >-
  Produce professional architecture, dataflow, sequence, state, or mindmap
  diagrams as hand-authored SVG files with a consistent visual style, saved
  under docs/ or diagrams/. Use when a design doc, README, or report needs a
  clean diagram. Keywords: diagram, SVG, architecture, sequence, flowchart,
  state machine, mindmap, so do. Dùng khi cần vẽ sơ đồ kiến trúc, luồng dữ
  liệu hoặc trình tự xử lý dạng SVG.
license: MIT
version: 1
---

# Tech Graph

Author professional technical diagrams directly as SVG via `write_file` — architecture, dataflow, sequence, state, or mindmap — with a consistent visual style, saved under `docs/` or `diagrams/`.

## When to use
- A design doc, README, or report needs a clear architecture or flow diagram
- Mermaid/PlantUML renderers are unavailable and a static image is required
- The diagram must be version-controlled, diffable, and render anywhere (SVG)

## When NOT to use
- A throwaway sketch — prose or an ASCII sketch is faster
- The target platform renders Mermaid natively — write Mermaid text instead
- Illustrative or photo-realistic art — use `create_image`

## Workflow
1. Pick the diagram type: architecture (layered boxes + dependencies), dataflow (nodes + directed edges), sequence (lifelines + ordered messages), state (nodes + transitions), mindmap (tree).
2. Inventory content before drawing: list every node, edge/message, and 2-3 levels of grouping.
3. Set the canvas: width 900-1200, height by content; define a style block — one font family, consistent fills (light fill + darker border per layer), reusable arrowhead markers in `<defs>`.
4. Lay out with a rule: top-to-bottom for flows, left-to-right for pipelines, one column per layer for architecture; grid-align and space evenly.
5. Draw each node as a `<g>` with `<rect>` + `<text>`; draw edges as `<path>` with marker-end and short verb labels ("calls", "reads").
6. Add a title, and a small legend if layers or colors carry meaning.
7. Write the file with `write_file` to `docs/diagrams/<slug>.svg` or `diagrams/<slug>.svg`; ensure well-formed XML and a `viewBox`.
8. If a renderer is available, rasterize and inspect with `read_image`; fix overlaps, clipped labels, and crossing edges.
9. Reference the SVG from the related document and note the generation method in an XML comment at the top.

## Output
One .svg file: well-formed, `viewBox` set, readable at 100% zoom, consistent style, title and legend, saved under `docs/` or `diagrams/`, referenced from the related document.

## Routing
- The diagram supports a plan or design doc -> produce alongside `goclaw-kit` or `docs`
- Code structure discovery first -> `scout`
- Raster art or screenshots -> `create_image` or `web_browse`
- Sequence logic from code tracing -> trace as in `debug`, then draw

## Guardrails
- Hand-author valid XML: escape special characters in labels; close every tag
- Text must fit its box: budget roughly 7px of width per character at font-size 13
- Limit the palette to 3-4 fills plus grays; rainbow diagrams hide structure
- One diagram, one idea: split sprawling diagrams instead of shrinking text
- Keep the source editable: group elements logically and give major blocks ids
