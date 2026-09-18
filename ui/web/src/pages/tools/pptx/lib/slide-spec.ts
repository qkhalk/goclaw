import type { AnimSpec, DecorPrim, DeckTheme, Slide } from "../types";
import { DEFAULT_ICON_STROKE_WIDTH, getIcon } from "./icon-library";

/**
 * Single source of truth for slide geometry. Every layout emits a flat list
 * of drawing primitives on the fixed 1280×720 stage; the HTML preview
 * (slide-view.tsx) and the .pptx export (pptx-export.ts) render the exact
 * same list, so what you see is what PowerPoint gets.
 *
 * No alpha anywhere: semi-transparent accents are pre-blended with the
 * slide background (blend()) because pptxgenjs text/shape transparency is
 * unevenly supported — a solid blended hex renders identically in both.
 */

export const STAGE_W = 1280;
export const STAGE_H = 720;

/** Side margin of the content grid. */
const M = 96;
const CONTENT_W = STAGE_W - M * 2; // 1088

export type SlidePrim =
  | {
      kind: "rect";
      x: number;
      y: number;
      w: number;
      h: number;
      fill: string;
      radius?: number;
      anim?: AnimSpec;
    }
  | { kind: "ellipse"; x: number; y: number; w: number; h: number; fill: string; anim?: AnimSpec }
  | {
      kind: "frame";
      x: number;
      y: number;
      w: number;
      h: number;
      color: string;
      width: number;
      radius?: number;
      dash?: boolean;
      anim?: AnimSpec;
    }
  | {
      kind: "icon";
      x: number;
      y: number;
      w: number;
      h: number;
      name: string;
      color: string;
      strokeWidth: number;
      anim?: AnimSpec;
    }
  | {
      kind: "text";
      x: number;
      y: number;
      w: number;
      h: number;
      text: string;
      font: "heading" | "body";
      size: number;
      bold?: boolean;
      italic?: boolean;
      color: string;
      align?: "left" | "center" | "right";
      valign?: "top" | "middle" | "bottom";
      lineHeight?: number;
    }
  | { kind: "image"; x: number; y: number; w: number; h: number; source: string; alt: string };

function hexToRgb(hex: string): [number, number, number] {
  const c = hex.replace("#", "");
  return [
    parseInt(c.slice(0, 2), 16) || 0,
    parseInt(c.slice(2, 4), 16) || 0,
    parseInt(c.slice(4, 6), 16) || 0,
  ];
}

function rgbToHex(r: number, g: number, b: number): string {
  const to = (v: number) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, "0");
  return `#${to(r)}${to(g)}${to(b)}`;
}

/** Blend `over` on top of `base` at strength t (0..1), returned as solid hex. */
export function blend(over: string, base: string, t: number): string {
  const [r1, g1, b1] = hexToRgb(over);
  const [r2, g2, b2] = hexToRgb(base);
  return rgbToHex(r2 + (r1 - r2) * t, g2 + (g1 - g2) * t, b2 + (b1 - b2) * t);
}

/** Rough line count for a text box, used to size boxes identically in
 * preview and export (export boxes don't auto-grow). */
function estLines(text: string, w: number, size: number): number {
  const perLine = Math.max(8, Math.floor(w / (size * 0.52)));
  let lines = 0;
  for (const seg of text.split("\n")) lines += Math.max(1, Math.ceil(seg.length / perLine));
  return lines;
}

function textH(text: string, w: number, size: number, lineHeight = 1.3): number {
  return estLines(text, w, size) * size * lineHeight + size * 0.35;
}

interface Palette {
  bg: string;
  fg: string;
  accent: string;
  muted: string;
  panel: string;
  hairline: string;
  softAccent: string;
  ghostAccent: string;
}

function palette(theme: DeckTheme): Palette {
  const bg = theme.background || "#0f172a";
  return {
    bg,
    fg: theme.foreground || "#f8fafc",
    accent: theme.accent || "#38bdf8",
    muted: theme.muted || "#94a3b8",
    panel: blend(theme.foreground || "#f8fafc", bg, 0.05),
    hairline: blend(theme.muted || "#94a3b8", bg, 0.28),
    softAccent: blend(theme.accent || "#38bdf8", bg, 0.12),
    ghostAccent: blend(theme.accent || "#38bdf8", bg, 0.22),
  };
}

