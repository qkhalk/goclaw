import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Download, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/shared/empty-state";
import { useAuthStore } from "@/stores/use-auth-store";
import { useBuiltinTools } from "@/pages/builtin-tools/hooks/use-builtin-tools";
import { useStudioModules } from "./use-studio-modules";

/**
 * Route gate for a studio tool page: renders the page when its module is
 * installed, an install prompt when it was removed via the Tool Store.
 * Unknown defs (server predating the seed) pass through — the gate must
 * never lock users out of a page over a missing row.
 */
export function StudioGate({ module: name, children }: { module: string; children: ReactNode }) {
  const { t } = useTranslation("tools");
  const { modules, loading } = useStudioModules();
  const { setTenantConfig } = useBuiltinTools();
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";

  const mod = modules.find((m) => m.meta.name === name);
  if (loading || !mod) {
    // Brief skeleton rather than a flash of the denied screen.
    return (
      <div className="flex h-dvh items-center justify-center" data-testid="studio-gate-loading">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (mod.installed) return <>{children}</>;

  const Icon = mod.meta.icon;
  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <EmptyState
        icon={Icon}
        title={t("store.not_installed_title", { name: t(mod.meta.labelKey, { ns: "sidebar" }) })}
        description={t("store.not_installed_desc")}
        action={
          isAdmin ? (
            <Button onClick={() => void setTenantConfig(name, true)} className="min-h-11 sm:min-h-9">
              <Download className="mr-2 h-4 w-4" />
              {t("store.install_now")}
            </Button>
          ) : undefined
        }
      />
    </div>
  );
}
