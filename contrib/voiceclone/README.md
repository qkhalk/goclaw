# Voice-Clone Worker (OpenVoice v2 + edge-tts)

Standalone sidecar that gives GoClaw **voice cloning** without loading the
gateway: text goes in, audio clipped to a registered voice comes out.

```
text ──▶ edge-tts (reads Vietnamese/English/...) ──▶ OpenVoice v2 ToneColorConverter ──▶ WAV in the cloned voice
                                                              ▲
                                    speaker embedding from your 30–60s sample
```

- **MIT-licensed stack** (OpenVoice v2 code + model are MIT) — safe for
  monetized channels.
- Runs on **CPU** (any modern desktop; ~a few seconds per sentence). NVIDIA GPU
  is only an accelerator.
- The GoClaw gateway never runs inference — it just proxies to this worker
  (`tts.clone.endpoint` in the gateway config).

## Setup (Windows)

```powershell
cd contrib\voiceclone
python -m venv .venv
.venv\Scripts\activate

# CPU-only torch first (avoids the multi-GB CUDA download)
pip install torch --index-url https://download.pytorch.org/whl/cpu

pip install -r requirements.txt
# system dependency: ffmpeg must be on PATH (winget install ffmpeg)

set VOICECLONE_TOKEN=pick-a-long-secret
python server.py
```

Linux/macOS: same with `python3 -m venv`, `source .venv/bin/activate`.

On first clone use the worker downloads the OpenVoice v2 converter
checkpoints from HuggingFace (`myshell-ai/OpenVoiceV2`, `converter/` only,
~200MB) into `data/checkpoints_v2/`.

## Register your voice

Record **30–60 seconds** of yourself reading anything, in a quiet room, mic
close, natural pace (WAV/MP3/M4A…). Then:

```powershell
curl.exe -X POST "http://localhost:18795/v1/voices" ^
  -H "Authorization: Bearer %VOICECLONE_TOKEN%" ^
  -F "name=Giong anh" -F "file=@sample.wav"
# -> {"voice":{"id":"giong-anh","name":"Giong anh",...}}
```

- `GET  /v1/voices` — list registered voices
- `DELETE /v1/voices/{id}` — remove one
- `POST /v1/tts` — `{"text":"...","voice_id":"giong-anh"}` → `audio/wav`
  (`voice_id` omitted → plain edge-tts audio; `base_voice` picks the reading
  voice, `speed` 0.5–2.0)

## Point GoClaw at the worker

In the gateway config (JSON5):

```json5
{
  tts: {
    clone: {
      endpoint: "http://<worker-host>:18795", // enables the provider
      api_key: "same-as-VOICECLONE_TOKEN",    // encrypted at rest
      voice: "giong-anh",                     // optional default
    },
  },
}
```

Then in the video studio pick the voice under **“Giọng của tôi”** for any
scene — storyboard voices look like `clone:giong-anh`, and both the render
worker and the preview player route them to this pipeline.

> Gateway on a private network talking to a LAN worker? Set
> `GOCLAW_ALLOW_PRIVATE_PROVIDER_URLS=1` on the **gateway** so its SSRF guard
> accepts the private endpoint (logged as `security.provider_url.private_allowed`).

## HTTP API

| Method | Path | Description |
|---|---|---|
| GET | `/health` | `{ok, backend, device, voices}` |
| GET | `/v1/voices` | registered voices |
| POST | `/v1/voices` | multipart `name` + `file` (5s–10min, ≤32MB) |
| DELETE | `/v1/voices/{id}` | remove a voice + its embedding |
| POST | `/v1/tts` | JSON `{text, voice_id?, base_voice?, speed?}` → `audio/wav` / `audio/mpeg` |

## Browser-side cloning (no worker at all)

The studio can also clone **entirely in the browser** — the speaker embedding
and tone conversion run client-side via onnxruntime-web (WASM), so preview
narration in your cloned voice works even with no clone worker deployed and
never sends reference audio anywhere.

```
edge-tts (gateway, text→base audio) ──▶ ToneColorConverter ONNX (in WASM) ──▶ WAV in your voice
                                                  ▲
                    speaker embedding from your sample (browser, on registration)
```

Pipeline pieces (`ui/web/src/lib/voice-clone/`):

| File | Role |
|---|---|
| `spectrogram.ts` | torch-exact linear spectrogram (reflect pad + periodic Hann + `sqrt(re²+im²+1e-6)`) |
| `runtime.ts` | lazy onnxruntime-web loader; model bytes cached in Cache Storage; `VOICE_CLONE_MODEL_BASE_URL` points at the published ONNX assets |
| `index.ts` | public API: `registerVoiceFromBlob` / `convertUtterance` / voice store access |
| `store.ts` | IndexedDB voice registry (embeddings never leave the device) |

The ONNX graphs are produced once by `export_onnx.py` (this directory):

```bash
docker run --rm -v "$PWD:/work" -w /work python:3.11-slim bash -c "
  apt-get update -qq && apt-get install -y -qq git > /dev/null
  pip install --index-url https://download.pytorch.org/whl/cpu torch
  pip install onnx onnxruntime librosa soundfile huggingface_hub
  python contrib/voiceclone/export_onnx.py --out contrib/voiceclone/onnx
"
```

The script clones OpenVoice, downloads the `myshell-ai/OpenVoiceV2` converter
checkpoints from HuggingFace, exports two graphs — `speaker_encoder.onnx`
(spec `(1,513,T)` → embedding `(1,256)`) and `converter.onnx` (deterministic
`voice_conversion`: spec + source/target embeddings + zero noise + tau →
waveform `(1,1,T·256)`) — validates torch-vs-onnxruntime (max abs diff must
be `< 1e-3`), and writes `manifest.json` (I/O shapes, 22050 Hz, n_fft 1024,
hop 256) plus a spectrogram golden for the browser DSP tests. Host the
resulting `onnx/` directory (e.g. as a GitHub release asset) and point
`VOICE_CLONE_MODEL_BASE_URL` at it; until then the UI falls back to plain
edge audio.

The gateway-worker path (this sidecar) remains the backend for **server-side
renders** — the browser path covers interactive preview.

## Security notes

- Set `VOICECLONE_TOKEN` whenever the worker listens beyond `127.0.0.1` — the
  gateway forwards it as a bearer token.
- Only run this on a trusted LAN. It executes no shell input; uploads are
  transcoded through ffmpeg in a temp dir and embeddings stay in
  `VOICECLONE_DATA_DIR`.
- Clone only voices you have the right to use. Generated audio is your
  responsibility.

## Upgrading to deeper cloning (optional)

The gateway contract (`POST /v1/tts {text, voice_id}`) is worker-agnostic. To
switch to a fine-tuned zero-shot model later — e.g. F5-TTS with the
Vietnamese `hynt/F5-TTS-Vietnamese-ViVoice` checkpoint — reimplement the same
five endpoints against that model and nothing else in GoClaw changes. Note the
hynt checkpoint is CC-BY-NC-SA (non-commercial); OpenVoice (MIT) is the
safe-for-monetization default.
