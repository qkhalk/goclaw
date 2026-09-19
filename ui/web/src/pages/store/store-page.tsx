import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Check, Loader2, PackageOpen, Settings2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAuthStore } from "@/stores/use-auth-store";
import { useBuiltinTools } from "@/pages/builtin-tools/hooks/use-builtin-tools";
import { McpConnectCard } from "./mcp-connect-card";
import { ROUTES } from "@/lib/routes";
import { cn } from "@/lib/utils";
import { useStudioModules, type StudioModule } from "./use-studio-modules";

/**
 * Tool Store — installable product tools (studio modules). Install/uninstall
 * flips the per-tenant builtin tool override (existing API); the sidebar, the
 * route gates and later the MCP hub exposure all follow the same state.
 */
export function StorePage() {
  const { t } = useTranslation("tools");
  const { t: tNav } = useTranslation("sidebar");
  const navigate = useNavigate();
  const { modules, loading } = useStudioModules();
  const { setTenantConfig } = useBuiltinTools();
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";
  const [busy, setBusy] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<string | null>(null);

  async function toggle(mod: StudioModule) {
    // Second click on Uninstall confirms; Install acts immediately.
    if (mod.installed && confirming !== mod.meta.name) {
      setConfirming(mod.meta.name);
      return;
    }
    setBusy(mod.meta.name);
    try {
      await setTenantConfig(mod.meta.name, !mod.installed);
    } catch {
      // toast already raised by the hook
    } finally {
      setBusy(null);
      setConfirming(null);
    }
  }

  if (loading) {
    return (
      <div className="flex h-dvh items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6">
      <header className="mb-6">
        <h1 className="text-xl font-semibold">{t("store.title")}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t("store.subtitle")}</p>
      </header>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {modules.map((mod) => {
          const Icon = mod.meta.icon;
          const ramNote = typeof mod.def?.metadata?.ram_note === "string" ? mod.def.metadata.ram_note : null;
          const isBusy = busy === mod.meta.name;
          const isConfirming = confirming === mod.meta.name && mod.installed;
          return (
            <div
              key={mod.meta.name}
              className={cn(
                "flex flex-col rounded-lg border p-4 transition-colors",
                mod.installed ? "border-primary/40 bg-primary/5" : "border-border",
              )}
            >
              <div className="flex items-start gap-3">
                <div className="rounded-md bg-muted p-2">
                  <Icon className="h-5 w-5" />
                </div>
                <div className="min-w-0 flex-1">
                  <h3 className="truncate text-sm font-semibold">{tNav(mod.meta.labelKey)}</h3>
                  <p className="mt-1 line-clamp-3 text-xs text-muted-foreground">
                    {t(`store.${mod.meta.name}.desc`)}
                  </p>
                </div>
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-1.5">
                <span
                  className={cn(
                    "rounded-full px-2 py-0.5 text-[11px]",
                    mod.installed ? "bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
                  )}
                >
                  {mod.installed ? t("store.installed") : t("store.not_installed")}
                </span>
                {ramNote && (
                  <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
                    {ramNote}
                  </span>
                )}
              </div>
              <div className="mt-4 flex items-center gap-2">
                {isAdmin ? (
                  <Button
                    variant={mod.installed ? "outline" : "default"}
                    size="sm"
                    disabled={isBusy}
                    onClick={() => void toggle(mod)}
                    className={cn("min-h-11 sm:min-h-9", isConfirming && "border-destructive text-destructive hover:bg-destructive/10")}
                    aria-label={t(mod.installed ? "store.uninstall" : "store.install", {
                      name: tNav(mod.meta.labelKey),
                    })}
                  >
                    {isBusy ? (
                      <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                    ) : mod.installed ? (
                      <Trash2 className="mr-2 h-3.5 w-3.5" />
                    ) : (
                      <Check className="mr-2 h-3.5 w-3.5" />
                    )}
                    {isConfirming ? t("store.confirm_uninstall") : t(mod.installed ? "store.uninstall" : "store.install")}
                  </Button>
                ) : (
                  !mod.installed && <p className="text-xs text-muted-foreground">{t("store.admin_required")}</p>
                )}
                {mod.installed && (
                  <Button variant="ghost" size="sm" onClick={() => navigate(mod.meta.route)} className="min-h-11 sm:min-h-9">
                    {t("store.open")}
                  </Button>
                )}
              </div>
            </div>
          );
        })}
      </div>

      <section className="mt-8 rounded-lg border p-4">
        <div className="flex items-center gap-2">
          <Settings2 className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">{t("store.agent_tools_title")}</h2>
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{t("store.agent_tools_desc")}</p>
        <Button variant="outline" size="sm" onClick={() => navigate(ROUTES.BUILTIN_TOOLS)} className="mt-3 min-h-11 sm:min-h-9">
          <PackageOpen className="mr-2 h-3.5 w-3.5" />
          {t("store.manage_agent_tools")}
        </Button>
      </section>

      <McpConnectCard />
    </div>
  );
}
