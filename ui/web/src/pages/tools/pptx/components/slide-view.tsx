import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { Deck, DeckTheme, Slide } from "../types";
import { buildSlidePrims, STAGE_H, STAGE_W, type SlidePrim } from "../lib/slide-spec";

/**
 * HTML preview of one deck slide. The slide is authored as a flat primitive
 * list on a fixed 1280×720 stage (lib/slide-spec.ts) and scaled with a CSS
 * transform to the wrapper's width, so the preview keeps exact proportions
 * at any size (full preview and thumbnails alike) and renders exactly what
 * lib/pptx-export.ts writes into the .pptx.
 */

interface SlideViewProps {
  slide: Slide;
  theme: Deck["theme"];
  className?: string;
}

export function SlideView({ slide, theme, className }: SlideViewProps) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(0);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const update = () => setScale(el.clientWidth / STAGE_W);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const prims = useMemo(() => buildSlidePrims(slide, theme), [slide, theme]);

  return (
    <div
      ref={wrapRef}
      className={cn(
        "relative w-full overflow-hidden bg-muted",
        className,
      )}
      style={{ aspectRatio: `${STAGE_W} / ${STAGE_H}` }}
    >
      <div
        style={{
          width: STAGE_W,
          height: STAGE_H,
          transform: `scale(${scale})`,
          transformOrigin: "top left",
          backgroundColor: theme.background,
          color: theme.foreground,
        }}
      >
        {prims.map((p, i) => (
          <PrimView key={i} prim={p} theme={theme} />
        ))}
      </div>
    </div>
  );
}

function PrimView({ prim, theme }: { prim: SlidePrim; theme: DeckTheme }) {
  const heading = cssStack(theme.font_heading);
  const body = cssStack(theme.font_body);

  switch (prim.kind) {
    case "rect":
      return (
        <div
          aria-hidden
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            backgroundColor: prim.fill,
            borderRadius: prim.radius,
          }}
        />
      );

    case "ellipse":
      return (
        <div
          aria-hidden
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            backgroundColor: prim.fill,
            borderRadius: "50%",
          }}
        />
      );

    case "frame":
      return (
        <div
          aria-hidden
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            border: `${prim.width}px ${prim.dash ? "dashed" : "solid"} ${prim.color}`,
            borderRadius: prim.radius,
          }}
        />
      );

    case "text":
      return (
        <div
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            display: "flex",
            alignItems:
              prim.valign === "middle" ? "center" : prim.valign === "bottom" ? "flex-end" : "flex-start",
            fontFamily: prim.font === "heading" ? heading : body,
            fontSize: prim.size,
            fontWeight: prim.bold ? 700 : 400,
            fontStyle: prim.italic ? "italic" : undefined,
            color: prim.color,
            lineHeight: prim.lineHeight ?? 1.3,
            textAlign: prim.align ?? "left",
            overflowWrap: "break-word",
          }}
        >
          <span style={{ width: "100%" }}>{prim.text}</span>
        </div>
      );

    case "image":
      return (
        <img
          src={prim.source.startsWith("/") ? prim.source : prim.source}
          alt={prim.alt}
          draggable={false}
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            objectFit: "contain",
          }}
        />
      );
  }
}

function cssStack(font: string | undefined): string {
  switch (font) {
    case "Arial":
      return "Arial, 'Liberation Sans', 'Helvetica Neue', sans-serif";
    case "Calibri":
      return "Calibri, Carlito, 'Segoe UI', sans-serif";
    case "Georgia":
      return "Georgia, 'Times New Roman', serif";
    case "Verdana":
      return "Verdana, DejaVu Sans, sans-serif";
    case "Tahoma":
      return "Tahoma, Verdana, sans-serif";
    case "Trebuchet MS":
      return "'Trebuchet MS', 'Segoe UI', sans-serif";
    case "Times New Roman":
      return "'Times New Roman', Times, serif";
    case "Courier New":
      return "'Courier New', monospace";
    default:
      return "'Segoe UI', sans-serif";
  }
}
