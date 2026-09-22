---
name: PPTX Visual Style
description: Theme systems for the PPTX Studio - palette construction with contrast rules, dark and light deck themes, PowerPoint-safe font pairings, per-layout typography scale, and icon/frame decoration color roles for slide JSON.
version: 2
---

# PPTX Visual Style

You are choosing the `theme` of a deck JSON: background, foreground, accent,
muted colors and two fonts. The studio renders slides at presentation scale,
so contrast and restraint matter more than decoration.

## Palette construction

Pick a strategy before picking colors:

- **Dark deck** (default for screens): one tinted near-black background
  (never pure black), warm-white foreground, one saturated accent, one muted
  gray-blue for secondary text. Example family: background #0f172a,
  foreground #f8fafc, accent #38bdf8, muted #94a3b8.
- **Light deck** (default for print or bright rooms): off-white background
  tinted toward the accent hue (never pure white), near-black foreground,
  one saturated accent, mid-gray muted. Example: #faf9f6 / #1c1917 /
  #b45309 / #78716c.
- One accent only. It marks section numbers, bullet markers, stat values
  and rules — never body text, never whole backgrounds.

Rules:

- background↔foreground contrast at least 4.5:1 (7:1 preferred). When in
  doubt darken the background, not the accent.
- muted text must still pass 4.5:1 on the background. If it fails, lighten
  muted instead of shrinking it.
- accent must pass 3:1 against the background for large text and markers.
- Vary hue family between decks: deep blue, forest green, warm plum, slate
  teal, burnt sienna, ink indigo. Never the same family twice in a row for
  the same user.
- Never pure #000000 or #ffffff anywhere. Never neon accents on light
  backgrounds.

## Fonts (PowerPoint-safe pairings)

Both fonts must come from: Arial, Calibri, Georgia, Verdana, Tahoma,
Trebuchet MS, Times New Roman, Courier New. Pair on a contrast axis:

| Mood | font_heading | font_body |
|---|---|---|
| Corporate / neutral | Arial | Calibri |
| Editorial / classic | Georgia | Verdana |
| Friendly / soft | Trebuchet MS | Calibri |
| Vintage / formal | Times New Roman | Tahoma |
| Technical / code-heavy | Courier New | Arial |

- Vietnamese and CJK content: Arial, Calibri, Verdana, Tahoma and Georgia
  render diacritics reliably; avoid Trebuchet MS for long Vietnamese body
  text.
- One pairing per deck. Never two serifs, never two similar sans.

## Typography scale (per layout, for preview harmony)

- `title` slide: title huge (the biggest text in the deck), subtitle at
  roughly a third of its size, muted.
- `section`: title slightly smaller than the title slide, accent-tinged.
- `bullets`: title commanding, bullets generous body size, one line each
  wherever possible.
- `stats`: values are the heroes (near title-slide size, accent color),
  labels small and muted.
- `quote`: quote text large and italic-feeling, author small and muted.

## Icon and frame color roles

Decks are decorated with line icons and geometric frames (see the
pptx-deck-design skill for the vocabulary and JSON fields). Decoration colors
follow the same one-accent discipline:

- Icons read in the accent color by default — the studio fills `color`
  from the theme accent when omitted. Keep it that way unless the icon sits
  on an accent-filled chip; then use the background color for contrast.
- Frames: `band` takes the accent; `corner`, `outline`, `dots` and `ring`
  look best in the muted color (or omitted — muted is their default). A
  frame in full-strength accent competes with the type.
- Decoration must never be the loudest thing on the slide: if squinting at
  the slide shows the frame before the title, lower the decoration contrast.
- No real photos, no gradients, no shadows. Icons and frames are flat
  geometry, exactly like the palette.

## Self-check before emitting the theme

1. All five fields present, all colors #RRGGBB, both fonts on the safe list.
2. Contrast pairs pass the thresholds above.
3. Exactly one accent, used sparingly in the deck design.
4. The palette is not the one you used for this user's previous deck.
