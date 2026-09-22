/**
 * Public API of the browser voice-clone pipeline.
 *
 * registerVoiceFromBlob — decode reference audio, extract the 256-dim
 *   speaker embedding with the ONNX speaker encoder, persist in IndexedDB.
 * convertUtterance — synthesize the utterance with the plain edge provider,
 *   then tone-convert it to the registered voice entirely in-browser.
 *
 * Every function is fail-soft for callers: VoiceCloneUnavailableError is
 * thrown when the model assets are not published yet, and the narration hook
 * converts that into a fallback to plain edge audio (preview never breaks).
 */

import type { F32 } from "./audio";
import type { Tensor } from "onnxruntime-web";
import { decodeToMonoResampled, encodeWav16, trimSilence } from "./audio";
import { fingerprintEmbedding } from "./fingerprint";
import { loadVoiceCloneRuntime, type VoiceCloneRuntime } from "./runtime";
import { linearSpectrogram } from "./spectrogram";
import { makeVoiceRecord, putVoice } from "./store";
import { MODEL_SAMPLE_RATE, type ClonedVoice, type SpectrogramParams, VoiceCloneUnavailableError } from "./types";

// --- re-exports (stable surface for hooks/components) ---
export { listVoices, removeVoice, putVoice, getVoice, cachedFingerprint, refreshVoiceFingerprints } from "./store";
export {
  CLONE_VOICE_PREFIX,
  MODEL_SAMPLE_RATE,
  cloneVoiceId,
  isCloneVoice,
  toCloneVoiceId,
  VoiceCloneUnavailableError,
} from "./types";
export type { ClonedVoice, SpectrogramParams, VoiceCloneManifest } from "./types";
export { isModelConfigured, VOICE_CLONE_MODEL_BASE_URL } from "./runtime";

/** Reference audio longer than this is truncated before embedding (compute cap). */
const MAX_REF_SECONDS = 120;
/** Rejects reference clips shorter than this — embeddings are garbage below it. */
const MIN_REF_SECONDS = 1.5;

export interface RegisterVoiceOptions {
  /** Human label shown in voice pickers. */
  name: string;
  /** Reference audio in any browser-decodable container. */
  blob: Blob;
  /** Edge-tts base voice utterances are synthesized with before conversion. */
  baseVoice: string;
  /** Called with fractions in [0,1] — decode/spec/embed/store milestones. */
  onProgress?: (fraction: number) => void;
}

/**
 * Decodes reference audio, extracts the speaker embedding in-browser and
 * stores the voice in IndexedDB. Throws (rather than falls back) — the
 * caller owns the UX for a user-initiated registration.
 */
export async function registerVoiceFromBlob(opts: RegisterVoiceOptions): Promise<ClonedVoice> {
  const runtime = await loadVoiceCloneRuntime();
  opts.onProgress?.(0.15);

  const bytes = await opts.blob.arrayBuffer();
  let mono = await decodeToMonoResampled(bytes, MODEL_SAMPLE_RATE);
  opts.onProgress?.(0.35);

  mono = trimSilence(mono, MODEL_SAMPLE_RATE);
  if (mono.length > MAX_REF_SECONDS * MODEL_SAMPLE_RATE) {
    mono = mono.slice(0, MAX_REF_SECONDS * MODEL_SAMPLE_RATE) as F32;
  }
  if (mono.length < MIN_REF_SECONDS * MODEL_SAMPLE_RATE) {
    throw new VoiceCloneUnavailableError("reference audio is too short (need ≥1.5s of speech)");
  }

  const params = spectrogramParamsOf(runtime);
  const spec = linearSpectrogram(mono, params);
  if (!spec) throw new VoiceCloneUnavailableError("reference audio produced no spectrogram frames");
  opts.onProgress?.(0.5);

  const embedding = await runSpeakerEncoder(runtime, spec, params);
  opts.onProgress?.(0.85);

  const record = makeVoiceRecord(
    opts.name,
    embedding,
    fingerprintEmbedding(embedding),
    opts.baseVoice,
    mono.length / MODEL_SAMPLE_RATE,
  );
  await putVoice(record);
  opts.onProgress?.(1);
  return record;
}

export interface ConvertUtteranceOptions {
  text: string;
  /** Locally registered voice (the target timbre). */
  voice: ClonedVoice;
  /** Auth headers for POST /v1/tts/synthesize (from useHttp().getAuthHeaders()). */
  authHeaders: Record<string, string>;
  onProgress?: (fraction: number) => void;
}

/**
 * Renders `text` in the registered voice: edge-tts synthesizes the base
 * reading, then the ONNX tone-color converter re-timbres it in-browser.
 * Returns 16-bit PCM WAV at MODEL_SAMPLE_RATE.
 */
