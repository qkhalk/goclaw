---
name: remotion
description: >-
  Create programmatic videos in React with Remotion: composition model, useCurrentFrame
  and interpolation, parametrized videos from data, and the rendering pipeline. Use
  when video needs component logic, data-driven scenes, or reusable animation
  primitives. Keywords: Remotion, React video, useCurrentFrame, interpolate, spring,
  composition, Sequence, render video, video React, video theo dữ liệu. Dùng khi cần
  tạo video bằng React/Remotion hoặc video được tham số hóa theo dữ liệu.
license: MIT
version: 1
---

# Remotion

Build videos as React components with Remotion: every frame is a pure function of
the frame number, scenes are components, and variants come from props.

## When to use
- Videos driven by data: personalized names, metrics, lists, per-user variants.
- Scene logic benefits from React components and reusable animation primitives.
- Batch rendering many videos from one template.

## When NOT to use
- A single simple timeline in plain HTML — use html-video (no build step).
- The HeyGen HyperFrames CLI is the mandated pipeline — use hyperframes.
- Interactive (non-video) web animation — this renders to files, not live pages.

## Workflow
1. Confirm fit, then scaffold a Remotion project: set fps, width, height, and
   durationInFrames in the video config; register compositions in the Root file.
2. Think in frames, not clocks: no timers or wall-clock reads — every visual value
   derives from `useCurrentFrame()`; use `interpolate()` with input/output ranges
   and easing, and `spring()` for natural motion.
3. Build scenes as components: each scene is a component scheduled with
   `<Sequence from={} durationInFrames={}>`; cross-fade transitions by overlapping
   sequences and interpolating opacity.
4. Compose the timeline: one root composition assembling scenes in order; keep
   scene components pure so any frame renders identically at any time.
5. Parametrize: define defaultProps (ideally with a schema) on the composition;
   render variants by passing different props — batch renders for per-user video.
6. Handle assets: load local media with `staticFile()`, gate rendering on fonts
   and data with `delayRender`/`continueRender`, and preload images.
7. Add audio: `<Audio>` sequences with interpolated volume; align cuts to frames
   using fps math (samples = seconds * fps), not wall-clock offsets.
8. Preview in Remotion Studio while iterating, then render MP4 via the local CLI
   (or serverless/cloud rendering at scale); verify duration, fps, and audio sync
   of the output.

## Output
- A Remotion project with at least one parametrized composition, the rendered
  MP4, and the exact render command (plus props) used — so variants are one
  command away.

## Routing
- Plain HTML timelines without a build step: html-video.
- HyperFrames CLI pipeline: hyperframes.
- 3D content inside the video: threejs feeding Remotion frames.
- Scene visual design: frontend-design.

## Guardrails
- Render functions must stay pure and deterministic: seeded randomness only, no
  current-date reads in visuals.
- Keep durationInFrames consistent with fps math when changing either.
- Respect licenses of fonts, music, and footage bundled into renders.
- Verify at least one full render before claiming the pipeline works.
