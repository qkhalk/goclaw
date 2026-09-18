import { useTranslation } from "react-i18next";
import { LayoutTemplate } from "lucide-react";
import { getIconDef } from "../lib/icon-library";
import { STORYBOARD_TEMPLATES, type StoryboardTemplate } from "../lib/templates";
import type { Scene } from "../hooks/use-timeline";

interface TemplateGalleryProps {
  /** Load the template's scenes into the timeline (one undo step). */
  onApply: (scenes: Scene[]) => void;
}

/**
 * Horizontal strip of ready-made storyboard presets. Each card previews its
 * palette (gradient swatches) and glyph set so the result is guessable
 * before applying; applying is one undo step in the timeline.
 */
export function TemplateGallery({ onApply }: TemplateGalleryProps) {
  const { t } = useTranslation("toolbox");

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2 px-1">
        <LayoutTemplate className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden />
        <span className="text-sm font-medium">{t("video.templates.title")}</span>
        <span className="truncate text-xs text-muted-foreground">
          {t("video.templates.hint")}
        </span>
      </div>
      <div className="flex gap-2 overflow-x-auto pb-1 overscroll-contain">
        {STORYBOARD_TEMPLATES.map((tpl) => (
          <TemplateCard key={tpl.key} template={tpl} onApply={onApply} />
        ))}
      </div>
    </div>
  );
}

function TemplateCard({
  template,
  onApply,
}: {
  template: StoryboardTemplate;
  onApply: (scenes: Scene[]) => void;
}) {
  const { t } = useTranslation("toolbox");
  const totalSec = template.scenes.reduce(
    (acc, s) => acc + (Number(s.duration_sec) || 0),
    0,
  );
  // First three scene backgrounds summarize the look.
  const preview = template.scenes.slice(0, 3);

  return (
    <button
      type="button"
      onClick={() => onApply(template.scenes)}
      title={t(`video.templates.${template.key}`)}
      className="group w-36 shrink-0 overflow-hidden rounded-lg border text-left transition-[border-color,transform] duration-200 hover:-translate-y-0.5 active:scale-[0.98] border-border hover:border-primary/50"
    >
      <div className="flex h-14">
        {preview.map((s, i) => {
          const def = s.type === "icon" ? getIconDef(s.icon?.name) : undefined;
          return (
            <div
              key={i}
              className="relative flex flex-1 items-center justify-center"
              style={{
                background: s.gradient
                  ? `linear-gradient(135deg, ${s.gradient.from}, ${s.gradient.to})`
                  : (s.color ?? "#000000"),
              }}
            >
              {def && (
                <svg
                  viewBox="0 0 24 24"
                  className="h-4 w-4"
                  fill="none"
                  stroke={s.icon?.color || "#ffffff"}
                  strokeWidth={2}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden
                >
                  {def.d.map((d, j) => (
                    <path key={j} d={d} />
                  ))}
                </svg>
              )}
              {s.type === "image" && (
                <span className="text-[10px] text-white/80">{i + 1}</span>
              )}
            </div>
          );
        })}
      </div>
      <div className="border-t px-2 py-1.5">
        <p className="truncate text-xs font-medium transition-colors group-hover:text-foreground">
          {t(`video.templates.${template.key}`)}
        </p>
        <p className="text-[10px] tabular-nums text-muted-foreground">
          {t("video.scenes_count", { n: template.scenes.length })} ·{" "}
          {totalSec.toFixed(0)}s
        </p>
      </div>
    </button>
  );
}
