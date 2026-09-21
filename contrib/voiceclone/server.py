# Self-hosted voice-clone worker
#
# Pipeline: edge-tts (reads the text in any language) -> ToneColorConverter
# (OpenVoice v2) recolors the audio with a registered speaker embedding.
# Runs fine on CPU; NVIDIA GPU only makes it faster.
#
# Setup (see README.md):
#   python -m venv .venv && source .venv/bin/activate  (Windows: .venv\Scripts\activate)
#   pip install -r requirements.txt
#   python server.py
#
# Environment:
#   VOICECLONE_HOST      listen address        (default 0.0.0.0 — LAN; set 127.0.0.1 to stay local)
#   VOICECLONE_PORT      listen port           (default 18795)
#   VOICECLONE_TOKEN     required bearer token (empty = no auth — only safe on localhost)
#   VOICECLONE_DATA_DIR  voice store           (default ./data)
#   VOICECLONE_DEVICE    "cpu" / "cuda"        (default: auto)

import io
import json
import os
import re
import subprocess
import time
import uuid
from pathlib import Path
from typing import Optional

import edge_tts
import uvicorn
from fastapi import Depends, FastAPI, File, Form, Header, HTTPException, Request, UploadFile
from fastapi.responses import JSONResponse, Response

HOST = os.environ.get("VOICECLONE_HOST", "0.0.0.0")
PORT = int(os.environ.get("VOICECLONE_PORT", "18795"))
TOKEN = os.environ.get("VOICECLONE_TOKEN", "")
DATA_DIR = Path(os.environ.get("VOICECLONE_DATA_DIR", "./data"))
DEVICE = os.environ.get("VOICECLONE_DEVICE", "")

MAX_TEXT_CHARS = 2000
MAX_UPLOAD_BYTES = 32 << 20
ALLOWED_EXTS = {".wav", ".mp3", ".m4a", ".aac", ".ogg", ".webm", ".flac"}
# OpenVoice v2 converter works at 22.05 kHz mono.
WORK_SRATE = 22050

app = FastAPI(title="goclaw voice-clone worker", version="1.0")

# ---- lazy model state -------------------------------------------------------

_model = {"converter": None, "device": "", "src_se": {}}


def _pick_device() -> str:
    if DEVICE:
        return DEVICE
    try:
        import torch

        return "cuda" if torch.cuda.is_available() else "cpu"
    except Exception:
        return "cpu"


def _checkpoints_dir() -> Path:
    """OpenVoice v2 converter checkpoints, downloaded from HuggingFace on
    first use (myshell-ai/OpenVoiceV2, converter/ subset only)."""
    root = DATA_DIR / "checkpoints_v2"
    cfg = root / "converter" / "config.json"
    ckpt = root / "converter" / "checkpoint.pth"
    if not (cfg.exists() and ckpt.exists()):
        from huggingface_hub import snapshot_download

        snapshot_download(
            "myshell-ai/OpenVoiceV2",
            local_dir=str(root),
            allow_patterns=["converter/*"],
        )
    return root


def get_converter():
    """Load the ToneColorConverter on first use so /health stays instant and
    the worker can serve plain edge-tts even without OpenVoice installed."""
    if _model["converter"] is None:
        from openvoice.api import ToneColorConverter

        device = _pick_device()
        root = _checkpoints_dir()
        conv = ToneColorConverter(str(root / "converter" / "config.json"), device=device)
        conv.load_ckpt(str(root / "converter" / "checkpoint.pth"))
        _model["converter"] = conv
        _model["device"] = device
    return _model["converter"]


# ---- voice registry ---------------------------------------------------------

def _registry_path() -> Path:
    return DATA_DIR / "voices.json"


def _load_registry() -> dict:
    p = _registry_path()
    if not p.exists():
        return {}
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except Exception:
        return {}


def _save_registry(reg: dict) -> None:
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    _registry_path().write_text(json.dumps(reg, ensure_ascii=False, indent=2), encoding="utf-8")


def _slugify(name: str) -> str:
    slug = re.sub(r"[^a-zA-Z0-9_-]+", "-", name).strip("-").lower()
    return slug[:48] if slug else ""


