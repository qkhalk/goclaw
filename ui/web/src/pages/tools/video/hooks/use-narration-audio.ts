import { useCallback, useEffect, useRef, useState } from "react";
import { useHttp } from "@/hooks/use-ws";
import {
  cachedFingerprint,
  cloneVoiceId,
  convertUtterance,
  getVoice,
  isCloneVoice,
  refreshVoiceFingerprints,
} from "@/lib/voice-clone";

/**
 * Real-TTS narration audio for the video studio.
 *
 * POST /v1/tts/synthesize (provider "edge", no API key) renders each scene's
 * narration; clips are cached by (voice, embedding fingerprint, text) so
 * preview playback, the listen button, and the duration-fit hint share one
 * synthesis. The server endpoint caps input at 500 chars — longer texts
 * synthesize truncated.
 *
 * Locally registered clone voices ("clone:<id>" present in IndexedDB) go
 * through the browser voice-clone pipeline: plain edge synthesis followed by
 * in-browser tone conversion. The gateway clone worker (provider "clone")
 * remains the backend for voices that only exist there. Every clone path
 * falls back to plain edge audio on failure — preview must never break.
 */

const MAX_SYNTH_CHARS = 500;

function clipText(text: string): string {
  const t = text.trim();
  return t.length > MAX_SYNTH_CHARS ? t.slice(0, MAX_SYNTH_CHARS) : t;
}

/** The edge voice used when a clone voice cannot render its base voice. */
const DEFAULT_FALLBACK_VOICE = "vi-VN-HoaiMyNeural";

export interface NarrationAudioController {
  /** Synthesize (or fetch from cache) audio for text+voice. Resolves null
   * for empty text; rejects on HTTP/network failure. */
  prepare: (text: string, voice?: string) => Promise<HTMLAudioElement | null>;
  /** Cached element for text+voice, undefined when not yet synthesized. */
  get: (text: string, voice?: string) => HTMLAudioElement | undefined;
  /** Synthesized duration for text+voice in seconds, or undefined. */
  durationOf: (text: string, voice?: string) => number | undefined;
  /** True while a synthesis for text+voice is in flight. */
  isLoading: (text: string, voice?: string) => boolean;
  /** Pause every cached element (seek/pause/scene switches). */
  pauseAll: () => void;
  /** Number of in-flight synthesis requests (drives UI status). */
  pendingCount: number;
}

export function useNarrationAudio(defaultVoice?: string): NarrationAudioController {
  const http = useHttp();
  const cacheRef = useRef(new Map<string, HTMLAudioElement>());
  const inflightRef = useRef(new Map<string, Promise<HTMLAudioElement | null>>());
  const urlsRef = useRef<string[]>([]);
  const [pending, setPending] = useState<Set<string>>(new Set());

  useEffect(() => {
    // Warm the id -> fingerprint mirror so cache keys carry the embedding
    // fingerprint of previously registered voices from the first call.
    void refreshVoiceFingerprints().catch(() => {
      // IndexedDB unavailable — cache keys degrade to voice+text only.
    });
    return () => {
      for (const el of cacheRef.current.values()) el.pause();
      for (const url of urlsRef.current) URL.revokeObjectURL(url);
    };
  }, []);

  const keyFor = useCallback(
    (text: string, voice?: string) => {
      const v = (voice || defaultVoice || "").trim();
      // Clone-voice keys carry the embedding fingerprint so re-registering a
      // voice invalidates previously synthesized clips. The fingerprint is
      // warmed on mount (and by every store write), so this stays sync.
      const fp = isCloneVoice(v) ? (cachedFingerprint(cloneVoiceId(v)) ?? "") : "";
      return `${v}::${fp}::${clipText(text)}`;
    },
    [defaultVoice],
  );

  const prepare = useCallback(
    (text: string, voice?: string): Promise<HTMLAudioElement | null> => {
      const clipped = clipText(text);
      if (!clipped) return Promise.resolve(null);
      const key = keyFor(clipped, voice);
      const cached = cacheRef.current.get(key);
      if (cached) return Promise.resolve(cached);
      const inflight = inflightRef.current.get(key);
      if (inflight) return inflight;

      const voiceId = (voice || defaultVoice || "").trim();
      const p = (async () => {
        setPending((prev) => new Set(prev).add(key));
        try {
          const blob = await synthesizeBlob(voiceId, clipped, defaultVoice, http.getAuthHeaders());
          const url = URL.createObjectURL(blob);
          urlsRef.current.push(url);
          const el = new Audio(url);
          el.preload = "auto";
          await new Promise<void>((resolve, reject) => {
            el.onloadedmetadata = () => resolve();
            el.onerror = () => reject(new Error("audio decode failed"));
            // Safety: don't hang forever on metadata that never arrives.
            setTimeout(() => resolve(), 4000);
          });
          cacheRef.current.set(keyFor(clipped, voice), el);
          return el;
        } finally {
          setPending((prev) => {
            const next = new Set(prev);
            next.delete(key);
            return next;
          });
          inflightRef.current.delete(key);
        }
      })();
      inflightRef.current.set(key, p);
      return p;
    },
    [http, keyFor, defaultVoice],
  );

  const get = useCallback(
    (text: string, voice?: string) => cacheRef.current.get(keyFor(text, voice)),
    [keyFor],
  );

  const durationOf = useCallback(
    (text: string, voice?: string) => {
      const el = cacheRef.current.get(keyFor(text, voice));
      if (!el || !Number.isFinite(el.duration)) return undefined;
      return el.duration;
    },
    [keyFor],
  );

  const isLoading = useCallback(
    (text: string, voice?: string) => pending.has(keyFor(text, voice)),
    [pending, keyFor],
  );

  const pauseAll = useCallback(() => {
    for (const el of cacheRef.current.values()) el.pause();
  }, []);

  return { prepare, get, durationOf, isLoading, pauseAll, pendingCount: pending.size };
}

