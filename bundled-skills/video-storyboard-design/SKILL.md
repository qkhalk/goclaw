---
name: Video Storyboard Design
description: Structure short vertical videos like a pro editor - hook, pacing, scene rhythm, captions, and the storyboard JSON contract for the video pipeline.
version: 2
---

# Video Storyboard Design

You are designing storyboards for short vertical videos (9:16, 1080x1920).
The goal is videos people watch to the end: a strong hook in the first 3
seconds, steady rhythm, one idea per scene.

## Target shape

- Total length 20 to 60 seconds. Never open with a 2-minute plan.
- 3 to 8 scenes. Fewer, better scenes beat many weak ones.
- 2 to 4 seconds per scene for energy; 5 to 6 seconds only for slow, calm
  moods. The hard cap per scene is 30s but staying under 6 keeps retention.

## The 3-beat structure

1. HOOK (scene 1, 2-3s): a color scene with one bold line, font_size 56-72.
   State the most surprising fact or the promise. No throat-clearing like
   "Welcome back" or "Today we will".
2. BODY (middle scenes): alternate image scenes and color scenes. Each scene
   carries exactly one fact or one step. Pair every image with a short
   caption (max 8 words, bottom position) so it reads like news footage.
3. CLOSE (last scene, 3-4s): a color scene with a takeaway line or question
   that invites comments ("What would you build?").

## Image scenes

- Real imagery is the DEFAULT, not the fallback. When the user gives no
  photos, run image_search yourself: 2-4 English keywords per visual beat
  (e.g. "halong bay sunset", "server room datacenter") and pick the direct
  image URLs for your image scenes. Aim for images in half to two-thirds of
  the scenes — a video of only colored cards reads as unfinished.
- When the request references an article or page (news recaps, product
  launches), fetch it with web_fetch and mine real image URLs before
  designing: the og:image meta tag, the hero photo, and inline article
  photos. Use DIRECT image URLs (ends .jpg/.jpeg/.png/.webp, or a CDN image
  link) as scene "source"; the render worker downloads them at render time.
- Skip logos, avatars, icons, ads and tracking pixels — photos only. Pick
  the 2-4 strongest, visually distinct images that map to your scene facts.
- Only go all-color when the user asks for text-only or neither image_search
  nor web_fetch yields anything usable. Never invent or guess image URLs —
  only URLs returned by image_search, found via web_fetch, or given by the
  user.
- Apply subtle ken_burns to every static image (zoom_from 1.0, zoom_to 1.12)
  and vary pan between scenes using the direction words left, up, right
  (e.g. "ken_burns": {"zoom_from": 1.0, "zoom_to": 1.12, "pan": "left"}).
- Never place two image scenes back to back without different pan or zoom
  directions; never place two color scenes with similar colors back to back.

## Captions

- Max 8 words. If the line does not fit, split the scene in two.
- Write captions in the user's language. Use their tone: punchy for social,
  neutral for news.
- font_size: 56-72 for the hook and close, 40-52 for image captions.
- Position: center for color scenes, bottom for image scenes.
- Style per beat: "chip" (rounded dark chip behind the text) for hooks,
  prices and stat lines; "mono" (monospace, reads like a code eyebrow,
  e.g. "// PHAN 1") for section labels; plain for the rest. Add the style
  inside the caption object: {"text": "...", "style": "chip"}.
- With narration, captions reveal word-by-word in sync with the voice —
  write the caption as the compressed headline of the narration sentence,
  never the same full sentence twice.

## Voice-over (narration)

- When the user asks for voice, spoken audio, TTS, or a narrated video, add a
  narration object to every scene: "narration": {"text": "..."}.
- Narration text is spoken, not read: full natural sentences in the user's
  language. Pacing depends on language speed: Vietnamese is spoken at
  ~2.3 words/sec so use 9-11 words for 4s, 6-8 words for 3s. English is
  faster (~2.8 wps) so 11-12 words for 4s, 8-9 for 3s. Never paste the
  caption into narration; the caption is the on-screen headline, narration
  is the voice-over sentence.
- Vietnamese is spoken well by the default voice, so voice can stay omitted.
  Only set "voice" when the user names a specific voice.
- Without an explicit voice request, omit narration entirely — captions only.

## Layers (timed overlays)

Scenes support timed overlay layers (max 8 per scene) drawn over the visual
and under the caption — use them to emphasize a price, badge, keyword or a
logo instead of stuffing everything into the caption:

- `{"kind": "text", "text": "SALE 50%", "y": 0.3, "font_size": 72,
  "fill": "#FACC15", "start": 0.5, "duration": 2}` — keyword/price pop.
- `{"kind": "shape", "shape": "rect", "x": 0.1, "y": 0.15, "w": 0.8,
  "h": 0.18, "fill": "#000000", "opacity": 0.55}` — translucent scrim behind
  a caption or text layer.
- `{"kind": "image", "source": "media/logo.png", "x": 0.75, "y": 0.08,
  "w": 0.18}` — corner logo/watermark.

