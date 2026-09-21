#!/usr/bin/env python3
"""
One-time export of OpenVoice v2's ToneColorConverter to ONNX for in-browser
inference (onnxruntime-web / WASM). Run inside a container:

    docker run --rm -v "$PWD:/work" -w /work python:3.11-slim bash -c "
      apt-get update -qq && apt-get install -y -qq git > /dev/null
      pip install --index-url https://download.pytorch.org/whl/cpu torch \\
        && pip install onnx onnxruntime librosa soundfile huggingface_hub
      python contrib/voiceclone/export_onnx.py --out contrib/voiceclone/onnx
    "

The script:
  1. clones myshell-ai/OpenVoice at a PINNED commit (REPO_REF below) into a
     temp dir,
  2. downloads myshell-ai/OpenVoiceV2 `converter/config.json` +
     `converter/checkpoint.pth` from HuggingFace at a PINNED revision, and
     records the checkpoint SHA-256 in the manifest,
  3. wraps and exports TWO graphs (see I/O CONTRACT below),
  4. validates torch reference vs onnxruntime on realistic inputs and prints
     the max abs diff (target < 1e-3),
  5. writes converter.onnx, speaker_encoder.onnx, manifest.json to --out.

I/O CONTRACT (differs intentionally from the upstream Python API):
  * The torch STFT is NOT part of the graphs. The browser computes the linear
    spectrogram itself (see ui/web/src/lib/voice-clone/spectrogram.ts):
    reflect-pad (n_fft-hop)/2, periodic Hann window, center=False,
    magnitude sqrt(re^2+im^2+eps). This is exactly OpenVoice
    `spectrogram_torch`. The voice-conversion path consumes the LINEAR
    spectrogram (n_fft/2+1 = 513 bins) — there is NO mel projection / log.

  speaker_encoder.onnx
    inputs:  spec  float32 (1, 513, T)   linear spectrogram, channel-major
    outputs: emb   float32 (1, 256)      L2-unnormalized speaker embedding
    (= ToneColorConverter.extract_se's `model.ref_enc(spec.transpose(1,2))`)

  converter.onnx
    inputs:  spec         float32 (1, 513, T)  linear spectrogram (source audio)
             spec_lengths int64   (1,)                frames per batch (= T)
             g_src        float32 (1, 256, 1)         source speaker embedding
             g_tgt        float32 (1, 256, 1)         target speaker embedding
             noise        float32 (1, 192, T)         z noise; pass zeros for
                                                      deterministic conversion
             tau          float32 ()                  blend factor (0.3 default)
    outputs: audio        float32 (1, 1, T*256)  waveform at 22050 Hz
    (= SynthesizerTrn.voice_conversion with PosteriorEncoder's randn_like
     replaced by the explicit `noise` input so browser inference is
     deterministic and reproducible. OpenVoice samples randn at runtime; a
     zero noise gives z = m, i.e. the MAP path — quality-equivalent here
     because zero_g=True means the posterior encoder/decoder are not even
     speaker-conditioned.)

  Deviations from upstream (all deliberate, no behaviour loss):
    - torch.stft stays in the browser (documented above).
    - noise/tau are explicit inputs instead of torch.randn_like / hardcoded.
    - The wavmark audio watermark is NOT applied (it is a separate post-pass
      in Python, not part of the model; enable_watermark=False equivalent).
    - GRU/weight-norm layers export as standard ONNX ops (opset 17).
"""

import argparse
import datetime
import json
import os
import shutil
import subprocess
import sys
import tempfile
import types

import numpy as np
import torch
import torch.nn as nn

REPO_URL = "https://github.com/myshell-ai/OpenVoice"
# Pinned upstream refs — re-running the export months later must produce the
# same weights. Both are first-party MIT sources (myshell-ai); bump deliberately.
REPO_REF = "74a1d147b17a8c3092dd5430504bd83ef6c7eb23"  # OpenVoice main, 2025-04-19
HF_REPO = "myshell-ai/OpenVoiceV2"
HF_REVISION = "f36e7edfe1684461a8343844af60babc2efbb727"
HF_FILES = ["converter/config.json", "converter/checkpoint.pth"]
OPSET = 17


def log(msg: str) -> None:
    print(f"[export] {msg}", flush=True)


