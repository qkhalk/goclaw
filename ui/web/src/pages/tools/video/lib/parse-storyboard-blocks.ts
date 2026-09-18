import type { Scene } from "../hooks/use-timeline";
import type { SceneTransition } from "../components/scene-transition";
import type { Storyboard } from "../hooks/use-video-export";
import { ICON_NAMES } from "./icon-library";

/**
 * Bridge between the video-designer agent's ```storyboard fenced blocks and
 * the video editor timeline. Strict about the hard contract (scene shapes,
 * durations, whitelists) and forgiving about optional meta (canvas/audio/
 * output get editor defaults when missing), mirroring
 * ../pptx/lib/parse-deck-blocks.ts.
 *
 * The video-designer agent's system prompt (server side) instructs replies
 * to carry one ```storyboard JSON``` block with this shape (all fields
 * snake_case, matching the /v1/video storyboard contract):
 *
 * {
 *   "version": 1,
 *   "canvas": { "width": 1080, "height": 1920, "fps": 30 },   // optional
 *   "audio":  { "bgm_path": "audio/bgm.mp3", "bgm_volume": 0.2 }, // optional
 *   "output": { "height": 720 },                              // optional
 *   "scenes": [{
 *     "type": "image" | "video" | "color" | "icon",
 *     "source": "https://... or file path",      // image | video, required
 *     "color": "#0F172A",                        // color | icon background
 *     "gradient": { "from": "#0F172A", "to": "#334155" }, // optional, browser-only
 *     "icon": { "name": "zap", "color": "#7DD3FC" },      // icon, name from library
 *     "duration_sec": 4,
 *     "fit": "cover" | "contain",                // image | video
 *     "mute": true,                              // video
 *     "ken_burns": { "zoom_from": 1, "zoom_to": 1.12, "pan": "none" },
 *     "caption": { "text": "…", "position": "top" | "center" | "bottom", "font_size": 48 },
 *     "narration": "TTS text",
 *     "transition": "none" | "fade" | "crossfade" | "slide_left" | "slide_up",
 *     "transform": { "scale": 1, "x": 0, "y": 0, "rotate": 0, "opacity": 1 },
 *     "filter": { "brightness": 1, "contrast": 1, "saturate": 1, "blur": 0 }
 *   }]
 * }
 */

export interface StoryboardBlock {
  json: string;
  start: number;
  end: number;
}

// A block only matches once its closing fence exists, so a half-streamed
// reply simply produces no blocks yet.
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
  | { ok: true; storyboard: Storyboard; raw: string }
  | { ok: false; error: string; raw: string };

const TRANSITIONS: ReadonlySet<string> = new Set<SceneTransition>([
  "none",
  "fade",
  "crossfade",
  "slide_left",
  "slide_up",
]);
const PANS: ReadonlySet<string> = new Set(["none", "left", "right", "up", "down"]);
const CAPTION_POSITIONS: ReadonlySet<string> = new Set(["top", "center", "bottom"]);
const HEX_RE = /^#[0-9a-fA-F]{6}$/;

const MAX_SCENES = 60;
const MAX_SCENE_SEC = 30;
const MIN_SCENE_SEC = 1;

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
  const d = data as Partial<Storyboard>;
  if (d.version !== undefined && d.version !== 1) {
    return { ok: false, error: `unsupported version ${String(d.version)}`, raw };
  }

  const canvasErr = canvasError(d.canvas);
  if (canvasErr) return { ok: false, error: canvasErr, raw };
  const metaErr = audioOutputError(d);
  if (metaErr) return { ok: false, error: metaErr, raw };

  if (!Array.isArray(d.scenes) || d.scenes.length === 0) {
    return { ok: false, error: "scenes must be a non-empty array", raw };
  }
  if (d.scenes.length > MAX_SCENES) {
    return {
      ok: false,
      error: `too many scenes (${d.scenes.length}, max ${MAX_SCENES})`,
      raw,
    };
  }
  for (let i = 0; i < d.scenes.length; i++) {
    const err = sceneError(d.scenes[i], i);
    if (err) return { ok: false, error: err, raw };
  }

  return {
    ok: true,
    storyboard: {
      version: 1,
      canvas: d.canvas ?? { width: 1080, height: 1920, fps: 30 },
      scenes: d.scenes as Scene[],
      audio: d.audio,
      output: d.output,
    },
    raw,
  };
}