Geometry is normalized 0..1 from the top-left (x, y, w; shapes also h).
Timing is scene-relative seconds via `start` + `duration` (duration 0 = to
the scene end). All layers accept `opacity` 0..1; text layers also `align`
(left/center/right within the box). Layers render on every surface: browser
preview, client export and the server renderer.

## The JSON contract

Always end the design reply with one fenced ```storyboard block:

```storyboard
{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"output":{"height":720},"scenes":[{"type":"color","color":"#0f172a","duration_sec":3,"caption":{"text":"HOOK LINE","position":"center","font_size":64},"transition":"none"},{"type":"image","source":"https://example.com/photo.jpg","duration_sec":4,"ken_burns":{"zoom_from":1.0,"zoom_to":1.12,"pan":"left"},"caption":{"text":"Key fact here","position":"bottom","font_size":44},"layers":[{"kind":"text","text":"NEW","x":0.68,"y":0.1,"w":0.25,"font_size":56,"fill":"#FACC15","start":0.5,"duration":2}],"transition":"crossfade"},{"type":"color","color":"#0f172a","duration_sec":3,"caption":{"text":"What would you build?","position":"center","font_size":56},"transition":"fade"}]}
```

Field rules that fail rendering when broken:
- version is always 1; canvas defaults to 1080x1920 at 30 fps.
- image and video scenes MUST have source (https URL or workspace path).
- color scenes MUST have color as #RRGGBB.
- duration_sec is required, 1 to 30 per scene.
- ken_burns.pan is a single word: "none", "left", "right", "up" or "down" —
  never a coordinate object like {"from_x":...}. Vary pan between scenes.
- caption.position is one of top, center, bottom.
- narration, when used, is an object: {"text": "...", "voice": "optional"}.
- layers, when used, is an array (max 8) of layer objects: text layers need
  text; shape layers are kind "shape" with shape "rect" and a #RRGGBB fill;
  image layers need source; card layers take fill + opacity (0.08..0.25
  reads as a glass panel) + radius 0..0.2; icon layers take "icon": "<name>"
  (check, zap, users, cpu, database, git-branch, globe, heart, star,
  trending-up, shield, layers, code, terminal, book-open, message-circle,
  clock, eye, lock, package, settings, bar-chart-2, arrow-right, download,
  play, target, search, calendar, camera, music, wifi, cloud, coffee) with
  fill as the stroke color; "chip": true puts the glyph on a tinted rounded
  tile in the same color. Text layers accept "font": "body" (default),
  "display" (bold — headlines, big numbers) or "mono" (eyebrow labels like
  // PART 1, code, metrics). Card layers accept "border": true (contrast
  ring — use it when the card tone sits close to the background). Any layer
  accepts "anim": "fade"|"up"|"down"|"left"|"right"|"pop" (a ~0.45s
  entrance at its start). start must be inside the scene and
  start+duration must not exceed the scene's duration_sec.
- transition is the enter transition for each scene: "none", "fade",
  "crossfade", "slide_left", or "slide_up". Default to "crossfade" for the
  first body scene and "fade" for the closing scene. Omit or "none" only
  when a hard cut is intentional (e.g. hook scene).
- output.height is 480, 720 or 1080. Use 720 for social posts.

## Composed frames (no photo needed)

When a beat has no strong photo, don't settle for a bare caption — build the
frame from layers: a translucent card panel as the stage, an icon carrying
the meaning, and a short text layer. Give each an entrance animation and
stagger starts 0.25-0.35s apart so the frame builds up while the narrator
speaks. Example:

```json
{"type":"color","color":"#0D1117","color2":"#1E293B","grid":true,"glow":"#38BDF8","vignette":true,"duration_sec":4,
 "layers":[
   {"kind":"card","x":0.1,"y":0.34,"w":0.8,"h":0.22,"fill":"#1E293B","opacity":0.5,"radius":0.03,"anim":"up","start":0.4},
   {"kind":"icon","icon":"git-branch","x":0.14,"y":0.38,"w":0.11,"fill":"#A78BFA","anim":"pop","start":0.7},
   {"kind":"text","text":"2.000 contributors","x":0.3,"y":0.41,"font_size":56,"fill":"#FFFFFF","anim":"left","start":0.95}
 ]}
```

Use one composed-frame grammar across the video: same card opacity, one
icon stroke color family, one entrance direction family.

## Typography discipline

- Headlines: "font": "display", max 14 characters per line. Split longer
  headlines into two stacked text layers (second line starts ~0.2s later) —
  never shrink below 48px or let text overflow its box.
- Eyebrows: "font": "mono", short uppercase labels with a // prefix, in the
  accent color, above the headline.
- Add a thin accent bar between eyebrow and headline: a card layer with
  h ≈ 0.008, w ≈ 0.1, full opacity, in the accent color, "anim": "left".

## Revision etiquette

When the user sends the current storyboard back with a request ("shorter",
"bluer", "more energy"), return a FULL new storyboard block with the change
applied, not a diff. Keep what worked; change only what they asked.
