import type { Scene } from "../hooks/use-timeline";
import type { Storyboard } from "../video-tool-page";

/**
 * Wire translation between the editor's scene shape and the server contract
 * (internal/video/types.go):
 * - The editor keeps narration split: narration (text) + narration_voice
 *   (optional per-scene override). The wire format is an object
 *   {text, voice} — submit must convert or the gateway 400s on unmarshal.
 * - The storyboard-level default voice (meta.narration_voice, client-only)
 *   is applied at submit time and stripped from the wire payload: the
 *   server Storyboard has no such field.
 * - Designer storyboards arrive with the wire object; applying one to the
 *   editor flattens it back to narration + narration_voice.
 */

type WireNarration = { text: string; voice?: string };
type WireScene = Omit<Scene, "narration"> & { narration?: WireNarration };
export type WireStoryboard = Omit<Storyboard, "scenes" | "narration_voice"> & {
  scenes: WireScene[];
};

/** Editor scenes from any accepted shape: wire {text, voice} narration is
 * flattened to narration + narration_voice; strings pass through. */
export function normalizeEditorScenes(scenes: Scene[]): Scene[] {
  return scenes.map((sc) => {
    if (sc.narration === undefined || typeof sc.narration === "string") return sc;
    const n = sc.narration as unknown as WireNarration;
    const next: Scene = { ...sc, narration: n?.text || undefined };
    if (n?.voice) next.narration_voice = n.voice;
    else delete next.narration_voice;
    return next;
  });
}

/** Wire storyboard for POST /v1/video/jobs (narration as {text, voice?}). */
export function toWireStoryboard(sb: Storyboard, defaultVoice?: string): WireStoryboard {
  const { narration_voice: _globalVoice, ...metaOnly } = sb;
  const fallbackVoice = defaultVoice || _globalVoice;
  return {
    ...metaOnly,
    scenes: sb.scenes.map((sc) => {
      const voice = sc.narration_voice || fallbackVoice;
      if (typeof sc.narration === "string" && sc.narration.trim()) {
        const { narration, narration_voice: _drop, ...rest } = sc;
        return {
          ...rest,
          narration: { text: narration.trim(), ...(voice ? { voice } : {}) },
        } as WireScene;
      }
      if (sc.narration && typeof sc.narration === "object") {
        return sc as unknown as WireScene;
      }
      const { narration: _dropNarr, narration_voice: _dropVoice, ...rest } = sc;
      return rest as WireScene;
    }),
  };
}