function isInt(v: unknown, min: number, max: number): v is number {
  return typeof v === "number" && Number.isInteger(v) && v >= min && v <= max;
}

function isNum(v: unknown, min: number, max: number): v is number {
  return typeof v === "number" && Number.isFinite(v) && v >= min && v <= max;
}

function isHexColor(v: unknown): v is string {
  return typeof v === "string" && HEX_RE.test(v);
}

function canvasError(canvas: unknown): string | null {
  if (canvas === undefined) return null;
  if (!canvas || typeof canvas !== "object") return "canvas must be an object";
  const c = canvas as { width?: unknown; height?: unknown; fps?: unknown };
  if (!isInt(c.width, 64, 1920)) return "canvas.width must be an integer 64..1920";
  if (!isInt(c.height, 64, 1920)) return "canvas.height must be an integer 64..1920";
  if (!isInt(c.fps, 1, 60)) return "canvas.fps must be an integer 1..60";
  return null;
}

function audioOutputError(d: Partial<Storyboard>): string | null {
  if (d.audio !== undefined) {
    if (!d.audio || typeof d.audio !== "object") return "audio must be an object";
    if (d.audio.bgm_path !== undefined && typeof d.audio.bgm_path !== "string") {
      return "audio.bgm_path must be a string";
    }
    if (d.audio.bgm_volume !== undefined && !isNum(d.audio.bgm_volume, 0, 1)) {
      return "audio.bgm_volume must be a number 0..1";
    }
  }
  if (d.output !== undefined) {
    if (!d.output || typeof d.output !== "object") return "output must be an object";
    if (d.output.height !== undefined && d.output.height !== 480 && d.output.height !== 720 && d.output.height !== 1080) {
      return "output.height must be 480, 720 or 1080";
    }
  }
  return null;
}

