import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { AnimatePresence, motion } from "framer-motion";
import { ChevronLeft, ChevronRight, Notebook, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { cssFont } from "../types";
import type { Deck, DeckTheme, Slide, SlideElement, SlideTransition } from "../types";
import { elementsOf } from "../lib/elements";
import { STAGE_H, STAGE_W } from "../lib/slide-spec";

/**
 * Fullscreen presentation mode. Slides render through the same element list
 * as the studio preview (elementsOf), scaled to fit the viewport; each
 * transition plays via framer-motion. Keyboard: →/Space/← navigate, N notes,
 * Esc exit. The speaker notes panel overlays the bottom on demand.
 */

interface PresentationProps {
  deck: Deck;
  index: number;
  onIndexChange: (i: number) => void;
  onExit: () => void;
}

const spring = { type: "spring", stiffness: 320, damping: 34 } as const;

function variantsFor(t: SlideTransition) {
  switch (t) {
    case "fade":
      return {
        initial: { opacity: 0 },
        animate: { opacity: 1 },
        exit: { opacity: 0 },
        transition: { duration: 0.35 },
      };
    case "push":
      return {
        initial: { x: "60%" },
        animate: { x: 0 },
        exit: { x: "-60%" },
        transition: spring,
      };
    case "wipe":
      return {
        initial: { clipPath: "inset(0 100% 0 0)" },
        animate: { clipPath: "inset(0 0% 0 0)" },
        exit: { clipPath: "inset(0 0 0 100%)" },
        transition: { duration: 0.4 },
      };
    case "zoom":
      return {
        initial: { scale: 0.7, opacity: 0 },
        animate: { scale: 1, opacity: 1 },
        exit: { scale: 1.15, opacity: 0 },
        transition: spring,
      };
    default:
      return {
        initial: { opacity: 1 },
        animate: { opacity: 1 },
        exit: { opacity: 1 },
        transition: { duration: 0 },
      };
  }
}

export function Presentation({ deck, index, onIndexChange, onExit }: PresentationProps) {
  const { t } = useTranslation("toolbox");
  const wrapRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(0.5);
  const [notesOpen, setNotesOpen] = useState(false);

  const slide: Slide | undefined = deck.slides[index];
  const variants = useMemo(() => variantsFor(slide?.transition ?? "fade"), [slide?.transition]);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const update = () => {
      const s = Math.min(el.clientWidth / STAGE_W, el.clientHeight / STAGE_H);
      setScale(s || 0.5);
    };
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const go = useCallback(
    (dir: -1 | 1) => {
      const next = index + dir;
      if (next < 0 || next >= deck.slides.length) return;
      onIndexChange(next);
    },
    [index, deck.slides.length, onIndexChange],
  );

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      switch (e.key) {
        case "ArrowRight":
        case "PageDown":
        case " ":
          e.preventDefault();
          go(1);
          break;
        case "ArrowLeft":
        case "PageUp":
          e.preventDefault();
          go(-1);
          break;
        case "n":
        case "N":
          setNotesOpen((v) => !v);
          break;
        case "Escape":
          onExit();
          break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [go, onExit]);

  // Best-effort fullscreen (must be user-gesture initiated; the click on the
  // Trình chiếu button qualifies — this effect runs in that gesture's task).
  useEffect(() => {
    document.documentElement.requestFullscreen?.().catch(() => {});
    return () => {
      if (document.fullscreenElement) void document.exitFullscreen().catch(() => {});
    };
  }, []);

  if (!slide) return null;

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black" ref={wrapRef}>
      {/* Slide area */}
      <div className="relative flex flex-1 items-center justify-center overflow-hidden">
        <AnimatePresence mode="popLayout" initial={false}>
          <motion.div
            key={index}
            initial={variants.initial}
            animate={variants.animate}
            exit={variants.exit}
            transition={variants.transition}
            style={{
              width: STAGE_W * scale,
              height: STAGE_H * scale,
              overflow: "hidden",
              backgroundColor: deck.theme.background,
            }}
            className="rounded-sm shadow-2xl"
          >
            <div
              style={{
                width: STAGE_W,
                height: STAGE_H,
                transform: `scale(${scale})`,
                transformOrigin: "top left",
              }}
            >
              <SlideSurface slide={slide} theme={deck.theme} />
            </div>
          </motion.div>
        </AnimatePresence>

        {/* Click zones */}
        <button
          type="button"
          aria-label={t("pptx.present_prev")}
          className="absolute inset-y-0 left-0 w-1/4 cursor-w-resize opacity-0"
          onClick={() => go(-1)}
        />
        <button
          type="button"
          aria-label={t("pptx.present_next")}
          className="absolute inset-y-0 right-0 w-1/4 cursor-e-resize opacity-0"
          onClick={() => go(1)}
        />
      </div>

      {/* Chrome */}
      <div className="flex items-center gap-2 border-t border-white/10 bg-black/80 px-3 py-2 text-white">
        <Button variant="ghost" size="icon-sm" aria-label={t("pptx.present_prev")} onClick={() => go(-1)} disabled={index === 0} className="text-white hover:bg-white/10">
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <span className="text-xs tabular-nums text-white/70">
          {index + 1} / {deck.slides.length}
        </span>
        <Button variant="ghost" size="icon-sm" aria-label={t("pptx.present_next")} onClick={() => go(1)} disabled={index >= deck.slides.length - 1} className="text-white hover:bg-white/10">
          <ChevronRight className="h-4 w-4" />
        </Button>
        <Button
          variant={notesOpen ? "secondary" : "ghost"}
          size="sm"
          onClick={() => setNotesOpen((v) => !v)}
          className="ml-auto text-white hover:bg-white/10"
        >
          <Notebook className="mr-1.5 h-3.5 w-3.5" />
          {t("pptx.present_notes")}
        </Button>
        <Button variant="ghost" size="sm" onClick={onExit} className="text-white hover:bg-white/10">
          <X className="mr-1.5 h-3.5 w-3.5" />
          {t("pptx.present_exit")}
        </Button>
      </div>

      {/* Speaker notes */}
      <AnimatePresence>
        {notesOpen && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="overflow-hidden border-t border-white/10 bg-zinc-950 px-4 py-3 text-sm text-white/90"
          >
            <p className={cn("whitespace-pre-wrap", !slide.notes && "italic text-white/40")}>
              {slide.notes || t("pptx.present_no_notes")}
            </p>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

/** Renders a slide's element list at stage scale (1) inside the presenter. */
function SlideSurface({ slide, theme }: { slide: Slide; theme: DeckTheme }) {
  const elements = useMemo(() => elementsOf(slide, theme), [slide, theme]);
  return (
    <div style={{ position: "relative", width: STAGE_W, height: STAGE_H, backgroundColor: theme.background, color: theme.foreground }}>
      {elements.map((el) => (
        <div
          key={el.id}
          style={{
            position: "absolute",
            left: el.x,
            top: el.y,
            width: el.w,
            height: el.h,
            transform: el.rotate ? `rotate(${el.rotate}deg)` : undefined,
          }}
        >
          <PrimSurface prim={el} theme={theme} />
        </div>
      ))}
    </div>
  );
}

function PrimSurface({ prim, theme }: { prim: SlideElement; theme: DeckTheme }) {
  switch (prim.kind) {
    case "rect":
      return <div style={{ width: "100%", height: "100%", backgroundColor: prim.fill, borderRadius: prim.radius }} />;
    case "ellipse":
      return <div style={{ width: "100%", height: "100%", backgroundColor: prim.fill, borderRadius: "50%" }} />;
    case "frame":
      return (
        <div
          style={{
            width: "100%",
            height: "100%",
            border: `${prim.width}px ${prim.dash ? "dashed" : "solid"} ${prim.color}`,
            borderRadius: prim.radius,
          }}
        />
      );
    case "text":
      return (
        <div
          style={{
            width: "100%",
            height: "100%",
            display: "flex",
            alignItems: prim.valign === "middle" ? "center" : prim.valign === "bottom" ? "flex-end" : "flex-start",
            fontFamily: prim.fontFamily ? cssFont(prim.fontFamily) : cssFont(theme[prim.font === "heading" ? "font_heading" : "font_body"]),
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
      return <img src={prim.source} alt={prim.alt} draggable={false} style={{ width: "100%", height: "100%", objectFit: "contain" }} />;
  }
}
