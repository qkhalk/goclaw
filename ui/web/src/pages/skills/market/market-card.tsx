import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Download, Loader2, RefreshCw, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { cn } from "@/lib/utils";
import type { MarketSkill } from "../hooks/use-skill-market";

/** Which market action is currently in flight for a card. */
export type MarketBusyAction = "install" | "uninstall" | "update" | null;

interface MarketCardProps {
  skill: MarketSkill;
  /** Admin/owner only — viewers never see install/uninstall controls. */
  canManage: boolean;
  /** Action in flight for this card (null = idle); disables the other buttons. */
  busyAction: MarketBusyAction;
  onInstall: (slug: string) => void;
  onUninstall: (slug: string) => void;
  onUpdate: (slug: string) => void;
}

/** One bundled-skill card in the market grid. */
export function MarketCard({ skill, canManage, busyAction, onInstall, onUninstall, onUpdate }: MarketCardProps) {
  const { t } = useTranslation("skills");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const busy = busyAction !== null;

  return (
    <div
      className={cn(
        "flex flex-col rounded-lg border p-4 transition-colors",
        skill.installed ? "border-primary/40 bg-primary/5" : "border-border",
      )}
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-2">
          <h3 className="truncate text-sm font-semibold" title={skill.name}>
            {skill.name}
          </h3>
          {skill.installed && (
            <span className="shrink-0 rounded-full bg-primary/15 px-2 py-0.5 text-[11px] font-medium text-primary">
              {t("market.installed")}
            </span>
          )}
        </div>
        <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{skill.description}</p>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
          {skill.category}
        </span>
        {skill.updateAvailable && (
          <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-600 dark:text-amber-400">
            {t("market.updateAvailable")}
          </span>
        )}
        {skill.version && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
            v{skill.installedVersion ?? skill.version}
          </span>
        )}
      </div>

      {skill.requires && skill.requires.length > 0 && (
        <p className="mt-2 truncate text-[11px] text-muted-foreground">
          {t("market.requires")}: {skill.requires.join(", ")}
        </p>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-2">
        {canManage ? (
          <>
            {!skill.installed && (
              <Button
                size="sm"
                disabled={busy}
                onClick={() => onInstall(skill.slug)}
                className="min-h-11 sm:min-h-9"
              >
                {busyAction === "install" ? (
                  <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Download className="mr-2 h-3.5 w-3.5" />
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
                className="min-h-11 sm:min-h-9"
              >
                {busyAction === "update" ? (
                  <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="mr-2 h-3.5 w-3.5" />
                )}
                {busyAction === "update" ? t("market.updating") : t("market.update")}
              </Button>
            )}
            {skill.installed && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => setConfirmOpen(true)}
                className="min-h-11 sm:min-h-9"
              >
                {busyAction === "uninstall" ? (
                  <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Trash2 className="mr-2 h-3.5 w-3.5" />
                )}
                {busyAction === "uninstall" ? t("market.uninstalling") : t("market.uninstall")}
              </Button>
            )}
          </>
        ) : (
          !skill.installed && (
            <p className="text-xs text-muted-foreground">{t("market.adminRequired")}</p>
          )
        )}
      </div>

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
    </div>
  );
}
