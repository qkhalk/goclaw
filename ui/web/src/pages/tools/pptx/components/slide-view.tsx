import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { AnimSpec, Deck, DeckTheme, Slide } from "../types";
import { buildSlidePrims, STAGE_H, STAGE_W, type SlidePrim } from "../lib/slide-spec";
import { getIcon } from "../lib/icon-library";

/**
 * HTML preview of one deck slide. The slide is authored as a flat primitive
 * list on a fixed 1280×720 stage (lib/slide-spec.ts) and scaled with a CSS
 * transform to the wrapper's width, so the preview keeps exact proportions
 * at any size (full preview and thumbnails alike) and renders exactly what
 * lib/pptx-export.ts writes into the .pptx.
 *
 * When `animate` is set (the main stage only — never thumbnails), primitives
 * carrying an `anim` spec play a one-shot CSS entrance animation on mount.
 * The .pptx export has no equivalent: animation is a preview-only layer, so
 * the keyframes live here instead of the global stylesheet.
 */

const ANIM_STYLE_ID = "pptx-prim-anim-styles";

/** One-shot entrance keyframes for animated primitives (preview only). */
const ANIM_CSS = `
@keyframes pptx-anim-fade-in { from { opacity: 0 } to { opacity: 1 } }
@keyframes pptx-anim-slide-up { from { opacity: 0; transform: translateY(24px) } to { opacity: 1; transform: none } }
@keyframes pptx-anim-slide-left { from { opacity: 0; transform: translateX(36px) } to { opacity: 1; transform: none } }
@keyframes pptx-anim-scale-in { from { opacity: 0; transform: scale(0.85) } to { opacity: 1; transform: none } }
.pptx-prim-anim { animation-duration: 0.55s; animation-timing-function: cubic-bezier(0.22, 0.9, 0.34, 1); animation-fill-mode: both; }
.pptx-anim-fade-in { animation-name: pptx-anim-fade-in }
.pptx-anim-slide-up { animation-name: pptx-anim-slide-up }
.pptx-anim-slide-left { animation-name: pptx-anim-slide-left }
.pptx-anim-scale-in { animation-name: pptx-anim-scale-in }
`;

function ensureAnimStyles(): void {
  if (typeof document === "undefined" || document.getElementById(ANIM_STYLE_ID)) return;
  const el = document.createElement("style");
  el.id = ANIM_STYLE_ID;
  el.textContent = ANIM_CSS;
  document.head.appendChild(el);
}

interface SlideViewProps {
  slide: Slide;
  theme: Deck["theme"];
  className?: string;
  /** Play `anim` entrance effects on mount (main stage only). */
  animate?: boolean;
}

export function SlideView({ slide, theme, className, animate }: SlideViewProps) {
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

  useEffect(() => {
    if (animate) ensureAnimStyles();
  }, [animate]);

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
          <PrimView key={i} prim={p} theme={theme} animate={animate} />
        ))}
      </div>
    </div>
  );
}

/** className + delay for an animated primitive; undefined when static. */
function animProps(
  anim: AnimSpec | undefined,
  animate: boolean,
): { className?: string; style?: { animationDelay?: string } } {
  if (!anim || !animate) return {};
  return {
    className: `pptx-prim-anim pptx-anim-${anim.effect}`,
    style: anim.delayMs ? { animationDelay: `${Math.min(anim.delayMs, 60000) / 1000}s` } : undefined,
  };
}

function PrimView({ prim, theme, animate }: { prim: SlidePrim; theme: DeckTheme; animate?: boolean }) {
  const heading = cssStack(theme.font_heading);
  const body = cssStack(theme.font_body);

  switch (prim.kind) {
    case "rect": {
      const anim = animProps(prim.anim, !!animate);
      return (
        <div
          aria-hidden
          className={anim.className}
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            backgroundColor: prim.fill,
            borderRadius: prim.radius,
            ...anim.style,
          }}
        />
      );
    }

    case "ellipse": {
      const anim = animProps(prim.anim, !!animate);
      return (
        <div
          aria-hidden
          className={anim.className}
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            backgroundColor: prim.fill,
            borderRadius: "50%",
            ...anim.style,
          }}
        />
      );
    }

    case "frame": {
      const anim = animProps(prim.anim, !!animate);
      return (
        <div
          aria-hidden
          className={anim.className}
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            border: `${prim.width}px ${prim.dash ? "dashed" : "solid"} ${prim.color}`,
            borderRadius: prim.radius,
            ...anim.style,
          }}
        />
      );
    }

    case "icon": {
      const anim = animProps(prim.anim, !!animate);
      const icon = getIcon(prim.name);
      if (!icon) return null; // unknown name: skip like the export does
      return (
        <svg
          aria-hidden
          viewBox="0 0 24 24"
          className={anim.className}
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            ...anim.style,
          }}
          fill="none"
          stroke={prim.color}
          strokeWidth={prim.strokeWidth}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {icon.paths.map((d, i) => (
            <path key={i} d={d} />
          ))}
        </svg>
      );
    }

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