def sha256_of(path: str) -> str:
    import hashlib

    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def fetch_sources(workdir: str) -> tuple[str, str, str, str, str]:
    """Clone OpenVoice + download HF checkpoints at pinned refs.

    Returns (repo, ckpt, cfg, commit, ckpt_sha256)."""
    repo = os.path.join(workdir, "OpenVoice")
    if not os.path.isdir(repo):
        subprocess.run(
            ["git", "clone", "--depth", "1", REPO_URL, repo],
            check=True,
            capture_output=True,
        )
        subprocess.run(
            ["git", "-C", repo, "checkout", "--quiet", REPO_REF],
            check=True,
            capture_output=True,
        )
    commit = (
        subprocess.run(
            ["git", "-C", repo, "rev-parse", "HEAD"],
            check=True,
            capture_output=True,
            text=True,
        )
        .stdout.strip()[:12]
    )
    if commit != REPO_REF[:12]:
        raise SystemExit(f"cloned commit {commit} != pinned {REPO_REF[:12]} — refusing to export")
    ckpt_dir = os.path.join(workdir, "ckpt", "converter")
    os.makedirs(ckpt_dir, exist_ok=True)
    ckpt = os.path.join(ckpt_dir, "checkpoint.pth")
    cfg = os.path.join(ckpt_dir, "config.json")
    if not os.path.exists(ckpt):
        from huggingface_hub import hf_hub_download

        for fname, dest in zip(HF_FILES, [cfg, ckpt]):
            p = hf_hub_download(repo_id=HF_REPO, filename=fname, revision=HF_REVISION)
            shutil.copy(p, dest)
            log(f"downloaded {fname} (rev {HF_REVISION[:12]}) -> {dest}")
    return repo, ckpt, cfg, commit, sha256_of(ckpt)


def strip_weight_norm(module: nn.Module) -> None:
    """Remove torch weight_norm parametrizations in-place (export-friendly)."""
    from torch.nn.utils import parametrize

    for _name, child in module.named_modules():
        if parametrize.is_parametrized(child):
            for pname in list(child.parametrizations.keys()):
                parametrize.remove_parametrizations(child, pname, leave_parametrized=True)


def load_converter(repo: str, cfg_path: str, ckpt_path: str):
    sys.path.insert(0, repo)
    # Import the upstream package lazily so it can patch itself first.
    from openvoice import commons  # noqa: F401  (registers deps)
    from openvoice.models import SynthesizerTrn
    from openvoice import utils

    hps = utils.get_hparams_from_file(cfg_path)
    model = SynthesizerTrn(
        len(getattr(hps, "symbols", [])),
        hps.data.filter_length // 2 + 1,
        n_speakers=hps.data.n_speakers,
        **hps.model,
    )
    model.eval()
    # weights_only=True: checkpoint["model"] is a plain tensor state dict —
    # no pickle code execution, so a compromised upstream cannot run code at
    # export time.
    checkpoint = torch.load(ckpt_path, map_location="cpu", weights_only=True)
    missing, unexpected = model.load_state_dict(checkpoint["model"], strict=False)
    log(f"load_state_dict strict=False missing={len(missing)} unexpected={len(unexpected)}")
    if unexpected:
        log(f"  unexpected keys (ok): {unexpected[:6]}{'...' if len(unexpected) > 6 else ''}")
    if missing:
        log(f"  missing keys: {missing[:6]}{'...' if len(missing) > 6 else ''}")
    strip_weight_norm(model)
    return model, hps


class SpeakerEncoderWrapper(nn.Module):
    """spec (1, freq, T) -> embedding (1, 256). Mirrors extract_se."""

    def __init__(self, ref_enc: nn.Module):
        super().__init__()
        self.ref_enc = ref_enc

    def forward(self, spec: torch.Tensor) -> torch.Tensor:
        return self.ref_enc(spec.transpose(1, 2))


