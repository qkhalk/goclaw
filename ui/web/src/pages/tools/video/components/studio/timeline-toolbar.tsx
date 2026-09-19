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
}

/**
 * Slim strip between the canvas area and the timeline: undo/redo, add scene,
 * and the narration shortcut. Scene count and total duration live once, in
 * the top bar — no duplicate summary here.
 */
export function TimelineToolbar({
  canUndo,
  canRedo,
  onUndo,
  onRedo,
  onAddScene,
  onScrollToNarration,
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
        className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9 sm:min-w-9"
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
        className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9 sm:min-w-9"
      >
        <Redo2 className="h-4 w-4" />
      </Button>

      <span aria-hidden className="mx-1 h-5 w-px bg-white/10" />

      <Button
        variant="ghost"
        size="sm"
        onClick={onAddScene}
        className="min-h-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9"
      >
        <Plus className="h-4 w-4" />
        <span className="hidden sm:inline">{t("video.add_scene")}</span>
      </Button>
      <Button
        variant="ghost"
        size="sm"
        onClick={onScrollToNarration}
        className="min-h-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9"
        title={t("video.studio.toolbar.narration_hint")}
      >
        <Mic className="h-4 w-4" />
        <span className="hidden md:inline">{t("video.studio.toolbar.narration")}</span>
      </Button>
    </div>
  );
}
