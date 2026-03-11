---
name: ai-multimodal
description: Analyze images/audio/video with Gemini API. Generate images (Imagen 4), videos (Veo 3). Use for vision analysis, transcription, OCR, design extraction, multimodal AI.
---

# AI Multimodal

Process audio, images, videos, documents, and generate images/videos using Google Gemini's multimodal API.

## Setup

```bash
export GEMINI_API_KEY="your-key"  # Get from https://aistudio.google.com/apikey
pip install google-genai python-dotenv pillow
```

### API Key Rotation (Optional)

For high-volume usage or when hitting rate limits, configure multiple API keys:

```bash
# Primary key (required)
export GEMINI_API_KEY="key1"

# Additional keys for rotation (optional)
export GEMINI_API_KEY_2="key2"
export GEMINI_API_KEY_3="key3"
```

**Features:**
- Auto-rotates on rate limit (429/RESOURCE_EXHAUSTED) errors
- 60-second cooldown per key after rate limit
- Logs rotation events with `--verbose` flag
- Backward compatible: single key still works

## Quick Start

**Verify setup**: `python {baseDir}/scripts/check_setup.py`
**Analyze media**: `python {baseDir}/scripts/gemini_batch_process.py --files <file> --task <analyze|transcribe|extract>`
**Generate content**: `python {baseDir}/scripts/gemini_batch_process.py --task <generate|generate-video> --prompt "description"`

> **Stdin support**: You can pipe files directly via stdin (auto-detects PNG/JPG/PDF/WAV/MP3).
> - `cat image.png | python {baseDir}/scripts/gemini_batch_process.py --task analyze --prompt "Describe this"`
> - `python {baseDir}/scripts/gemini_batch_process.py --files image.png --task analyze` (traditional)

## Models

- **Image generation**: `imagen-4.0-generate-001` (standard), `imagen-4.0-ultra-generate-001` (quality), `imagen-4.0-fast-generate-001` (speed)
- **Video generation**: `veo-3.1-generate-preview` (8s clips with audio)
- **Analysis**: `gemini-2.5-flash` (recommended), `gemini-2.5-pro` (advanced)

## Scripts

- **`gemini_batch_process.py`**: CLI orchestrator for `transcribe|analyze|extract|generate|generate-video` that auto-resolves API keys, picks sensible default models per task, streams files inline vs File API, and saves structured outputs.
- **`media_optimizer.py`**: ffmpeg/Pillow-based preflight tool that compresses/resizes/converts audio, image, and video inputs, enforces target sizes/bitrates, splits long clips into hour chunks.
- **`document_converter.py`**: Gemini-powered converter that uploads PDFs/images/Office docs, applies a markdown-preserving prompt, batches multiple files.
- **`check_setup.py`**: Interactive readiness checker that verifies Python deps and GEMINI_API_KEY availability.

Use `--help` for options.

## References

Load for detailed guidance:

| Topic | File | Description |
|-------|------|-------------|
| Music | `{baseDir}/references/music-generation.md` | Lyria RealTime API for background music generation. |
| Audio | `{baseDir}/references/audio-processing.md` | Audio formats, transcription, non-speech analysis, TTS models. |
| Images | `{baseDir}/references/vision-understanding.md` | Vision capabilities, captioning, OCR, multi-image workflows. |
| Image Gen | `{baseDir}/references/image-generation.md` | Imagen 4, generate_images vs generate_content APIs, editing. |
| Video | `{baseDir}/references/video-analysis.md` | Video analysis, clipping, FPS control, multi-video comparison. |
| Video Gen | `{baseDir}/references/video-generation.md` | Veo models, text-to-video, image-to-video, camera control. |

## Limits

**Formats**: Audio (WAV/MP3/AAC, 9.5h), Images (PNG/JPEG/WEBP, 3.6k), Video (MP4/MOV, 6h), PDF (1k pages)
**Size**: 20MB inline, 2GB File API
**Important:**
- Audio/video transcription >15 min: split into chunks (max 15 min each) and transcribe separately.
- Video transcription: extract audio via ffmpeg first, then split and transcribe.

## Resources

- [API Docs](https://ai.google.dev/gemini-api/docs/)
- [Pricing](https://ai.google.dev/pricing)
