# Creative Tools (Studio)

GoClaw ships creative apps that run where they fit best — interactive editing
in the browser, heavy rendering on the server. They live in the web
dashboard's **Tools** section, and are also being extracted into a standalone
**GoTools** app you can deploy without GoClaw.

## PPTX Studio (`/tools/pptx`)

Build slide decks by chatting with **`pptx-designer`** — a design-only agent
restricted to a minimal tool allowlist. Slides render live in the browser as
you converse, and export as **real `.pptx` files** (not screenshots).

The designer is backed by two skills bundled into the binary:
`pptx-deck-design` (deck structure and narrative) and `pptx-visual-style`
(visual coherence). It intentionally does not run the full agent loop — a
single streamed completion turns your intent into slide blocks the editor
understands.

## Video Editor (`/tools/video`)

Canvas + timeline editing with:

- TTS voiceover per scene (Edge TTS voices, e.g. `vi-VN-HoaiMyNeural`)
- Scene enter transitions and per-scene transform/filter
- Render-job management with progress

Interactive edits run **client-side**; final renders go through the
server-side pipeline (below).

### Server-side video pipeline

Final renders are executed by `videoworker` — a standalone ffmpeg worker
binary, deployed independently from the gateway:

```
agent tool render_video ──► videoworker HTTP API (127.0.0.1:18791)
                                  │  storyboard JSON → ffmpeg
                                  ▼
                          rendered .mp4 + WS progress events
```

Agents trigger renders directly with the `render_video` tool; job lifecycle
(queued → running → done/failed, cancel, preview, delete) is exposed over
HTTP and WebSocket events. Full deployment guide (systemd unit, resource
limits, fonts, narration): [Self-Hosting → Video worker sidecar](/en/self-hosting#video-worker-sidecar).

## Watermark Remover (`/tools/watermark`)

Fully **client-side** image and video watermark removal using a calibrated
inverse-alpha unblend. Video clips are restored frame-by-frame in realtime
while recording — **nothing leaves the browser**; there are no server
round-trips at all.

## GoTools — the standalone studio

The studio apps are being extracted into **[qkhalk/gotools](https://github.com/qkhalk/gotools)**
(GoTools): a self-contained Go server + SPA with its own SQLite database,
deployable at its own domain **without a GoClaw gateway**.

- The same three tools — watermark remover, PPTX studio, video studio (with
  its own videoworker sidecar)
- An **admin panel for models/providers**: register API keys, add models,
  verify with a test call
- One binary per domain — no dependency on GoClaw's PostgreSQL, channels or
  agent runtime

GoClaw's built-in Tools section continues to exist for gateway users;
GoTools is for teams who want just the creative suite.
