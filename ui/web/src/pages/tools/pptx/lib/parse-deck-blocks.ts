import type { AnimEffect, AnimSpec, DecorPrim, FrameVariant, Deck, Slide, SlideLayout } from "../types";
import { ICON_NAMES } from "./icon-library";

/**
 * Bridge between the pptx-designer agent's ```deck fenced blocks and the
 * PPTX Studio. Strict about the hard contract (version, theme shape, layout
 * fields) and forgiving about optional fields, mirroring
 * lib/parse-storyboard-blocks.ts from the video tool.
 */

export interface DeckBlock {
  json: string;
  start: number;
  end: number;
}

// A block only matches once its closing fence exists, so a half-streamed
// reply simply produces no blocks yet.
const FENCE_RE = /```deck[ \t]*\r?\n([\s\S]*?)```/g;

export function extractDeckBlocks(text: string | null | undefined): DeckBlock[] {
  if (!text) return [];
  const blocks: DeckBlock[] = [];
  FENCE_RE.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = FENCE_RE.exec(text)) !== null) {
    const json = m[1]?.trim();
    if (json) blocks.push({ json, start: m.index, end: m.index + m[0].length });
  }
  return blocks;
}

/** Content with complete deck fences removed (the card replaces them). */
export function stripDeckBlocks(text: string): string {
  if (!text) return text;
  return text.replace(FENCE_RE, "").trim();
}

export type ParsedDeck =
  | { ok: true; deck: Deck; raw: string }
  | { ok: false; error: string; raw: string };

const LAYOUTS = new Set<SlideLayout>([
  "title",
  "section",
  "bullets",
  "two_column",
  "quote",
  "stats",
  "image",
  "end",
]);
const HEX_RE = /^#[0-9a-fA-F]{6}$/;
const ANIM_EFFECTS = new Set<string>(["fade-in", "slide-up", "slide-left", "scale-in"]);
const FRAME_VARIANTS = new Set<string>(["corner", "outline", "band", "dots", "ring"]);

export function parseDeck(raw: string): ParsedDeck {
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return { ok: false, error: "invalid JSON", raw };
  }
  if (!data || typeof data !== "object" || Array.isArray(data)) {
    return { ok: false, error: "not an object", raw };
  }
  const d = data as Partial<Deck>;
  if (d.version !== 1) {
    return { ok: false, error: `unsupported version ${String(d.version)}`, raw };
  }
  const themeErr = themeError(d.theme);
  if (themeErr) return { ok: false, error: themeErr, raw };
  if (!Array.isArray(d.slides) || d.slides.length === 0) {
    return { ok: false, error: "slides must be a non-empty array", raw };
  }
  if (d.slides.length > 40) {
    return { ok: false, error: `too many slides (${d.slides.length}, max 40)`, raw };
  }
  for (let i = 0; i < d.slides.length; i++) {
    const err = slideError(d.slides[i], i);
    if (err) return { ok: false, error: err, raw };
  }
  // Optional decoration fields are forgiving: invalid entries are dropped,
  // never rejected, so one bad icon name can't fail a whole deck.
  const slides = d.slides.map((s) => sanitizeSlide(s as Slide));
  return { ok: true, deck: { ...(d as Deck), version: 1, slides }, raw };
}

function finiteNum(v: unknown): number | null {
  return typeof v === "number" && Number.isFinite(v) ? v : null;
}

function optColor(v: unknown): string | undefined {
  return typeof v === "string" && HEX_RE.test(v) ? v : undefined;
}

function sanitizeAnim(v: unknown): AnimSpec | undefined {
  if (!v || typeof v !== "object") return undefined;
  const a = v as Record<string, unknown>;
  if (typeof a.effect !== "string" || !ANIM_EFFECTS.has(a.effect)) return undefined;
  const delayMs = finiteNum(a.delayMs);
  return {
    effect: a.effect as AnimEffect,
    ...(delayMs !== null ? { delayMs: Math.max(0, Math.min(60000, Math.round(delayMs))) } : {}),
  };
}

