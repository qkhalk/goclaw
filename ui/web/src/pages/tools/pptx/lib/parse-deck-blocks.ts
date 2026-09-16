import type { Deck, Slide, SlideLayout } from "../types";

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
  return { ok: true, deck: { ...(d as Deck), version: 1 }, raw };
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
