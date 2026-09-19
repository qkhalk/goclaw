import { useEffect, useRef, useState } from "react";
import type { Ref } from "react";
import { useTranslation } from "react-i18next";
import {
  AlignCenter,
  AlignLeft,
  AlignRight,
  ArrowDown,
  ArrowUp,
  Bold,
  Copy,
  Italic,
  Minus,
  Plus,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import type { DeckTheme, SlideElement } from "../types";

/**
 * Floating one-row context toolbar for the selected slide element, anchored
 * just above the selection by the interactive stage (which owns the stage
 * math). Direct-manipulation only: every control patches the element in
 * place — there is no inspector panel. Color opens a small popover with
 * theme swatches plus the native picker.
 */

interface ElementToolbarProps {
  el: SlideElement;
  theme: DeckTheme;
  /** Parent supplies measurement + positioning. */
  ref?: Ref<HTMLDivElement>;
  onPatch: (patch: Partial<SlideElement>) => void;
  onMoveZ: (dir: -1 | 1) => void;
  onDuplicate: () => void;
  onDelete: () => void;
}

const TOOLBTN =
  "min-h-11 min-w-11 sm:min-h-8 sm:min-w-8 text-muted-foreground hover:text-foreground";

export function ElementToolbar({ el, theme, ref, onPatch, onMoveZ, onDuplicate, onDelete }: ElementToolbarProps) {
  const { t } = useTranslation("toolbox");

  const align = el.kind === "text" ? (el.align ?? "left") : "left";
  const cycleAlign = () => {
    if (el.kind !== "text") return;
    onPatch({ align: align === "left" ? "center" : align === "center" ? "right" : "left" } as Partial<SlideElement>);
  };
  const AlignIcon = align === "center" ? AlignCenter : align === "right" ? AlignRight : AlignLeft;

  const isText = el.kind === "text";
  const isShape = el.kind === "rect" || el.kind === "ellipse";
  const colorValue = isText ? el.color : isShape ? el.fill : el.kind === "frame" ? el.color : undefined;
  const colorLabel = isText ? t("pptx.stage.color") : t("pptx.stage.fill");
  const patchColor = (v: string) => {
    if (isText) onPatch({ color: v } as Partial<SlideElement>);
    else if (isShape) onPatch({ fill: v } as Partial<SlideElement>);
    else if (el.kind === "frame") onPatch({ color: v } as Partial<SlideElement>);
  };

  return (
    <div
      ref={ref}
      // stopPropagation: the toolbar floats over the canvas; clicks here must
      // never reach the slide's deselect handler. Positioning (left/top) and
      // the overflow clamp live on the wrapper the stage renders.
      onPointerDown={(e) => e.stopPropagation()}
      className="flex w-max items-center gap-0.5 rounded-md border bg-popover/95 p-0.5 shadow-md backdrop-blur"
    >
      {isText && (
        <>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("pptx.stage.size")}
            title={t("pptx.stage.size")}
            className={cn(TOOLBTN, "shrink-0")}
            onClick={() => onPatch({ size: fontSizeStep(el.size, -1) } as Partial<SlideElement>)}
          >
            <Minus className="h-3.5 w-3.5" />
          </Button>
          <SizeInput label={t("pptx.stage.size")} value={el.size} onCommit={(v) => onPatch({ size: v } as Partial<SlideElement>)} />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("pptx.stage.size")}
            title={t("pptx.stage.size")}
            className={cn(TOOLBTN, "shrink-0")}
            onClick={() => onPatch({ size: fontSizeStep(el.size, 1) } as Partial<SlideElement>)}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
          <Separator />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("pptx.stage.bold")}
            title={t("pptx.stage.bold")}
            aria-pressed={!!el.bold}
            className={cn(TOOLBTN, "shrink-0", el.bold && "bg-accent text-accent-foreground hover:text-accent-foreground")}
            onClick={() => onPatch({ bold: !el.bold } as Partial<SlideElement>)}
          >
            <Bold className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("pptx.stage.italic")}
            title={t("pptx.stage.italic")}
            aria-pressed={!!el.italic}
            className={cn(TOOLBTN, "shrink-0", el.italic && "bg-accent text-accent-foreground hover:text-accent-foreground")}
            onClick={() => onPatch({ italic: !el.italic } as Partial<SlideElement>)}
          >
            <Italic className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t("pptx.stage.align")}
            title={t("pptx.stage.align")}
            className={cn(TOOLBTN, "shrink-0")}
            onClick={cycleAlign}
          >
            <AlignIcon className="h-3.5 w-3.5" />
          </Button>
        </>
      )}

      {colorValue !== undefined && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={colorLabel}
              title={colorLabel}
              className={cn(TOOLBTN, "shrink-0")}
            >
              <span
                className="h-4 w-4 rounded-sm border border-border"
                style={{ backgroundColor: colorValue }}
                aria-hidden
              />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-40 p-1.5">
            <div className="grid grid-cols-4 gap-1">
              {[theme.foreground, theme.muted, theme.accent, theme.background, "#ffffff", "#000000"].map((c) => (
                <DropdownMenuItem key={c} asChild>
                  <button
                    type="button"
                    aria-label={c}
                    className="h-9 w-full cursor-pointer rounded-sm border border-border transition-colors hover:ring-2 hover:ring-ring"
                    style={{ backgroundColor: c }}
                    onClick={() => patchColor(c)}
                  />
                </DropdownMenuItem>
              ))}
            </div>
            <label className="mt-1 flex cursor-pointer items-center justify-between gap-2 rounded-sm px-1 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-accent-foreground">
              {t("pptx.stage.color")}
              <input
                type="color"
                value={/^#[0-9a-fA-F]{6}$/.test(colorValue) ? colorValue : "#000000"}
                onChange={(e) => patchColor(e.target.value)}
                className="h-6 w-9 cursor-pointer rounded-sm border border-border bg-transparent p-0.5"
                aria-label={t("pptx.stage.color")}
              />
            </label>
          </DropdownMenuContent>
        </DropdownMenu>
      )}

      {(isText || colorValue !== undefined) && <Separator />}

      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t("pptx.stage.z_up")}
        title={t("pptx.stage.z_up")}
        className={cn(TOOLBTN, "shrink-0")}
        onClick={() => onMoveZ(1)}
      >
        <ArrowUp className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t("pptx.stage.z_down")}
        title={t("pptx.stage.z_down")}
        className={cn(TOOLBTN, "shrink-0")}
        onClick={() => onMoveZ(-1)}
      >
        <ArrowDown className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t("pptx.duplicate")}
        title={t("pptx.duplicate")}
        className={cn(TOOLBTN, "shrink-0")}
        onClick={onDuplicate}
      >
        <Copy className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t("pptx.stage.delete")}
        title={t("pptx.stage.delete")}
        className={cn(TOOLBTN, "shrink-0 text-destructive hover:text-destructive")}
        onClick={onDelete}
      >
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

