# GoClaw Video Worker

Standalone ffmpeg-based video renderer for the GoClaw video creation pipeline (Phase 2).

## Requirements

- Go 1.26+ (for building)
- ffmpeg 7.x+ with libx264 and aac support
- ffprobe (usually bundled with ffmpeg)
- (Optional) `pip install edge-tts` for narration synthesis

## Building

```bash
go build -o videoworker ./cmd/videoworker/
```

## Quick Start

```bash
# Create working directories
mkdir -p /tmp/vw /tmp/vw-output

# Run (dev mode, no auth)
./videoworker --work-dir /tmp/vw --output-dir /tmp/vw-output

# Run with auth
./videoworker \
  --addr 127.0.0.1:18791 \
  --token my-secret-token \
  --work-dir /tmp/vw \
  --output-dir /tmp/vw-output \
  --font-file /usr/share/fonts/truetype/noto/NotoSans-Regular.ttf
```

## API

### POST /v1/jobs — Submit a render job

```bash
curl -X POST http://127.0.0.1:18791/v1/jobs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jobId": "test-001",
    "storyboard": {
      "version": 1,
      "canvas": { "width": 1080, "height": 1920, "fps": 30 },
      "scenes": [
        {
          "type": "color",
          "color": "#FF0000",
          "duration_sec": 3,
          "caption": { "text": "Hello World", "position": "center" }
        }
      ],
      "output": { "format": "mp4", "height": 720 }
    }
  }'
```

Response: `202 Accepted`

```json
{"jobId":"test-001","status":"queued"}
```

### GET /v1/jobs/{id} — Check job status

```bash
curl http://127.0.0.1:18791/v1/jobs/test-001 \
  -H "Authorization: Bearer $TOKEN"
```

Response: `200 OK`

```json
{
  "jobId": "test-001",
  "status": "done",
  "progress": 100,
  "outputPath": "/tmp/vw-output/test-001.mp4",
  "outputSizeBytes": 1234567,
  "durationMs": 5432
}
```

### POST /v1/jobs/{id}/cancel — Cancel a job

```bash
curl -X POST http://127.0.0.1:18791/v1/jobs/test-001/cancel \
  -H "Authorization: Bearer $TOKEN"
```

### GET /health — Health check

```bash
curl http://127.0.0.1:18791/health
```

## Deployment (systemd)

### 1. Install ffmpeg and fonts

```bash
apt update && apt install -y ffmpeg fonts-noto-core
```

### 2. Install the binary

```bash
go build -o /usr/local/bin/videoworker ./cmd/videoworker/
```

### 3. Create service user and directories

```bash
useradd -r -s /bin/false goclaw
mkdir -p /var/lib/goclaw/video-tmp /var/www/goclaw/videos
chown goclaw:goclaw /var/lib/goclaw/video-tmp /var/www/goclaw/videos
```

### 4. Install systemd unit

```bash
cp deploy/videoworker.service /etc/systemd/system/
# Edit the service file to set your token:
#   Environment=GOCLAW_VIDEO_TOKEN=your-secret-token
systemctl daemon-reload
systemctl enable --now goclaw-videoworker
```

### 5. Verify

```bash
systemctl status goclaw-videoworker
journalctl -u goclaw-videoworker -f
curl http://127.0.0.1:18791/health
```

## Configuration Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `127.0.0.1:18791` | HTTP listen address |
| `--token` | (empty) | Bearer auth token (empty = no auth) |
| `--work-dir` | `/tmp/videoworker` | Temp files directory |
| `--output-dir` | `/var/www/videos` | Rendered video output |
| `--ffmpeg-path` | `ffmpeg` | Path to ffmpeg binary |
| `--font-file` | (empty) | Font file for captions (skip captions if empty) |
| `--max-scene-sec` | `30` | Max seconds per scene |
| `--max-queue` | `5` | Max queued jobs (returns 409 when full) |
| `--narr-voice` | `vi-VN-HoaiMyNeural` | Default Edge TTS voice |
| `--ttl-minutes` | `120` | Orphan temp dir cleanup TTL |

## Resource Limits

The systemd unit enforces:
- **MemoryMax=600MB** — peak usage ~200-350MB during render
- **CPUQuota=100%** — 1 CPU core (by design, `-threads 1`)
- **Restart=on-failure** — auto-restart with 5s backoff
- **NoNewPrivileges=true** — security hardening

## Narration (Edge TTS)

The worker uses the `edge-tts` Python CLI for text-to-speech narration:

```bash
pip install edge-tts
```

Available voices: `edge-tts --list-voices`

Common Vietnamese voices:
- `vi-VN-HoaiMyNeural` (female, default)
- `vi-VN-NamMinhNeural` (male)

## Crash Recovery

- Worker state is in-memory only — a crash loses running jobs
- Gateway detects lost jobs via poll timeout and marks them `failed`
- Orphaned temp directories are cleaned at startup and every 15 minutes
- Manual cleanup: `rm -rf /var/lib/goclaw/video-tmp/vwjob-*`

## Reverse Proxy (Nginx)

If exposing beyond localhost:

```nginx
location /v1/jobs {
    proxy_pass http://127.0.0.1:18791;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 600s;  # long render jobs
}
```

## Testing

```bash
# Unit tests (no ffmpeg needed)
go test -v ./internal/vworker/...

# Integration test (requires ffmpeg)
go test -v -tags integration ./internal/vworker/...
```
