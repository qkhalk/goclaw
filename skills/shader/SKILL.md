---
name: shader
description: >-
  Write GLSL fragment shaders for procedural graphics: SDF shapes, value/simplex/
  cellular noise, fBm, HSB color, animation via uniforms, minimal WebGL boilerplate,
  and debugging tips. Use for visual effects, animated backgrounds, and generative
  art on the web. Keywords: GLSL, WebGL, fragment shader, SDF, noise, fBm, procedural,
  generative art, vertex, uniform, shader đồ họa, hiệu ứng, nền động, nghệ thuật
  thuật toán. Dùng khi cần viết shader GLSL, hiệu ứng đồ họa thủ tục hoặc nền động
  WebGL.
license: MIT
version: 1
---

# Shader

Create GLSL fragment shaders for procedural graphics: signed distance fields, layered
noise, palette-based color, and time-driven animation inside a tiny WebGL page.

## When to use
- Animated backgrounds, hero effects, transitions, or generative art on a web page.
- Procedural imagery: terrain, clouds, fire, water, plasma, patterns.
- Converting a visual idea into math: SDF shapes blended with smooth minimum.

## When NOT to use
- Full 3D scenes with meshes, cameras, lights, models — use threejs.
- UI animation that plain CSS transitions handle fine — use frontend-design.
- Long-form video rendering — pipe the shader through html-video instead.

## Workflow
1. Clarify the target: standalone HTML with a WebGL canvas, or a fragment shader for
   an existing engine. Note target resolution and frame rate.
2. Scaffold minimal boilerplate: canvas, WebGL context, fullscreen quad, vertex
   shader passing UVs, fragment shader receiving `u_resolution` and `u_time`.
   Keep the JavaScript under ~60 lines.
3. Build the fragment shader in layers, testing each in isolation:
   - UV setup: aspect-correct, centered coordinates.
   - Shapes: SDF primitives (circle, box) combined with smooth min/max.
   - Noise: hash function, then value/simplex/cellular noise; stack octaves as fBm.
   - Color: map the scalar field through a cosine palette or HSB conversion;
     2-3 hues max, plus a vignette.
4. Animate exclusively via uniforms (`u_time`, `u_mouse`); keep motion continuous.
   Seed any randomness with fixed constants so output is reproducible.
5. Debug systematically: print intermediate values as grayscale, isolate one layer
   at a time, clamp outputs to expose NaN black holes, and watch out for `mediump`
   precision artifacts on mobile GPUs.
6. Optimize: prefer 2D noise where 3D is unnecessary, keep per-pixel loop counts
   bounded, and hoist invariant computation out of loops.
7. Write the file with write_file, open it via web_browse or exec, screenshot, and
   iterate on composition, palette, and motion until it looks intentional.

## Output
- A single self-contained HTML file with embedded GLSL and minimal JS boilerplate,
  running near 60fps on a typical laptop, deterministic across runs.
- The fragment shader is commented per layer (UV, shape, noise, color, motion).

## Routing
- 3D scenes with geometry, models, or lighting: threejs.
- Turning the shader into video: html-video (frame-accurate capture).
- Integrating the effect into a designed page: frontend-design.

## Guardrails
- Fixed seeds everywhere — no frame output may depend on wall-clock randomness
  beyond the animated time uniform.
- Verify mobile precision (`mediump` vs `highp`) before claiming support.
- Do not copy licensed shader code verbatim; implement from technique descriptions.
- Provide a static fallback frame for `prefers-reduced-motion` users.
