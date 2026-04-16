import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronUp, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";

const LS_KEY = "goclaw:hooks:beta-card-collapsed";

function loadCollapsed(): boolean {
  try {
    return localStorage.getItem(LS_KEY) === "1";
  } catch {
    return false;
  }
}

function saveCollapsed(v: boolean) {
  try {
    localStorage.setItem(LS_KEY, v ? "1" : "0");
  } catch {
    // ignore quota/private-mode errors
  }
}

// BetaInfoCard explains what the Hooks feature does for end users. Renders at
// the top of the hooks list page. State persists per-browser via localStorage
// so dismissed users don't see it expand on every reload.
export function BetaInfoCard() {
  const { t } = useTranslation("hooks");
  const [collapsed, setCollapsed] = useState<boolean>(loadCollapsed);

  const toggle = () => {
    const next = !collapsed;
    setCollapsed(next);
    saveCollapsed(next);
  };

  return (
    <div className="rounded-lg border border-blue-200 bg-blue-50 dark:border-blue-900/50 dark:bg-blue-900/10">
      <button
        type="button"
        onClick={toggle}
        className="flex w-full items-center gap-2 px-4 py-3 text-left"
        aria-expanded={!collapsed}
      >
        <Sparkles className="h-4 w-4 shrink-0 text-blue-600 dark:text-blue-400" />
        <span className="text-sm font-medium text-blue-900 dark:text-blue-100">
          {t("beta.title")}
        </span>
        <span className="rounded-full bg-blue-200/80 px-1.5 py-0.5 text-2xs font-semibold uppercase text-blue-800 dark:bg-blue-800/40 dark:text-blue-200">
          {t("beta.badge")}
        </span>
        <div className="flex-1" />
        {collapsed ? (
          <ChevronDown className="h-4 w-4 text-blue-600 dark:text-blue-400" />
        ) : (
          <ChevronUp className="h-4 w-4 text-blue-600 dark:text-blue-400" />
        )}
      </button>

      {!collapsed && (
        <div className="space-y-3 px-4 pb-4 text-sm text-blue-900/90 dark:text-blue-100/90">
          <p>{t("beta.description")}</p>

          <div className="grid gap-3 sm:grid-cols-3">
            <BetaPoint title={t("beta.howItWorksTitle1")} body={t("beta.howItWorksBody1")} />
            <BetaPoint title={t("beta.howItWorksTitle2")} body={t("beta.howItWorksBody2")} />
            <BetaPoint title={t("beta.howItWorksTitle3")} body={t("beta.howItWorksBody3")} />
          </div>

          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="sm"
              onClick={toggle}
              className="h-7 text-xs text-blue-700 hover:bg-blue-100 hover:text-blue-900 dark:text-blue-300 dark:hover:bg-blue-900/30 dark:hover:text-blue-100"
            >
              {t("beta.collapse")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function BetaPoint({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded border border-blue-200/60 bg-white/60 p-3 dark:border-blue-900/40 dark:bg-blue-950/30">
      <p className="text-xs font-semibold text-blue-900 dark:text-blue-100">{title}</p>
      <p className="mt-1 text-xs text-blue-800/80 dark:text-blue-200/70">{body}</p>
    </div>
  );
}