class ConverterWrapper(nn.Module):
    """Deterministic re-implementation of SynthesizerTrn.voice_conversion.

    Matches upstream step-for-step, INCLUDING the OpenVoiceV2 converter's
    zero_g=True semantics (models.py:voice_conversion): enc_q and dec see
    torch.zeros_like(g) while the flow keeps the REAL g_src/g_tgt — the timbre
    transfer happens in the flow, and the posterior encoder/decoder are not
    speaker-conditioned. PosteriorEncoder's torch.randn_like(m) * tau *
    exp(logs) uses the explicit `noise` input so browser inference is
    deterministic (zero noise = MAP path z = m).
    """

    def __init__(self, model: nn.Module):
        super().__init__()
        self.model = model

    def forward(
        self,
        spec: torch.Tensor,
        spec_lengths: torch.Tensor,
        g_src: torch.Tensor,
        g_tgt: torch.Tensor,
        noise: torch.Tensor,
        tau: torch.Tensor,
    ) -> torch.Tensor:
        m = self.model
        zero_g = bool(getattr(m, "zero_g", False))
        g_enc = torch.zeros_like(g_src) if zero_g else g_src
        g_dec = torch.zeros_like(g_tgt) if zero_g else g_tgt
        # --- PosteriorEncoder.forward (noise made explicit) ---
        enc_q = m.enc_q
        x_mask = torch.unsqueeze(
            _sequence_mask(spec_lengths, spec.size(2)), 1
        ).to(spec.dtype)
        x = enc_q.pre(spec) * x_mask
        x = enc_q.enc(x, x_mask, g=g_enc)
        stats = enc_q.proj(x) * x_mask
        mm, logs = torch.split(stats, enc_q.out_channels, dim=1)
        z = (mm + noise * tau * torch.exp(logs)) * x_mask
        # --- flow invert + decode (upstream voice_conversion body) ---
        z_p = m.flow(z, x_mask, g=g_src)
        z_hat = m.flow(z_p, x_mask, g=g_tgt, reverse=True)
        o_hat = m.dec(z_hat * x_mask, g=g_dec)
        return o_hat


def _sequence_mask(lengths: torch.Tensor, max_len: int | None = None) -> torch.Tensor:
    """Copy of openvoice.commons.sequence_mask (avoids importing the module)."""
    if max_len is None:
        max_len = int(lengths.max().item())
    ids = torch.arange(0, max_len, device=lengths.device, dtype=torch.long)
    mask = torch.lt(ids.unsqueeze(0), lengths.unsqueeze(1)).float()
    return mask


def voiced_signal(seconds: float, sr: int) -> np.ndarray:
    """A deterministic voiced-ish signal, so validation runs on inputs shaped
    like real audio rather than raw gaussian noise."""
    t = np.linspace(0.0, seconds, int(sr * seconds), endpoint=False)
    f0 = 180.0
    y = 0.4 * np.sin(2 * np.pi * f0 * t)
    y += 0.25 * np.sin(2 * np.pi * 2 * f0 * t)
    y += 0.125 * np.sin(2 * np.pi * 3 * f0 * t)
    y *= 0.6 + 0.4 * np.sin(2 * np.pi * 3.0 * t)  # slow AM, like speech
    return y.astype(np.float32)


def spec_from_signal(y: np.ndarray, n_fft: int, hop: int) -> torch.Tensor:
    """Signal -> spectrogram_torch replica (center=False, reflect pad, hann,
    +1e-6) so the exported graph is validated on genuine pipeline inputs.
    return_complex=False matters: upstream sums re²+im² over the trailing
    real/imag axis; with complex output that same expression would sum over
    time instead and silently produce a garbage spectrogram."""
    y_t = torch.from_numpy(y).unsqueeze(0)
    pad = (n_fft - hop) // 2
    y_p = torch.nn.functional.pad(y_t, (pad, pad), mode="reflect")
    window = torch.hann_window(n_fft)
    spec = torch.stft(
        y_p, n_fft, hop_length=hop, win_length=n_fft, window=window,
        center=False, pad_mode="reflect", normalized=False, onesided=True,
        return_complex=False,
    )
    return torch.sqrt(spec.pow(2).sum(-1) + 1e-6)


