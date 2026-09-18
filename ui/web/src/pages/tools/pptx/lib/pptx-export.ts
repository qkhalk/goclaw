import PptxGenJS from "pptxgenjs";
import type { Deck, DeckTheme } from "../types";
import { buildSlidePrims, type SlidePrim } from "./slide-spec";
import { iconSvgString } from "./icon-library";

/**
 * Client-side deck → .pptx export (the studio's "heavy processing stays in
 * the browser" rule). The slide geometry comes from the same primitive list
 * the HTML preview renders (lib/slide-spec.ts), so preview and export are
 * pixel-identical by construction. Primitives map to native PowerPoint text
 * boxes and shapes — the exported file stays fully editable.
 *
 * Canvas: LAYOUT_WIDE 13.33×7.5in at 96dpi-equivalent → 1280×720px.
 * px → in = px / 96 (both axes); font px → pt = px × 72/96 = px × 0.75.
 *
 * Icon primitives are vector line art in the preview; PowerPoint has no
 * native SVG-path shape, so they are rasterized to PNG data URLs client-side
 * (string → Image → canvas at 3× → dataURL) and placed with addImage. Rasters
 * are cached per (icon, color, strokeWidth, size). `anim` is preview-only and
 * skipped silently here.
 */

const IN_W = 13.33;
const IN_H = 7.5;

const fontPx2pt = (px: number) => Math.round(px * 0.75 * 10) / 10;

function hex(color: string | undefined, fallback: string): string {
  const c = (color ?? "").replace("#", "");
  return /^[0-9a-fA-F]{6}$/.test(c) ? c : fallback.replace("#", "");
}

/**
 * Raster cache keyed by (icon, color, strokeWidth, w×h at 1×). The encoded
 * SVG is drawn onto a canvas at 3× the stage pixel size so the exported PNG
 * stays crisp when PowerPoint scales it. `null` results (rasterization
 * unavailable, e.g. no DOM) are cached too so failures cost one attempt.
 */
const iconRasterCache = new Map<string, string | null>();

async function rasterizeIcon(
  name: string,
  color: string,
  strokeWidth: number,
  wPx: number,
  hPx: number,
): Promise<string | null> {
  const key = `${name}|${color}|${strokeWidth}|${Math.round(wPx)}x${Math.round(hPx)}`;
  const hit = iconRasterCache.get(key);
  if (hit !== undefined) return hit;

  let out: string | null = null;
  const svg = iconSvgString(name, color, strokeWidth);
  const scale = 3;
  if (svg && wPx > 0 && hPx > 0 && typeof document !== "undefined") {
    try {
      const img = new Image();
      await new Promise<void>((resolve, reject) => {
        img.onload = () => resolve();
        img.onerror = () => reject(new Error("icon svg decode failed"));
        img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
      });
      const canvas = document.createElement("canvas");
      canvas.width = Math.max(1, Math.round(wPx * scale));
      canvas.height = Math.max(1, Math.round(hPx * scale));
      const ctx = canvas.getContext("2d");
      if (ctx) {
        ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
        out = canvas.toDataURL("image/png");
      }
    } catch {
      out = null; // degrade silently — a missing decoration never blocks export
    }
  }
  iconRasterCache.set(key, out);
  return out;
}

export async function exportDeckPptx(deck: Deck, fileName: string): Promise<void> {
  const pptx = new PptxGenJS();
  pptx.layout = "LAYOUT_WIDE";
  pptx.author = "GoClaw PPTX Studio";

  const th = deck.theme;
  const bg = hex(th.background, "#0f172a");
  const head = th.font_heading || "Arial";
  const body = th.font_body || "Calibri";

  for (const slide of deck.slides) {
    const s = pptx.addSlide();
    s.background = { color: bg };

    for (const prim of buildSlidePrims(slide, th)) {
      await drawPrim(s, prim, th, head, body);
    }

    if (slide.notes) s.addNotes(slide.notes);
  }

  await pptx.writeFile({ fileName });
}

