import { useTranslation } from "react-i18next";
import { ICON_LIBRARY } from "../lib/icon-library";
import { cn } from "@/lib/utils";

interface IconPickerProps {
  value: string | undefined;
  onChange: (name: string) => void;
}

/**
 * Grid picker for icon scenes: the bundled 24x24 line icons rendered as
 * inline SVG (same path data the canvas draws). Touch targets are 44px on
 * mobile, 28px on desktop grids.
 */
export function IconPicker({ value, onChange }: IconPickerProps) {
  const { t } = useTranslation("toolbox");

  return (
    <div
      role="listbox"
      aria-label={t("video.icon")}
      className="grid grid-cols-6 gap-1 sm:grid-cols-8"
    >
      {ICON_LIBRARY.map((icon) => {
        const selected = icon.name === value;
        return (
          <button
            key={icon.name}
            type="button"
            role="option"
            aria-selected={selected}
            aria-label={icon.name}
            title={icon.name}
            onClick={() => onChange(icon.name)}
            className={cn(
              "flex aspect-square min-h-11 items-center justify-center rounded-md border transition-colors sm:min-h-7",
              selected
                ? "border-primary bg-primary/10 text-primary"
                : "border-transparent bg-muted/40 text-foreground/80 hover:border-border hover:text-foreground",
            )}
          >
            <svg
              viewBox="0 0 24 24"
              className="h-4 w-4 sm:h-3.5 sm:w-3.5"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden
            >
              {icon.d.map((d, i) => (
                <path key={i} d={d} />
              ))}
            </svg>
          </button>
        );
      })}
    </div>
  );
}
