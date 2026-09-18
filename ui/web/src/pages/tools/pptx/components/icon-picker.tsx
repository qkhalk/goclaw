import { useMemo, useState } from "react";
import { Ban, ChevronDown, Search } from "lucide-react";
import { cn } from "@/lib/utils";
import { Input } from "@/components/ui/input";
import { ICONS, getIcon } from "../lib/icon-library";

interface IconPickerProps {
  /** Currently selected icon name (undefined/null = none). */
  value?: string | null;
  /** Called with the picked name, or null when cleared. */
  onChange: (name: string | null) => void;
  /** Accessible label for the trigger button. */
  label: string;
  /** Placeholder for the search field. */
  searchPlaceholder: string;
  /** Label of the "no icon" option. */
  clearLabel: string;
  className?: string;
}

/** One line icon at a given pixel size (stroke inherits text color). */
export function IconGlyph({ name, size = 20, strokeWidth = 2 }: { name: string; size?: number; strokeWidth?: number }) {
  const icon = getIcon(name);
  if (!icon) return null;
  return (
    <svg
      aria-hidden
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {icon.paths.map((d, i) => (
        <path key={i} d={d} />
      ))}
    </svg>
  );
}

/**
 * Searchable grid picker over the studio's curated line-icon vocabulary
 * (lib/icon-library.ts). Renders as a trigger button with an inline
 * expandable panel — no portal, so it behaves inside dialogs and mobile
 * sheets alike. Grid buttons keep a ≥44px touch target.
 */
export function IconPicker({ value, onChange, label, searchPlaceholder, clearLabel, className }: IconPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return ICONS;
    return ICONS.filter((i) => i.name.includes(q.replace(/\s+/g, "-")));
  }, [query]);

  const current = getIcon(value);

  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={label}
        aria-expanded={open}
        title={label}
        className={cn(
          "flex h-11 w-11 shrink-0 items-center justify-center rounded-md border transition-colors sm:h-9 sm:w-9",
          current ? "border-primary/50 text-primary" : "border-border text-muted-foreground hover:text-foreground",
          open && "bg-accent",
        )}
      >
        {current ? <IconGlyph name={current.name} size={18} /> : <Ban className="h-4 w-4" />}
      </button>

      {open && (
        <div className="pptx-enter-soft flex w-64 flex-col gap-2 rounded-lg border bg-background p-2 shadow-md">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={searchPlaceholder}
              className="h-9 pl-7 text-base sm:text-sm"
            />
          </div>
          <div className="grid max-h-44 grid-cols-5 gap-1 overflow-y-auto overscroll-contain">
            <button
              type="button"
              onClick={() => {
                onChange(null);
                setOpen(false);
              }}
              aria-label={clearLabel}
              title={clearLabel}
              className={cn(
                "flex h-11 w-11 items-center justify-center rounded-md border text-muted-foreground transition-colors hover:bg-accent",
                !value && "border-primary",
              )}
            >
              <Ban className="h-4 w-4" />
            </button>
            {filtered.map((icon) => (
              <button
                key={icon.name}
                type="button"
                onClick={() => {
                  onChange(icon.name);
                  setOpen(false);
                }}
                aria-label={icon.name}
                title={icon.name}
                className={cn(
                  "flex h-11 w-11 items-center justify-center rounded-md border text-foreground transition-colors hover:bg-accent",
                  value === icon.name && "border-primary text-primary",
                )}
              >
                <IconGlyph name={icon.name} size={20} />
              </button>
            ))}
          </div>
          {filtered.length === 0 && (
            <p className="py-2 text-center text-xs text-muted-foreground">{clearLabel}</p>
          )}
          <button
            type="button"
            onClick={() => setOpen(false)}
            className="flex h-8 items-center justify-center gap-1 rounded text-xs text-muted-foreground transition-colors hover:text-foreground"
          >
            <ChevronDown className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
    </div>
  );
}