async function drawPrim(
  s: PptxGenJS.Slide,
  prim: SlidePrim,
  theme: DeckTheme,
  head: string,
  body: string,
): Promise<void> {
  const X = (v: number) => (v / 1280) * IN_W;
  const Y = (v: number) => (v / 720) * IN_H;

  switch (prim.kind) {
    case "rect":
    case "ellipse": {
      s.addShape(prim.kind === "rect" ? "rect" : "ellipse", {
        x: X(prim.x),
        y: Y(prim.y),
        w: X(prim.w),
        h: Y(prim.h),
        fill: { color: hex(prim.fill, "#000000") },
        ...(prim.kind === "rect" && prim.radius
          ? { rectRadius: Math.min(prim.radius / 96, Math.min(X(prim.w), Y(prim.h)) / 2) }
          : {}),
      });
      break;
    }

    case "frame": {
      s.addShape("rect", {
        x: X(prim.x),
        y: Y(prim.y),
        w: X(prim.w),
        h: Y(prim.h),
        fill: { color: hex(theme.background || "#0f172a", "#0f172a"), transparency: 100 },
        line: {
          color: hex(prim.color, "#94a3b8"),
          width: Math.max(0.5, prim.width * 0.75),
          ...(prim.dash ? { dashType: "dash" } : {}),
        },
      });
      break;
    }

    case "text": {
      s.addText(prim.text, {
        x: X(prim.x),
        y: Y(prim.y),
        w: X(prim.w),
        h: Y(prim.h),
        fontFace: prim.font === "heading" ? head : body,
        fontSize: fontPx2pt(prim.size),
        bold: prim.bold,
        italic: prim.italic,
        color: hex(prim.color, "#f8fafc"),
        align: prim.align ?? "left",
        valign: prim.valign ?? "top",
        lineSpacingMultiple: prim.lineHeight ?? 1.3,
        margin: 0,
        fit: "shrink",
      });
      break;
    }

    case "icon": {
      // Rasterize the vector icon (preview-only `anim` is skipped silently).
      const dataUrl = await rasterizeIcon(prim.name, prim.color, prim.strokeWidth, prim.w, prim.h);
      if (dataUrl) {
        s.addImage({
          data: dataUrl,
          x: X(prim.x),
          y: Y(prim.y),
          w: X(prim.w),
          h: Y(prim.h),
        });
      }
      break;
    }

    case "image": {
      const dataUrl = await fetchImageDataUrl(prim.source);
      if (dataUrl) {
        s.addImage({
          data: dataUrl,
          x: X(prim.x),
          y: Y(prim.y),
          w: X(prim.w),
          h: Y(prim.h),
          sizing: { type: "contain", w: X(prim.w), h: Y(prim.h) },
        });
      } else {
        // Degrade exactly like the preview's empty-image placeholder.
        s.addShape("rect", {
          x: X(prim.x),
          y: Y(prim.y),
          w: X(prim.w),
          h: Y(prim.h),
          fill: { color: hex(theme.background || "#0f172a", "#0f172a"), transparency: 100 },
          line: { color: hex(theme.muted || "#94a3b8", "#94a3b8"), width: 1, dashType: "dash" },
        });
      }
      break;
    }
  }
}

/** Fetch an image source (URL or same-origin workspace path) as a data URL. */
async function fetchImageDataUrl(source: string): Promise<string | null> {
  if (!source.trim()) return null;
  try {
    const res = await fetch(source);
    if (!res.ok) return null;
    const blob = await res.blob();
    if (!blob.type.startsWith("image/")) return null;
    return await new Promise<string | null>((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : null);
      reader.onerror = () => resolve(null);
      reader.readAsDataURL(blob);
    });
  } catch {
    // Cross-origin blocks or dead URLs degrade to the dashed placeholder.
    return null;
  }
}