# ---- auth -------------------------------------------------------------------

async def require_token(authorization: str = Header(default="")) -> None:
    if not TOKEN:
        return
    if authorization != f"Bearer {TOKEN}":
        raise HTTPException(status_code=401, detail="invalid or missing bearer token")


# ---- ffmpeg helpers ---------------------------------------------------------

def _run_ffmpeg(src: Path, dst: Path) -> None:
    cmd = [
        "ffmpeg", "-nostdin", "-y", "-hide_banner", "-loglevel", "error",
        "-i", str(src), "-ac", "1", "-ar", str(WORK_SRATE), str(dst),
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
    if proc.returncode != 0 or not dst.exists():
        raise HTTPException(status_code=400, detail=f"ffmpeg failed on upload: {proc.stderr[-300:]}")


def _probe_duration(path: Path) -> float:
    try:
        out = subprocess.run(
            ["ffprobe", "-v", "error", "-show_entries", "format=duration",
             "-of", "default=noprint_wrappers=1:nokey=1", str(path)],
            capture_output=True, text=True, timeout=60,
        )
        return float(out.stdout.strip())
    except Exception:
        return 0.0


# ---- endpoints --------------------------------------------------------------

@app.get("/health")
async def health(_: None = Depends(require_token)):
    reg = _load_registry()
    return {
        "ok": True,
        "backend": "openvoice-v2" if _model["converter"] is not None else "edge-only (openvoice loads on first clone use)",
        "device": _model["device"],
        "voices": len(reg),
    }


@app.get("/v1/voices")
async def list_voices(_: None = Depends(require_token)):
    reg = _load_registry()
    voices = [
        {"id": v["id"], "name": v["name"], "created_at": v.get("created_at", "")}
        for v in sorted(reg.values(), key=lambda v: v.get("created_at", ""))
    ]
    return {"voices": voices}


@app.post("/v1/voices")
async def register_voice(
    name: str = Form(...),
    file: UploadFile = File(...),
    _: None = Depends(require_token),
):
    name = name.strip()
    if not name or len(name) > 100:
        raise HTTPException(status_code=400, detail="name is required (max 100 chars)")
    ext = Path(file.filename or "").suffix.lower()
    if ext not in ALLOWED_EXTS:
        raise HTTPException(status_code=415, detail=f"unsupported audio format {ext!r}")

    raw = await file.read()
    if len(raw) > MAX_UPLOAD_BYTES:
        raise HTTPException(status_code=413, detail="reference audio exceeds 32MB")
    if not raw:
        raise HTTPException(status_code=400, detail="empty audio file")

    # Materialize the upload, transcode to the working format, sanity-check
    # the duration before spending time on the embedding.
    tmp_in = DATA_DIR / f"upload-{uuid.uuid4().hex}{ext}"
    tmp_wav = tmp_in.with_suffix(".wav")
    try:
        DATA_DIR.mkdir(parents=True, exist_ok=True)
        tmp_in.write_bytes(raw)
        _run_ffmpeg(tmp_in, tmp_wav)
        dur = _probe_duration(tmp_wav)
        if dur < 5:
            raise HTTPException(status_code=400, detail="reference audio too short — need at least 5 seconds (30-60s recommended)")
        if dur > 600:
            raise HTTPException(status_code=400, detail="reference audio too long — trim to a few minutes at most")

        # Extract the speaker embedding with OpenVoice.
        try:
            from openvoice import se_extractor

            converter = get_converter()
            se_dir = DATA_DIR / "se"
            se_dir.mkdir(parents=True, exist_ok=True)
            target_se, _audio_name = se_extractor.get_se(
                str(tmp_wav), converter, target_dir=str(se_dir), vad=True
            )
        except HTTPException:
            raise
        except Exception as exc:  # openvoice missing / checkpoint download failed / CPU OOM
            raise HTTPException(status_code=503, detail=f"voice-clone backend unavailable: {exc}")

        se_path = DATA_DIR / "se" / f"{uuid.uuid4().hex}.pth"
        # se_extractor returns a torch tensor — persist it next to the registry.
        try:
            import torch

            torch.save(target_se, se_path)
        except Exception as exc:
            raise HTTPException(status_code=503, detail=f"failed to persist embedding: {exc}")

        reg = _load_registry()
        voice_id = _slugify(name)
        if not voice_id or voice_id in reg:
            voice_id = f"{voice_id or 'voice'}-{uuid.uuid4().hex[:6]}"
        reg[voice_id] = {
            "id": voice_id,
            "name": name,
            "created_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
            "se_path": str(se_path),
            "duration_sec": round(dur, 1),
        }
        _save_registry(reg)
        return {"voice": {"id": voice_id, "name": name, "created_at": reg[voice_id]["created_at"]}}
    finally:
        tmp_in.unlink(missing_ok=True)
        tmp_wav.unlink(missing_ok=True)


@app.delete("/v1/voices/{voice_id}")
async def delete_voice(voice_id: str, _: None = Depends(require_token)):
    reg = _load_registry()
    entry = reg.pop(voice_id, None)
    if entry is None:
        raise HTTPException(status_code=404, detail="voice not found")
    _save_registry(reg)
    if entry.get("se_path"):
        Path(entry["se_path"]).unlink(missing_ok=True)
    return {"ok": True}


@app.post("/v1/tts")
async def tts(request: Request, _: None = Depends(require_token)):
    try:
        body = json.loads((await request.body()) or b"{}")
    except Exception:
        raise HTTPException(status_code=400, detail="invalid json body")
    text = str(body.get("text", "")).strip()
    voice_id = str(body.get("voice_id", "") or "")
    base_voice = str(body.get("base_voice", "") or "vi-VN-HoaiMyNeural")
    speed = float(body.get("speed", 1.0) or 1.0)
    if not text:
        raise HTTPException(status_code=400, detail="text is required")
    if len(text) > MAX_TEXT_CHARS:
        raise HTTPException(status_code=422, detail=f"text exceeds {MAX_TEXT_CHARS} chars")
    if not 0.5 <= speed <= 2.0:
        raise HTTPException(status_code=422, detail="speed must be within [0.5, 2.0]")

    work = Path(__import__("tempfile").mkdtemp(prefix="vc-"))
    try:
        mp3 = work / "base.mp3"
        wav = work / "base.wav"
        communicate = edge_tts.Communicate(text, base_voice, rate="+0%")
        await communicate.save(str(mp3))
        if not mp3.exists() or mp3.stat().st_size == 0:
            raise HTTPException(status_code=502, detail="edge-tts produced no audio")
        _run_ffmpeg(mp3, wav)

        reg = _load_registry()
        if not voice_id or voice_id not in reg:
            if voice_id and voice_id != "default":
                raise HTTPException(status_code=400, detail=f"voice not found: {voice_id}")
            # No clone voice requested — plain edge audio, no conversion.
            return Response(content=mp3.read_bytes(), media_type="audio/mpeg")

        entry = reg[voice_id]
        try:
            import torch

            converter = get_converter()
            tgt_se = torch.load(entry["se_path"], map_location=converter.device)
            key = f"{base_voice}|{round(speed, 2)}"
            src_se = _model["src_se"].get(key)
            if src_se is None:
                from openvoice import se_extractor

                src_se, _ = se_extractor.get_se(str(wav), converter, vad=False)
                _model["src_se"][key] = src_se
            out = work / "out.wav"
            # tau/messaging controls how strongly the timbre is shifted —
            # defaults follow the OpenVoice v2 demo.
            converter.convert(
                audio_src_path=str(wav), src_se=src_se, tgt_se=tgt_se,
                output_path=str(out), tau=0.3, message="",
            )
            if not out.exists():
                raise HTTPException(status_code=502, detail="tone conversion produced no audio")
            return Response(content=out.read_bytes(), media_type="audio/wav")
        except HTTPException:
            raise
        except Exception as exc:
            raise HTTPException(status_code=503, detail=f"voice-clone backend unavailable: {exc}")
    finally:
        for p in work.iterdir():
            p.unlink(missing_ok=True)
        work.rmdir()


if __name__ == "__main__":
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    uvicorn.run(app, host=HOST, port=PORT, log_level="info")
