import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowUpRight } from "lucide-react";
import { cn } from "@/lib/utils";

interface LiveStripProps {
  children: ReactNode;
  /** Internal route to open when the strip action is clicked. */
  to?: string;
  /** In-page action (e.g. switch the agent-detail tab) instead of a route. */
  actionLabel?: string;
  onAction?: () => void;
  /** Optional muted line under the chips (names, hints, empty-state reasons). */
  note?: string;
}

/**
 * "On this agent" strip rendered at the top of each capabilities tab.
 * Bridges the static architecture explainer with this agent's live state:
 * real config values, stats from memory/KG/evolution endpoints, and a
 * jump to the page that manages the data.
 */
export function LiveStrip({ children, to, actionLabel, onAction, note }: LiveStripProps) {
  const { t } = useTranslation("v3-capabilities");

  return (
    <div className="rounded-lg border bg-muted/30 p-3 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-2xs font-medium uppercase tracking-wide text-muted-foreground">
          {t("live.onAgent")}
        </span>
        {onAction ? (
          <button
            type="button"
            onClick={onAction}
            className="inline-flex items-center gap-0.5 text-2xs font-medium text-blue-600 hover:underline dark:text-blue-400 cursor-pointer py-1 px-2 -mr-2"
          >
            {actionLabel ?? t("live.viewDetails")}
            <ArrowUpRight className="h-3 w-3" />
          </button>
        ) : to ? (
          <Link
            to={to}
            className="inline-flex items-center gap-0.5 text-2xs font-medium text-blue-600 hover:underline dark:text-blue-400 py-1 px-2 -mr-2"
          >
            {t("live.viewDetails")}
            <ArrowUpRight className="h-3 w-3" />
          </Link>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-1.5">{children}</div>
      {note && <p className="text-2xs text-muted-foreground italic">{note}</p>}
    </div>
  );
}

interface LiveChipProps {
  icon: LucideIcon;
  label: string;
  value: ReactNode;
  /** "on" renders green, "off" renders muted; undefined keeps default emphasis. */
  tone?: "on" | "off";
}

export function LiveChip({ icon: Icon, label, value, tone }: LiveChipProps) {
  return (
    <span className="inline-flex max-w-full items-center gap-1.5 rounded-md border bg-background px-2 py-1">
      <Icon className="h-3 w-3 shrink-0 text-blue-500" />
      <span className="text-2xs text-muted-foreground whitespace-nowrap">{label}</span>
      <span
        className={cn(
          "text-2xs font-mono font-medium truncate",
          tone === "on" && "text-green-600 dark:text-green-400",
          tone === "off" && "text-muted-foreground",
        )}
      >
        {value}
      </span>
    </span>
  );
}