/** Font-size step for the −/+ buttons: finer steps for small text. */
function fontSizeStep(size: number, dir: -1 | 1): number {
  const step = size < 24 ? 2 : size < 60 ? 4 : 8;
  return Math.min(300, Math.max(8, size + dir * step));
}

/** Hairline divider between toolbar clusters. */
function Separator() {
  return <span className="mx-0.5 h-5 w-px shrink-0 bg-border" aria-hidden />;
}

/** Compact numeric readout for the font size; typing commits on blur or
 * Enter so partial input never fights the clamp. */
function SizeInput({ label, value, onCommit }: { label: string; value: number; onCommit: (v: number) => void }) {
  const [draft, setDraft] = useState(String(value));
  const focused = useRef(false);
  useEffect(() => {
    if (!focused.current) setDraft(String(value));
  }, [value]);

  const commit = () => {
    focused.current = false;
    if (draft === "") {
      setDraft(String(value));
      return;
    }
    const n = Number(draft);
    if (Number.isFinite(n)) onCommit(Math.min(300, Math.max(8, Math.round(n))));
    else setDraft(String(value));
  };

  return (
    <input
      type="text"
      inputMode="numeric"
      value={draft}
      aria-label={label}
      title={label}
      onChange={(e) => setDraft(e.target.value.replace(/[^\d]/g, ""))}
      onFocus={() => (focused.current = true)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          (e.target as HTMLInputElement).blur();
        }
        e.stopPropagation();
      }}
      className="h-11 w-10 shrink-0 rounded-sm bg-transparent text-center text-base tabular-nums text-foreground outline-none focus:bg-accent sm:h-8 sm:text-sm"
    />
  );
}