export async function convertUtterance(opts: ConvertUtteranceOptions): Promise<Blob> {
  const { voice, onProgress } = opts;
  const runtime = await loadVoiceCloneRuntime();
  onProgress?.(0.05);

  // 1. Base reading via the existing gateway edge provider.
  const res = await fetch("/v1/tts/synthesize", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...opts.authHeaders },
    body: JSON.stringify({ text: opts.text, provider: "edge", voice_id: voice.baseVoice }),
  });
  if (!res.ok) {
    throw new VoiceCloneUnavailableError(`edge synthesize failed (${res.status})`);
  }
  const baseAudio = await res.arrayBuffer();
  onProgress?.(0.25);

  // 2. Same front-end as registration: mono 22.05k, trim, linear spec.
  const mono = trimSilence(
    await decodeToMonoResampled(baseAudio, MODEL_SAMPLE_RATE),
    MODEL_SAMPLE_RATE,
  );
  const params = spectrogramParamsOf(runtime);
  const spec = linearSpectrogram(mono, params);
  if (!spec) throw new VoiceCloneUnavailableError("utterance produced no spectrogram frames");
  onProgress?.(0.4);

  // 3. Source embedding from the actual edge audio (OpenVoice tone transfer).
  const srcEmb = await runSpeakerEncoder(runtime, spec, params);
  onProgress?.(0.6);

  // 4. Tone-convert towards the registered target embedding; zero noise keeps
  //    the pass deterministic (see ConverterWrapper in export_onnx.py).
  const wav = await runConverter(runtime, spec, params, srcEmb, voice.embedding);
  onProgress?.(0.95);
  const blob = encodeWav16(wav, MODEL_SAMPLE_RATE);
  onProgress?.(1);
  return blob;
}

// --- internal graph feeding ---

function spectrogramParamsOf(runtime: VoiceCloneRuntime): SpectrogramParams {
  return runtime.manifest.spectrogram;
}

/** Runs the speaker encoder on a channel-major linear spectrogram. */
async function runSpeakerEncoder(
  runtime: VoiceCloneRuntime,
  spec: F32,
  params: SpectrogramParams,
): Promise<F32> {
  const ort = await importOrtNamespace();
  const bins = params.n_fft / 2 + 1;
  const frames = spec.length / bins;
  const input = encoderInputName(runtime);
  const x = new ort.Tensor("float32", spec, [1, bins, frames]);
  const out = await runtime.encoder.run({ [input]: x });
  const raw = out[outputName(runtime.encoder, input)]?.data;
  if (!(raw instanceof Float32Array) || raw.length === 0) {
    throw new VoiceCloneUnavailableError("speaker encoder returned no embedding");
  }
  return new Float32Array(raw); // own copy → plain ArrayBuffer backing
}

/**
 * Runs the tone-color converter. `noise` is zeros (deterministic MAP path,
 * see export_onnx.py ConverterWrapper docs) and `tau` comes from the voice
 * record's manifest default.
 */
async function runConverter(
  runtime: VoiceCloneRuntime,
  spec: F32,
  params: SpectrogramParams,
  srcEmb: F32,
  tgtEmb: Float32Array,
): Promise<F32> {
  const ort = await importOrtNamespace();
  const desc = runtime.manifest.converter;
  const bins = params.n_fft / 2 + 1;
  const frames = spec.length / bins; // channel-major layout: bins × frames
  const gin = dimOf(desc.inputs, "g_src", 1);
  const inter = dimOf(desc.inputs, "noise", 1);
  if (srcEmb.length !== gin || tgtEmb.length !== gin) {
    throw new VoiceCloneUnavailableError(
      `embedding size mismatch (got ${srcEmb.length}/${tgtEmb.length}, want ${gin})`,
    );
  }

  const feeds: Record<string, Tensor> = {};
  feeds[nameOf(desc.inputs, "spec")] = new ort.Tensor("float32", spec, [1, bins, frames]);
  feeds[nameOf(desc.inputs, "spec_lengths")] = new ort.Tensor(
    "int64",
    BigInt64Array.from([BigInt(frames)]),
    [1],
  );
  feeds[nameOf(desc.inputs, "g_src")] = new ort.Tensor("float32", gMatrix(srcEmb, gin), [1, gin, 1]);
  feeds[nameOf(desc.inputs, "g_tgt")] = new ort.Tensor("float32", gMatrix(new Float32Array(tgtEmb), gin), [1, gin, 1]);
  feeds[nameOf(desc.inputs, "noise")] = new ort.Tensor(
    "float32",
    new Float32Array(inter * frames),
    [1, inter, frames],
  );
  feeds[nameOf(desc.inputs, "tau")] = new ort.Tensor(
    "float32",
    Float32Array.of(runtime.manifest.tau),
    [],
  );

  const out = await runtime.converter.run(feeds);
  const raw = out[desc.outputs[0] ?? "audio"]?.data;
  if (!(raw instanceof Float32Array) || raw.length === 0) {
    throw new VoiceCloneUnavailableError("converter returned no audio");
  }
  return new Float32Array(raw);
}

/** Reshapes a (gin,) embedding into the (gin, 1) column vector the graph wants. */
function gMatrix(emb: Float32Array, gin: number): Float32Array<ArrayBuffer> {
  const out = new Float32Array(gin);
  for (let i = 0; i < gin && i < emb.length; i++) out[i] = emb[i] ?? 0;
  return out;
}

function importOrtNamespace(): Promise<typeof import("onnxruntime-web")> {
  return import("onnxruntime-web");
}

function encoderInputName(runtime: VoiceCloneRuntime): string {
  const fromManifest = runtime.manifest.speaker_encoder.inputs[0]?.name;
  return fromManifest ?? runtime.encoder.inputNames[0] ?? "spec";
}

function outputName(session: { outputNames: readonly string[] }, fallback: string): string {
  return session.outputNames[0] ?? fallback;
}

function nameOf(
  inputs: readonly { name: string }[],
  logical: string,
): string {
  return inputs.find((i) => i.name === logical)?.name ?? logical;
}

/** Numeric value at `axis` of the named input's declared shape. */
function dimOf(
  inputs: readonly { name: string; shape: readonly (number | string)[] }[],
  logical: string,
  axis: number,
): number {
  const input = inputs.find((i) => i.name === logical);
  const v = input?.shape[axis];
  return typeof v === "number" ? v : 0;
}
