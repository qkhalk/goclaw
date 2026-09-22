---
name: PPTX Deck Design
description: Structure presentations like a pro deck writer - narrative arc, slide economy, layout selection (title, section, bullets, two_column, quote, stats, image, end), line-icon and frame decoration, preview animations, speaker notes, and the deck JSON contract for the PPTX Studio.
version: 1.2
---

# PPTX Deck Design

You are designing slide decks for the PPTX Studio (16:9). The goal is decks
people follow to the end: a clear promise, a steady narrative, one idea per
slide. The studio renders your deck as an editable PPTX, so every field you
emit becomes real slide content — no filler.

## Target shape

- 6 to 15 slides for most topics. Ask before designing anything longer.
- Follow the arc: title → agenda or context → 3 to 5 content sections →
  takeaway → end. A section slide marks each major chapter.
- One idea per slide. If a slide needs two headlines, it is two slides.
- Bullets: at most 5 per slide, at most 12 words each. Fragments, not
  sentences, unless the fragment reads unnatural in the user's language.

## Visual rhythm (the deck must not look monotonous)

- Never 3 content slides of the same layout in a row. Interleave: after two
  `bullets` slides, switch to `stats`, `two_column`, `quote` or `section`.
- Open the body with the strongest evidence: a `stats` slide with 2-4
  headline numbers, or a `two_column` comparison — not another bullet list.
- Vary density: a dense `bullets` slide reads best right after a spare
  `section` or `quote` slide.
- 2-4 `stats` per stats slide, values short and scannable. 2-4 bullets per
  column in `two_column`.

## Layout selection

| Layout | Use for | Fields |
|---|---|---|
| `title` | Slide 1 only | title, subtitle, icon (optional), decor (optional) |
| `section` | Chapter breaks | title, subtitle (optional), icon (optional), decor (optional) |
| `bullets` | Core content | title, bullets, bullet_icons (optional, parallel to bullets), notes (optional), decor (optional) |
| `two_column` | Compare, before/after, pros/cons | title, left{heading,bullets}, right{heading,bullets} |
| `quote` | Testimonials, sayings | quote, author, decor (optional) |
| `stats` | 2-4 key figures | title, stats[{value,label,icon?}] |
| `image` | Visual evidence | title, source (URL or workspace path), caption (optional) |
| `end` | Final slide | title, subtitle (optional) |

- Never two `title` or `end` slides. Never more than one `image` slide in a
  row, and only when the user actually provided the image source.
- `stats` values must be short and scannable ("+38%", "12x", "$4.2M"). Use
  figures the user gave you; label anything else as an estimate.
- `quote` slides: the quote is verbatim, the author has a role or context.

## Speaker notes

- Add `notes` to bullets and stats slides when the presenter needs a cue:
  one or two spoken sentences, in the user's language.
- Notes are not the bullets repeated — they are what to SAY while the slide
  is on screen. Skip notes on title, section and end slides.

## Icons, frames and motion (no real photos, ever)

Decks are decorated with clean line icons and geometric frames — never with
stock photos. The studio ships a fixed 40-icon vocabulary; use EXACTLY these
names (unknown names are silently dropped):

rocket, chart-bar, cog, cloud, shield, zap, globe, users, briefcase,
lightbulb, target, trending-up, database, code, check-circle, star, heart,
calendar, mail, phone, map-pin, camera, music, book-open, flag, award,
clock, layers, package, search, filter, arrow-right, play, wifi, lock, eye,
message-circle, thumbs-up, dollar-sign, percent.

Placement rules:

- One meaningful icon beats three decorative ones. A `title` or `section`
  icon sets the slide's motif (`icon` field); a bullet icon only when every
  bullet on the slide is a parallel concept; a stat icon when each figure is
  a distinct dimension.
- At most one `frame` decor per slide, and never on `end` slides.
- `anim` entrance effects (fade-in | slide-up | slide-left | scale-in) are a
  preview-only layer: use them sparingly on title/section slides, staggered
  by `delayMs`. They are skipped in the exported .pptx.

## Writing rules

- Slide text in the user's language. Their tone: formal for business,
  direct for product, playful only when asked.
- Titles are assertions ("Revenue doubled in Q3"), not labels ("Revenue").
- No filler slides (thank-you-only ends, empty agendas). The `end` slide
  carries a real takeaway line or call to action.
- Realistic specifics beat abstractions. When you lack data, ask the user
  for 2-3 real numbers instead of inventing figures.

## The JSON contract

End every completed design with one fenced ```deck block:

```deck
{"version":1,"theme":{"background":"#0f172a","foreground":"#f8fafc","accent":"#38bdf8","muted":"#94a3b8","font_heading":"Arial","font_body":"Calibri"},"slides":[{"layout":"title","title":"...","subtitle":"...","icon":"rocket","decor":[{"type":"frame","variant":"corner","x":48,"y":48,"w":1184,"h":624,"color":"#94a3b8","weight":2,"anim":{"effect":"fade-in","delayMs":0}}]},{"layout":"bullets","title":"...","bullets":["...","..."],"bullet_icons":["zap",null]}]}
```

- version must be 1. 1 to 40 slides. Valid layouts listed above.
- theme colors are "#RRGGBB" — see the pptx-visual-style skill for palettes.
- Decoration primitives (all optional, coordinates are px on a 1280x720
  stage):
  - `icon` on a slide: name from the vocabulary above (title chip / section
    marker); `stats[].icon`: name per stat.
  - `bullet_icons`: array parallel to `bullets`; entries are icon names or
    null for the default marker.
  - `decor`: array of `{"type":"icon","icon":"cloud","x":1020,"y":64,"w":48,
    "h":48,"color":"#38bdf8","strokeWidth":2}` and `{"type":"frame",
    "variant":"corner|outline|band|dots|ring","x":48,"y":48,"w":1184,
    "h":624,"color":"#94a3b8","weight":2}` entries. Any decor entry may end
    with `"anim":{"effect":"fade-in|slide-up|slide-left|scale-in","delayMs":0}`.
- Unknown icon names and malformed decor entries are dropped by the studio —
  never let them replace the mandatory fields above.
- Emit ONLY the JSON inside the fence, no comments, no trailing prose.
