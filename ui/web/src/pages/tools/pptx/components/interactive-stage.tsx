import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowDown,
  ArrowUp,
  Bold,
  Copy,
  ImagePlus,
  Italic,
  Lock,
  LockOpen,
  Redo2,
  RotateCw,
  Square,
  Trash2,
  Type,
  Undo2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cssFont, SAFE_FONTS, newElementId, type DeckTheme, type Slide, type SlideElement } from "../types";
import { elementsOf, freeSlide, isFreeForm, MAX_ELEMENTS, patchElement } from "../lib/elements";
import { STAGE_H, STAGE_W } from "../lib/slide-spec";

/**
 * Canva-style direct-manipulation stage for one slide. Renders the slide's
 * element list (compiled from the layout for v1 slides) and supports:
 * click-select, drag-move, 8-handle resize, rotate, double-click inline text
 * editing, element toolbar (z-order/duplicate/lock/delete) and a style
 * inspector (font/size/bold/italic/color/align).
 *
 * Structural edits materialize the compiled prims into an explicit element
 * list on first touch (freeSlide) — the slide then stops following theme
 * recoloring, which the page surfaces as a hint chip.
 */

const HANDLE = 8; // handle hit size in stage px
const ROT_HANDLE_DIST = 30;

type DragMode =
  | { type: "move"; id: string; startX: number; startY: number; orig: { x: number; y: number } }
  | {
      type: "resize";
      id: string;
      handle: string;
      startX: number;
      startY: number;
      orig: { x: number; y: number; w: number; h: number };
    }
  | {
      type: "rotate";
      id: string;
      cx: number;
      cy: number;
      startAngle: number;
      origRotate: number;
    }
  | null;

interface InteractiveStageProps {
  slide: Slide;
  theme: DeckTheme;
  onChange: (next: Slide) => void;
}

