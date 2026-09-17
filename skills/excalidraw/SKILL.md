---
name: excalidraw
description: >-
  Create Excalidraw diagrams as .excalidraw JSON files: scene and element
  schema, shapes, arrows, text, frames, hand-drawn styling, and export to
  PNG/SVG. Use when a whiteboard-style diagram or architecture sketch file is
  needed that stays editable. Keywords: excalidraw, whiteboard, diagram json,
  sketch, draw. Dùng khi cần vẽ sơ đồ dạng bảng trắng, tạo file excalidraw
  hoặc xuất ảnh PNG/SVG từ sơ đồ.
license: MIT
version: 1
---

# Excalidraw

Generate editable Excalidraw scenes as JSON — a whiteboard-style diagram the
user can keep rearranging, not a frozen image.

## When to use
- The user asks for an Excalidraw file, whiteboard sketch, or editable
  diagram.
- An architecture/flow sketch should stay editable by the team.
- Exporting a diagram to PNG/SVG from a scene definition.

## When NOT to use
- A diagram inline in markdown/docs is enough → `mermaidjs-v11` (simpler,
  versionable as text).
- Precise engineering drawings (CAD-like) are required.

## Scene schema essentials
- Top level: `{"type": "excalidraw", "version": 2, "source": "...",
  "elements": [...], "appState": {"viewBackgroundColor": "#ffffff"},
  "files": {}}`.
- Common element fields: `id` (unique string), `type`, `x`, `y`, `width`,
  `height`, `angle`, `strokeColor`, `backgroundColor`, `fillStyle`
  ("hachure" gives the hand-drawn look), `strokeWidth`, `roughness` (0=architect,
  1=artist, 2=cartoonist), `seed`, `version`, `groupIds`, `frameId`,
  `boundElements`.
- Types: `rectangle`, `ellipse`, `diamond`, `line`, `arrow`, `text`,
  `freedraw`, `frame`.
- Arrows/lines: `points` is an array of `[dx, dy]` offsets RELATIVE to the
  element's own `x`,`y`; set `startArrowhead`/`endArrowhead` ("arrow",
  "triangle", "dot").
- Text: `text`, `fontSize`, `fontFamily` (1=hand-drawn, 2=normal, 3=code),
  `textAlign`. For a label inside a shape: create the text with
  `containerId` set to the shape's id and add the text's id to the shape's
  `boundElements` as `{"type": "text", "id": ...}`.
- Frames: `type: "frame"`, `name`, and give children the frame's id as
  their `frameId`. Coordinates grow down-right (y points down).

## Workflow
1. Plan the diagram as a list: entities (shapes), relations (arrows),
   labels, grouping into frames.
2. Assign a grid layout (e.g. 260px columns, 200px rows) so nothing
   overlaps; compute shape sizes from label length before placing text.
3. Generate the JSON and save it with `write_file` as `<name>.excalidraw`;
   validate that it parses as JSON (e.g. a quick parse check via `exec`)
   and that every arrow endpoint lands inside its target shape.
4. Export: if a converter is already available locally (CLI or headless
   browser), run it via `exec` to produce PNG/SVG. Install tools only from
   trusted, verified sources — never pipe remote scripts into a shell.
5. Hand off: tell the user which file was written and how to open it
   (excalidraw.com or an editor plugin) and where the exported image is.

## Output
A valid `<name>.excalidraw` JSON file (and optional exported PNG/SVG), plus
a one-line description of the diagram's structure.

## Routing
- Text-inline diagram for docs/README → `mermaidjs-v11`.
- The sketch feeds a design discussion → `design` or `architect`.

## Guardrails
- Deterministic ids and seeds; regenerate, never hand-tweak, when layout
  changes.
- Verify JSON parses before telling the user it is done — a broken scene
  fails silently in the app.
- Keep element counts reasonable (<100); split sprawling scenes into frames
  or separate files.
