import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useTranslation } from "react-i18next";
import { cssFont, newElementId, type DeckTheme, type Slide, type SlideElement } from "../types";
import { elementsOf, materialize, isFreeForm, MAX_ELEMENTS, patchElement } from "../lib/elements";
import { STAGE_H, STAGE_W } from "../lib/slide-spec";
import { ElementToolbar } from "./element-toolbar";

/**
 * Canva-style direct-manipulation stage for one slide: a scrollable neutral
 * surface holding the 16:9 slide box. Everything happens on the canvas —
 * click-select (with outline + 8 resize handles + rotate handle), drag to
 * move, corner/edge resize (images keep their aspect on corners), and
 * double-click inline text editing. A floating one-row context toolbar
 * (element-toolbar.tsx) anchors to the selection; there is no inspector
 * panel.
 *
 * Works on every slide: layout-driven (v1) slides are edited through the
 * compiled prims shown on screen and are promoted to an explicit element
 * list on first edit (materialize keeps the on-screen ids stable, so an
 * in-flight drag survives the promotion). The page drives element ops
 * through the {@link InteractiveStageHandle} ref and mirrors selection and
 * history depths back via callbacks for ribbon disabled states.
 */

const HANDLE = 8; // handle hit size in stage px
const ROT_HANDLE_DIST = 30;
/** Padding around the slide box inside the scrollable canvas area. */
const CANVAS_PAD = 20;
/** Gap between the selection and the floating toolbar. */
const TOOLBAR_GAP = 8;
/** Zoom multiplier bounds (1 = fit to canvas area). */
export const MIN_ZOOM = 0.5;
export const MAX_ZOOM = 2;

type DragMode =
  | {
      type: "move";
      id: string;
      startX: number;
      startY: number;
      orig: { x: number; y: number };
      snapshot: Slide;
      undoPushed: boolean;
    }
  | {
      type: "resize";
      id: string;
      handle: string;
      startX: number;
      startY: number;
      orig: { x: number; y: number; w: number; h: number };
      snapshot: Slide;
      undoPushed: boolean;
    }
  | {
      type: "rotate";
      id: string;
      cx: number;
      cy: number;
      startAngle: number;
      origRotate: number;
      snapshot: Slide;
      undoPushed: boolean;
    }
  | null;

/** Element-level operations the ribbon (and other shells) can drive. */
export interface InteractiveStageHandle {
  addElement: (partial: Partial<SlideElement> & { kind: SlideElement["kind"] }) => void;
  addImageFile: (file: File | undefined) => void;
  duplicateElement: () => void;
  removeElement: () => void;
  moveZ: (dir: -1 | 1) => void;
  undo: () => void;
  redo: () => void;
}

interface InteractiveStageProps {
  slide: Slide;
  theme: DeckTheme;
  onChange: (next: Slide) => void;
  /** Zoom multiplier on top of the fit scale (1 = fit). */
  zoom?: number;
  /** Effective scale (fit × zoom) — the page shows it as a percentage. */
  onScaleChange?: (fraction: number) => void;
  /** Whether an element is currently selected (ribbon enable state). */
  onSelectionChange?: (hasSelection: boolean) => void;
  /** Undo/redo stack depths (ribbon disabled states). */
  onHistoryChange?: (hist: { undo: number; redo: number }) => void;
}