function sceneError(scene: unknown, index: number): string | null {
  if (!scene || typeof scene !== "object") {
    return `scene ${index + 1}: not an object`;
  }
  const s = scene as Partial<Scene> & { gradient?: unknown; icon?: unknown };

  if (s.type !== "image" && s.type !== "video" && s.type !== "color" && s.type !== "icon") {
    return `scene ${index + 1}: unknown type ${String(s.type)} (image, video, color, icon)`;
  }
  if (!isNum(s.duration_sec, MIN_SCENE_SEC, MAX_SCENE_SEC)) {
    return `scene ${index + 1}: duration_sec must be a number ${MIN_SCENE_SEC}..${MAX_SCENE_SEC}`;
  }

  switch (s.type) {
    case "image":
    case "video":
      if (typeof s.source !== "string" || !s.source.trim()) {
        return `scene ${index + 1}: ${s.type} scenes need a source`;
      }
      if (s.fit !== undefined && s.fit !== "cover" && s.fit !== "contain") {
        return `scene ${index + 1}: fit must be "cover" or "contain"`;
      }
      break;
    case "color":
      if (!isHexColor(s.color)) {
        return `scene ${index + 1}: color scenes need a #RRGGBB color`;
      }
      break;
    case "icon":
      if (s.color !== undefined && !isHexColor(s.color)) {
        return `scene ${index + 1}: color must be #RRGGBB`;
      }
      break;
  }

  if (s.type === "icon") {
    if (!s.icon || typeof s.icon !== "object") {
      return `scene ${index + 1}: icon scenes need an icon object`;
    }
    const icon = s.icon as { name?: unknown; color?: unknown };
    if (typeof icon.name !== "string" || !ICON_NAMES.has(icon.name)) {
      return `scene ${index + 1}: unknown icon name ${String(icon.name)}`;
    }
    if (icon.color !== undefined && !isHexColor(icon.color)) {
      return `scene ${index + 1}: icon.color must be #RRGGBB`;
    }
  }

  if (s.gradient !== undefined) {
    if (!s.gradient || typeof s.gradient !== "object") {
      return `scene ${index + 1}: gradient must be an object`;
    }
    const g = s.gradient as { from?: unknown; to?: unknown };
    if (!isHexColor(g.from) || !isHexColor(g.to)) {
      return `scene ${index + 1}: gradient.from/to must be #RRGGBB`;
    }
  }

  if (s.caption !== undefined) {
    if (!s.caption || typeof s.caption !== "object") {
      return `scene ${index + 1}: caption must be an object`;
    }
    if (typeof s.caption.text !== "string" || !s.caption.text.trim()) {
      return `scene ${index + 1}: caption.text must be a non-empty string`;
    }
    if (s.caption.position !== undefined && !CAPTION_POSITIONS.has(s.caption.position)) {
      return `scene ${index + 1}: caption.position must be top, center or bottom`;
    }
    if (s.caption.font_size !== undefined && !isNum(s.caption.font_size, 8, 512)) {
      return `scene ${index + 1}: caption.font_size must be a number 8..512`;
    }
  }

  if (s.transition !== undefined && !TRANSITIONS.has(s.transition)) {
    return `scene ${index + 1}: unknown transition ${String(s.transition)}`;
  }

  if (s.ken_burns !== undefined) {
    if (!s.ken_burns || typeof s.ken_burns !== "object") {
      return `scene ${index + 1}: ken_burns must be an object`;
    }
    const kb = s.ken_burns;
    if (!isNum(kb.zoom_from, 0.5, 3)) return `scene ${index + 1}: ken_burns.zoom_from must be 0.5..3`;
    if (!isNum(kb.zoom_to, 0.5, 3)) return `scene ${index + 1}: ken_burns.zoom_to must be 0.5..3`;
    if (kb.pan !== undefined && !PANS.has(kb.pan)) {
      return `scene ${index + 1}: ken_burns.pan must be none, left, right, up or down`;
    }
  }

  if (s.transform !== undefined) {
    if (!s.transform || typeof s.transform !== "object") {
      return `scene ${index + 1}: transform must be an object`;
    }
    const t = s.transform;
    if (t.scale !== undefined && !isNum(t.scale, 0.1, 5)) {
      return `scene ${index + 1}: transform.scale must be 0.1..5`;
    }
    if (t.x !== undefined && !isNum(t.x, -100, 100)) {
      return `scene ${index + 1}: transform.x must be -100..100`;
    }
    if (t.y !== undefined && !isNum(t.y, -100, 100)) {
      return `scene ${index + 1}: transform.y must be -100..100`;
    }
    if (t.rotate !== undefined && !isNum(t.rotate, -360, 360)) {
      return `scene ${index + 1}: transform.rotate must be -360..360`;
    }
    if (t.opacity !== undefined && !isNum(t.opacity, 0, 1)) {
      return `scene ${index + 1}: transform.opacity must be 0..1`;
    }
  }

  if (s.filter !== undefined) {
    if (!s.filter || typeof s.filter !== "object") {
      return `scene ${index + 1}: filter must be an object`;
    }
    const f = s.filter;
    if (f.brightness !== undefined && !isNum(f.brightness, 0, 5)) {
      return `scene ${index + 1}: filter.brightness must be 0..5`;
    }
    if (f.contrast !== undefined && !isNum(f.contrast, 0, 5)) {
      return `scene ${index + 1}: filter.contrast must be 0..5`;
    }
    if (f.saturate !== undefined && !isNum(f.saturate, 0, 5)) {
      return `scene ${index + 1}: filter.saturate must be 0..5`;
    }
    if (f.blur !== undefined && !isNum(f.blur, 0, 50)) {
      return `scene ${index + 1}: filter.blur must be 0..50`;
    }
  }

  return null;
}
