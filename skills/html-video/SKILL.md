---
name: html-video
description: >-
  Produce MP4 videos from HTML/CSS/JS animation templates: timeline design,
  deterministic animation (seeded randomness), frame-accurate capture with a
  headless browser, encoding settings, and audio sync. Use for programmatic videos,
  intros, promos, and motion graphics. Keywords: HTML video, MP4, animation, headless
  browser, frame capture, ffmpeg, encoding, motion graphics, timeline, video tự động,
  tạo video, hoạt hình HTML. Dùng khi cần tạo video MP4 từ template HTML/CSS/JS.
license: MIT
version: 1
---

# HTML Video

Turn an HTML/CSS/JS animation template into an MP4: one deterministic timeline,
captured frame-by-frame with a headless browser, encoded cleanly with audio in sync.

## When to use
- Programmatic videos: intros, promos, product explainers, animated stats.
- Rendering the same video with different text/data per audience.
- Motion graphics authored in HTML/CSS rather than a video editor.

## When NOT to use
- The pipeline should be React components — use remotion.
- A HyperFrames CLI pipeline is already available — use hyperframes.
- Pure 3D rendered content — compose it with threejs, then capture here.

## Workflow
1. Define the spec up front: duration, resolution (1080p default), fps (30 or 60),
   aspect ratio (16:9, 9:16, 1:1), scene list, and audio track if any.
2. Design one global timeline: scenes with fixed start times and durations in
   seconds; all animation derives from a single clock — CSS animations with
   computed delays or a JS `t` variable. No requestAnimationFrame drift.
3. Make rendering deterministic: seed and pre-generate every random value, keep
   wall-clock time out of the animation path, and preload fonts and images before
   capture (wait for document fonts to be ready).
4. Build the template as a single HTML file sized exactly to the output resolution;
   preview it in a normal browser and fix layout before any capture.
5. Capture frame-accurately: with a headless browser, seek a JS function to each
   frame time (frame i at i/fps) and screenshot to numbered files. Full-screen
   recording is a fallback only — it drops frames.
6. Encode: assemble the numbered stills into MP4 (H.264, yuv420p, quality CRF
   around 18 for near-transparent compression); verify duration and fps of the
   output file before delivering.
7. Sync audio: prepare the track independently, align its offset to the timeline
   (scene boundaries are the checkpoints), mux as AAC, and verify sync at the
   loudest beats.
8. QC pass: watch the render end to end — check dropped frames, late-asset layout
   shifts, and audio drift; re-render only affected scenes when the pipeline
   supports it, then re-verify.

## Output
- Final MP4 at the requested spec, plus the reusable HTML template and the exact
  capture and encode commands used, so the video can be regenerated.

## Routing
- React component-driven or heavily parametrized video: remotion.
- HyperFrames CLI pipeline when available: hyperframes.
- Visual design of scenes: frontend-design. 3D scenes: threejs.

## Guardrails
- Randomness must be seeded; no nondeterministic values may reach a frame.
- Verify output duration and fps against the spec before claiming done.
- Keep intermediate frames on disk until the final QC pass completes.
- Confirm asset licenses (fonts, music, images) before rendering.
