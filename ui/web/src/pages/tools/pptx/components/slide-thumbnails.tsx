import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Deck } from "../types";
import { SlideView } from "./slide-view";

/**
 * PowerPoint-style slide thumbnail rail: a numbered vertical list (the deck
 * model has no sections — see types.ts) with the current slide highlighted.
 * Click to switch; the trailing dashed button adds a slide. `open` toggles
 * the rail (status bar / View ribbon) so small screens can reclaim the width.
 */

interface SlideThumbnailsProps {
  deck: Deck;
  selected: number;
  onSelect: (index: number) => void;
  onAdd: () => void;
  open: boolean;
}

export function SlideThumbnails({ deck, selected, onSelect, onAdd, open }: SlideThumbnailsProps) {
  const { t } = useTranslation("toolbox");

  return (
    <aside
      aria-label={t("pptx.ribbon.thumbnails")}
      className={cn(
        "w-[120px] shrink-0 flex-col border-r bg-card/30 sm:w-[156px]",
        open ? "flex" : "hidden",
      )}
    >
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-1.5 sm:p-2">
        <ul className="flex flex-col gap-1.5 sm:gap-2">
          {deck.slides.map((s, i) => (
            <li key={i}>
              <button
                type="button"
                onClick={() => onSelect(i)}
                aria-current={i === selected}
                title={t("pptx.layout." + s.layout)}
                className={cn(
                  "flex w-full items-start gap-1 rounded-md p-0.5 text-left transition-colors sm:p-1",
                  i === selected ? "bg-accent/50" : "hover:bg-accent/25",
                )}
              >
                <span className="w-3.5 shrink-0 pt-0.5 text-right text-[10px] leading-none tabular-nums text-muted-foreground sm:w-4">
                  {i + 1}
                </span>
                <span
                  className={cn(
                    "min-w-0 flex-1 overflow-hidden rounded-sm border-2",
                    i === selected ? "border-primary" : "border-transparent",
                  )}
                >
                  <SlideView slide={s} theme={deck.theme} />
                </span>
              </button>
            </li>
          ))}
        </ul>
        <button
          type="button"
          onClick={onAdd}
          disabled={deck.slides.length >= 40}
          className="mt-2 flex min-h-11 w-full items-center justify-center gap-1 rounded-md border border-dashed text-xs text-muted-foreground transition-colors hover:bg-accent disabled:opacity-40"
        >
          <Plus className="h-3.5 w-3.5" />
          <span className="truncate">{t("pptx.add_slide")}</span>
        </button>
      </div>
    </aside>
  );
}
