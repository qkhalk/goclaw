# media-probe

Zero-dependency MCP stdio server exposing `probe` — one tool that runs
ffprobe on an image/video/audio file and returns duration, codec,
resolution, bitrate and stream layout as JSON.

- **Runtime:** node (no npm dependencies — nothing to install, no supply chain)
- **Entry:** `src/index.js`
- **RAM:** ~60MB when running; the gateway lazy-starts it on first tool call
- **Requires:** `ffprobe` on PATH (standard companion of ffmpeg; the gateway
  host already has it for video rendering)

Tool surface:

```
probe(path: string) -> ffprobe JSON (format + streams)
```

## Local test

```
(printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n'; sleep 1) | node src/index.js
```

## Install via Tool Store

This folder ships with the gateway's embedded catalog (`mcp/catalog.go`);
installing from the Store clones this folder at the running release tag,
smoke-tests the MCP handshake, and registers it as a stdio server.
