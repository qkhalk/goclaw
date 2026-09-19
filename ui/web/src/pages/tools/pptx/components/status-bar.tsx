import { useTranslation } from "react-i18next";
import { Notebook, PanelLeft, ZoomIn, ZoomOut } from "lucide-react";
import { Slider } from "@/components/ui/slider";
import { cn } from "@/lib/utils";
import { MAX_ZOOM, MIN_ZOOM } from "./interactive-stage";

/**
 * PowerPoint-style status bar: slide position, speaker-notes toggle, and the
 * zoom slider with the live percentage of the editing canvas (real CSS scale
 * — the same fraction the stage reports via onScaleChange). The thumbnail
 * rail toggle only shows on small screens where the rail is hidden to save
 * width.
 */

interface StatusBarProps {
  index: number;
  total: number;
  notesOpen: boolean;
  onToggleNotes: () => void;
  thumbsOpen: boolean;
  onToggleThumbs: () => void;
  zoom: number;
  onZoomChange: (zoom: number) => void;
  scalePct: number;
}

export function StatusBar({
  index,
  total,
  notesOpen,
  onToggleNotes,
  thumbsOpen,
  onToggleThumbs,
  zoom,
  onZoomChange,
  scalePct,
}: StatusBarProps) {
  const { t } = useTranslation("toolbox");

  return (
    <footer className="flex h-11 shrink-0 items-center gap-1 border-t bg-card/40 px-2 sm:h-8 sm:gap-2">
      <button
        type="button"
        onClick={onToggleThumbs}
        title={t("pptx.ribbon.thumbnails")}
        aria-label={t("pptx.ribbon.thumbnails")}
        aria-pressed={thumbsOpen}
        className={cn(
          "flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground md:hidden sm:h-8 sm:w-8",
          thumbsOpen && "bg-accent text-accent-foreground",
        )}
      >
        <PanelLeft className="h-4 w-4" aria-hidden />
      </button>
      <span className="whitespace-nowrap text-xs tabular-nums text-muted-foreground">
        {t("pptx.status.slide_of", { x: index + 1, y: total })}
      </span>
      <button
        type="button"
        onClick={onToggleNotes}
        title={t("pptx.ribbon.notes")}
        aria-label={t("pptx.ribbon.notes")}
        aria-pressed={notesOpen}
        className={cn(
          "flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground sm:h-8 sm:w-8",
          notesOpen && "bg-accent text-accent-foreground",
        )}
      >
        <Notebook className="h-4 w-4" aria-hidden />
      </button>

      <div className="ml-auto flex items-center gap-0.5 sm:gap-1.5">
        <button
          type="button"
          onClick={() => onZoomChange(Math.max(MIN_ZOOM, Math.round((zoom - 0.1) * 10) / 10))}
          disabled={zoom <= MIN_ZOOM}
          title={t("pptx.ribbon.zoom_out")}
          aria-label={t("pptx.ribbon.zoom_out")}
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40 sm:h-8 sm:w-8"
        >
          <ZoomOut className="h-4 w-4" aria-hidden />
        </button>
        <Slider
          value={[Math.round(zoom * 100)]}
          min={50}
          max={200}
          step={5}
          onValueChange={(v) => onZoomChange(Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, (v[0] ?? 100) / 100)))}
          className="w-20 sm:w-32"
          aria-label={t("pptx.ribbon.group_zoom")}
        />
        <button
          type="button"
          onClick={() => onZoomChange(Math.min(MAX_ZOOM, Math.round((zoom + 0.1) * 10) / 10))}
          disabled={zoom >= MAX_ZOOM}
          title={t("pptx.ribbon.zoom_in")}
          aria-label={t("pptx.ribbon.zoom_in")}
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40 sm:h-8 sm:w-8"
        >
          <ZoomIn className="h-4 w-4" aria-hidden />
        </button>
        <span className="w-10 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
          {scalePct}%
        </span>
      </div>
    </footer>
  );
}
