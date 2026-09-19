import { useTranslation } from "react-i18next";
import { Mic, Plus, Redo2, Undo2 } from "lucide-react";
import { Button } from "@/components/ui/button";

interface TimelineToolbarProps {
  canUndo: boolean;
  canRedo: boolean;
  onUndo: () => void;
  onRedo: () => void;
  onAddScene: () => void;
  /** Scroll the scene editor to the narration field (mic shortcut). */
  onScrollToNarration: () => void;
  sceneCount: number;
  totalSec: number;
}

/**
 * Horizontal strip between the canvas area and the timeline: undo/redo,
 * add scene, narration shortcut, and a right-aligned time summary. Every
 * button is wired to an existing handler — no dead controls.
 */
export function TimelineToolbar({
  canUndo,
  canRedo,
  onUndo,
  onRedo,
  onAddScene,
  onScrollToNarration,
  sceneCount,
  totalSec,
}: TimelineToolbarProps) {
  const { t } = useTranslation("toolbox");

  return (
    <div className="flex items-center gap-1 border-y border-white/[0.06] bg-[#1b1d23] px-2 py-1.5">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onUndo}
        disabled={!canUndo}
        aria-label={t("video.timeline.undo")}
        title={t("video.timeline.undo")}
        className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-8 sm:min-w-8"
      >
        <Undo2 className="h-4 w-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onRedo}
        disabled={!canRedo}
        aria-label={t("video.timeline.redo")}
        title={t("video.timeline.redo")}
        className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-8 sm:min-w-8"
      >
        <Redo2 className="h-4 w-4" />
      </Button>

      <span aria-hidden className="mx-1 h-5 w-px bg-white/10" />

      <Button
        variant="ghost"
        size="sm"
        onClick={onAddScene}
        className="min-h-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-8"
      >
        <Plus className="h-4 w-4" />
        <span className="hidden sm:inline">{t("video.add_scene")}</span>
      </Button>
      <Button
        variant="ghost"
        size="sm"
        onClick={onScrollToNarration}
        className="min-h-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-8"
        title={t("video.studio.toolbar.narration_hint")}
      >
        <Mic className="h-4 w-4" />
        <span className="hidden md:inline">{t("video.studio.toolbar.narration")}</span>
      </Button>

      <span className="ml-auto whitespace-nowrap pr-1 font-mono text-[11px] tabular-nums text-zinc-500">
        {t("video.scenes_count", { n: sceneCount })} ·{" "}
        {t("video.total_duration", { sec: totalSec.toFixed(1) })}
      </span>
    </div>
  );
}