function sanitizeDecor(decor: unknown): DecorPrim[] | undefined {
  if (!Array.isArray(decor)) return undefined;
  const out: DecorPrim[] = [];
  for (const item of decor.slice(0, 24)) {
    if (!item || typeof item !== "object") continue;
    const d = item as Record<string, unknown>;
    const x = finiteNum(d.x);
    const y = finiteNum(d.y);
    const w = finiteNum(d.w);
    const h = finiteNum(d.h);
    if (x === null || y === null || w === null || h === null) continue;
    const color = optColor(d.color);
    const anim = sanitizeAnim(d.anim);
    if (d.type === "icon") {
      if (typeof d.icon !== "string" || !ICON_NAMES.has(d.icon)) continue;
      const sw = finiteNum(d.strokeWidth);
      out.push({
        type: "icon",
        icon: d.icon,
        x, y, w, h,
        ...(color ? { color } : {}),
        ...(sw !== null && sw > 0 ? { strokeWidth: Math.min(10, sw) } : {}),
        ...(anim ? { anim } : {}),
      });
    } else if (d.type === "frame") {
      if (typeof d.variant !== "string" || !FRAME_VARIANTS.has(d.variant)) continue;
      const weight = finiteNum(d.weight);
      out.push({
        type: "frame",
        variant: d.variant as FrameVariant,
        x, y, w, h,
        ...(color ? { color } : {}),
        ...(weight !== null ? { weight: Math.max(1, Math.min(40, weight)) } : {}),
        ...(anim ? { anim } : {}),
      });
    }
  }
  return out.length > 0 ? out : undefined;
}

/** Strip optional decoration fields the studio can't render (unknown icon
 * names, malformed decor entries) while keeping the hard contract intact. */
function sanitizeSlide(s: Slide): Slide {
  const next: Slide = { ...s };
  if (next.icon !== undefined && (typeof next.icon !== "string" || !ICON_NAMES.has(next.icon))) {
    delete next.icon;
  }
  if (next.bullet_icons !== undefined) {
    if (!Array.isArray(next.bullet_icons)) {
      delete next.bullet_icons;
    } else {
      // Parallel to `bullets`; null marks "no icon for this bullet".
      const cleaned = next.bullet_icons.map((n) => (typeof n === "string" && ICON_NAMES.has(n) ? n : null));
      if (cleaned.some((n) => n !== null)) {
        next.bullet_icons = cleaned;
      } else {
        delete next.bullet_icons;
      }
    }
  }
  if (next.stats !== undefined && Array.isArray(next.stats)) {
    next.stats = next.stats.map((st) => {
      if (st && typeof st === "object" && st.icon !== undefined) {
        if (typeof st.icon !== "string" || !ICON_NAMES.has(st.icon)) {
          const { icon: _drop, ...rest } = st;
          return rest as typeof st;
        }
      }
      return st;
    });
  }
  const decor = sanitizeDecor(next.decor);
  if (decor) next.decor = decor;
  else delete next.decor;
  return next;
}

function themeError(theme: unknown): string | null {
  if (!theme || typeof theme !== "object") return "theme must be an object";
  const t = theme as Record<string, unknown>;
  for (const key of ["background", "foreground", "accent", "muted"]) {
    if (typeof t[key] !== "string" || !HEX_RE.test(t[key] as string)) {
      return `theme.${key} must be #RRGGBB`;
    }
  }
  for (const key of ["font_heading", "font_body"]) {
    if (typeof t[key] !== "string" || !(t[key] as string).trim()) {
      return `theme.${key} must be a font name`;
    }
  }
  return null;
}

function slideError(slide: unknown, index: number): string | null {
  if (!slide || typeof slide !== "object") {
    return `slide ${index + 1}: not an object`;
  }
  const s = slide as Partial<Slide>;
  if (!s.layout || !LAYOUTS.has(s.layout)) {
    return `slide ${index + 1}: unknown layout ${String(s.layout)}`;
  }
  switch (s.layout) {
    case "title":
    case "section":
    case "end":
      if (!String(s.title ?? "").trim()) {
        return `slide ${index + 1}: ${s.layout} slides need a title`;
      }
      break;
    case "bullets":
      if (!String(s.title ?? "").trim()) {
        return `slide ${index + 1}: bullets slides need a title`;
      }
      if (!Array.isArray(s.bullets) || s.bullets.length === 0) {
        return `slide ${index + 1}: bullets slides need a non-empty bullets array`;
      }
      break;
    case "two_column":
      if (!String(s.title ?? "").trim()) {
        return `slide ${index + 1}: two_column slides need a title`;
      }
      for (const side of ["left", "right"] as const) {
        const col = s[side];
        if (!col || typeof col !== "object" || !Array.isArray(col.bullets)) {
          return `slide ${index + 1}: two_column needs ${side} with a bullets array`;
        }
      }
      break;
    case "quote":
      if (!String(s.quote ?? "").trim()) {
        return `slide ${index + 1}: quote slides need a quote`;
      }
      break;
    case "stats":
      if (!Array.isArray(s.stats) || s.stats.length < 2 || s.stats.length > 4) {
        return `slide ${index + 1}: stats slides need 2 to 4 stats`;
      }
      break;
    case "image":
      if (!String(s.source ?? "").trim()) {
        return `slide ${index + 1}: image slides need a source`;
      }
      break;
  }
  return null;
}