/** Title + accent tick shared by content layouts. Returns the y where the
 * body area starts. */
function contentHeader(prims: SlidePrim[], slide: Slide, p: Palette, size = 46): number {
  const title = slide.title ?? "";
  const th = textH(title, CONTENT_W, size, 1.15);
  prims.push({
    kind: "text", x: M, y: 64, w: CONTENT_W, h: th, text: title,
    font: "heading", size, bold: true, color: p.fg, lineHeight: 1.15,
  });
  const tickY = 64 + th + 14;
  prims.push({ kind: "rect", x: M, y: tickY, w: 56, h: 6, fill: p.accent });
  return tickY + 6 + 34;
}

const HEX_RE = /^#[0-9a-fA-F]{6}$/;

/** Push one icon primitive; unknown icon names vanish silently so a deck
 * hand-edited with a bad name never breaks preview or export. */
function pushIcon(
  prims: SlidePrim[],
  name: string | null | undefined,
  x: number,
  y: number,
  w: number,
  h: number,
  color: string,
  strokeWidth = DEFAULT_ICON_STROKE_WIDTH,
  anim?: AnimSpec,
): void {
  if (!getIcon(name)) return;
  prims.push({ kind: "icon", x, y, w, h, name: name as string, color, strokeWidth, anim });
}

/** Expand one frame decor into base geometry prims (rect / ellipse / frame),
 * so preview and export share one path and the export needs no new shape
 * vocabulary beyond the icon raster. */
function expandFrame(d: Extract<DecorPrim, { type: "frame" }>, prims: SlidePrim[], p: Palette): void {
  const weight = Math.max(1, Math.min(40, Math.round(d.weight ?? (d.variant === "band" ? 10 : 2))));
  const fallback = d.variant === "band" ? p.accent : d.variant === "ring" ? p.ghostAccent : p.muted;
  const color = d.color && HEX_RE.test(d.color) ? d.color : fallback;
  const anim = d.anim;

  switch (d.variant) {
    case "corner": {
      // L-brackets at the four corners of the box.
      const arm = Math.max(14, Math.min(46, Math.round(Math.min(d.w, d.h) * 0.22)));
      const t = weight;
      const rects: Array<[number, number, number, number]> = [
        [d.x, d.y, arm, t], [d.x, d.y, t, arm], // top-left
        [d.x + d.w - arm, d.y, arm, t], [d.x + d.w - t, d.y, t, arm], // top-right
        [d.x, d.y + d.h - t, arm, t], [d.x, d.y + d.h - arm, t, arm], // bottom-left
        [d.x + d.w - arm, d.y + d.h - t, arm, t], [d.x + d.w - t, d.y + d.h - arm, t, arm], // bottom-right
      ];
      for (const [x, y, w, h] of rects) prims.push({ kind: "rect", x, y, w, h, fill: color, anim });
      break;
    }
    case "outline": {
      prims.push({ kind: "frame", x: d.x, y: d.y, w: d.w, h: d.h, color, width: weight, radius: 12, anim });
      break;
    }
    case "band": {
      // Accent side strip hugging the left edge of the box.
      prims.push({ kind: "rect", x: d.x, y: d.y, w: weight, h: d.h, fill: color, anim });
      break;
    }
    case "dots": {
      // Dot grid inset in the box; spacing grows to cap the prim count.
      const inset = 14;
      const dot = 5;
      const cols = Math.max(1, Math.floor((d.w - inset * 2) / 28) + 1);
      const rows = Math.max(1, Math.floor((d.h - inset * 2) / 28) + 1);
      const stepX = cols > 1 ? (d.w - inset * 2) / (cols - 1) : 0;
      const stepY = rows > 1 ? (d.h - inset * 2) / (rows - 1) : 0;
      const maxDots = 160;
      const total = cols * rows;
      const colStride = total <= maxDots ? 1 : 2;
      for (let r = 0; r < rows; r += colStride) {
        for (let c = 0; c < cols; c += colStride) {
          prims.push({
            kind: "ellipse",
            x: d.x + inset + c * stepX - dot / 2,
            y: d.y + inset + r * stepY - dot / 2,
            w: dot,
            h: dot,
            fill: color,
            anim,
          });
        }
      }
      break;
    }
    case "ring": {
      // Ghost circle: a frame whose radius rounds the rect into a circle.
      prims.push({
        kind: "frame",
        x: d.x,
        y: d.y,
        w: d.w,
        h: d.h,
        color,
        width: weight,
        radius: Math.min(d.w, d.h) / 2,
        anim,
      });
      break;
    }
  }
}