/** Picks the pipeline for a voice id. Every failure path lands on plain edge. */
async function synthesizeBlob(
  voiceId: string,
  clipped: string,
  defaultVoice: string | undefined,
  authHeaders: Record<string, string>,
): Promise<Blob> {
  if (isCloneVoice(voiceId)) {
    const rec = await getVoice(cloneVoiceId(voiceId));
    if (rec) {
      try {
        // In-browser: edge base reading + local tone conversion.
        return await convertUtterance({ text: clipped, voice: rec, authHeaders });
      } catch (err) {
        console.warn("narration: browser voice-clone failed, using plain edge audio", err);
        return synthesizeViaGateway(
          clipped,
          "edge",
          rec.baseVoice || defaultVoice || DEFAULT_FALLBACK_VOICE,
          authHeaders,
        );
      }
    }
    // Not local — the voice may live on the gateway clone worker.
    try {
      return await synthesizeViaGateway(clipped, "clone", cloneVoiceId(voiceId), authHeaders);
    } catch (err) {
      console.warn("narration: gateway clone voice failed, using plain edge audio", err);
      return synthesizeViaGateway(
        clipped,
        "edge",
        defaultVoice || DEFAULT_FALLBACK_VOICE,
        authHeaders,
      );
    }
  }
  return synthesizeViaGateway(clipped, "edge", voiceId || undefined, authHeaders);
}

/** POST /v1/tts/synthesize against an explicit provider. */
async function synthesizeViaGateway(
  text: string,
  provider: string,
  voiceId: string | undefined,
  authHeaders: Record<string, string>,
): Promise<Blob> {
  const res = await fetch("/v1/tts/synthesize", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders },
    body: JSON.stringify({ text, provider, ...(voiceId ? { voice_id: voiceId } : {}) }),
  });
  if (!res.ok) {
    const msg = await res.text().catch(() => "");
    throw new Error(msg || `TTS failed (${res.status})`);
  }
  return res.blob();
}

/** Edge-tts voices offered when /v1/tts/capabilities is unreachable. */
export const FALLBACK_EDGE_VOICES = [
  { voice_id: "vi-VN-HoaiMyNeural", name: "Hoài My · vi-VN" },
  { voice_id: "vi-VN-NamMinhNeural", name: "Nam Minh · vi-VN" },
  { voice_id: "en-US-AriaNeural", name: "Aria · en-US" },
  { voice_id: "en-US-GuyNeural", name: "Guy · en-US" },
  { voice_id: "en-US-JennyNeural", name: "Jenny · en-US" },
  { voice_id: "en-GB-SoniaNeural", name: "Sonia · en-GB" },
  { voice_id: "zh-CN-XiaoxiaoNeural", name: "Xiaoxiao · zh-CN" },
  { voice_id: "ja-JP-NanamiNeural", name: "Nanami · ja-JP" },
];
