/**
 * Lazy onnxruntime-web loader for the voice-clone pipeline.
 *
 * The runtime (~2 MB wasm) and the model files (~20 MB total) are only
 * fetched when a user actually registers/uses a cloned voice. Model bytes
 * are cached in Cache Storage so the download happens once.
 *
 * The wasm binaries onnxruntime-web fetches at runtime come from the pinned
 * jsdelivr URL for the exact installed version (ort's default behavior, made
 * explicit below). Self-hosting them behind the app origin is a documented
 * follow-up for offline/air-gapped deployments.
 */

import type { InferenceSession, Tensor } from "onnxruntime-web";
import type { VoiceCloneManifest } from "./types";
import { MODEL_SAMPLE_RATE, VoiceCloneUnavailableError } from "./types";

/**
 * Base URL hosting converter.onnx, speaker_encoder.onnx and manifest.json.
 * PLACEHOLDER — the orchestrator must point this at the release asset that
 * carries the files produced by contrib/voiceclone/export_onnx.py. An empty
 * string marks the feature as not-yet-published: callers treat it as
 * unavailable and fall back to plain edge-tts audio.
 */
export const VOICE_CLONE_MODEL_BASE_URL = "";

/** Must match the exact onnxruntime-web version in package.json (pinned, no
 * caret) — the wasm binaries fetched here and the bundled JS are versioned
 * together and mismatch subtly breaks inference. */
const ORT_VERSION = "1.30.0";
const RUNTIME_WASM_BASE = `https://cdn.jsdelivr.net/npm/onnxruntime-web@${ORT_VERSION}/dist/`;
const CACHE_NAME = "goclaw-voice-clone-v1";

export type OrtSession = InferenceSession;
export type OrtTensor = Tensor;

interface OrtNamespace {
  env: {
    wasm: { wasmPaths: string; numThreads: number; simd: boolean };
  };
  InferenceSession: {
    create(
      uri: string | Uint8Array | ArrayBuffer,
      options?: { executionProviders?: readonly string[] },
    ): Promise<InferenceSession>;
  };
  Tensor: typeof Tensor;
}

let runtimePromise: Promise<VoiceCloneRuntime> | null = null;

/** True when a model base URL has been configured (feature is published). */
export function isModelConfigured(): boolean {
  return VOICE_CLONE_MODEL_BASE_URL.length > 0;
}

export interface VoiceCloneRuntime {
  manifest: VoiceCloneManifest;
  converter: OrtSession;
  encoder: OrtSession;
}

/**
 * Loads (once) and returns the shared runtime. Concurrency-safe: parallel
 * callers share the same promise.
 */
export function loadVoiceCloneRuntime(): Promise<VoiceCloneRuntime> {
  if (!isModelConfigured()) {
    return Promise.reject(
      new VoiceCloneUnavailableError("model base URL is not configured yet"),
    );
  }
  if (!runtimePromise) {
    runtimePromise = loadRuntimeInternal().catch((err) => {
      runtimePromise = null; // allow retry after a transient failure
      throw err;
    });
  }
  return runtimePromise;
}

async function loadRuntimeInternal(): Promise<VoiceCloneRuntime> {
  const ort = await importOrt();
  ort.env.wasm.wasmPaths = RUNTIME_WASM_BASE;
  ort.env.wasm.numThreads = 1; // no cross-origin-isolation on plain http
  ort.env.wasm.simd = true;

  const manifestBytes = await fetchCached(manifestUrl());
  const manifest = JSON.parse(new TextDecoder().decode(manifestBytes)) as VoiceCloneManifest;

  // The whole DSP pipeline decodes/resamples at MODEL_SAMPLE_RATE; a manifest
  // from a differently-configured checkpoint would silently produce garbled
  // audio instead of failing loudly.
  if (manifest.spectrogram.sample_rate !== MODEL_SAMPLE_RATE) {
    throw new VoiceCloneUnavailableError(
      `manifest sample rate ${manifest.spectrogram.sample_rate} != supported ${MODEL_SAMPLE_RATE}`,
    );
  }

  const [converter, encoder] = await Promise.all([
    ort.InferenceSession.create(
      await fetchCached(`${VOICE_CLONE_MODEL_BASE_URL}/${manifest.converter.file}`),
      { executionProviders: ["wasm"] },
    ),
    ort.InferenceSession.create(
      await fetchCached(`${VOICE_CLONE_MODEL_BASE_URL}/${manifest.speaker_encoder.file}`),
      { executionProviders: ["wasm"] },
    ),
  ]);
  return { manifest, converter, encoder };
}

/** Model manifest location (kept as a function so the constant stays single-sourced). */
function manifestUrl(): string {
  return `${VOICE_CLONE_MODEL_BASE_URL}/manifest.json`;
}

/** Dynamic import of the onnxruntime-web wasm build. */
async function importOrt(): Promise<OrtNamespace> {
  const mod = (await import("onnxruntime-web")) as unknown as OrtNamespace & {
    default?: OrtNamespace;
  };
  return mod.default ?? mod;
}

/** Fetch with Cache Storage backing — downloads model bytes exactly once. */
async function fetchCached(url: string): Promise<ArrayBuffer> {
  let cache: Cache | null = null;
  if (typeof caches !== "undefined") {
    cache = await caches.open(CACHE_NAME);
    const hit = await cache.match(url);
    if (hit) return hit.arrayBuffer();
  }
  const res = await fetch(url);
  if (!res.ok) {
    throw new VoiceCloneUnavailableError(`fetch ${url} failed (${res.status})`);
  }
  const buf = await res.arrayBuffer();
  if (cache) {
    // Fire-and-forget: a failed cache write must not break the load.
    void cache.put(url, new Response(buf.slice(0)));
  }
  return buf;
}

/** Test hook: clears the memoized runtime (and optionally the cache). */
export async function resetRuntimeForTests(clearCache = false): Promise<void> {
  runtimePromise = null;
  if (clearCache && typeof caches !== "undefined") {
    await caches.delete(CACHE_NAME);
  }
}
