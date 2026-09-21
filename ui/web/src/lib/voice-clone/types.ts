/**
 * Shared types for the browser voice-clone pipeline (OpenVoice v2 style
 * tone-color conversion running fully client-side via onnxruntime-web).
 *
 * Heavy compute (mel spectrogram, speaker embedding, tone conversion) runs in
 * the browser on purpose — the gateway box is 1 vCPU / 512 MB and must never
 * touch this workload. The server only sees the final rendered audio.
 */

/** Sampling rate every model in the pipeline consumes/produces (Hz). */
export const MODEL_SAMPLE_RATE = 22050;

/** Voice ids used by the narration hook are prefixed with this marker. */
export const CLONE_VOICE_PREFIX = "clone:";

/** Returns true when `voice` refers to a locally registered cloned voice. */
export function isCloneVoice(voice: string | undefined | null): boolean {
  return typeof voice === "string" && voice.startsWith(CLONE_VOICE_PREFIX);
}

/** Strips the "clone:" prefix; returns the raw local voice id. */
export function cloneVoiceId(voice: string): string {
  return voice.slice(CLONE_VOICE_PREFIX.length);
}

/** Wraps a local id into the "clone:<id>" voice id used by callers. */
export function toCloneVoiceId(id: string): string {
  return CLONE_VOICE_PREFIX + id;
}

/**
 * Spectrogram parameters shared by the speaker encoder and the tone color
 * converter. Mirrors the OpenVoice v2 `spectrogram_torch` frontend; the
 * authoritative values live in the manifest JSON shipped next to the ONNX
 * models and must match `contrib/voiceclone/export_onnx.py` output.
 *
 * Note: OpenVoice v2 consumes the LINEAR spectrogram (n_fft/2+1 bins,
 * magnitude with epsilon guard) — no mel projection, no log.
 */
export interface SpectrogramParams {
  /** FFT size (1024 for OpenVoice v2). Must be a power of two. */
  readonly n_fft: number;
  /** Hop between frames in samples (256 for OpenVoice v2). */
  readonly hop_length: number;
  /** Window length in samples (1024 for OpenVoice v2). */
  readonly win_length: number;
  readonly sample_rate: number;
  /** Numerical floor inside sqrt(re²+im²+ε) (1e-6 in spectrogram_torch). */
  readonly epsilon: number;
}

/** Manifest shipped alongside the ONNX models (see export_onnx.py). */
export interface VoiceCloneManifest {
  readonly format_version: 1;
  readonly converter: ModelIODescriptor;
  readonly speaker_encoder: ModelIODescriptor;
  readonly spectrogram: SpectrogramParams;
  /** Default tau (timbre-blend factor) used by the converter. */
  readonly tau: number;
  readonly created_at: string;
  readonly openvoice_commit: string;
}

export interface ModelIODescriptor {
  readonly file: string;
  readonly inputs: readonly ModelInput[];
  readonly outputs: readonly string[];
}

export interface ModelInput {
  readonly name: string;
  readonly shape: readonly (number | string)[];
  readonly dtype: "float32" | "int64";
}

/** A locally registered voice stored in IndexedDB. */
export interface ClonedVoice {
  /** Local id (no "clone:" prefix) — uuid-ish string. */
  id: string;
  name: string;
  /** 256-dim speaker embedding extracted from the reference audio. */
  embedding: Float32Array;
  /** Deterministic fingerprint of `embedding` — part of narration cache keys. */
  fingerprint: string;
  /** Edge-tts base voice used to synthesize the source utterance. */
  baseVoice: string;
  sampleRate: number;
  /** Seconds of reference audio the embedding was computed from. */
  refSeconds: number;
  createdAt: number;
}

/** Error thrown when the model assets are not configured/fetchable. */
export class VoiceCloneUnavailableError extends Error {
  constructor(reason: string) {
    super(`voice-clone unavailable: ${reason}`);
    this.name = "VoiceCloneUnavailableError";
  }
}
