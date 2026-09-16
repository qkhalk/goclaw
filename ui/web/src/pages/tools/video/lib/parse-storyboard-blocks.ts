import type { Layer, Scene } from "../hooks/use-timeline";
import type { Storyboard } from "../video-tool-page";

/**
 * Bridge between the designer agent's ```storyboard fenced blocks and the
 * Video Editor. Parsing is intentionally forgiving about optional scene
 * fields (transition/fit/ken_burns are the editor's job) and strict about
 * the hard contract: version 1, non-empty scenes, valid types, durations in
 * range, source for image/video, #RRGGBB for color.
 */

export interface StoryboardBlock {
  json: string;
  start: number;
  end: number;
}

// A block only matches once its closing fence exists, so a half-streamed
// reply simply produces no blocks yet — no separate "closing" detection.
const FENCE_RE = /```storyboard[ \t]*\r?\n([\s\S]*?)```/g;

export function extractStoryboardBlocks(text: string | null | undefined): StoryboardBlock[] {
  if (!text) return [];
  const blocks: StoryboardBlock[] = [];
  FENCE_RE.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = FENCE_RE.exec(text)) !== null) {
    const json = m[1]?.trim();
    if (json) blocks.push({ json, start: m.index, end: m.index + m[0].length });
  }
  return blocks;
}

/** Content with complete storyboard fences removed (the card replaces them). */
export function stripStoryboardBlocks(text: string): string {
  if (!text) return text;
  return text.replace(FENCE_RE, "").trim();
}

export type ParsedStoryboard =
  | { ok: true; sb: Storyboard; raw: string }
  | { ok: false; error: string; raw: string };

const SCENE_TYPES = new Set(["image", "video", "color"]);
const HEX_RE = /^#[0-9a-fA-F]{6}$/;

export function parseStoryboard(raw: string): ParsedStoryboard {
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return { ok: false, error: "invalid JSON", raw };
  }
  if (!data || typeof data !== "object" || Array.isArray(data)) {
    return { ok: false, error: "not an object", raw };
  }
  const sb = data as Partial<Storyboard>;
  if (sb.version !== 1) {
    return { ok: false, error: `unsupported version ${String(sb.version)}`, raw };
  }
  if (!Array.isArray(sb.scenes) || sb.scenes.length === 0) {
    return { ok: false, error: "scenes must be a non-empty array", raw };
  }
  for (let i = 0; i < sb.scenes.length; i++) {
    const err = sceneError(sb.scenes[i], i);
    if (err) return { ok: false, error: err, raw };
  }
  return {
    ok: true,
    sb: { ...(sb as Storyboard), version: 1 },
    raw,
  };
}

function sceneError(scene: unknown, index: number): string | null {
  if (!scene || typeof scene !== "object") {
    return `scene ${index + 1}: not an object`;
  }
  const sc = scene as Partial<Scene>;
  if (!sc.type || !SCENE_TYPES.has(sc.type)) {
    return `scene ${index + 1}: unknown type ${String(sc.type)}`;
  }
  if (typeof sc.duration_sec !== "number" || !Number.isFinite(sc.duration_sec)) {
    return `scene ${index + 1}: duration_sec must be a number`;
  }
  if ((sc.type === "image" || sc.type === "video") && !String(sc.source ?? "").trim()) {
    return `scene ${index + 1}: ${sc.type} scenes need a source`;
  }
  if (sc.type === "color" && !(typeof sc.color === "string" && HEX_RE.test(sc.color))) {
    return `scene ${index + 1}: color scenes need #RRGGBB`;
  }
  if (sc.layers !== undefined) {
    if (!Array.isArray(sc.layers)) {
      return `scene ${index + 1}: layers must be an array`;
    }
    if (sc.layers.length > 8) {
      return `scene ${index + 1}: at most 8 layers`;
    }
    for (let j = 0; j < sc.layers.length; j++) {
      const err = layerError(sc.layers[j], index, j, sc.duration_sec);
      if (err) return err;
    }
  }
  return null;
}

const LAYER_KINDS = new Set(["text", "shape", "image"]);

function layerError(layer: unknown, sceneIdx: number, layerIdx: number, sceneSec: number): string | null {
  if (!layer || typeof layer !== "object") {
    return `scene ${sceneIdx + 1} layer ${layerIdx + 1}: not an object`;
  }
  const l = layer as Partial<Layer>;
  if (!l.kind || !LAYER_KINDS.has(l.kind)) {
    return `scene ${sceneIdx + 1} layer ${layerIdx + 1}: unknown kind ${String(l.kind)}`;
  }
  if (l.kind === "text" && !String(l.text ?? "").trim()) {
    return `scene ${sceneIdx + 1} layer ${layerIdx + 1}: text layers need text`;
  }
  if (l.kind === "shape" && !(typeof l.fill === "string" && HEX_RE.test(l.fill))) {
    return `scene ${sceneIdx + 1} layer ${layerIdx + 1}: shape layers need a #RRGGBB fill`;
  }
  if (l.kind === "image" && !String(l.source ?? "").trim()) {
    return `scene ${sceneIdx + 1} layer ${layerIdx + 1}: image layers need a source`;
  }
  const start = typeof l.start === "number" ? l.start : 0;
  if (start < 0 || start >= sceneSec) {
    return `scene ${sceneIdx + 1} layer ${layerIdx + 1}: start must be within the scene`;
  }
  if (l.duration !== undefined && l.duration < 0) {
    return `scene ${sceneIdx + 1} layer ${layerIdx + 1}: duration must be positive`;
  }
  return null;
}
