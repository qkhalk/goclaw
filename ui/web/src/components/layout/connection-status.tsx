import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Download, Loader2 } from "lucide-react";
import { useAuthStore } from "@/stores/use-auth-store";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";
import { cleanVersion } from "@/lib/clean-version";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { applyGatewayUpdate, useGatewayUpdateCheck } from "@/hooks/use-gateway-update";
import { useHttp } from "@/hooks/use-ws";

// formatVersion normalizes the server version and ensures the release "v"
// prefix ("3.19.0" -> "v3.19.0"); "dev" and already-prefixed values pass
// through untouched.
function formatVersion(v: string): string {
  const cleaned = cleanVersion(v);
  if (!cleaned || cleaned === "dev" || cleaned.startsWith("v")) return cleaned;
  return `v${cleaned}`;
}

const ROLE_STYLES: Record<string, string> = {
  admin: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300",
  owner: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300",
  operator: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300",
  viewer: "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300",
};

const UPDATE_SETTLE_TIMEOUT_MS = 3 * 60_000;

export function ConnectionStatus({ collapsed }: { collapsed?: boolean }) {
  const { t } = useTranslation("common");
  const http = useHttp();
  const connected = useAuthStore((s) => s.connected);
  const serverVersion = useAuthStore((s) => s.serverInfo?.version);
  const tenantName = useAuthStore((s) => s.tenantName);
  const role = useAuthStore((s) => s.role);

  const { data: check } = useGatewayUpdateCheck();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [applying, setApplying] = useState(false);
  // The tag we applied — used to detect the post-restart version flip.
  const appliedTagRef = useRef<string | null>(null);
  const settleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const updateAvailable = check?.available === true && !!check.version;
  const appliedTag = appliedTagRef.current;
  const appliedArrived =
    applying &&
    connected &&
    !!appliedTag &&
    !!serverVersion &&
    cleanVersion(serverVersion) === cleanVersion(appliedTag);

  // After the apply POST the gateway replaces its binary and restarts: the
  // WS drops, then reconnects reporting the NEW version. Success is that
  // version flip; a long silence means the restart is still in progress.
  useEffect(() => {
    if (!applying) return;
    if (appliedArrived) {
      toast.success(t("update.success", { version: formatVersion(appliedTag ?? "") }));
      setApplying(false);
      appliedTagRef.current = null;
      return;
    }
    if (!connected && appliedTag) return; // mid-restart — keep waiting
    const timer = setTimeout(() => {
      toast.info(t("update.timeout"));
      setApplying(false);
      appliedTagRef.current = null;
    }, UPDATE_SETTLE_TIMEOUT_MS);
    settleTimerRef.current = timer;
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [applying, appliedArrived, connected]);

  async function handleConfirm() {
    if (!check?.version) return;
    setConfirmOpen(false);
    setApplying(true);
    appliedTagRef.current = check.version;
    try {
      await applyGatewayUpdate(http, check.version);
      toast.info(t("update.applying"));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
      setApplying(false);
      appliedTagRef.current = null;
    }
  }

  return (
    <div className="space-y-1.5">
      {/* Tenant + role (expanded only) */}
      {!collapsed && tenantName && (
        <div className="flex items-center justify-between gap-1.5 text-xs overflow-hidden">
          <span className="truncate font-medium text-foreground/80">{tenantName}</span>
          {role && (
            <span className={cn("shrink-0 rounded-full px-1.5 py-0.5 text-2xs font-medium", ROLE_STYLES[role] ?? ROLE_STYLES.viewer)}>
              {role}
            </span>
          )}
        </div>
      )}

      {/* Connection status */}
      <div className="flex items-center gap-2 text-xs text-muted-foreground overflow-hidden">
        <span
          className={cn(
            "h-2 w-2 shrink-0 rounded-full",
            connected ? "bg-green-500" : "bg-red-500",
          )}
        />
        {!collapsed && (
          <span className="truncate">
            {connected ? t("connected") : t("disconnected")}
            {connected && serverVersion && (
              <span className="ml-1 opacity-60">
                · {formatVersion(serverVersion)}
              </span>
            )}
          </span>
        )}
      </div>

      {/* Update available → clickable row (expanded) / pulsing dot (collapsed) */}
      {(updateAvailable || applying) && !collapsed && (
        <button
          type="button"
          disabled={applying}
          onClick={() => setConfirmOpen(true)}
          className={cn(
            "flex w-full min-h-8 items-center gap-1.5 overflow-hidden rounded-md px-2 py-1 text-left text-xs font-medium transition-colors",
            applying
              ? "cursor-wait text-muted-foreground"
              : "bg-amber-500/10 text-amber-600 hover:bg-amber-500/20 dark:text-amber-400",
          )}
        >
          {applying ? (
            <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
          ) : (
            <Download className="h-3.5 w-3.5 shrink-0" />
          )}
          <span className="min-w-0 flex-1 truncate">
            {applying
              ? t("update.applying_short")
              : t("update.available", { version: formatVersion(check?.version ?? "") })}
          </span>
        </button>
      )}
      {applying && collapsed && (
        <span className="flex h-2 w-2 shrink-0 animate-pulse rounded-full bg-amber-500" aria-label={t("update.applying_short")} />
      )}

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("update.confirm_title", { version: formatVersion(check?.version ?? "") })}
        description={t("update.confirm_desc")}
        confirmLabel={t("update.confirm_label")}
        loading={applying}
        onConfirm={handleConfirm}
      />
    </div>
  );
}
