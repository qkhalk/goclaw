import { useTranslation } from "react-i18next";
import { Clapperboard, Download, ListVideo, Loader2, Wand2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { InlineEditText } from "@/components/ui/inline-edit-text";
import { cn } from "@/lib/utils";

/** mm:ss timecode used in the top bar summary chip. */
export function formatTopbarTime(totalSec: number): string {
  const m = Math.floor(totalSec / 60);
  const s = Math.floor(totalSec % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

interface StudioTopbarProps {
  /** Client-only project title (localStorage); empty = placeholder. */
  title: string;
  onTitleChange: (title: string) => void;
  sceneCount: number;
  totalSec: number;
  jobsCount: number;
  onOpenJobs: () => void;
  designerOpen: boolean;
  onToggleDesigner: () => void;
  /** Open the Render inspector tab (Filmora-style top-right Export). */
  onExport: () => void;
  exportDisabled: boolean;
  isExporting: boolean;
}

/**
 * Filmora-style studio top bar: brand mark + editable project title on the
 * left, jobs / designer / Export on the right. Pure chrome — all state and
 * handlers live in the page.
 */
export function StudioTopbar({
  title,
  onTitleChange,
  sceneCount,
  totalSec,
  jobsCount,
  onOpenJobs,
  designerOpen,
  onToggleDesigner,
  onExport,
  exportDisabled,
  isExporting,
}: StudioTopbarProps) {
  const { t } = useTranslation("toolbox");

  return (
    <header className="flex h-14 shrink-0 items-center gap-3 border-b border-white/[0.06] bg-[#1b1d23] px-3 sm:px-4">
      {/* Brand mark + project title */}
      <span
        aria-hidden
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-rose-500 to-orange-500 text-white shadow-sm"
      >
        <Clapperboard className="h-4 w-4" />
      </span>
      <div className="flex min-w-0 flex-col">
        <InlineEditText
          value={title}
          onSave={onTitleChange}
          placeholder={t("video.studio.untitled_project")}
          emptyLabel={t("video.studio.untitled_project")}
          maxLength={80}
          minLength={0}
          className="font-semibold"
          wrapperClassName="min-w-0"
          ariaLabel={t("video.studio.project_title")}
        />
        <span className="text-[11px] leading-tight text-zinc-500">
          {t("video.scenes_count", { n: sceneCount })} ·{" "}
          {formatTopbarTime(totalSec)}
        </span>
      </div>

      <div className="ml-auto flex shrink-0 items-center gap-1.5 sm:gap-2">
        {/* Render jobs */}
        <Button
          variant="ghost"
          size="sm"
          onClick={onOpenJobs}
          className="relative min-h-11 px-2.5 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9"
          title={t("video.jobs_title")}
        >
          <ListVideo className="h-4 w-4 sm:mr-1.5" />
          <span className="hidden sm:inline">{t("video.jobs_title")}</span>
          {jobsCount > 0 && (
            <Badge
              variant="secondary"
              className="ml-1 hidden h-5 min-w-5 px-1.5 tabular-nums sm:inline-flex"
            >
              {jobsCount}
            </Badge>
          )}
        </Button>

        {/* AI designer toggle */}
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleDesigner}
          aria-pressed={designerOpen}
          className={cn(
            "min-h-11 px-2.5 sm:min-h-9",
            designerOpen
              ? "bg-primary/15 text-primary hover:bg-primary/20 hover:text-primary"
              : "text-zinc-300 hover:bg-white/5 hover:text-white",
          )}
          title={t("video.designer.toggle")}
        >
          <Wand2 className="h-4 w-4 sm:mr-1.5" />
          <span className="hidden md:inline">{t("video.designer.title")}</span>
        </Button>

        {/* Export (opens the Render inspector tab) */}
        <Button
          size="sm"
          onClick={onExport}
          disabled={exportDisabled && !isExporting}
          className="min-h-11 bg-gradient-to-r from-rose-500 to-orange-500 px-3 text-white hover:from-rose-400 hover:to-orange-400 sm:min-h-9"
        >
          {isExporting ? (
            <Loader2 className="h-4 w-4 animate-spin sm:mr-1.5" />
          ) : (
            <Download className="h-4 w-4 sm:mr-1.5" />
          )}
          <span className="hidden sm:inline">{t("video.studio.export")}</span>
        </Button>
      </div>
    </header>
  );
}
