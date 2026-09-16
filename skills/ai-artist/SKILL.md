---
name: ai-artist
description: >-
  Generate and refine AI images with the create_image tool: prompt craft (subject, style,
  lighting, composition), negative prompts, aspect ratios, iteration — and when to
  generate vs source imagery. Use for any image generation task. Keywords: AI image,
  prompt, aspect ratio, create_image. Dùng khi cần tạo ảnh AI, vẽ minh họa, tạo hình ảnh
  minh họa nội dung.
license: MIT
version: 1
---

# AI Artist

Produce better generated images through deliberate prompt construction and iteration, and know when generation is the wrong tool compared with sourcing or shooting real imagery.

## When to use
- Illustrations, concept art, hero images, social/og images, mock imagery
- Iterating on a generated image that is close but not right
- Choosing aspect ratio, style, and composition for a visual brief
- Deciding between generating, sourcing stock, or shooting real imagery

## When NOT to use
- Reading or analyzing existing images → `read_image` (see `ai-multimodal`)
- Image post-processing (crop, compress, format conversion) → `media-processing`
- Video or audio generation → `create_video` / `create_audio` workflows

## Workflow
1. Write the brief first in one sentence: subject, action, mood, and usage (hero banner? thumbnail? icon?). Every prompt decision serves this brief.
2. Structure the prompt: subject and action → setting and composition (close-up, wide shot, rule of thirds) → style and medium (photoreal, watercolor, isometric 3D) → lighting and palette (golden hour, high-key, teal-and-orange) → quality terms sparingly.
3. Match aspect ratio to destination: 16:9 hero, 1:1 social tile, 9:16 story, 4:3 doc illustration; state it in the `create_image` call rather than cropping later.
4. Use negative prompts for recurring failure modes (text artifacts, extra fingers, watermarks, cluttered background) instead of restating them positively.
5. Generate a first batch, then inspect candidates with `read_image`; critique against the brief before touching the prompt — change one variable at a time (composition, then style, then lighting) so you learn what each knob does.
6. Iterate toward the brief: reuse the prompt fragments that worked; keep a short changelog of prompt versions tried and what each change did.
7. Add text in images only if the model reliably renders it — otherwise generate clean art and overlay real text with code; generated text is often misspelled.
8. Check usage and ethics: avoid prompting for living artists' signature styles, trademarks, or the likeness of real people; note the generation tool's licensing terms in the deliverable.
9. Deliver with metadata: final prompt, negative prompt, aspect ratio, and tool parameters alongside the file so the result is reproducible.

## Output
- The requested image file(s) plus a reproducibility note: final prompt, negative prompt, aspect ratio, and parameters; for series, a consistent style block reused across prompts.

## Routing
- Inspecting or comparing generated candidates → `read_image`, see `ai-multimodal`
- Compression, cropping, format work → `media-processing`
- Video or audio deliverables → `create_video` / `create_audio` / `render_video`

## Guardrails
- Do not generate deceptive images of real people, credentials, documents, or news events.
- Never prompt for hateful, explicit, or infringing content; refuse and offer alternatives.
- Generated images are drafts until reviewed with `read_image` or by a human — never ship unseen.
- Disclose AI generation when the deliverable could be mistaken for photography.
