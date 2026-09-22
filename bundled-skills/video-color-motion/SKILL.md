---
name: Video Color and Motion
description: Pick palettes and motion that look designed, not generated - harmonious hex colors, gentle Ken Burns, readable captions, and tasteful transitions.
version: 2
---

# Video Color and Motion

Design taste in one page: which colors to pair, how much motion to add, and
which transitions to use so a short vertical video feels intentional.

## Palette building blocks

Pick ONE scheme per video and stay in it. All values are valid #RRGGBB.

- Midnight tech: #0f172a, #1e293b, #312e81 accents #38bdf8, #a5b4fc. Best
  for AI, robotics, finance, dev tools.
- Warm news: #fef3e2, #fde68a accents #c2410c, #7c2d12. Best for human
  stories, food, community topics.
- Fresh minimal: #f8fafc, #e2e8f0 accents #0f766e, #134e4a. Best for
  productivity, tutorials, calm explainers.
- Bold alert: #450a0a, #7f1d1d accents #fbbf24, #fca5a5. Best for urgent
  warnings, price drops, breaking topics.

Rules:
- Background colors must sit far enough from white that white captions pass
  contrast; dark backgrounds (luminance under 25%) with white text are the
  safe default.
- Never repeat the same background color on consecutive scenes; step through
  the scheme (darkest to second, then an accent-dark mix) so cuts feel
  deliberate.
- Use at most one saturated accent per scene; the rest stays quiet.

## Motion that reads as quality

- Ken Burns on images: zoom_from 1.0 to zoom_to 1.12 (in) or 1.12 down to
  1.0 (out). Anything above 1.15 looks like a screensaver.
- Pan: pick one direction per scene and alternate between scenes
  (left, then up, then right). Never pan two scenes the same way in a row.
- Keep zooms slow: the motion should be felt, not noticed.
- Color scenes need no zoom; their motion is the cut itself.

## Color-scene atmosphere (glow + vignette)

Dark color scenes come alive with two optional fields — use them by default
on dark scenes, skip them on light backgrounds:

- `"glow": "#RRGGBB"` — two soft orbs tinted with this color drift slowly
  behind the text. Pick the accent from the scene's own scheme (Midnight
  tech → #38bdf8, Bold alert → #fbbf24, Warm news → #c2410c). One glow
  color per video keeps the look coherent.
- `"vignette": true` — gently darkens the frame edges; adds depth to flat
  gradients at zero cost to readability.
- `"grid": true` stays a deliberate choice for tech/developer topics.
- `"grain": true` adds subtle animated film grain — sparingly, at most on
  one or two scenes for texture, never everywhere.

## Transitions

- fade: the neutral choice for news, explainers, and anything calm. Default.
- slide_left: forward motion for progressions, lists, and steps.
- crossfade: only between two image scenes with related content.
- Avoid stacking transitions on every scene; identical fades create rhythm.

## Caption rendering

- Captions render in a bold display font (Be Vietnam Pro) with a soft drop
  shadow — crisp on any dark or mid background.
- `"style": "chip"` puts the line on a rounded translucent chip: use it for
  hooks, prices and stat lines. `"style": "mono"` renders a monospace
  eyebrow (JetBrains Mono) for section markers like "// PHAN 1".
- All-caps only for hooks under 5 words; sentence case otherwise.
- Numbers with units read better split: "10.000 robot mỗi năm" beats
  "10k robots/yr" in Vietnamese copy.
- With narration the caption reveals word-by-word with the voice, so the
  caption can be the headline while narration carries the sentence.

## Pre-flight check before you answer

1. Consecutive scenes never share a background color or pan direction.
2. Every image scene has ken_burns with zoom within 1.0 to 1.12.
3. Every caption is 8 words or fewer and positioned for its scene type.
4. The whole video uses one palette scheme, one glow accent and one
   transition family.
5. Dark color scenes set vignette true and a glow from the scheme.
6. Composed frames stagger their layer starts 0.25-0.35s apart; entrance
   directions and card opacity stay consistent across the video.
7. Headlines use "font": "display" and stay under 14 characters per line;
   eyebrows use "font": "mono"; icons that carry meaning sit on
   "chip": true tiles.
8. The storyboard block is valid JSON on a single fence, version 1.
