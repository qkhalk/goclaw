---
name: Video Storyboard Design
description: Structure short vertical videos like a pro editor - hook, pacing, scene rhythm, captions, and the storyboard JSON contract for the video pipeline.
version: 1
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

- Ask the user for 2-4 photos or URLs when the topic suits real footage
  (news, products, places). Images make videos feel produced, not generated.
- When the request references an article or page (news recaps, product
  launches), fetch it with web_fetch and mine real image URLs before
  designing: the og:image meta tag, the hero photo, and inline article
  photos. Use DIRECT image URLs (ends .jpg/.jpeg/.png/.webp, or a CDN image
  link) as scene "source"; the render worker downloads them at render time.
- Skip logos, avatars, icons, ads and tracking pixels — photos only. Pick
  the 2-4 strongest, visually distinct images that map to your scene facts.
- If the fetch fails or yields nothing usable, fall back to a color-scene
  design; never invent or guess image URLs.
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

## Voice-over (narration)

- When the user asks for voice, spoken audio, TTS, or a narrated video, add a
  narration object to every scene: "narration": {"text": "..."}.
- Narration text is spoken, not read: full natural sentences in the user's
  language, about 12-15 words for a 4s scene, 8-10 words for a 3s scene.
  Never paste the caption into narration; the caption is the on-screen
  headline, narration is the voice-over sentence.
- Vietnamese is spoken well by the default voice, so voice can stay omitted.
  Only set "voice" when the user names a specific voice.
- Without an explicit voice request, omit narration entirely — captions only.

## The JSON contract

Always end the design reply with one fenced ```storyboard block:

```storyboard
{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"output":{"height":720},"scenes":[{"type":"color","color":"#0f172a","duration_sec":3,"caption":{"text":"HOOK LINE","position":"center","font_size":64},"transition":"fade"}]}
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
- output.height is 480, 720 or 1080. Use 720 for social posts.

## Revision etiquette

When the user sends the current storyboard back with a request ("shorter",
"bluer", "more energy"), return a FULL new storyboard block with the change
applied, not a diff. Keep what worked; change only what they asked.
