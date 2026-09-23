import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw, Trash2, Download, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { cn } from "@/lib/utils";
import type { MarketSkill } from "../hooks/use-skill-market";

/** Which market action is currently in flight for a row. */
export type MarketBusyAction = "install" | "uninstall" | "update" | null;

interface MarketRowProps {
  skill: MarketSkill;
  /** Admin/owner only — viewers never see install/uninstall controls. */
  canManage: boolean;
  /** Action in flight for this row (null = idle); disables the other buttons. */
  busyAction: MarketBusyAction;
  onInstall: (slug: string) => void;
  onUninstall: (slug: string) => void;
  onUpdate: (slug: string) => void;
}

/** One bundled-skill row of the market table — compact, scannable, one
 *  primary action per row. */
export function MarketRow({
  skill,
  canManage,
  busyAction,
  onInstall,
  onUninstall,
  onUpdate,
}: MarketRowProps) {
  const { t } = useTranslation("skills");
  const busy = busyAction !== null;
  const [confirmOpen, setConfirmOpen] = useState(false);

  return (
    <tr className={cn("border-b transition-colors last:border-0 hover:bg-muted/40", skill.installed && "bg-primary/[0.04]")}>
      <td className="max-w-0 px-4 py-2.5">
        <div className="flex items-baseline gap-2">
          <span className="shrink-0 text-sm font-medium" title={skill.name}>
            {skill.name}
          </span>
          {skill.updateAvailable && (
            <span className="shrink-0 rounded-full bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium leading-none text-amber-600 dark:text-amber-400">
              {t("market.updateAvailable")}
            </span>
          )}
        </div>
        <p className="mt-0.5 truncate text-xs text-muted-foreground" title={skill.description}>
          {skill.description}
        </p>
      </td>
      <td className="px-4 py-2.5">
        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
          {skill.category}
        </span>
      </td>
      <td className="px-4 py-2.5">
        {skill.installed ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-primary/15 px-2 py-0.5 text-[11px] font-medium text-primary">
            <span className="h-1.5 w-1.5 rounded-full bg-primary" aria-hidden />
            {t("market.installed")}
          </span>
        ) : (
          <span className="text-[11px] text-muted-foreground">{t("market.notInstalled")}</span>
        )}
      </td>
      <td className="px-4 py-2.5">
        <div className="flex items-center justify-end gap-1">
          {canManage ? (
            <>
              {!skill.installed && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onInstall(skill.slug)}
                  className="h-8 gap-1.5 px-2.5 text-xs min-h-11 sm:min-h-9"
                >
                  {busyAction === "install" ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Download className="h-3.5 w-3.5" />
                  )}
                  {busyAction === "install" ? t("market.installing") : t("market.install")}
                </Button>
              )}
              {skill.installed && skill.updateAvailable && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onUpdate(skill.slug)}
                  className="h-8 gap-1.5 px-2.5 text-xs min-h-11 sm:min-h-9"
                >
                  {busyAction === "update" ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <RefreshCw className="h-3.5 w-3.5" />
                  )}
                  {busyAction === "update" ? t("market.updating") : t("market.update")}
                </Button>
              )}
              {skill.installed && (
                <>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => setConfirmOpen(true)}
                    className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive min-h-11 sm:min-h-9 sm:w-8"
                    aria-label={t("market.uninstall")}
                    title={t("market.uninstall")}
                  >
                    {busyAction === "uninstall" ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Trash2 className="h-3.5 w-3.5" />
                    )}
                  </Button>
                  <ConfirmDialog
                    open={confirmOpen}
                    onOpenChange={setConfirmOpen}
                    title={t("market.confirmUninstallTitle")}
                    description={t("market.confirmUninstallDescription", { name: skill.name })}
                    confirmLabel={t("market.confirmLabel")}
                    variant="destructive"
                    loading={busy}
                    onConfirm={() => {
                      setConfirmOpen(false);
                      onUninstall(skill.slug);
                    }}
                  />
                </>
              )}
            </>
          ) : (
            !skill.installed && (
              <span className="text-[11px] text-muted-foreground">{t("market.adminRequired")}</span>
            )
          )}
        </div>
      </td>
    </tr>
  );
}
