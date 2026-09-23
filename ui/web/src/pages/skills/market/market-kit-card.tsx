import { useTranslation } from "react-i18next";
import { Download, Layers, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { MarketKit } from "../hooks/use-skill-market";

interface MarketKitCardProps {
  kit: MarketKit;
  /** True when a kit install is in flight for this kit. */
  busy: boolean;
  /** Any other action in flight — disables this card's buttons. */
  disabled: boolean;
  selected: boolean;
  canManage: boolean;
  onSelect: (slug: string) => void;
  onInstallMissing: (kit: MarketKit) => void;
}

/** One bundled skill kit card. Clicking the card drills the grid down to the
 *  kit's skills; the button installs every not-yet-installed kit skill. */
export function MarketKitCard({
  kit,
  busy,
  disabled,
  selected,
  canManage,
  onSelect,
  onInstallMissing,
}: MarketKitCardProps) {
  const { t } = useTranslation("skills");
  const missing = kit.skills.length - kit.installedCount;
  const installLabel = missing === kit.skills.length
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
        "flex cursor-pointer flex-col rounded-lg border p-4 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        selected ? "border-primary bg-primary/5 ring-1 ring-primary" : "border-border hover:bg-accent/40",
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary/10">
            <Layers className="h-[18px] w-[18px] text-primary" />
          </span>
          <h3 className="truncate text-sm font-semibold" title={kit.name}>
            {kit.name}
          </h3>
        </div>
        {kit.version && (
          <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
            v{kit.version}
          </span>
        )}
      </div>
      {kit.description && (
        <p className="mt-2 line-clamp-2 text-xs text-muted-foreground">{kit.description}</p>
      )}
      {kit.subKits && kit.subKits.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {kit.subKits.map((sub) => (
            <span
              key={sub.slug}
              className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground"
            >
              {sub.name} · {sub.skills.length}
            </span>
          ))}
        </div>
      )}
      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
          {t("market.kitSkillsCount", { count: kit.skills.length })}
        </span>
        <span
          className={cn(
            "rounded-full px-2 py-0.5 text-[11px]",
            missing === 0
              ? "bg-primary/15 text-primary"
              : "bg-amber-500/15 font-medium text-amber-600 dark:text-amber-400",
          )}
        >
          {missing === 0
            ? t("market.kitAllInstalled")
            : t("market.kitInstalledCount", { count: kit.installedCount })}
        </span>
      </div>
      {canManage && missing > 0 && (
        <div className="mt-4">
          <Button
            size="sm"
            disabled={disabled}
            onClick={(e) => {
              e.stopPropagation();
              onInstallMissing(kit);
            }}
            className="min-h-11 w-full sm:min-h-9 sm:w-auto"
          >
            {busy ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Download className="mr-2 h-3.5 w-3.5" />
            )}
            {busy ? t("market.installing") : installLabel}
          </Button>
        </div>
      )}
    </div>
  );
}
