import type { Scene } from "../hooks/use-timeline";
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
  return null;
}