/** Layer the slide's decor primitives (icons + frames) on top of the layout. */
function appendDecor(prims: SlidePrim[], slide: Slide, p: Palette): void {
  for (const d of slide.decor ?? []) {
    if (d.type === "frame") {
      expandFrame(d, prims, p);
    } else if (d.type === "icon") {
      const color = d.color && HEX_RE.test(d.color) ? d.color : p.accent;
      pushIcon(
        prims, d.icon, d.x, d.y, d.w, d.h, color,
        d.strokeWidth && d.strokeWidth > 0 ? d.strokeWidth : DEFAULT_ICON_STROKE_WIDTH,
        d.anim,
      );
    }
  }
}

export function buildSlidePrims(slide: Slide, theme: DeckTheme): SlidePrim[] {
  const p = palette(theme);
  const prims: SlidePrim[] = [];

  switch (slide.layout) {
    case "title": {
      // Ghost ring bleeding off the top-right corner + soft accent pool
      // bottom-right: quiet geometry that frames the type instead of
      // decorating it.
      prims.push({
        kind: "ellipse", x: 940, y: -210, w: 500, h: 500, fill: p.ghostAccent,
      });
      prims.push({
        kind: "ellipse", x: 1042, y: -108, w: 296, h: 296, fill: p.bg,
      });
      prims.push({
        kind: "ellipse", x: 1005, y: 462, w: 330, h: 330, fill: p.softAccent,
      });
      prims.push({ kind: "rect", x: M, y: 236, w: 88, h: 10, fill: p.accent });
      // Icon chip + corner brackets: only when the slide carries an icon, so
      // decks without the new primitives render exactly as before.
      const chip = getIcon(slide.icon);
      if (chip) {
        prims.push({ kind: "rect", x: M, y: 148, w: 64, h: 64, fill: p.softAccent, radius: 16 });
        pushIcon(prims, slide.icon, M + 15, 163, 34, 34, p.accent);
        const t = 3;
        const arm = 30;
        const brackets: Array<[number, number, number, number]> = [
          [28, 28, arm, t], [28, 28, t, arm],
          [STAGE_W - 28 - arm, 28, arm, t], [STAGE_W - 28 - t, 28, t, arm],
          [28, STAGE_H - 28 - t, arm, t], [28, STAGE_H - 28 - arm, t, arm],
          [STAGE_W - 28 - arm, STAGE_H - 28 - t, arm, t], [STAGE_W - 28 - t, STAGE_H - 28 - arm, t, arm],
        ];
        for (const [x, y, w, h] of brackets) prims.push({ kind: "rect", x, y, w, h, fill: p.muted });
      }
      const titleH = textH(slide.title ?? "", 1088, 76, 1.12);
      prims.push({
        kind: "text", x: M, y: 274, w: 1088, h: titleH, text: slide.title ?? "",
        font: "heading", size: 76, bold: true, color: p.fg, lineHeight: 1.12,
      });
      if (slide.subtitle) {
        prims.push({
          kind: "text", x: M, y: 274 + titleH + 26, w: 920,
          h: textH(slide.subtitle, 920, 30, 1.4), text: slide.subtitle,
          font: "body", size: 30, color: p.muted, lineHeight: 1.4,
        });
      }
      break;
    }

    case "section": {
      // Full-height accent edge strip: a chapter marker, not decoration.
      prims.push({ kind: "rect", x: 0, y: 0, w: 12, h: STAGE_H, fill: p.accent });
      prims.push({
        kind: "ellipse", x: 950, y: 150, w: 380, h: 380, fill: p.softAccent,
      });
      prims.push({ kind: "rect", x: M, y: 300, w: 120, h: 10, fill: p.accent });
      // Big leading icon between the band and the title (icon slides only).
      if (getIcon(slide.icon)) {
        pushIcon(prims, slide.icon, M, 186, 84, 84, p.accent, 1.75);
      }
      const titleH = textH(slide.title ?? "", 1000, 62, 1.12);
      prims.push({
        kind: "text", x: M, y: 338, w: 1000, h: titleH, text: slide.title ?? "",
        font: "heading", size: 62, bold: true, color: p.fg, lineHeight: 1.12,
      });
      if (slide.subtitle) {
        prims.push({
          kind: "text", x: M, y: 338 + titleH + 24, w: 880,
          h: textH(slide.subtitle, 880, 28, 1.4), text: slide.subtitle,
          font: "body", size: 28, color: p.muted, lineHeight: 1.4,
        });
      }
      break;
    }

    case "bullets": {
      const bodyTop = contentHeader(prims, slide, p);
      // Editorial hairline anchoring the right edge.
      prims.push({ kind: "rect", x: 1182, y: 64, w: 2, h: STAGE_H - 128, fill: p.hairline });
      const items = slide.bullets ?? [];
      const icons = slide.bullet_icons ?? [];
      const size = items.length > 5 ? 28 : 31;
      const gap = items.length > 5 ? 22 : 30;
      let y = bodyTop;
      items.forEach((b, i) => {
        const icon = getIcon(icons[i]);
        if (icon) {
          // Leading icon replaces the square marker; text column unchanged.
          pushIcon(prims, icons[i], M, y + size * 0.66 - 14, 28, 28, p.accent, 2);
        } else {
          prims.push({ kind: "rect", x: M, y: y + size * 0.42, w: 12, h: 12, fill: p.accent });
        }
        const h = textH(b, CONTENT_W - 44, size, 1.32);
        prims.push({
          kind: "text", x: M + 44, y, w: CONTENT_W - 44, h, text: b,
          font: "body", size, color: p.fg, lineHeight: 1.32,
        });
        y += Math.max(h, size * 1.32) + gap;
      });
      break;
    }

    case "two_column": {
      const bodyTop = contentHeader(prims, slide, p);
      const panelY = bodyTop + 6;
      const panelH = STAGE_H - 72 - panelY;
      const panelW = 524;
      const cols: Array<{ heading?: string; bullets?: string[] } | undefined> = [slide.left, slide.right];
      cols.forEach((col, i) => {
        const x = i === 0 ? M : M + panelW + 40;
        prims.push({ kind: "rect", x, y: panelY, w: panelW, h: panelH, fill: p.panel, radius: 14 });
        let cy = panelY + 40;
        if (col?.heading) {
          prims.push({ kind: "rect", x: x + 36, y: cy + 15, w: 22, h: 5, fill: p.accent });
          const hh = textH(col.heading, panelW - 120, 30, 1.2);
          prims.push({
            kind: "text", x: x + 72, y: cy, w: panelW - 120, h: hh, text: col.heading,
            font: "heading", size: 30, bold: true, color: p.fg, lineHeight: 1.2,
          });
          cy += Math.max(hh, 36) + 26;
        }
        for (const b of col?.bullets ?? []) {
          prims.push({ kind: "ellipse", x: x + 38, y: cy + 14, w: 8, h: 8, fill: p.muted });
          const h = textH(b, panelW - 118, 26, 1.35);
          prims.push({
            kind: "text", x: x + 66, y: cy, w: panelW - 118, h, text: b,
            font: "body", size: 26, color: p.fg, lineHeight: 1.35,
          });
          cy += Math.max(h, 35) + 18;
        }
      });
      break;
    }

    case "quote": {
      prims.push({
        kind: "text", x: 116, y: 40, w: 300, h: 300, text: "\u201C",
        font: "heading", size: 260, bold: true, color: p.ghostAccent, lineHeight: 1,
      });
      prims.push({ kind: "rect", x: 210, y: 214, w: 4, h: 292, fill: p.hairline });
      const qH = textH(slide.quote ?? "", 820, 40, 1.42);
      prims.push({
        kind: "text", x: 244, y: 214, w: 820, h: qH, text: slide.quote ?? "",
        font: "heading", size: 40, italic: true, color: p.fg, lineHeight: 1.42,
      });
      if (slide.author) {
        prims.push({
          kind: "text", x: 244, y: 552, w: 820, h: 44,
          text: `\u2014 ${slide.author}`,
          font: "body", size: 25, color: p.muted, align: "right",
        });
      }
      break;
    }

    case "stats": {
      const bodyTop = contentHeader(prims, slide, p);
      const stats = slide.stats ?? [];
      const n = Math.max(1, Math.min(4, stats.length));
      const slot = CONTENT_W / n;
      const valueSize = n >= 4 ? 66 : n === 3 ? 76 : 84;
      // Icon-per-stat slides shift the value block down uniformly so the
      // stat row stays on one baseline; iconless decks keep the old geometry.
      const iconShift = stats.some((st) => getIcon(st.icon)) ? 58 : 0;
      stats.slice(0, 4).forEach((st, i) => {
        const x = M + i * slot;
        if (i > 0) {
          prims.push({ kind: "rect", x: x - 20, y: bodyTop + 16, w: 2, h: 240 + iconShift, fill: p.hairline });
        }
        prims.push({ kind: "rect", x, y: bodyTop, w: slot - 44, h: 4, fill: p.accent });
        if (iconShift > 0) {
          pushIcon(prims, st.icon, x, bodyTop + 18, 42, 42, p.accent, 1.75);
        }
        prims.push({
          kind: "text", x, y: bodyTop + 30 + iconShift, w: slot - 44, h: valueSize * 1.1,
          text: st.value, font: "heading", size: valueSize, bold: true,
          color: p.accent, lineHeight: 1.05,
        });
        prims.push({
          kind: "text", x, y: bodyTop + 30 + iconShift + valueSize * 1.1 + 18, w: slot - 60,
          h: textH(st.label, slot - 60, 25, 1.35), text: st.label,
          font: "body", size: 25, color: p.muted, lineHeight: 1.35,
        });
      });
      break;
    }

    case "image": {
      const bodyTop = contentHeader(prims, slide, p, 44);
      const frameY = bodyTop + 4;
      const frameH = STAGE_H - 96 - frameY - (slide.caption ? 64 : 48);
      prims.push({
        kind: "frame", x: 240, y: frameY, w: 800, h: frameH,
        color: p.hairline, width: 2, radius: 10,
      });
      if (slide.source) {
        prims.push({
          kind: "image", x: 252, y: frameY + 12, w: 776, h: frameH - 24,
          source: slide.source, alt: slide.caption ?? slide.title ?? "",
        });
      } else {
        prims.push({
          kind: "frame", x: 252, y: frameY + 12, w: 776, h: frameH - 24,
          color: p.hairline, width: 2, radius: 8, dash: true,
        });
        prims.push({
          kind: "text", x: 352, y: frameY + frameH / 2 - 24, w: 576, h: 48,
          text: slide.caption || "image", font: "body", size: 26,
          color: p.muted, align: "center", valign: "middle",
        });
      }
      if (slide.caption && slide.source) {
        prims.push({
          kind: "text", x: 200, y: frameY + frameH + 18, w: 880, h: 40,
          text: slide.caption, font: "body", size: 23, color: p.muted, align: "center",
        });
      }
      break;
    }

    case "end": {
      const cx = STAGE_W / 2;
      prims.push({ kind: "ellipse", x: cx - 34, y: 236, w: 16, h: 16, fill: p.accent });
      prims.push({ kind: "ellipse", x: cx - 8, y: 236, w: 16, h: 16, fill: p.ghostAccent });
      prims.push({ kind: "ellipse", x: cx + 18, y: 236, w: 16, h: 16, fill: p.hairline });
      const titleH = textH(slide.title ?? "", 960, 62, 1.15);
      prims.push({
        kind: "text", x: cx - 480, y: 292, w: 960, h: titleH, text: slide.title ?? "",
        font: "heading", size: 62, bold: true, color: p.fg, align: "center", lineHeight: 1.15,
      });
      if (slide.subtitle) {
        prims.push({
          kind: "text", x: cx - 420, y: 292 + titleH + 22, w: 840,
          h: textH(slide.subtitle, 840, 27, 1.4), text: slide.subtitle,
          font: "body", size: 27, color: p.muted, align: "center", lineHeight: 1.4,
        });
      }
      prims.push({ kind: "rect", x: cx - 80, y: 560, w: 160, h: 2, fill: p.hairline });
      break;
    }
  }

  // Decor primitives (icons + frames) layer on top of every layout.
  appendDecor(prims, slide, p);

  return prims;
}
