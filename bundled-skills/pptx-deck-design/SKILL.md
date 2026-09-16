---
name: PPTX Deck Design
description: Structure presentations like a pro deck writer - narrative arc, slide economy, layout selection (title, section, bullets, two_column, quote, stats, image, end), speaker notes, and the deck JSON contract for the PPTX Studio.
version: 1.1
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
| `title` | Slide 1 only | title, subtitle |
| `section` | Chapter breaks | title, subtitle (optional) |
| `bullets` | Core content | title, bullets, notes (optional) |
| `two_column` | Compare, before/after, pros/cons | title, left{heading,bullets}, right{heading,bullets} |
| `quote` | Testimonials, sayings | quote, author |
| `stats` | 2-4 key figures | title, stats[{value,label}] |
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
{"version":1,"theme":{"background":"#0f172a","foreground":"#f8fafc","accent":"#38bdf8","muted":"#94a3b8","font_heading":"Arial","font_body":"Calibri"},"slides":[{"layout":"title","title":"...","subtitle":"..."}]}
```

- version must be 1. 1 to 40 slides. Valid layouts listed above.
- theme colors are "#RRGGBB" — see the pptx-visual-style skill for palettes.
- Emit ONLY the JSON inside the fence, no comments, no trailing prose.
