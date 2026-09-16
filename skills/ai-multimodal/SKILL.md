---
name: ai-multimodal
description: >-
  Analyze images, audio, video, and documents with LLMs: frame selection, transcription
  strategy, chart understanding, and combining modalities. Use whenever a task involves
  understanding media files via read_image, read_video, read_audio, or read_document.
  Keywords: multimodal, vision, audio, video, OCR. Dùng khi cần đọc hiểu ảnh, video, âm
  thanh, tài liệu, biểu đồ.
license: MIT
version: 1
---

# AI Multimodal

Get reliable answers from images, audio, video, and documents with LLMs: pick the right tool per modality, sample media correctly, and combine modalities when one is not enough.

## When to use
- Extracting facts from screenshots, charts, diagrams, or scanned documents
- Summarizing or answering questions about audio recordings or video files
- Choosing between reading frames, audio, or metadata for a video question
- Building a description, transcript, or structured data from media input

## When NOT to use
- Generating new media → `ai-artist`, `create_video`, `create_audio`, `tts`
- Converting or transcoding media files → `media-processing`
- Text already in a plain file → just `read_file`; do not route text through vision

## Workflow
1. Classify input and question first: one image → `read_image`; document/PDF → `read_document` (text layer first, OCR fallback); audio → `read_audio` for transcription or summary; video → `read_video`, then decide whether frames, audio, or both answer the question.
2. For video Q&A: sample frames across the timeline (start, middle, end, plus scene changes) before dense reading; in talking-head videos the audio usually carries the content — prefer `read_audio` there and use frames only for slides or on-screen text.
3. For images: ask specific, enumerable questions ("list every axis label", "read the total") rather than "what is this"; request structured output (JSON) when the data will be processed further.
4. For charts and diagrams: transcribe data points and labels explicitly, then reason over the transcription; mark eyeballed values as approximate.
5. For documents: prefer the embedded text layer over OCR; use OCR (via the `ocr` skill or `read_document`) for scans; capture tables as structured rows and note page numbers for citations.
6. For audio: state the language explicitly if known; request timestamps on long recordings; for multi-speaker content, ask for speaker turns or note the limitation.
7. Combine modalities when evidence may conflict (slide text vs spoken narration): report both and flag disagreements instead of silently picking one.
8. Extract before you infer: quote or transcribe the literal content first, then interpret; never answer purely from a thumbnail-level impression.
9. Deliver structured results: transcript with timestamps, tables of extracted values, per-frame findings, and an explicit list of what could not be read.

## Output
- A structured extraction: transcript (timed if audio/video), data tables from charts/documents, per-image findings with citations (file, page, timestamp), and unresolved items flagged.

## Routing
- Creating media → `ai-artist`, `create_video`, `create_audio`, `tts`
- Transcoding/conversion → `media-processing`; OCR pipeline details → `ocr`
- Archiving extracted knowledge → `graphify` or vault tools

## Guardrails
- Treat extracted numbers and text as OCR-grade evidence; verify critical values before acting on them.
- Never fabricate unreadable content; mark it unreadable with the attempted method.
- Respect privacy: do not transcribe or describe personal data beyond the task's need.
- Mind context budgets: sample long media first; do not paste raw long transcripts into the answer.
