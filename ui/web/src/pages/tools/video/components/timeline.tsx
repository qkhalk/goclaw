import { useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Undo2, Redo2, GripVertical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { Scene } from "../hooks/use-timeline";

// ── Types ──

interface TimelineProps {
  scenes: Scene[];
  selectedIndex: number;
  onSelect: (index: number) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
  onMove: (from: number, to: number) => void;
  canUndo: boolean;
  canRedo: boolean;
  onUndo: () => void;
  onRedo: () => void;
}

// ── Scene thumbnail ──

function SceneThumb({ scene, index }: { scene: Scene; index: number }) {
  if (scene.type === "color") {
    return (
      <div
        className="h-full w-full rounded"
        style={{ backgroundColor: scene.color || "#000" }}
      />
    );
  }

  if (scene.source) {
    return (
      <img
        src={scene.source}
        alt={`Scene ${index + 1}`}
        className="h-full w-full rounded object-cover"
        crossOrigin="anonymous"
        onError={(e) => {
          (e.target as HTMLImageElement).style.display = "none";
        }}
      />
    );
  }

  return (
    <div className="flex h-full w-full items-center justify-center rounded bg-muted text-xs text-muted-foreground">
      {index + 1}
    </div>
  );
}

// ── Component ──

export function Timeline({
  scenes,
  selectedIndex,
  onSelect,
  onAdd,
  canUndo,
  canRedo,
  onUndo,
  onRedo,
}: TimelineProps) {
  const { t } = useTranslation("toolbox");
  const scrollRef = useRef<HTMLDivElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);

  // Auto-scroll to selected scene
  useEffect(() => {
    selectedRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "nearest",
      inline: "center",
    });
  }, [selectedIndex]);

  return (
    <div className="flex flex-col gap-2">
      {/* Header */}
      <div className="flex items-center justify-between px-1">
        <span className="text-sm font-medium">{t("video.timeline.title")}</span>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onUndo}
            disabled={!canUndo}
            aria-label={t("video.timeline.undo")}
            className="min-h-9 min-w-9"
          >
            <Undo2 className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onRedo}
            disabled={!canRedo}
            aria-label={t("video.timeline.redo")}
            className="min-h-9 min-w-9"
          >
            <Redo2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Scrollable scene strip */}
      <div
        ref={scrollRef}
        className="flex gap-2 overflow-x-auto pb-1 overscroll-contain"
      >
        {scenes.map((scene, i) => {
          const isSelected = i === selectedIndex;
          const totalDur = scenes.reduce((a, s) => a + (Number(s.duration_sec) || 0), 0);
          const sceneDur = Number(scene.duration_sec) || 0;
          const widthPct = totalDur > 0 ? Math.max(80, (sceneDur / totalDur) * 200) : 80;

          return (
            <button
              key={i}
              ref={isSelected ? selectedRef : undefined}
              type="button"
              onClick={() => onSelect(i)}
              className={cn(
                "group relative flex-shrink-0 flex flex-col gap-1 rounded-lg border-2 p-1 transition-all cursor-pointer",
                "hover:border-primary/50",
                isSelected
                  ? "border-primary bg-primary/5 shadow-sm"
                  : "border-transparent bg-muted/30",
              )}
              style={{ width: widthPct, minWidth: 80 }}
            >
              {/* Thumbnail */}
              <div className="relative h-12 w-full overflow-hidden rounded">
                <SceneThumb scene={scene} index={i} />
                {/* Drag handle (visual only) */}
                <div className="absolute left-0.5 top-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                  <GripVertical className="h-3 w-3 text-white drop-shadow" />
                </div>
              </div>

              {/* Label + duration */}
              <div className="flex items-center justify-between px-0.5">
                <span className="truncate text-[10px] font-medium">
                  {t("video.scene_n", { n: i + 1 })}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {sceneDur}s
                </span>
              </div>
            </button>
          );
        })}

        {/* Add scene button */}
        <button
          type="button"
          onClick={onAdd}
          className={cn(
            "flex flex-shrink-0 flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed p-1",
            "border-muted-foreground/30 hover:border-primary/50 hover:bg-primary/5 transition-all cursor-pointer",
          )}
          style={{ width: 80, minWidth: 80 }}
        >
          <div className="flex h-12 w-full items-center justify-center rounded bg-muted/30">
            <Plus className="h-4 w-4 text-muted-foreground" />
          </div>
          <span className="text-[10px] text-muted-foreground">
            {t("video.add_scene")}
          </span>
        </button>
      </div>

      {/* Drag reorder hint */}
      <div className="flex items-center gap-2 px-1 text-[10px] text-muted-foreground">
        <span>
          {t("video.scenes_count", { n: scenes.length })} ·{" "}
          {t("video.total_duration", {
            sec: scenes
              .reduce((a, s) => a + (Number(s.duration_sec) || 0), 0)
              .toFixed(1),
          })}
        </span>
      </div>
    </div>
  );
}