def onnx_export(*args, **kwargs) -> None:
    """torch.onnx.export pinned to the legacy TorchScript exporter — the
    battle-tested path for VITS-family graphs (GRU + weight-norm + dynamic
    axes). torch>=2.9 defaults to the dynamo exporter, which additionally
    requires the `onnxscript` package; older torches have no `dynamo` kwarg."""
    try:
        torch.onnx.export(*args, **kwargs, dynamo=False)
    except TypeError:
        torch.onnx.export(*args, **kwargs)


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--out", default="onnx", help="output dir for onnx+manifest")
    ap.add_argument("--seconds", type=float, default=2.0, help="validation clip length")
    ap.add_argument("--frames", type=int, default=0, help="override spec frames")
    args = ap.parse_args()

    out_dir = os.path.abspath(args.out)
    os.makedirs(out_dir, exist_ok=True)

    with tempfile.TemporaryDirectory(prefix="ovexport_") as workdir:
        repo, ckpt, cfg, commit, ckpt_sha = fetch_sources(workdir)
        model, hps = load_converter(repo, cfg, ckpt)

        sr = hps.data.sampling_rate
        n_fft = hps.data.filter_length
        hop = hps.data.hop_length
        win = hps.data.win_length
        freq_bins = n_fft // 2 + 1
        gin = hps.model.gin_channels
        inter = hps.model.inter_channels
        tau_default = 0.3
        log(f"config: sr={sr} n_fft={n_fft} hop={hop} win={win} freqs={freq_bins} "
            f"gin={gin} inter={inter} zero_g={getattr(hps.model, 'zero_g', False)}")

        # --- wrap + export speaker encoder ---
        se_wrap = SpeakerEncoderWrapper(model.ref_enc).eval()
        signal = voiced_signal(args.seconds, sr)
        spec = spec_from_signal(signal, n_fft, hop)
        T = spec.shape[-1]
        with torch.no_grad():
            onnx_export(
                se_wrap,
                (spec,),
                os.path.join(out_dir, "speaker_encoder.onnx"),
                input_names=["spec"],
                output_names=["emb"],
                dynamic_axes={"spec": {2: "T"}, "emb": {1: "T"}},
                opset_version=OPSET,
                do_constant_folding=True,
            )

        # --- wrap + export converter ---
        conv_wrap = ConverterWrapper(model).eval()
        noise = torch.zeros(1, inter, T)
        lengths = torch.tensor([T], dtype=torch.long)
        g_src = torch.randn(1, gin, 1) * 0.1
        g_tgt = torch.randn(1, gin, 1) * 0.1
        tau = torch.tensor(tau_default)
        with torch.no_grad():
            onnx_export(
                conv_wrap,
                (spec, lengths, g_src, g_tgt, noise, tau),
                os.path.join(out_dir, "converter.onnx"),
                input_names=["spec", "spec_lengths", "g_src", "g_tgt", "noise", "tau"],
                output_names=["audio"],
                dynamic_axes={
                    "spec": {2: "T"},
                    "noise": {2: "T"},
                    "audio": {2: "Ta"},
                },
                opset_version=OPSET,
                do_constant_folding=True,
            )

        # --- validation: torch reference vs onnxruntime ---
        import onnxruntime as ort

        def max_diff(a: np.ndarray, b: np.ndarray) -> float:
            return float(np.max(np.abs(a - b)))

        sess_se = ort.InferenceSession(
            os.path.join(out_dir, "speaker_encoder.onnx"), providers=["CPUExecutionProvider"]
        )
        ref_emb = se_wrap(spec).detach().numpy()
        ort_emb = sess_se.run(["emb"], {"spec": spec.numpy()})[0]
        diff_se = max_diff(ref_emb, ort_emb)
        log(f"speaker_encoder: torch{ref_emb.shape} vs ort{ort_emb.shape} "
            f"max_abs_diff={diff_se:.3e}")

        with torch.no_grad():
            ref_audio = conv_wrap(spec, lengths, g_src, g_tgt, noise, tau).detach().numpy()
        sess_cv = ort.InferenceSession(
            os.path.join(out_dir, "converter.onnx"), providers=["CPUExecutionProvider"]
        )
        ort_audio = sess_cv.run(
            ["audio"],
            {
                "spec": spec.numpy(),
                "spec_lengths": lengths.numpy(),
                "g_src": g_src.numpy(),
                "g_tgt": g_tgt.numpy(),
                "noise": noise.numpy(),
                "tau": tau.numpy(),
            },
        )[0]
        diff_cv = max_diff(ref_audio, ort_audio)
        log(f"converter: torch{ref_audio.shape} vs ort{ort_audio.shape} "
            f"max_abs_diff={diff_cv:.3e}")

        # A second, longer clip to prove the dynamic axes behave.
        spec2 = spec_from_signal(voiced_signal(args.seconds * 3, sr), n_fft, hop)
        T2 = spec2.shape[-1]
        with torch.no_grad():
            ref2 = conv_wrap(
                spec2,
                torch.tensor([T2], dtype=torch.long),
                g_src, g_tgt,
                torch.zeros(1, inter, T2), tau,
            ).detach().numpy()
        ort2 = sess_cv.run(
            ["audio"],
            {
                "spec": spec2.numpy(),
                "spec_lengths": np.array([T2], dtype=np.int64),
                "g_src": g_src.numpy(),
                "g_tgt": g_tgt.numpy(),
                "noise": np.zeros((1, inter, T2), dtype=np.float32),
                "tau": tau.numpy(),
            },
        )[0]
        diff_long = max_diff(ref2, ort2)
        log(f"converter@3x length: max_abs_diff={diff_long:.3e}")

        # End-to-end sanity: SE of the clip, then convert spec -> itself-ish.
        emb = sess_se.run(["emb"], {"spec": spec.numpy()})[0]
        g = emb.reshape(1, gin, 1)
        rec = sess_cv.run(
            ["audio"],
            {
                "spec": spec.numpy(),
                "spec_lengths": lengths.numpy(),
                "g_src": g,
                "g_tgt": g,
                "noise": noise.numpy(),
                "tau": tau.numpy(),
            },
        )[0]
        log(f"identity round-trip audio rms={float(np.sqrt((rec ** 2).mean())):.4f} "
            f"(non-zero expected)")

        # --- manifest ---
        manifest = {
            "format_version": 1,
            "converter": {
                "file": "converter.onnx",
                "inputs": [
                    {"name": "spec", "shape": [1, freq_bins, "T"], "dtype": "float32"},
                    {"name": "spec_lengths", "shape": [1], "dtype": "int64"},
                    {"name": "g_src", "shape": [1, gin, 1], "dtype": "float32"},
                    {"name": "g_tgt", "shape": [1, gin, 1], "dtype": "float32"},
                    {"name": "noise", "shape": [1, inter, "T"], "dtype": "float32"},
                    {"name": "tau", "shape": [], "dtype": "float32"},
                ],
                "outputs": ["audio"],
            },
            "speaker_encoder": {
                "file": "speaker_encoder.onnx",
                "inputs": [
                    {"name": "spec", "shape": [1, freq_bins, "T"], "dtype": "float32"},
                ],
                "outputs": ["emb"],
            },
            "spectrogram": {
                "n_fft": n_fft,
                "hop_length": hop,
                "win_length": win,
                "sample_rate": sr,
                "epsilon": 1e-6,
            },
            "tau": tau_default,
            "created_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
            "openvoice_commit": commit,
            "checkpoint_sha256": ckpt_sha,
            "openvoice_hf_revision": HF_REVISION,
        }
        with open(os.path.join(out_dir, "manifest.json"), "w", encoding="utf-8") as f:
            json.dump(manifest, f, indent=2)
        log(f"manifest written to {out_dir}/manifest.json")

        # --- goldens for the browser DSP unit tests ---
        # 100 ms of the deterministic voiced signal, full linear spectrogram
        # in linearSpectrogram's [freq][time] layout. The JS test feeds
        # `signal` through its port and compares against `spec`.
        short = voiced_signal(0.1, sr)
        spec_short = spec_from_signal(short, n_fft, hop)
        gold = {
            "params": manifest["spectrogram"],
            "signal": [float(v) for v in short],
            "spec": [[float(v) for v in row] for row in spec_short[0].detach().numpy()],
        }
        gold_path = os.path.join(out_dir, "spectrogram_golden.json")
        with open(gold_path, "w", encoding="utf-8") as f:
            json.dump(gold, f)
        log(f"browser-test golden written to {gold_path} "
            f"(paste into src/lib/voice-clone/spectrogram.test.ts fixtures)")

        print("\n=== VALIDATION SUMMARY ===")
        print(f"speaker_encoder torch-vs-ort max_abs_diff: {diff_se:.6e}")
        print(f"converter       torch-vs-ort max_abs_diff: {diff_cv:.6e}")
        print(f"converter 3x    torch-vs-ort max_abs_diff: {diff_long:.6e}")
        ok = diff_se < 1e-3 and diff_cv < 1e-3 and diff_long < 1e-3
        print(f"PASS (< 1e-3): {ok}")
        if not ok:
            sys.exit(1)


if __name__ == "__main__":
    main()
