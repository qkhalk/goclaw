/**
 * IndexedDB-backed store for locally registered cloned voices.
 * Framework-free; embeddings are stored as raw Float32Array records.
 */

import type { ClonedVoice } from "./types";
import { MODEL_SAMPLE_RATE } from "./types";

const DB_NAME = "goclaw-voice-clone";
const DB_VERSION = 1;
const STORE = "voices";

let dbPromise: Promise<IDBDatabase> | null = null;

function openDb(): Promise<IDBDatabase> {
  if (!dbPromise) {
    dbPromise = new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(STORE)) {
          db.createObjectStore(STORE, { keyPath: "id" });
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error ?? new Error("indexedDB open failed"));
    });
  }
  return dbPromise;
}

function tx<T>(mode: IDBTransactionMode, fn: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return openDb().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const t = db.transaction(STORE, mode);
        const req = fn(t.objectStore(STORE));
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error ?? new Error("indexedDB request failed"));
      }),
  );
}

/** Lists all locally registered voices, newest first. */
export async function listVoices(): Promise<ClonedVoice[]> {
  const all = await tx<ClonedVoice[]>("readonly", (s) => s.getAll() as IDBRequest<ClonedVoice[]>);
  return all.sort((a, b) => b.createdAt - a.createdAt);
}

/** Deletes a voice by local id. Returns true when a row was removed. */
export async function removeVoice(id: string): Promise<boolean> {
  const existed = await getVoice(id);
  if (!existed) return false;
  await tx("readwrite", (s) => s.delete(id) as IDBRequest<undefined>);
  fpCache.delete(id);
  return true;
}

/**
 * Sync id -> fingerprint mirror, so the narration hook can build stable
 * cache keys without awaiting IndexedDB on every keystroke. Populated by
 * reads/writes below; `refreshVoiceFingerprints` warms it in bulk.
 */
const fpCache = new Map<string, string>();

/** Returns the known fingerprint for a local voice id, if loaded. */
export function cachedFingerprint(id: string): string | undefined {
  return fpCache.get(id);
}

/** Loads the voice list and warms the fingerprint cache. */
export async function refreshVoiceFingerprints(): Promise<void> {
  for (const v of await listVoices()) fpCache.set(v.id, v.fingerprint);
}

/** Inserts or updates a voice record. */
export async function putVoice(voice: ClonedVoice): Promise<void> {
  await tx("readwrite", (s) => s.put(voice) as IDBRequest<IDBValidKey>);
  fpCache.set(voice.id, voice.fingerprint);
}

/** Returns one voice by local id, or undefined. */
export async function getVoice(id: string): Promise<ClonedVoice | undefined> {
  const v = await tx<ClonedVoice | undefined>("readonly", (s) =>
    s.get(id) as IDBRequest<ClonedVoice | undefined>,
  );
  if (v) fpCache.set(v.id, v.fingerprint);
  return v;
}

/** Builds a fresh ClonedVoice record (id assigned here). */
export function makeVoiceRecord(
  name: string,
  embedding: Float32Array,
  fingerprint: string,
  baseVoice: string,
  refSeconds: number,
): ClonedVoice {
  return {
    id: newId(),
    name,
    embedding,
    fingerprint,
    baseVoice,
    sampleRate: MODEL_SAMPLE_RATE,
    refSeconds,
    createdAt: Date.now(),
  };
}

/** Crypto-random id; falls back to Math.random when unavailable. */
export function newId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  return `v-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}
