import type { Layer, Scene } from "../hooks/use-timeline";
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
 * - Motion-layer fields (highlights, counter/toggle/bars/stack/stamp/cta)
 *   and scene style_pack are clamped to the server contract (validate.go)
 *   in BOTH directions, so a stray inspector value can't 400 the render.
 *   Storyboards without the new fields pass through exactly as before.
 */

type WireNarration = { text: string; voice?: string };
type WireScene = Omit<Scene, "narration"> & { narration?: WireNarration };
export type WireStoryboard = Omit<Storyboard, "scenes" | "narration_voice"> & {
  scenes: WireScene[];
};

const VALID_STYLE_PACKS = new Set(["tech_dark", "neon_lab", "paper_light", "bold_red"]);
const HEX_COLOR = /^#[0-9a-fA-F]{6}$/;

function clampInt(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, Math.round(v)));
}

function clampNum(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v));
}

/** Clamp one layer's motion fields to the server contract. Only new-field
 * kinds/entries are touched; plain legacy layers return unchanged. */
function sanitizeLayer(l: Layer): Layer {
  let next = l;
  if (next.highlights) {
    const highlights = next.highlights
      .filter(
        (h) =>
          h &&
          typeof h.word === "string" &&
          h.word.trim() !== "" &&
          typeof h.color === "string" &&
          HEX_COLOR.test(h.color),
      )
      .slice(0, 6);
    next = { ...next, highlights: highlights.length > 0 ? highlights : undefined };
  }
  switch (next.kind) {
    case "counter":
      next = { ...next, decimals: clampInt(next.decimals ?? 0, 0, 2) };
      break;
    case "toggle_grid":
      next = {
        ...next,
        cols: clampInt(next.cols ?? 0, 0, 4),
        rows: clampInt(next.rows ?? 0, 0, 4),
        cadence: clampNum(next.cadence ?? 0, 0, 2),
      };
      break;
    case "compare_bars":
      next = {
        ...next,
        width_a: clampNum(next.width_a ?? 0, 0, 1),
        width_b: clampNum(next.width_b ?? 0, 0, 1),
      };
      break;
    case "stack": {
      next = { ...next, n: clampInt(next.n ?? 0, 0, 6) };
      if (next.labels) {
        const labels = next.labels.slice(0, 6);
        next = { ...next, labels: labels.length > 0 ? labels : undefined };
      }
      break;
    }
    case "stamp":
      next = { ...next, angle: clampNum(next.angle ?? -8, -30, 30) };
      break;
    default:
      break;
  }
  return next;
}

/** Strip an unknown style_pack (the server rejects the submit outright) and
 * clamp every layer's motion fields. Scenes without any of the new fields
 * return object-identical to before. */
function sanitizeScene(sc: Scene): Scene {
  let next = sc;
  if (next.style_pack !== undefined && !VALID_STYLE_PACKS.has(next.style_pack)) {
    next = { ...next, style_pack: undefined };
  }
  if (next.layers && next.layers.length > 0) {
    next = { ...next, layers: next.layers.map(sanitizeLayer) };
  }
  return next;
}

/** Editor scenes from any accepted shape: wire {text, voice} narration is
 * flattened to narration + narration_voice; strings pass through. */
export function normalizeEditorScenes(scenes: Scene[]): Scene[] {
  return scenes.map((sc) => {
    let next = sc;
    if (sc.narration !== undefined && typeof sc.narration !== "string") {
      const n = sc.narration as unknown as WireNarration;
      next = { ...sc, narration: n?.text || undefined };
      if (n?.voice) next.narration_voice = n.voice;
      else delete next.narration_voice;
    }
    return sanitizeScene(next);
  });
}

/** Wire storyboard for POST /v1/video/jobs (narration as {text, voice?}). */
export function toWireStoryboard(sb: Storyboard, defaultVoice?: string): WireStoryboard {
  const { narration_voice: _globalVoice, ...metaOnly } = sb;
  const fallbackVoice = defaultVoice || _globalVoice;
  return {
    ...metaOnly,
    scenes: sb.scenes.map((rawScene) => {
      const sc = sanitizeScene(rawScene);
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
