---
name: threejs
description: >-
  Build Three.js 3D web scenes: renderer, camera, and lights setup, GLTF model
  loading, materials, postprocessing, performance via instancing and LOD, plus
  WebGPU notes. Use when adding 3D visuals, product viewers, or animated 3D
  backgrounds to web pages. Keywords: Three.js, WebGL, 3D, GLTF, scene, camera,
  lighting, materials, postprocessing, instancing, LOD, đồ họa 3D, cảnh 3D, mô hình
  3D. Dùng khi cần dựng cảnh 3D trên web với Three.js.
license: MIT
version: 1
---

# Three.js

Build interactive 3D web scenes with Three.js: a correct render loop first, then
content, materials, and only the performance work the scene actually needs.

## When to use
- 3D product viewers, model showcases, or interactive 3D explainers.
- Animated 3D backgrounds or scene-based effects on web pages.
- Loading and presenting GLTF/glTF models with proper lighting and tone mapping.

## When NOT to use
- Pure 2D procedural effects — a fragment shader (shader skill) is simpler.
- Video production from 3D animation — capture through html-video.
- Page design and layout around the canvas — frontend-design.

## Workflow
1. Decide integration: standalone page, embedded canvas in an existing app, or
   React (prefer react-three-fiber only if the codebase is already React).
2. Scaffold the core: `WebGLRenderer` with antialias, device pixel ratio clamped
   to 2, a `PerspectiveCamera`, a resize handler, and an animation loop stepped by
   clock delta (never assume constant frame time).
3. Light the scene: hemisphere or ambient fill plus one directional key light;
   enable shadows only on that key light with a tuned shadow map size.
4. Add content: procedural `Geometry` + `MeshStandardMaterial` for simple shapes;
   `GLTFLoader` (with DRACO/KTX2 loaders when assets require) for models; center
   and scale loaded models to fit the camera framing.
5. Make it look real: environment map via `PMREMGenerator`, ACES filmic tone
   mapping, sRGB output encoding.
6. Add interaction: OrbitControls for viewers, raycasting for picking, pointer
   events that work with touch.
7. Optimize only where measured: `InstancedMesh` for repeated objects, merge static
   geometry, LOD for heavy models, compressed textures, and `dispose()` for every
   geometry/texture/renderer on teardown.
8. Consider postprocessing (bloom, depth of field) only if the design needs it —
   note the mobile cost; mention the WebGPU renderer as an opt-in for modern
   targets only.
9. Verify: screenshot in a browser, check the console for warnings, test on a
   touch device profile, and confirm FPS with real content.

## Output
- Working scene code integrated into the target page, 60fps on desktop hardware,
  graceful on mobile, with all resources disposed on unmount.
- A note on the heaviest draw calls and what was done (or deferred) about them.

## Routing
- 2D procedural backgrounds and effects: shader.
- Design of the page around the 3D canvas: frontend-design or ui-styling.
- Rendering the 3D animation to video: html-video.

## Guardrails
- Always dispose geometries, materials, textures, and the renderer on teardown.
- Clamp pixel ratio; never render at native DPR on 4K mobile screens.
- Do not block first paint on model loading — show a progress state.
- Verify scenes work without a mouse (touch and keyboard fallbacks).
