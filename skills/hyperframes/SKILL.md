---
name: hyperframes
description: >-
  Build HTML-first programmatic videos with the HeyGen HyperFrames CLI: template
  structure, scene composition, render and preview commands, API key handling, and
  a fallback to plain HTML-video capture when the CLI is unavailable. Use for
  HTML-driven video generation when the HyperFrames toolchain exists. Keywords:
  HyperFrames, HeyGen, programmatic video, HTML video, CLI render, template, scene,
  video HTML, render video, tạo video HTML. Dùng khi cần tạo video bằng HeyGen
  HyperFrames CLI hoặc fallback sang HTML-video.
license: MIT
version: 1
---

# HyperFrames

Author HTML-first videos and render them through the HeyGen HyperFrames CLI — with a
clean fallback to headless-browser capture when the CLI is missing or fails.

## When to use
- The HeyGen HyperFrames CLI is available and the task is HTML-driven video.
- Template-based videos where scenes are HTML documents.
- A request explicitly names HyperFrames or HeyGen video rendering.

## When NOT to use
- CLI not installed, unauthenticated, or quota-blocked — fall back to html-video.
- React component-driven video — use remotion.
- One-off animation with no CLI tooling requirements — plain html-video is simpler.

## Workflow
1. Detect the environment: check whether the HyperFrames CLI exists on PATH (run
   its version and help commands) and whether an API key is present in the
   environment. If installation is needed, follow the official package manager
   instructions — never pipe remote shell scripts straight into a shell.
2. Read the installed CLI's own help and docs before scripting commands; flags and
   template contracts vary between versions. Do not guess command syntax.
3. Author the template: one HTML document describing the scenes, each with a fixed
   duration. Apply html-video determinism rules: seeded values, preloaded fonts
   and assets, a single timeline, no wall-clock reads.
4. Compose scenes per the CLI's template contract (its documented scene markers or
   timing attributes); keep styles inline and assets local so the template is
   self-contained.
5. Preview first: run the CLI's preview or local serve mode and verify scene order,
   timing, and layout in the browser before spending any render quota.
6. Render: invoke the render command with explicit output path, resolution, and
   fps. Keep the API key in an environment variable — never in the template, the
   command history, or logs.
7. Verify the output: duration, resolution, and audio sync against the spec;
   re-render failed or drifted scenes rather than the whole video when supported.
8. Fallback: if the CLI is missing, unauthenticated, or failing after one retry,
   render the same template with the html-video approach (headless browser frame
   capture plus encoder) and say so in the delivery note.

## Output
- Rendered MP4 from the HyperFrames pipeline (or the fallback path), plus the
  template HTML file and the exact commands used.

## Routing
- No CLI available or fallback rendering: html-video.
- React-driven or data-parametrized video: remotion.
- Scene art direction: frontend-design.

## Guardrails
- Never hard-code API keys in files, templates, or commits.
- Verify the CLI version and help output before scripting against it.
- Keep the HTML template as the single source of truth so the fallback path can
  consume it unchanged.
- Distinguish auth/quota errors from code errors before switching approaches.
