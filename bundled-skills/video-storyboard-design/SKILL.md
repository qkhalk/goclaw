---
name: Video Storyboard Design
description: Structure short-form videos like a pro editor - scene pacing and narrative arcs (tech news, product teaser, quote, photo story), transition choice, ken_burns motion, punchy captions, TTS narration tone, scene-source strategy, and the storyboard JSON contract for the Video Studio.
version: 1
---

# Video Storyboard Design

You are designing storyboards for the Video Studio (vertical 1080x1920 by
default). The editor previews your storyboard in the browser and can render
it to video, so every scene you emit becomes real footage — no filler scenes.

## Pacing

- 2 to 5 seconds per scene for most beats. Under 2s only for hard cuts in a
  rapid montage; over 5s only when narration genuinely needs the room.
- 8 to 20 scenes covers most short-form videos (30-75 seconds total).
  Ask before designing anything longer; never exceed 60 scenes.
- The first 2 scenes are the hook: the strongest visual plus the boldest
  claim. Viewers decide in 3 seconds whether to keep watching.
- Vary rhythm: follow a long narration-heavy scene with a short punchy one.
  Three same-length scenes in a row feels mechanical.

## Narrative arcs

- **Tech news**: hook (the headline) → what happened → why it matters →
  concrete number or demo → what to watch next. Keep it to 6-10 scenes.
- **Product teaser**: hook (the pain or the promise) → problem in one scene
  → product reveal → 2-4 feature beats, one feature per scene → call to
  action. Features go slow (4-5s), the reveal fast (2-3s).
- **Quote**: the quote itself, split across 2-3 scenes if long → who said it
  and why it matters → takeaway. Color scenes with generous captions work
  well here; keep transitions gentle (fade, crossfade).
- **Photo story**: one scene per photo, chronological or emotional order →
  ken_burns every photo → end on the strongest image with the takeaway.

## Scene sources: browser vs server

- `icon` and `gradient` scenes render in the browser editor only — the
  server render pipeline skips them. When the user will render on the
  server, build on `color` scenes with captions or real `image`/`video`
  sources instead.
- Only use `image`/`video` sources the user provided or approved. Never
  invent URLs.
- `icon` scenes are great for hook beats in browser-only workflows: one
  symbol (zap, flame, trending-up) on a strong color field says more than a
  caption.
- `color` scenes are the workhorse for text beats, section breaks and
  quotes — pair a deep background with a large centered caption.

## Motion (ken_burns)

- Every static `image` scene gets ken_burns: zoom from 1.0 to 1.08-1.15.
  Static photos read dead on screen; slight motion reads cinematic.
- Zoom in (1 → 1.1) to build energy, zoom out (1.1 → 1) to settle or close.
- Pan (left/right/up/down) when the image has an off-center subject; combine
  with mild zoom, not both aggressively.
- Skip ken_burns on `video` scenes (they already move) and on `color` scenes
  (nothing to move).

## Transitions

| Transition | Use for |
|---|---|
| `fade` | Default scene change, chapter breaks, quiet moments |
| `crossfade` | Photo stories, mood shifts between similar scenes |
| `slide_left` / `slide_up` | Lists, steps, progressions ("next point") |
| `none` | Rapid montage cuts, hard beats on the music |

- Never mix more than two transition types in one video unless it is a
  deliberate montage. Consistency reads as craft.
- The final scene fades or crossfades out; never ends on a hard `none` cut
  unless the content is punchy by design.

## Captions

- At most 8 words per caption. Punchy fragments, not sentences, in the user's
  language. Position `bottom` by default, `center` for quotes and hooks.
- Font size 48-64 for vertical video; 40 minimum so phones stay readable.
- Captions are not narration read back — they compress the narration to its
  sharpest 3-8 words.

## Narration (TTS)

- One short spoken sentence per scene, 8-20 words. Active voice, present
  tense where possible. Natural speech: contractions, no stage directions,
  no "scene" language ("in this scene we see..." is never emitted).
- Match the user's tone: formal for business news, direct and energetic for
  product teasers, warm for photo stories.
- Numbers: only figures the user provided or approved; label estimates as
  such in the narration, never invent statistics.

## Palette

- Dark base works best for vertical video: background `#0F172A` or similar
  deep slate, foreground `#F8FAFC`, one accent (`#38BDF8`, `#F97316`,
  `#A78BFA`) used sparingly for hooks and calls to action, muted `#94A3B8`.
- Light themes need foreground/background contrast of at least 4.5:1.
- One accent color per video. Two accents compete with the footage.
- Alternate scene backgrounds subtly (e.g. `#0F172A` and `#1E293B`) so long
  color-scene runs do not strobe between identical frames.

## The JSON contract

End every completed design with one fenced ```storyboard block:

```storyboard
{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"audio":{"bgm_path":"audio/bgm.mp3","bgm_volume":0.2},"output":{"height":720},"scenes":[{"type":"color","color":"#0F172A","duration_sec":3,"caption":{"text":"AI renders got faster","position":"center","font_size":56},"narration":"AI renders just got faster.","transition":"fade"},{"type":"image","source":"https://example.com/hero.jpg","fit":"cover","duration_sec":4,"ken_burns":{"zoom_from":1,"zoom_to":1.1,"pan":"none"},"caption":{"text":"Ten times quicker","position":"bottom","font_size":48},"transition":"crossfade"}]}
```

- version must be 1. Canvas edges max 1920px; output.height 480, 720 or
  1080. duration_sec 1..30 per scene, 60 scenes max.
- Emit ONLY the JSON inside the fence, no comments, no trailing prose.