export const InteractiveStage = forwardRef<InteractiveStageHandle, InteractiveStageProps>(
  function InteractiveStage(
    { slide, theme, onChange, zoom: zoomProp = 1, onScaleChange, onSelectionChange, onHistoryChange },
    ref,
  ) {
    const { t } = useTranslation("toolbox");
    const areaRef = useRef<HTMLDivElement>(null);
    const wrapRef = useRef<HTMLDivElement>(null);
    const centerRef = useRef<HTMLDivElement>(null);
    const toolbarRef = useRef<HTMLDivElement>(null);
    const [fit, setFit] = useState(1);
    const [histLen, setHistLen] = useState({ undo: 0, redo: 0 });
    const [selectedId, setSelectedId] = useState<string | null>(null);
    const [editingId, setEditingId] = useState<string | null>(null);
    const [tbPos, setTbPos] = useState<{ left: number; top: number } | null>(null);
    const dragRef = useRef<DragMode>(null);
    const undoStack = useRef<Slide[]>([]);
    const redoStack = useRef<Slide[]>([]);
    /** Set when the editor closes via Escape so the trailing blur commit
     * knows to discard the draft instead of applying it. */
    const cancelEditRef = useRef(false);

    const elements = useMemo(() => elementsOf(slide, theme), [slide, theme]);
    const freeForm = isFreeForm(slide);
    const selected = elements.find((el) => el.id === selectedId) ?? null;

    // Fit the 16:9 slide box into the canvas area (content rect excludes the
    // area's own padding — the inner wrapper carries it — and excludes
    // classic scrollbars, so the math stays honest while zoomed in).
    useEffect(() => {
      const el = areaRef.current;
      if (!el) return;
      const update = () => {
        const w = el.clientWidth - CANVAS_PAD * 2 - 2;
        const h = el.clientHeight - CANVAS_PAD * 2 - 2;
        setFit(Math.max(0.05, Math.min(w / STAGE_W, h / STAGE_H)));
      };
      update();
      const ro = new ResizeObserver(update);
      ro.observe(el);
      return () => ro.disconnect();
    }, []);

    const eff = Math.max(0.05, fit * Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoomProp)));

    // Slide switches remount this component (the page keys it by slide index),
    // so selection state resets with the mount — no deselect effect needed
    // here (one used to clear the selection right after addElement).

    // Mirror state up to the page for the ribbon/status bar. The setters on
    // the page side bail out on unchanged values, so identity churn of the
    // callbacks doesn't loop.
    useEffect(() => {
      onScaleChange?.(eff);
    }, [eff, onScaleChange]);
    useEffect(() => {
      onSelectionChange?.(selectedId !== null);
    }, [selectedId, onSelectionChange]);
    useEffect(() => {
      onHistoryChange?.(histLen);
    }, [histLen, onHistoryChange]);

    const pushUndo = useCallback(
      (snapshot: Slide) => {
        undoStack.current.push(snapshot);
        if (undoStack.current.length > 50) undoStack.current.shift();
        redoStack.current = [];
        setHistLen({ undo: undoStack.current.length, redo: redoStack.current.length });
      },
      [],
    );

    /** Apply an element patch, promoting a layout-driven slide to free-form
     * with its on-screen ids so in-flight interactions survive. */
    const applyPatch = useCallback(
      (id: string, patch: Partial<SlideElement>, recordHistory = true) => {
        const snapshot = slide;
        const base = materialize(slide, elements);
        const next = patchElement(base, id, patch);
        if (!next) return;
        if (recordHistory) pushUndo(snapshot);
        onChange(next);
      },
      [slide, elements, onChange, pushUndo],
    );

    /** Replace the whole element list (promotes layout-driven slides too). */
    const applyElements = useCallback(
      (nextElements: SlideElement[], recordHistory = true) => {
        const snapshot = slide;
        const next = { ...materialize(slide, elements), elements: nextElements };
        if (recordHistory) pushUndo(snapshot);
        onChange(next);
      },
      [slide, elements, onChange, pushUndo],
    );

    const undo = useCallback(() => {
      const prev = undoStack.current.pop();
      if (!prev) return;
      redoStack.current.push(slide);
      setHistLen({ undo: undoStack.current.length, redo: redoStack.current.length });
      onChange(prev);
    }, [slide, onChange]);

    const redo = useCallback(() => {
      const next = redoStack.current.pop();
      if (!next) return;
      undoStack.current.push(slide);
      setHistLen({ undo: undoStack.current.length, redo: redoStack.current.length });
      onChange(next);
    }, [slide, onChange]);

    // ── Pointer math ──

    const stagePoint = useCallback(
      (e: PointerEvent | React.PointerEvent): { x: number; y: number } => {
        const rect = wrapRef.current?.getBoundingClientRect();
        if (!rect) return { x: 0, y: 0 };
        return { x: (e.clientX - rect.left) / eff, y: (e.clientY - rect.top) / eff };
      },
      [eff],
    );

    const onElementPointerDown = (e: React.PointerEvent, el: SlideElement) => {
      if (el.locked || editingId === el.id) return;
      e.stopPropagation();
      setSelectedId(el.id);
      const pt = stagePoint(e);
      dragRef.current = {
        type: "move",
        id: el.id,
        startX: pt.x,
        startY: pt.y,
        orig: { x: el.x, y: el.y },
        snapshot: slide,
        undoPushed: false,
      };
      (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    };

    const onHandlePointerDown = (e: React.PointerEvent, el: SlideElement, handle: string) => {
      e.stopPropagation();
      const pt = stagePoint(e);
      dragRef.current = {
        type: "resize",
        id: el.id,
        handle,
        startX: pt.x,
        startY: pt.y,
        orig: { x: el.x, y: el.y, w: el.w, h: el.h },
        snapshot: slide,
        undoPushed: false,
      };
      (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    };

    const onRotatePointerDown = (e: React.PointerEvent, el: SlideElement) => {
      e.stopPropagation();
      const pt = stagePoint(e);
      const cx = el.x + el.w / 2;
      const cy = el.y + el.h / 2;
      dragRef.current = {
        type: "rotate",
        id: el.id,
        cx,
        cy,
        startAngle: (Math.atan2(pt.y - cy, pt.x - cx) * 180) / Math.PI,
        origRotate: el.rotate ?? 0,
        snapshot: slide,
        undoPushed: false,
      };
      (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    };

    const onPointerMove = useCallback(
      (e: React.PointerEvent) => {
        const drag = dragRef.current;
        if (!drag) return;
        const pt = stagePoint(e);
        const el = elements.find((x) => x.id === drag.id);
        if (!el) return;
        // History is recorded lazily: a plain click never pollutes the undo
        // stack (and never clears redo), only the first real change does.
        if (!drag.undoPushed) {
          pushUndo(drag.snapshot);
          drag.undoPushed = true;
        }

        if (drag.type === "move") {
          const dx = pt.x - drag.startX;
          const dy = pt.y - drag.startY;
          applyPatch(drag.id, {
            x: Math.round(drag.orig.x + dx),
            y: Math.round(drag.orig.y + dy),
          }, false);
        } else if (drag.type === "resize") {
          const dx = pt.x - drag.startX;
          const dy = pt.y - drag.startY;
          let { x, y, w, h } = drag.orig;
          const corner = drag.handle.length === 2;
          if (corner && el.kind === "image" && drag.orig.w > 0 && drag.orig.h > 0) {
            // Corner handles keep the image's aspect ratio (dominant axis wins).
            const ratio = drag.orig.w / drag.orig.h;
            if (Math.abs(dx) * drag.orig.h >= Math.abs(dy) * drag.orig.w) {
              w = drag.orig.w + (drag.handle.includes("w") ? -dx : dx);
              h = w / ratio;
            } else {
              h = drag.orig.h + (drag.handle.includes("n") ? -dy : dy);
              w = h * ratio;
            }
            if (w < 24) {
              w = 24;
              h = w / ratio;
            }
            if (h < 20) {
              h = 20;
              w = h * ratio;
            }
            if (drag.handle.includes("w")) x = drag.orig.x + (drag.orig.w - w);
            if (drag.handle.includes("n")) y = drag.orig.y + (drag.orig.h - h);
          } else {
            if (drag.handle.includes("e")) w = drag.orig.w + dx;
            if (drag.handle.includes("s")) h = drag.orig.h + dy;
            if (drag.handle.includes("w")) {
              w = drag.orig.w - dx;
              x = drag.orig.x + dx;
            }
            if (drag.handle.includes("n")) {
              h = drag.orig.h - dy;
              y = drag.orig.y + dy;
            }
          }
          applyPatch(
            drag.id,
            {
              x: Math.round(x),
              y: Math.round(y),
              w: Math.max(24, Math.round(w)),
              h: Math.max(20, Math.round(h)),
            },
            false,
          );
        } else if (drag.type === "rotate") {
          const angle = (Math.atan2(pt.y - drag.cy, pt.x - drag.cx) * 180) / Math.PI;
          let deg = drag.origRotate + (angle - drag.startAngle);
          if (e.shiftKey) deg = Math.round(deg / 15) * 15;
          else deg = Math.round(deg);
          applyPatch(drag.id, { rotate: ((deg % 360) + 360) % 360 }, false);
        }
      },
      [elements, stagePoint, applyPatch, pushUndo],
    );

    const onPointerUp = useCallback(() => {
      dragRef.current = null;
    }, []);

    // ── Element ops (ribbon/page drives these through the imperative handle) ──

    const addElement = (partial: Partial<SlideElement> & { kind: SlideElement["kind"] }) => {
      if (elements.length >= MAX_ELEMENTS) {
        return;
      }
      const base = { id: newElementId(), x: STAGE_W / 2 - 180, y: STAGE_H / 2 - 60, w: 360, h: 120 };
      const { kind, ...rest } = partial;
      let el: SlideElement;
      switch (kind) {
        case "text":
          el = {
            ...base,
            ...rest,
            kind: "text",
            text: t("pptx.stage.new_text"),
            font: "body",
            size: 36,
            bold: false,
            color: theme.foreground,
            align: "left",
          } as SlideElement;
          break;
        case "rect":
          el = { ...base, ...rest, kind: "rect", fill: theme.accent, radius: 12 } as SlideElement;
          break;
        case "ellipse":
          el = { ...base, ...rest, kind: "ellipse", fill: theme.accent } as SlideElement;
          break;
        case "frame":
          el = { ...base, ...rest, kind: "frame", color: theme.muted, width: 2 } as SlideElement;
          break;
        default:
          return;
      }
      const list = materialize(slide, elements).elements ?? [];
      applyElements([...list, el]);
      setSelectedId(el.id);
    };

    const addImageFile = (file: File | undefined) => {
      if (!file || !file.type.startsWith("image/")) return;
      const reader = new FileReader();
      reader.onload = () => {
        if (typeof reader.result !== "string") return;
        const img = new Image();
        img.onload = () => {
          if (elements.length >= MAX_ELEMENTS) return;
          // Fit inside the stage preserving aspect
          const maxW = STAGE_W * 0.6;
          const maxH = STAGE_H * 0.6;
          const ratio = Math.min(maxW / img.naturalWidth, maxH / img.naturalHeight, 1);
          const w = Math.round(img.naturalWidth * ratio);
          const h = Math.round(img.naturalHeight * ratio);
          const el: SlideElement = {
            id: newElementId(),
            kind: "image",
            x: Math.round((STAGE_W - w) / 2),
            y: Math.round((STAGE_H - h) / 2),
            w,
            h,
            source: reader.result as string,
            alt: file.name,
          };
          const list = materialize(slide, elements).elements ?? [];
          applyElements([...list, el]);
          setSelectedId(el.id);
        };
        img.src = reader.result;
      };
      reader.readAsDataURL(file);
    };

    const duplicateElement = () => {
      if (!selected) return;
      const copy = {
        ...selected,
        id: newElementId(),
        x: selected.x + 24,
        y: selected.y + 24,
      } as SlideElement;
      const list = materialize(slide, elements).elements ?? [];
      const idx = list.findIndex((x) => x.id === selected.id);
      const next = [...list];
      next.splice(idx + 1, 0, copy);
      applyElements(next);
      setSelectedId(copy.id);
    };

    const removeElement = () => {
      if (!selected || selected.locked) return;
      applyElements((materialize(slide, elements).elements ?? []).filter((x) => x.id !== selected.id));
      setSelectedId(null);
    };

    const moveZ = (dir: -1 | 1) => {
      if (!selected) return;
      const list = [...(materialize(slide, elements).elements ?? [])];
      const idx = list.findIndex((x) => x.id === selected.id);
      const to = idx + dir;
      if (to < 0 || to >= list.length) return;
      const [moved] = list.splice(idx, 1);
      list.splice(to, 0, moved!);
      applyElements(list);
    };

    // Ribbon/page access to the element ops (refreshed every render so the
    // closures always see the latest slide/selection).
    useImperativeHandle(ref, () => ({
      addElement,
      addImageFile,
      duplicateElement,
      removeElement,
      moveZ,
      undo,
      redo,
    }));

    /** Inline text commit (blur / Enter). Esc cancels instead. */
    const commitText = (el: SlideElement & { text: string }, value: string) => {
      setEditingId(null);
      if (cancelEditRef.current) {
        cancelEditRef.current = false;
        return; // cancelled — discard the draft
      }
      if (value !== el.text) applyPatch(el.id, { text: value } as Partial<SlideElement>);
    };

    // Keyboard: delete/arrows while an element is selected (not while editing)
    useEffect(() => {
      const handler = (e: KeyboardEvent) => {
        if (editingId) return;
        if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
        if (!selected) return;
        const tag = (e.target as HTMLElement)?.getAttribute?.("contenteditable");
        if (tag === "true") return;
        switch (e.key) {
          case "Delete":
          case "Backspace":
            if (!selected.locked) {
              e.preventDefault();
              removeElement();
            }
            break;
          case "ArrowLeft":
            e.preventDefault();
            applyPatch(selected.id, { x: selected.x - (e.shiftKey ? 10 : 1) });
            break;
          case "ArrowRight":
            e.preventDefault();
            applyPatch(selected.id, { x: selected.x + (e.shiftKey ? 10 : 1) });
            break;
          case "ArrowUp":
            e.preventDefault();
            applyPatch(selected.id, { y: selected.y - (e.shiftKey ? 10 : 1) });
            break;
          case "ArrowDown":
            e.preventDefault();
            applyPatch(selected.id, { y: selected.y + (e.shiftKey ? 10 : 1) });
            break;
          case "Escape":
            setSelectedId(null);
            break;
        }
      };
      window.addEventListener("keydown", handler);
      return () => window.removeEventListener("keydown", handler);
    }, [selected, editingId, applyPatch, removeElement]);

    const handles = ["nw", "n", "ne", "e", "se", "s", "sw", "w"] as const;
    const handleFrac: Record<string, { fx: number; fy: number; cursor: string }> = {
      nw: { fx: 0, fy: 0, cursor: "nwse-resize" },
      n: { fx: 0.5, fy: 0, cursor: "ns-resize" },
      ne: { fx: 1, fy: 0, cursor: "nesw-resize" },
      e: { fx: 1, fy: 0.5, cursor: "ew-resize" },
      se: { fx: 1, fy: 1, cursor: "nwse-resize" },
      s: { fx: 0.5, fy: 1, cursor: "ns-resize" },
      sw: { fx: 0, fy: 1, cursor: "nesw-resize" },
      w: { fx: 0, fy: 0.5, cursor: "ew-resize" },
    };

    // ── Floating toolbar anchoring ──
    // Measured after layout, positioned inside the centering wrapper so it
    // can overhang the slide box without being clipped by it. It scrolls
    // with the canvas (both rects move together).

    const selectedKey = selected?.id ?? null;
    useEffect(() => {
      setTbPos(null);
    }, [selectedKey]);

    useLayoutEffect(() => {
      const tb = toolbarRef.current;
      const center = centerRef.current;
      const wrap = wrapRef.current;
      if (!tb || !center || !wrap || !selected) return;
      const centerRect = center.getBoundingClientRect();
      const wrapRect = wrap.getBoundingClientRect();
      const ox = wrapRect.left - centerRect.left;
      const oy = wrapRect.top - centerRect.top;
      const tw = tb.offsetWidth;
      const th = tb.offsetHeight;
      const above = oy + selected.y * eff - th - TOOLBAR_GAP;
      const below = oy + (selected.y + selected.h) * eff + TOOLBAR_GAP;
      const top = above >= 0
        ? above
        : Math.max(2, Math.min(below, center.clientHeight - th - 2));
      const left = Math.min(
        Math.max(2, ox + (selected.x + selected.w / 2) * eff - tw / 2),
        Math.max(2, center.clientWidth - tw - 2),
      );
      setTbPos((prev) => (prev && prev.left === left && prev.top === top ? prev : { left, top }));
    }, [selected, eff]);

    return (
      <div className="flex h-full min-h-0 flex-col">
        {/* Canvas area: scrollable neutral surface, slide box centered at
            fit × zoom. Zoom > 1 makes the surface scroll like PowerPoint. */}
        <div
          ref={areaRef}
          className="min-h-0 flex-1 overflow-auto overscroll-contain rounded-lg border bg-muted/30"
        >
          <div
            ref={centerRef}
            className="relative flex min-h-full min-w-full items-center justify-center"
            style={{ padding: CANVAS_PAD }}
          >
            <div
              ref={wrapRef}
              className="relative shrink-0 overflow-hidden rounded-md shadow-md ring-1 ring-black/10"
              style={{ width: STAGE_W * eff, height: STAGE_H * eff }}
              onPointerDown={() => {
                setSelectedId(null);
                setEditingId(null);
              }}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
            >
              <div
                style={{
                  width: STAGE_W,
                  height: STAGE_H,
                  transform: `scale(${eff})`,
                  transformOrigin: "top left",
                  backgroundColor: theme.background,
                  color: theme.foreground,
                }}
              >
                {elements.map((el) => {
                  const isSelected = el.id === selectedId;
                  return (
                    <div
                      key={el.id}
                      style={{
                        position: "absolute",
                        left: 0,
                        top: 0,
                        width: STAGE_W,
                        height: STAGE_H,
                        pointerEvents: "none",
                      }}
                    >
                      <div
                        onPointerDown={(e) => onElementPointerDown(e, el)}
                        onDoubleClick={(e) => {
                          e.stopPropagation();
                          if (el.kind === "text" && !el.locked) {
                            cancelEditRef.current = false;
                            setEditingId(el.id);
                          }
                        }}
                        style={{
                          position: "absolute",
                          left: el.x,
                          top: el.y,
                          width: el.w,
                          height: el.h,
                          transform: el.rotate ? `rotate(${el.rotate}deg)` : undefined,
                          cursor: el.locked ? "default" : "move",
                          pointerEvents: "auto",
                          outline: isSelected ? "2px solid var(--primary)" : undefined,
                          outlineOffset: 2,
                        }}
                      >
                        {editingId === el.id && el.kind === "text" ? (
                          <textarea
                            autoFocus
                            defaultValue={el.text}
                            onBlur={(e) => commitText(el, e.target.value)}
                            onKeyDown={(e) => {
                              e.stopPropagation();
                              if (e.key === "Escape") {
                                // Cancel: drop the draft without patching.
                                e.preventDefault();
                                cancelEditRef.current = true;
                                setEditingId(null);
                              } else if (e.key === "Enter" && !e.shiftKey) {
                                e.preventDefault();
                                commitText(el, (e.target as HTMLTextAreaElement).value);
                              }
                            }}
                            onPointerDown={(e) => e.stopPropagation()}
                            style={{
                              width: "100%",
                              height: "100%",
                              resize: "none",
                              border: "none",
                              outline: "2px solid var(--primary)",
                              background: "transparent",
                              color: el.color,
                              fontFamily: el.fontFamily ? cssFont(el.fontFamily) : cssFont(theme[el.font === "heading" ? "font_heading" : "font_body"]),
                              fontSize: el.size,
                              fontWeight: el.bold ? 700 : 400,
                              fontStyle: el.italic ? "italic" : undefined,
                              lineHeight: el.lineHeight ?? 1.3,
                              textAlign: el.align ?? "left",
                              padding: 0,
                            }}
                          />
                        ) : (
                          <PrimStatic prim={el} theme={theme} />
                        )}
                      </div>

                      {/* Selection handles */}
                      {isSelected && !editingId && (
                        <>
                          {handles.map((h) => {
                            const { fx, fy, cursor } = handleFrac[h]!;
                            return (
                              <div
                                key={h}
                                onPointerDown={(e) => onHandlePointerDown(e, el, h)}
                                style={{
                                  position: "absolute",
                                  left: el.x + fx * el.w - HANDLE / 2,
                                  top: el.y + fy * el.h - HANDLE / 2,
                                  width: HANDLE,
                                  height: HANDLE,
                                  transform: el.rotate ? `rotate(${el.rotate}deg)` : undefined,
                                  background: "var(--background)",
                                  border: "2px solid var(--primary)",
                                  borderRadius: 2,
                                  cursor,
                                  pointerEvents: "auto",
                                }}
                              />
                            );
                          })}
                          {/* Rotate handle */}
                          <div
                            onPointerDown={(e) => onRotatePointerDown(e, el)}
                            title={t("pptx.stage.rotate")}
                            style={{
                              position: "absolute",
                              left: el.x + el.w / 2 - HANDLE / 2,
                              top: el.y + el.h + ROT_HANDLE_DIST - HANDLE / 2 - 10,
                              width: HANDLE,
                              height: HANDLE,
                              background: "var(--primary)",
                              borderRadius: "50%",
                              cursor: "grab",
                              pointerEvents: "auto",
                            }}
                          />
                        </>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Floating context toolbar: hidden until measured so it never
                flashes at the origin; the wrapper owns anchoring + clamping. */}
            {selected && !editingId && (
              <div
                className="absolute z-10 max-w-[calc(100%-8px)] overflow-x-auto"
                style={{
                  left: tbPos?.left ?? 0,
                  top: tbPos?.top ?? 0,
                  visibility: tbPos ? "visible" : "hidden",
                }}
              >
                <ElementToolbar
                  ref={toolbarRef}
                  el={selected}
                  theme={theme}
                  onPatch={(patch) => applyPatch(selected.id, patch)}
                  onMoveZ={moveZ}
                  onDuplicate={duplicateElement}
                  onDelete={removeElement}
                />
              </div>
            )}
          </div>
        </div>

        {/* Contextual hint while the slide still follows its layout template */}
        {!freeForm && (
          <p className="shrink-0 px-2 pt-1.5 text-center text-xs text-muted-foreground">
            {t("pptx.stage.layout_hint")}
          </p>
        )}
      </div>
    );
  },
);

/** Static (non-interactive) element render — same styles as slide-view's
 * PrimView, minus its own absolute positioning wrapper. */
function PrimStatic({ prim, theme }: { prim: SlideElement; theme: DeckTheme }) {
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
      return (
        <img
          src={prim.source}
          alt={prim.alt}
          draggable={false}
          style={{ width: "100%", height: "100%", objectFit: "contain", pointerEvents: "none" }}
        />
      );
  }
}
