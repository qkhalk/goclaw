import type { Scene } from "../hooks/use-timeline";
import type { Storyboard } from "../video-tool-page";

/**
 * Wire translation between the editor's simple scene shape and the server
 * contract (internal/video/types.go):
 * - Editor edits narration as a plain string; the wire format is an object
 *   {text, voice} — submit must convert or the gateway 400s on unmarshal.
 * - Designer storyboards arrive with the wire object; applying one to the
 *   editor flattens narration back to text (the worker's default vi voice
 *   covers the common case; the editor has no per-scene voice field).
 */

type WireNarration = { text: string; voice?: string };
type WireScene = Omit<Scene, "narration"> & { narration?: WireNarration };
export type WireStoryboard = Omit<Storyboard, "scenes"> & { scenes: WireScene[] };

/** Editor scenes (narration flattened to text) from any accepted shape. */
export function normalizeEditorScenes(scenes: Scene[]): Scene[] {
  return scenes.map((sc) => {
    if (sc.narration === undefined || typeof sc.narration === "string") return sc;
    const n = sc.narration as unknown as WireNarration;
    return { ...sc, narration: n?.text || undefined };
  });
}

/** Wire storyboard for POST /v1/video/jobs (narration as {text, voice?}). */
export function toWireStoryboard(sb: Storyboard): WireStoryboard {
  return {
    ...sb,
    scenes: sb.scenes.map((sc) => {
      if (typeof sc.narration === "string" && sc.narration.trim()) {
        const { narration, ...rest } = sc;
        return { ...rest, narration: { text: narration.trim() } };
      }
      if (sc.narration && typeof sc.narration === "object") {
        return sc as unknown as WireScene;
      }
      const { narration: _drop, ...rest } = sc;
      return rest;
    }),
  };
}
