import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowUpCircle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useHttp } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { toast } from "@/stores/use-toast-store";
import { cleanVersion } from "@/lib/clean-version";
import { cn } from "@/lib/utils";

interface SystemUpdateStatus {
  current: string;
  latest: string;
  update_available: boolean;
  url?: string;
  error?: string;
}

const CHECK_INTERVAL_MS = 30 * 60 * 1000; // re-check every 30 minutes
const HEALTH_POLL_MS = 2_000; // poll /health while the gateway restarts
const RESTART_TIMEOUT_MS = 60_000; // give up waiting after 60s
const NO_RESTART_GRACE_MS = 15_000; // health must stay up this long before we conclude no restart happened

type Phase = "idle" | "confirm" | "installing" | "restarting";

/**
 * UpdateBadge: sidebar footer widget for self-hosted gateway updates.
 *
 * Polls GET /v1/system/update on mount + every 30 min. When a newer release
 * exists, offers a one-click install: POST /v1/system/update/install swaps
 * the binary server-side and exits; systemd restarts into the new version.
 * While restarting we poll /health, waiting for it to go down once and come
 * back. Hidden for non-admin roles and when the endpoint answers 403.
 */
export function UpdateBadge({ collapsed }: { collapsed?: boolean }) {
  const { t } = useTranslation("common");
  const http = useHttp();
  const role = useAuthStore((s) => s.role);
  const connected = useAuthStore((s) => s.connected);

  const [status, setStatus] = useState<SystemUpdateStatus | null>(null);
  const [hidden, setHidden] = useState(false);
  const [phase, setPhase] = useState<Phase>("idle");
  const restartTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Stop the /health poll if the sidebar unmounts mid-restart.
  useEffect(() => {
    return () => {
      if (restartTimerRef.current !== null) clearInterval(restartTimerRef.current);
    };
  }, []);

  const isAdmin = role === "owner" || role === "admin";

  const check = useCallback(async () => {
    try {
      const res = await fetch("/v1/system/update", {
        headers: http.getAuthHeaders(),
        cache: "no-store",
      });
      if (res.status === 403 || res.status === 401) {
        setHidden(true);
        return;
      }
      if (!res.ok) return;
      const body = (await res.json()) as SystemUpdateStatus;
      setStatus(body);
    } catch {
      // Server unreachable: keep whatever we had, stay quiet.
    }
  }, [http]);

  useEffect(() => {
    if (!isAdmin || hidden) return;
    if (!connected) return;
    void check();
    const id = setInterval(() => void check(), CHECK_INTERVAL_MS);
    return () => clearInterval(id);
  }, [isAdmin, hidden, connected, check]);

  /** Poll /health until it fails once (shutdown) then succeeds again (new process). */
  const awaitRestart = useCallback(() => {
    const startedAt = Date.now();
    let sawDown = false;
    const stop = () => {
      if (restartTimerRef.current !== null) {
        clearInterval(restartTimerRef.current);
        restartTimerRef.current = null;
      }
    };
    stop();
    restartTimerRef.current = setInterval(async () => {
      const elapsed = Date.now() - startedAt;
      if (elapsed > RESTART_TIMEOUT_MS) {
        stop();
        setPhase("idle");
        toast.warning(
          t("systemUpdate.restartTimeout", "Restart not detected yet"),
          t("systemUpdate.restartTimeoutBody", "The gateway may still be restarting. Reload in a moment."),
        );
        return;
      }
      let up: boolean;
      try {
        const res = await fetch("/health", { cache: "no-store" });
        up = res.ok;
      } catch {
        up = false;
      }
      if (!up) {
        sawDown = true;
        return;
      }
      if (sawDown) {
        // Went down, came back: new process is live.
        stop();
        const version = status?.latest ?? "";
        toast.success(
          t("systemUpdate.success", "Updated to {{version}}", { version: cleanVersion(version) }),
        );
        setPhase("idle");
        void check();
        // Reload so the UI reconnects to the freshly started gateway.
        setTimeout(() => window.location.reload(), 1_200);
      } else if (elapsed > NO_RESTART_GRACE_MS) {
        // Health never dropped within the grace window: the install either
        // restarted faster than we could observe or did not happen.
        stop();
        setPhase("idle");
        toast.info(t("systemUpdate.noRestart", "Gateway is back online"));
        void check();
      }
    }, HEALTH_POLL_MS);
  }, [check, status?.latest, t]);

  const install = useCallback(async () => {
    setPhase("installing");
    let restartExpected = true;
    try {
      const res = await fetch("/v1/system/update/install", {
        method: "POST",
        headers: http.getAuthHeaders(),
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as { error?: string } | null;
        const message = body?.error ?? res.statusText;
        toast.error(t("systemUpdate.failed", "Update failed"), message);
        restartExpected = false;
      }
    } catch {
      // Network error likely means the gateway exited to restart. Proceed.
    }
    if (!restartExpected) {
      setPhase("idle");
      return;
    }
    setPhase("restarting");
    awaitRestart();
  }, [awaitRestart, http, t]);

  if (!isAdmin || hidden) return null;

  const busy = phase === "installing" || phase === "restarting";
  // While installing/restarting the gateway drops the WS connection, so the
  // badge must stay visible even when "connected" is false.
  if (!connected && !busy) return null;
  const fmt = (v: string) => cleanVersion(v);

  if (busy) {
    return (
      <div
        className={cn(
          "flex items-center gap-2 text-muted-foreground",
          collapsed ? "justify-center" : "",
        )}
      >
        <Loader2 className="size-4 shrink-0 animate-spin" />
        {!collapsed && (
          <span className="truncate text-base md:text-sm">
            {phase === "installing"
              ? t("systemUpdate.installing", "Downloading update...")
              : t("systemUpdate.restarting", "Restarting...")}
          </span>
        )}
      </div>
    );
  }

  if (phase === "confirm") {
    return (
      <div className="space-y-1.5">
        {!collapsed && (
          <p className="text-base leading-snug md:text-sm">
            {t("systemUpdate.confirmBody", "The gateway will restart and be offline for about 15 seconds.")}
          </p>
        )}
        <div className={cn("flex items-center gap-1.5", collapsed ? "flex-col" : "")}>
          <Button
            size="xs"
            className="min-h-11 sm:min-h-9 w-full px-2 text-base md:text-sm"
            onClick={() => void install()}
          >
            {t("systemUpdate.confirm", "Install and restart")}
          </Button>
          <Button
            size="xs"
            variant="outline"
            className="min-h-11 sm:min-h-9 px-2 text-base md:text-sm"
            onClick={() => setPhase("idle")}
          >
            {t("cancel", "Cancel")}
          </Button>
        </div>
      </div>
    );
  }

  if (status?.update_available && status.latest) {
    return (
      <div className="space-y-1.5">
        <Button
          size="xs"
          className="min-h-11 sm:min-h-9 w-full justify-start gap-2 px-2 text-base md:text-sm"
          onClick={() => setPhase("confirm")}
          title={t("systemUpdate.available", "Update to {{version}}", { version: fmt(status.latest) })}
        >
          <ArrowUpCircle className="size-4 shrink-0" />
          {!collapsed && (
            <span className="truncate">
              {t("systemUpdate.available", "Update to {{version}}", { version: fmt(status.latest) })}
            </span>
          )}
        </Button>
        {!collapsed && status.current && (
          <p className="truncate text-xs text-muted-foreground">
            {t("systemUpdate.currentVersion", "Current: {{version}}", { version: fmt(status.current) })}
          </p>
        )}
      </div>
    );
  }

  // No update: muted current-version line only (skip when nothing to show).
  if (!collapsed && status?.current && status.current !== "dev") {
    return (
      <p className="truncate text-xs text-muted-foreground" title={status.current}>
        {t("systemUpdate.currentVersion", "Current: {{version}}", { version: fmt(status.current) })}
      </p>
    );
  }
  if (collapsed) return null;

  return null;
}
