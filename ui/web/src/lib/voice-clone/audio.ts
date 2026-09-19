/**
 * Audio decode/resample/encode helpers for the voice-clone pipeline.
 * Browser-only APIs (AudioContext, OfflineAudioContext) — no Node support.
 *
 * All returned buffers are freshly allocated (`new Float32Array(n)`), so they
 * are `Float32Array<ArrayBuffer>` and safe to hand to AudioBuffer.copyToChannel
 * and the onnxruntime-web Tensor constructor under TS strict Float32Array
 * generics.
 */

/** A Float32Array backed by a plain ArrayBuffer (allocatable, transferable). */
export type F32 = Float32Array<ArrayBuffer>;

/** Decodes arbitrary audio bytes to mono Float32 at `targetRate` Hz.
 * Uses decodeAudioData + a second OfflineAudioContext pass for resampling,
 * which produces better quality than naive linear interpolation. */
export async function decodeToMonoResampled(data: ArrayBuffer, targetRate: number): Promise<F32> {
  const decodeCtx = getDecodeContext();
  const decoded = await decodeCtx.decodeAudioData(data.slice(0));
  return resampleToMono(decoded, targetRate);
}

/** AudioContext is lazily created (browsers limit concurrent instances). */
let decodeCtx: AudioContext | null = null;
function getDecodeContext(): AudioContext {
  if (!decodeCtx) {
    const Ctor =
      window.AudioContext ??
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    decodeCtx = new Ctor();
  }
  return decodeCtx;
}

/** Mixes an AudioBuffer down to mono and resamples to `targetRate`. */
export async function resampleToMono(buffer: AudioBuffer, targetRate: number): Promise<F32> {
  // Fast path: already mono at target rate.
  if (buffer.numberOfChannels === 1 && buffer.sampleRate === targetRate) {
    const direct = buffer.getChannelData(0);
    return new Float32Array(direct ?? []) as F32;
  }
  const mono = mixToMono(buffer);
  if (buffer.sampleRate === targetRate) return mono;

  const frames = Math.max(1, Math.ceil(buffer.duration * targetRate));
  const ctx = new OfflineAudioContext(1, frames, targetRate);
  const src = ctx.createBufferSource();
  // Wrap the mono mix in a target-rate-agnostic AudioBuffer for the graph.
  const srcBuf = ctx.createBuffer(1, mono.length, buffer.sampleRate);
  srcBuf.copyToChannel(mono, 0);
  src.buffer = srcBuf;
  src.connect(ctx.destination);
  src.start();
  const rendered = await ctx.startRendering();
  const out = rendered.getChannelData(0);
  return new Float32Array(out ?? []) as F32;
}

/** Averages all channels into one Float32Array at the buffer's own rate. */
export function mixToMono(buffer: AudioBuffer): F32 {
  const n = buffer.length;
  const out = new Float32Array(n);
  for (let c = 0; c < buffer.numberOfChannels; c++) {
    const ch = buffer.getChannelData(c);
    if (!ch) continue;
    for (let i = 0; i < n; i++) out[i] = (out[i] ?? 0) + (ch[i] ?? 0);
  }
  if (buffer.numberOfChannels > 1) {
    const k = 1 / buffer.numberOfChannels;
    for (let i = 0; i < n; i++) out[i] = (out[i] ?? 0) * k;
  }
  return out;
}

/**
 * Trims leading/trailing low-energy audio (simple energy VAD used before
 * speaker-embedding extraction). Returns the same data when no silence is
 * found. `thresholdDb` is relative to the loudest frame.
 */
export function trimSilence(samples: F32, rate: number, thresholdDb = -40, marginMs = 100): F32 {
  const frame = Math.max(1, Math.floor(rate * 0.02)); // 20 ms frames
  const frames = Math.floor(samples.length / frame);
  if (frames === 0) return samples;
  const energies = new Float32Array(frames);
  let maxE = 0;
  for (let f = 0; f < frames; f++) {
    let e = 0;
    for (let i = f * frame; i < (f + 1) * frame; i++) {
      const s = samples[i] ?? 0;
      e += s * s;
    }
    energies[f] = e;
    if (e > maxE) maxE = e;
  }
  if (maxE <= 0) return samples;
  const floor = maxE * Math.pow(10, thresholdDb / 10);
  let first = 0;
  while (first < frames && (energies[first] ?? 0) < floor) first++;
  let last = frames - 1;
  while (last > first && (energies[last] ?? 0) < floor) last--;
  const margin = Math.floor((marginMs / 1000) * rate);
  const start = Math.max(0, first * frame - margin);
  const end = Math.min(samples.length, (last + 1) * frame + margin);
  if (start === 0 && end === samples.length) return samples;
  return samples.slice(start, end) as F32;
}

/** Encodes mono Float32 samples to a 16-bit PCM WAV Blob. */
export function encodeWav16(samples: F32, rate: number): Blob {
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  writeAscii(view, 0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  writeAscii(view, 8, "WAVE");
  writeAscii(view, 12, "fmt ");
  view.setUint32(16, 16, true); // fmt chunk size
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, rate, true);
  view.setUint32(28, rate * 2, true); // byte rate
  view.setUint16(32, 2, true); // block align
  view.setUint16(34, 16, true); // bits per sample
  writeAscii(view, 36, "data");
  view.setUint32(40, samples.length * 2, true);
  let off = 44;
  for (let i = 0; i < samples.length; i++, off += 2) {
    const s = Math.max(-1, Math.min(1, samples[i] ?? 0));
    view.setInt16(off, s < 0 ? s * 0x8000 : s * 0x7fff, true);
  }
  return new Blob([buffer], { type: "audio/wav" });
}

function writeAscii(view: DataView, offset: number, text: string): void {
  for (let i = 0; i < text.length; i++) {
    view.setUint8(offset + i, text.charCodeAt(i));
  }
}

/** Peak amplitude (used for cheap input-level display and sanity checks). */
export function peakAmplitude(samples: F32): number {
  let peak = 0;
  for (let i = 0; i < samples.length; i++) {
    const a = Math.abs(samples[i] ?? 0);
    if (a > peak) peak = a;
  }
  return peak;
}
