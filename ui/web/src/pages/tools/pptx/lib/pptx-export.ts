import PptxGenJS from "pptxgenjs";
import JSZip from "jszip";
import type { Deck, DeckTheme, SlideElement } from "../types";
import type { SlidePrim } from "./slide-spec";
import { elementsOf } from "./elements";

/**
 * Client-side deck → .pptx export (the studio's "heavy processing stays in
 * the browser" rule). Slide geometry comes from the same element list the
 * HTML preview renders (explicit elements, or the compiled layout prims from
 * lib/slide-spec.ts), so preview and export are pixel-identical by
 * construction. Primitives map to native PowerPoint text boxes and shapes —
 * the exported file stays fully editable.
 *
 * Slide transitions are not exposed by pptxgenjs; after the file is written
 * to a buffer we reopen it with JSZip and inject `<p:transition>` XML into
 * each slide part — plain OOXML that PowerPoint honors natively.
 *
 * Canvas: LAYOUT_WIDE 13.33×7.5in at 96dpi-equivalent → 1280×720px.
 * px → in = px / 96 (both axes); font px → pt = px × 72/96 = px × 0.75.
 */

const IN_W = 13.33;
const IN_H = 7.5;

const fontPx2pt = (px: number) => Math.round(px * 0.75 * 10) / 10;

function hex(color: string | undefined, fallback: string): string {
  const c = (color ?? "").replace("#", "");
  return /^[0-9a-fA-F]{6}$/.test(c) ? c : fallback.replace("#", "");
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

    for (const prim of elementsOf(slide, th)) {
      await drawPrim(s, prim, th, head, body);
    }

    if (slide.notes) s.addNotes(slide.notes);
  }

  const hasTransitions = deck.slides.some((s) => s.transition && s.transition !== "none");
  if (!hasTransitions) {
    await pptx.writeFile({ fileName });
    return;
  }

  // Transitions requested: write to a blob, reopen with JSZip and inject the
  // per-slide <p:transition> elements, then save.
  const blob = (await pptx.write({ outputType: "blob" })) as Blob;
  const zip = await JSZip.loadAsync(blob);
  for (let i = 0; i < deck.slides.length; i++) {
    const transition = deck.slides[i]!.transition;
    if (!transition || transition === "none") continue;
    const path = `ppt/slides/slide${i + 1}.xml`;
    const xml = await zip.file(path)?.async("string");
    if (!xml) continue;
    zip.file(path, injectTransition(xml, transition));
  }
  const out = await zip.generateAsync({ type: "blob", mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation" });
  const url = URL.createObjectURL(out);
  const a = document.createElement("a");
  a.href = url;
  a.download = fileName;
  a.click();
  URL.revokeObjectURL(url);
}

/** Insert a <p:transition> element into a slide XML part. CT_Slide orders
 * children cSld → clrMapOvr → transition → timing, so the injection anchor
 * is the close of clrMapOvr (always present in pptxgenjs output) falling
 * back to the close of cSld. */
function injectTransition(slideXml: string, transition: string): string {
  const xml = transitionXml(transition);
  const clrIdx = slideXml.indexOf("</p:clrMapOvr>");
  if (clrIdx >= 0) {
    const at = clrIdx + "</p:clrMapOvr>".length;
    return slideXml.slice(0, at) + xml + slideXml.slice(at);
  }
  const cSldIdx = slideXml.indexOf("</p:cSld>");
  if (cSldIdx >= 0) {
    const at = cSldIdx + "</p:cSld>".length;
    return slideXml.slice(0, at) + xml + slideXml.slice(at);
  }
  return slideXml;
}

function transitionXml(transition: string): string {
  switch (transition) {
    case "fade":
      return '<p:transition spd="med"><p:fade/></p:transition>';
    case "push":
      return '<p:transition spd="med"><p:push dir="u"/></p:transition>';
    case "wipe":
      return '<p:transition spd="med"><p:wipe dir="r"/></p:transition>';
    case "zoom":
      return '<p:transition spd="med"><p:zoom dir="in"/></p:transition>';
    default:
      return "";
  }
}

async function drawPrim(
  s: PptxGenJS.Slide,
  prim: SlideElement | SlidePrim,
  theme: DeckTheme,
  head: string,
  body: string,
): Promise<void> {
  const X = (v: number) => (v / 1280) * IN_W;
  const Y = (v: number) => (v / 720) * IN_H;
  const rotate = "rotate" in prim && prim.rotate ? { rotate: prim.rotate } : {};

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
        ...rotate,
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
        ...rotate,
      });
      break;
    }

    case "text": {
      const explicit = "fontFamily" in prim && prim.fontFamily ? prim.fontFamily : undefined;
      s.addText(prim.text, {
        x: X(prim.x),
        y: Y(prim.y),
        w: X(prim.w),
        h: Y(prim.h),
        fontFace: explicit ?? (prim.font === "heading" ? head : body),
        fontSize: fontPx2pt(prim.size),
        bold: prim.bold,
        italic: prim.italic,
        color: hex(prim.color, "#f8fafc"),
        align: prim.align ?? "left",
        valign: prim.valign ?? "top",
        lineSpacingMultiple: prim.lineHeight ?? 1.3,
        margin: 0,
        fit: "shrink",
        ...rotate,
      });
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
          ...rotate,
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
          ...rotate,
        });
      }
      break;
    }
  }
}

/** Fetch an image source (URL or same-origin workspace path) as a data URL. */
async function fetchImageDataUrl(source: string): Promise<string | null> {
  if (!source.trim()) return null;
  if (source.startsWith("data:")) return source;
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
