/**
 * Deterministic fingerprint for stored speaker embeddings.
 *
 * Used by the narration cache key so re-registering a voice with different
 * reference audio invalidates previously synthesized clips. FNV-1a 32-bit is
 * weak on its own, so two independent hashes (forward + a fixed-rotation
 * pass) are combined; collisions would only cause a stale-sounding cache
 * entry, never a crash.
 */

/** Returns a stable hex fingerprint of the embedding bytes. */
export function fingerprintEmbedding(emb: Float32Array): string {
  const bytes = new Uint8Array(emb.buffer, emb.byteOffset, emb.byteLength);
  const h1 = fnv1a(bytes, 0x811c9dc5, false);
  const h2 = fnv1a(bytes, 0x9dc5811c, true);
  return h1.toString(16).padStart(8, "0") + h2.toString(16).padStart(8, "0");
}

function fnv1a(bytes: Uint8Array, basis: number, reverse: boolean): number {
  let hash = basis;
  const n = bytes.length;
  for (let i = 0; i < n; i++) {
    hash ^= reverse ? (bytes[n - 1 - i] ?? 0) : (bytes[i] ?? 0);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash;
}
