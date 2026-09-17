import { useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { Deck, DeckTheme, Slide, SlideElement } from "../types";
import { cssFont } from "../types";
import { STAGE_H, STAGE_W } from "../lib/slide-spec";
import { elementsOf } from "../lib/elements";

/**
 * HTML preview of one deck slide. The slide is either an explicit element
 * list (v2 free-form) or compiled to a flat primitive list on the fixed
 * 1280×720 stage (lib/slide-spec.ts) and scaled with a CSS transform to the
 * wrapper's width, so the preview keeps exact proportions at any size (full
 * preview and thumbnails alike) and renders exactly what
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

  const prims = useMemo(() => elementsOf(slide, theme), [slide, theme]);

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
        {prims.map((p) => (
          <PrimView key={p.id} prim={p} theme={theme} />
        ))}
      </div>
    </div>
  );
}

export function PrimView({ prim, theme }: { prim: SlideElement; theme: DeckTheme }) {
  const heading = cssStack(theme.font_heading);
  const body = cssStack(theme.font_body);

  const rotate = prim.rotate ? { transform: `rotate(${prim.rotate}deg)` } : undefined;

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
            ...rotate,
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
            ...rotate,
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
            ...rotate,
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
            fontFamily: prim.fontFamily ? cssFont(prim.fontFamily) : prim.font === "heading" ? heading : body,
            fontSize: prim.size,
            fontWeight: prim.bold ? 700 : 400,
            fontStyle: prim.italic ? "italic" : undefined,
            color: prim.color,
            lineHeight: prim.lineHeight ?? 1.3,
            textAlign: prim.align ?? "left",
            overflowWrap: "break-word",
            ...rotate,
          }}
        >
          <span style={{ width: "100%" }}>{prim.text}</span>
        </div>
      );

    case "image":
      return (
        <img
          src={prim.source}
          alt={prim.alt}
          draggable={false}
          style={{
            position: "absolute",
            left: prim.x,
            top: prim.y,
            width: prim.w,
            height: prim.h,
            objectFit: "contain",
            ...rotate,
          }}
        />
      );
  }
}

export function cssStack(font: string | undefined): string {
  return cssFont(font);
}