export function InteractiveStage({ slide, theme, onChange }: InteractiveStageProps) {
  const { t } = useTranslation("toolbox");
  const wrapRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const dragRef = useRef<DragMode>(null);
  const undoStack = useRef<Slide[]>([]);
  const redoStack = useRef<Slide[]>([]);

  const elements = useMemo(() => elementsOf(slide, theme), [slide, theme]);
  const freeForm = isFreeForm(slide);
  const selected = elements.find((el) => el.id === selectedId) ?? null;

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const update = () => setScale(el.clientWidth / STAGE_W || 1);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Slide switches remount this component (the page keys it by slide index),
  // so selection state resets with the mount — no deselect effect needed
  // here (one used to clear the selection right after addElement).

  const pushUndo = useCallback(
    (snapshot: Slide) => {
      undoStack.current.push(snapshot);
      if (undoStack.current.length > 50) undoStack.current.shift();
      redoStack.current = [];
    },
    [],
  );

  /** Apply an element patch; frees the slide first when needed. */
  const applyPatch = useCallback(
    (id: string, patch: Partial<SlideElement>, recordHistory = true) => {
      const snapshot = slide;
      const freed = freeSlide(slide, theme);
      const next = patchElement(freed, id, patch);
      if (!next) return;
      if (recordHistory) pushUndo(snapshot);
      onChange(next);
    },
    [slide, theme, onChange, pushUndo],
  );

  /** Replace the whole element list (frees the slide first). */
  const applyElements = useCallback(
    (elements: SlideElement[], recordHistory = true) => {
      const snapshot = slide;
      const freed = freeSlide(slide, theme);
      const next = { ...freed, elements };
      if (recordHistory) pushUndo(snapshot);
      onChange(next);
    },
    [slide, theme, onChange, pushUndo],
  );

  const undo = useCallback(() => {
    const prev = undoStack.current.pop();
    if (!prev) return;
    redoStack.current.push(slide);
    onChange(prev);
  }, [slide, onChange]);

  const redo = useCallback(() => {
    const next = redoStack.current.pop();
    if (!next) return;
    undoStack.current.push(slide);
    onChange(next);
  }, [slide, onChange]);

  // ── Pointer math ──

  const stagePoint = useCallback(
    (e: PointerEvent | React.PointerEvent): { x: number; y: number } => {
      const rect = wrapRef.current?.getBoundingClientRect();
      if (!rect) return { x: 0, y: 0 };
      return { x: (e.clientX - rect.left) / scale, y: (e.clientY - rect.top) / scale };
    },
    [scale],
  );

  const onElementPointerDown = (e: React.PointerEvent, el: SlideElement) => {
    if (el.locked || editingId === el.id) return;
    e.stopPropagation();
    setSelectedId(el.id);
    const pt = stagePoint(e);
    pushUndo(slide);
    dragRef.current = {
      type: "move",
      id: el.id,
      startX: pt.x,
      startY: pt.y,
      orig: { x: el.x, y: el.y },
    };
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
  };

  const onHandlePointerDown = (e: React.PointerEvent, el: SlideElement, handle: string) => {
    e.stopPropagation();
    const pt = stagePoint(e);
    pushUndo(slide);
    dragRef.current = {
      type: "resize",
      id: el.id,
      handle,
      startX: pt.x,
      startY: pt.y,
      orig: { x: el.x, y: el.y, w: el.w, h: el.h },
    };
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
  };

  const onRotatePointerDown = (e: React.PointerEvent, el: SlideElement) => {
    e.stopPropagation();
    const pt = stagePoint(e);
    const cx = el.x + el.w / 2;
    const cy = el.y + el.h / 2;
    pushUndo(slide);
    dragRef.current = {
      type: "rotate",
      id: el.id,
      cx,
      cy,
      startAngle: (Math.atan2(pt.y - cy, pt.x - cx) * 180) / Math.PI,
      origRotate: el.rotate ?? 0,
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
    [elements, stagePoint, applyPatch],
  );

  const onPointerUp = useCallback(() => {
    dragRef.current = null;
  }, []);

  // ── Element ops ──

  const addElement = (partial: Partial<SlideElement> & { kind: SlideElement["kind"] }) => {
    if ((slide.elements?.length ?? 0) >= MAX_ELEMENTS && freeForm) {
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
    const freed = freeSlide(slide, theme);
    applyElements([...(freed.elements ?? []), el]);
    setSelectedId(el.id);
  };

  const addImageFile = (file: File | undefined) => {
    if (!file || !file.type.startsWith("image/")) return;
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result !== "string") return;
      const img = new Image();
      img.onload = () => {
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
        const freed = freeSlide(slide, theme);
        applyElements([...(freed.elements ?? []), el]);
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
    const freed = freeSlide(slide, theme);
    const list = freed.elements ?? [];
    const idx = list.findIndex((x) => x.id === selected.id);
    const next = [...list];
    next.splice(idx + 1, 0, copy);
    applyElements(next);
    setSelectedId(copy.id);
  };

  const removeElement = () => {
    if (!selected) return;
    const freed = freeSlide(slide, theme);
    applyElements((freed.elements ?? []).filter((x) => x.id !== selected.id));
    setSelectedId(null);
  };

  const moveZ = (dir: -1 | 1) => {
    if (!selected) return;
    const freed = freeSlide(slide, theme);
    const list = [...(freed.elements ?? [])];
    const idx = list.findIndex((x) => x.id === selected.id);
    const to = idx + dir;
    if (to < 0 || to >= list.length) return;
    const [moved] = list.splice(idx, 1);
    list.splice(to, 0, moved!);
    applyElements(list);
  };

  /** Inline text commit (blur / Escape). */
  const commitText = (el: SlideElement & { text: string }, value: string) => {
    setEditingId(null);
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
          applyPatch(selected.id, { x: selected.x - (e.shiftKey ? 10 : 1) });
          break;
        case "ArrowRight":
          applyPatch(selected.id, { x: selected.x + (e.shiftKey ? 10 : 1) });
          break;
        case "ArrowUp":
          applyPatch(selected.id, { y: selected.y - (e.shiftKey ? 10 : 1) });
          break;
        case "ArrowDown":
          applyPatch(selected.id, { y: selected.y + (e.shiftKey ? 10 : 1) });
          break;
        case "Escape":
          setSelectedId(null);
          break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, editingId, applyPatch]);

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

  return (
    <div className="flex flex-col gap-2">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-1.5">
        <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => addElement({ kind: "text" })}>
          <Type className="mr-1.5 h-3.5 w-3.5" />
          {t("pptx.stage.add_text")}
        </Button>
        <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => addElement({ kind: "rect" })}>
          <Square className="mr-1.5 h-3.5 w-3.5" />
          {t("pptx.stage.add_shape")}
        </Button>
        <label className="flex h-11 shrink-0 cursor-pointer items-center gap-1.5 rounded-md border px-2.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground sm:h-8">
          <ImagePlus className="h-3.5 w-3.5" />
          <span>{t("pptx.stage.add_image")}</span>
          <input
            type="file"
            accept="image/*"
            className="hidden"
            onChange={(e) => {
              addImageFile(e.target.files?.[0]);
              e.target.value = "";
            }}
          />
        </label>
        <div className="ml-auto flex items-center gap-0.5">
          <Button variant="ghost" size="icon-sm" aria-label={t("pptx.stage.undo")} disabled={undoStack.current.length === 0} onClick={undo} className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8">
            <Undo2 className="h-3.5 w-3.5" />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={t("pptx.stage.redo")} disabled={redoStack.current.length === 0} onClick={redo} className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8">
            <Redo2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Stage */}
      <div
        ref={wrapRef}
        className="relative w-full overflow-hidden rounded-md border bg-muted"
        style={{ aspectRatio: `${STAGE_W} / ${STAGE_H}` }}
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
            transform: `scale(${scale})`,
            transformOrigin: "top left",
            backgroundColor: theme.background,
            color: theme.foreground,
          }}
        >
          {elements.map((el) => {
            const isSelected = el.id === selectedId && freeForm;
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
                    if (el.kind === "text" && freeForm && !el.locked) setEditingId(el.id);
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
                        if (e.key === "Escape") commitText(el, (e.target as HTMLTextAreaElement).value);
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

      {/* Element toolbar + inspector */}
      {selected && freeForm && (
        <div className="flex flex-col gap-3 rounded-md border bg-muted/20 p-3">
          <div className="flex flex-wrap items-center gap-1">
            <span className="mr-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {t(`pptx.stage.kind_${selected.kind}`)}
            </span>
            <Button variant="ghost" size="icon-sm" aria-label={t("pptx.stage.z_up")} onClick={() => moveZ(1)} className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8">
              <ArrowUp className="h-3.5 w-3.5" />
            </Button>
            <Button variant="ghost" size="icon-sm" aria-label={t("pptx.stage.z_down")} onClick={() => moveZ(-1)} className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8">
              <ArrowDown className="h-3.5 w-3.5" />
            </Button>
            <Button variant="ghost" size="icon-sm" aria-label={t("pptx.duplicate")} onClick={duplicateElement} className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8">
              <Copy className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={selected.locked ? t("pptx.stage.unlock") : t("pptx.stage.lock")}
              onClick={() => applyPatch(selected.id, { locked: !selected.locked })}
              className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8"
            >
              {selected.locked ? <Lock className="h-3.5 w-3.5" /> : <LockOpen className="h-3.5 w-3.5" />}
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("pptx.stage.delete")}
              onClick={removeElement}
              disabled={selected.locked}
              className="min-h-11 min-w-11 text-destructive hover:text-destructive sm:min-h-8 sm:min-w-8"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
            {selected.kind === "text" && (
              <div className="ml-2 flex items-center gap-1">
                <Button
                  variant={selected.bold ? "secondary" : "ghost"}
                  size="icon-sm"
                  aria-label={t("pptx.stage.bold")}
                  onClick={() => applyPatch(selected.id, { bold: !selected.bold } as Partial<SlideElement>)}
                  className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8"
                >
                  <Bold className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant={selected.italic ? "secondary" : "ghost"}
                  size="icon-sm"
                  aria-label={t("pptx.stage.italic")}
                  onClick={() => applyPatch(selected.id, { italic: !selected.italic } as Partial<SlideElement>)}
                  className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8"
                >
                  <Italic className="h-3.5 w-3.5" />
                </Button>
              </div>
            )}
          </div>

          {/* Inspector grid */}
          <div className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
            <NumField label={t("pptx.stage.x")}>
              <Input type="number" value={Math.round(selected.x)} onChange={(e) => applyPatch(selected.id, { x: Number(e.target.value) || 0 })} className="h-8 text-base md:text-sm" />
            </NumField>
            <NumField label={t("pptx.stage.y")}>
              <Input type="number" value={Math.round(selected.y)} onChange={(e) => applyPatch(selected.id, { y: Number(e.target.value) || 0 })} className="h-8 text-base md:text-sm" />
            </NumField>
            <NumField label={t("pptx.stage.w")}>
              <Input type="number" value={Math.round(selected.w)} min={24} onChange={(e) => applyPatch(selected.id, { w: Math.max(24, Number(e.target.value) || 24) })} className="h-8 text-base md:text-sm" />
            </NumField>
            <NumField label={t("pptx.stage.h")}>
              <Input type="number" value={Math.round(selected.h)} min={20} onChange={(e) => applyPatch(selected.id, { h: Math.max(20, Number(e.target.value) || 20) })} className="h-8 text-base md:text-sm" />
            </NumField>

            {selected.kind === "text" && (
              <>
                <div className="flex items-center gap-1.5">
                  <Label className="w-16 shrink-0 text-xs">{t("pptx.stage.font")}</Label>
                  <Select
                    value={selected.fontFamily ?? (selected.font === "heading" ? "__heading" : "__body")}
                    onValueChange={(v) =>
                      applyPatch(selected.id, {
                        ...(v === "__heading" || v === "__body"
                          ? { font: v === "__heading" ? "heading" : "body", fontFamily: undefined }
                          : { fontFamily: v }),
                      } as Partial<SlideElement>)
                    }
                  >
                    <SelectTrigger className="h-8 text-base md:text-sm" aria-label={t("pptx.stage.font")}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__heading">{t("pptx.stage.font_heading")}</SelectItem>
                      <SelectItem value="__body">{t("pptx.stage.font_body")}</SelectItem>
                      {SAFE_FONTS.map((f) => (
                        <SelectItem key={f} value={f}>
                          {f}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <NumField label={t("pptx.stage.size")}>
                  <Input
                    type="number"
                    value={selected.size}
                    min={8}
                    max={300}
                    onChange={(e) => applyPatch(selected.id, { size: Math.max(8, Number(e.target.value) || 8) } as Partial<SlideElement>)}
                    className="h-8 text-base md:text-sm"
                  />
                </NumField>
                <div className="flex items-center gap-1.5">
                  <Label className="w-16 shrink-0 text-xs">{t("pptx.stage.color")}</Label>
                  <Input
                    type="color"
                    value={selected.color}
                    onChange={(e) => applyPatch(selected.id, { color: e.target.value } as Partial<SlideElement>)}
                    className="h-8 w-12 cursor-pointer p-0.5"
                    aria-label={t("pptx.stage.color")}
                  />
                </div>
                <div className="flex items-center gap-1.5">
                  <Label className="w-16 shrink-0 text-xs">{t("pptx.stage.align")}</Label>
                  <Select value={selected.align ?? "left"} onValueChange={(v) => applyPatch(selected.id, { align: v } as Partial<SlideElement>)}>
                    <SelectTrigger className="h-8 text-base md:text-sm" aria-label={t("pptx.stage.align")}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="left">{t("pptx.stage.align_left")}</SelectItem>
                      <SelectItem value="center">{t("pptx.stage.align_center")}</SelectItem>
                      <SelectItem value="right">{t("pptx.stage.align_right")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </>
            )}

            {(selected.kind === "rect" || selected.kind === "ellipse") && (
              <div className="flex items-center gap-1.5">
                <Label className="w-16 shrink-0 text-xs">{t("pptx.stage.fill")}</Label>
                <Input
                  type="color"
                  value={selected.fill}
                  onChange={(e) => applyPatch(selected.id, { fill: e.target.value } as Partial<SlideElement>)}
                  className="h-8 w-12 cursor-pointer p-0.5"
                  aria-label={t("pptx.stage.fill")}
                />
              </div>
            )}

            <div className="flex items-center gap-1.5">
              <Label className="w-16 shrink-0 text-xs">{t("pptx.stage.rotation")}</Label>
              <Input
                type="number"
                value={selected.rotate ?? 0}
                min={0}
                max={359}
                onChange={(e) => applyPatch(selected.id, { rotate: ((Number(e.target.value) || 0) % 360 + 360) % 360 })}
                className="h-8 text-base md:text-sm"
              />
              <RotateCw className="h-3 w-3 shrink-0 text-muted-foreground" />
            </div>
          </div>
        </div>
      )}

      {!freeForm && (
        <p className="text-xs text-muted-foreground">{t("pptx.stage.layout_hint")}</p>
      )}
    </div>
  );
}

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

function NumField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-1.5">
      <Label className="w-16 shrink-0 text-xs">{label}</Label>
      {children}
    </div>
  );
}
