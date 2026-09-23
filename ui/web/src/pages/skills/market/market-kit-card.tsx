import { useTranslation } from "react-i18next";
import { ChevronRight, Download, Layers, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { MarketKit } from "../hooks/use-skill-market";

interface MarketKitCardProps {
  kit: MarketKit;
  /** Parent kit with sub-kits — rendered as the wide featured card. */
  featured?: boolean;
  className?: string;
  /** True when a kit install is in flight for this kit. */
  busy: boolean;
  /** Any other action in flight — disables this card's buttons. */
  disabled: boolean;
  selected: boolean;
  canManage: boolean;
  onSelect: (slug: string) => void;
  onInstallMissing: (kit: MarketKit) => void;
}

function ProgressBar({ value, total }: { value: number; total: number }) {
  const pct = total > 0 ? Math.min(100, Math.round((value / total) * 100)) : 0;
  return (
    <div
      className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={total}
    >
      <div
        className="h-full rounded-full bg-primary transition-[width] duration-300"
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

/** A bundled skill kit card. Kits are the market's navigation layer: click to
 *  narrow the table below to the kit's skills; the button installs every
 *  not-yet-installed kit skill. Featured (parent) kits show their sub-kits. */
export function MarketKitCard({
  kit,
  featured = false,
  className,
  busy,
  disabled,
  selected,
  canManage,
  onSelect,
  onInstallMissing,
}: MarketKitCardProps) {
  const { t } = useTranslation("skills");
  const missing = kit.skills.length - kit.installedCount;
  const installLabel =
    missing === kit.skills.length
      ? t("market.kitInstallAll")
      : t("market.kitInstallMissing", { count: missing });

  return (
    <div
      role="button"
      tabIndex={0}
      aria-pressed={selected}
      onClick={() => onSelect(kit.slug)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(kit.slug);
        }
      }}
      className={cn(
        "group cursor-pointer rounded-lg border bg-card transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        featured ? "p-4" : "flex items-center gap-3 p-3.5",
        selected
          ? "border-primary ring-1 ring-primary"
          : "border-border hover:border-primary/40 hover:bg-accent/30",
        className,
      )}
    >
      <span
        className={cn(
          "flex shrink-0 items-center justify-center rounded-md",
          featured ? "h-10 w-10 bg-primary/10" : "h-8 w-8 bg-muted",
        )}
      >
        <Layers className={cn("text-primary", featured ? "h-5 w-5" : "h-4 w-4")} />
      </span>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <h3 className={cn("truncate font-semibold", featured ? "text-sm" : "text-sm")} title={kit.name}>
            {kit.name}
          </h3>
          {kit.version && (
            <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
              v{kit.version}
            </span>
          )}
          {featured && kit.subKits?.map((sub) => (
            <span
              key={sub.slug}
              className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground"
            >
              {sub.name} · {sub.skills.length}
            </span>
          ))}
        </div>
        {featured && kit.description && (
          <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">{kit.description}</p>
        )}
        <div className={cn("flex items-center gap-2", featured ? "mt-2.5" : "mt-1.5")}>
          <ProgressBar value={kit.installedCount} total={kit.skills.length} />
          <span className="shrink-0 whitespace-nowrap text-[11px] tabular-nums text-muted-foreground">
            {kit.installedCount}/{kit.skills.length}
          </span>
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-2">
        {canManage && missing > 0 && (
          <Button
            size="sm"
            variant={featured ? "default" : "outline"}
            disabled={disabled}
            onClick={(e) => {
              e.stopPropagation();
              onInstallMissing(kit);
            }}
            className="h-9 gap-1.5 px-2.5 text-xs min-h-11 sm:min-h-9"
          >
            {busy ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Download className="h-3.5 w-3.5" />
            )}
            <span className="hidden sm:inline">
              {busy ? t("market.installing") : installLabel}
            </span>
            <span className="sm:hidden">{busy ? "…" : `+${missing}`}</span>
          </Button>
        )}
        {!featured && (
          <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
        )}
      </div>
    </div>
  );
}
