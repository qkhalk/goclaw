---
name: media-processing
description: "Process video/audio/images on the host with FFmpeg and ImageMagick: format conversion, compression, trimming/cutting, extracting audio, GIF creation, thumbnails/resize/watermark, batch operations, media metadata inspection. Use whenever the user asks to convert/chịn/cắt/ghép video or audio, extract sound from video, make a GIF, compress media for sending, resize/convert images in bulk, create thumbnails, fix rotation, or inspect media files. Triggers: convert video, nén video, cắt nhạc, tách âm thanh, đổi định dạng, compress, trim, mp4, mp3, gif, thumbnail, resize ảnh, ffmpeg. Do NOT use for downloading media from the web (that's a scraping/download task) or AI image generation (use create_image tool)."
license: Proprietary. Part of GoClaw bundled skills.
version: 1
inputs:
  - media_file
  - operation
outputs:
  - processed_media
allowed-tools:
  - filesystem
  - exec
quality-gates:
  - source_inspected
  - output_verified
deps:
  - system:ffmpeg
  - system:convert
---

# Media Processing (FFmpeg + ImageMagick)

Local media conversion and editing via command-line tools — inspect first,
transform second, verify third.

## Setup check

```bash
ffmpeg -version >/dev/null 2>&1 || echo MISSING_FFMPEG
convert -version >/dev/null 2>&1 || echo MISSING_IMAGEMAGICK
```

FFmpeg (~90 MB with codecs) and ImageMagick (~30 MB) — if missing, follow the
standard install policy: mention size and ask for approval above 100 MB
(FFmpeg qualifies; ImageMagick does not).

## Workflow (every task)

1. **Inspect first** — know the real format/codec/duration before transforming:
   ```bash
   ffprobe -v quiet -print_format json -show_format -show_streams input.mp4
   identify input.png                 # ImageMagick
   ```
2. **Transform** with a recipe below (always `-y` for non-interactive runs;
   write outputs to the workspace, never overwrite the source).
3. **Verify** — re-probe the output (duration/codec/size) and check the file
   size is sane before reporting. A silent 0-byte output is the classic
   failure.

## Recipes

**Video**
```bash
# Compress for sharing (visually decent, ~10x smaller than raw)
ffmpeg -y -i in.mp4 -c:v libx264 -crf 26 -preset medium -c:a aac -b:a 128k out.mp4
# Trim without re-encode (fast, keyframe-exact)
ffmpeg -y -ss 00:01:00 -to 00:02:30 -i in.mp4 -c copy out.mp4
# Extract audio
ffmpeg -y -i in.mp4 -vn -c:a libmp3lame -q:a 4 out.mp3
# Fix wrong rotation metadata
ffmpeg -y -i in.mp4 -c copy -metadata:s:v rotate=90 out.mp4
```

**GIF & thumbnails**
```bash
# Quality GIF (palette first — no palette = ugly banding)
ffmpeg -y -ss 5 -t 3 -i in.mp4 -vf "fps=12,scale=480:-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse" out.gif
# Thumbnail at 10s, 640px wide
ffmpeg -y -ss 10 -i in.mp4 -frames:v 1 -vf scale=640:-1 thumb.jpg
```

**Images (ImageMagick)**
```bash
# Batch resize to max 1600px, quality 82
mogrify -path out/ -resize '1600x1600>' -quality 82 *.jpg
# Convert HEIC/WEBP → JPG (batch)
mogrify -path out/ -format jpg *.heic
```

## Batch discipline

- Dry-run on ONE file, verify, then loop the batch (`for f in *.mov; do ...`).
- Report per-batch: file count, success/fail list, total size before → after.
- Long transcodes: state expected duration upfront (rule of thumb:
  libx264 medium ≈ 0.5–2x realtime on a mini-server) and run with nohup for
  files over ~15 minutes, polling progress instead of blocking.

## Output conventions

- Outputs to the agent workspace (`media/<task>-out/...`); never write into
  system paths or next to the source without the user asking.
- Default targets: video H.264/AAC MP4 (CRF 23–26), audio MP3 128–192k,
  images JPEG q82 or PNG when transparency matters. State the chosen target
  and why in one line.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `ffmpeg: command not found` | Ask approval (it's ~90MB), then `apt install ffmpeg` |
| Output plays locally but not on phone | Missing AAC audio or wrong pixel format → add `-pix_fmt yuv420p -c:a aac` |
| GIF is huge and ugly | Lower fps to 10–12, scale to ≤480px, and always use the two-pass palette filter |
| Audio/video drift after trim with `-c copy` | Cut on non-keyframes — re-encode instead (`-c:v libx264 -crf 24`) without `-ss` before `-i` |
| `convert` denies PDF operations | ImageMagick policy.xml restricts PDF — use `pdftoppm` (poppler) instead |
| HEIC unreadable | `apt install libheif-examples`, convert via `heif-convert`, or re-check ImageMagick delegates |
