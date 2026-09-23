import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Layers, Loader2, RefreshCw, X, Zap } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/shared/empty-state";
import { SearchInput } from "@/components/shared/search-input";
import { useAuthStore } from "@/stores/use-auth-store";
import { cn } from "@/lib/utils";
import { useSkillMarket, type MarketKit } from "../hooks/use-skill-market";
import { MarketCard, type MarketBusyAction } from "./market-card";
import { MarketKitCard } from "./market-kit-card";

/**
 * Market tab of /skills — browse the bundled-skill catalog grouped by kit
 * (kit.yaml bundles), client-side search + category filter, and
 * install/uninstall/update per skill or per whole kit (admin-only). All
 * endpoints are synchronous, so actions resolve in-place with toasts.
 */
export function MarketTab() {
  const { t } = useTranslation("skills");
  const role = useAuthStore((s) => s.role);
  const canManage = role === "admin" || role === "owner";
  const { skills, kits, loading, error, refetch, install, uninstall, update } = useSkillMarket();

  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");
  const [activeKitSlug, setActiveKitSlug] = useState<string | null>(null);
  const [busy, setBusy] = useState<{ slug: string; action: MarketBusyAction } | null>(null);
  const [kitBusy, setKitBusy] = useState<string | null>(null);

  const categories = useMemo(
    () => [...new Set(skills.map((s) => s.category).filter(Boolean))].sort(),
    [skills],
  );

  const activeKit = useMemo(
    () => kits.find((k) => k.slug === activeKitSlug) ?? null,
    [kits, activeKitSlug],
  );
  const activeKitSlugs = useMemo(() => new Set(activeKit?.skills ?? []), [activeKit]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return skills.filter((skill) => {
      if (activeKit && !activeKitSlugs.has(skill.slug)) return false;
      if (category !== "all" && skill.category !== category) return false;
      if (!q) return true;
      return (
        skill.name.toLowerCase().includes(q) ||
        skill.slug.toLowerCase().includes(q) ||
        (skill.description ?? "").toLowerCase().includes(q) ||
        skill.category.toLowerCase().includes(q)
      );
    });
  }, [skills, query, category, activeKit, activeKitSlugs]);

  const run = async (slug: string, action: MarketBusyAction, op: () => Promise<unknown>) => {
    if (!slug || busy) return;
    setBusy({ slug, action });
    try {
      await op();
    } catch {
      // toast already raised by the hook
    } finally {
      setBusy(null);
    }
  };

  const installKitMissing = async (kit: MarketKit) => {
    if (kitBusy) return;
    const missing = skills
      .filter((s) => kit.skills.includes(s.slug) && !s.installed)
      .map((s) => s.slug);
    if (missing.length === 0) return;
    setKitBusy(kit.slug);
    try {
      await install(missing);
    } catch {
      // toast already raised by the hook
    } finally {
      setKitBusy(null);
    }
  };

  if (loading && skills.length === 0 && !error) {
    return (
      <div className="flex min-h-40 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error && skills.length === 0) {
    return (
      <EmptyState
        icon={Zap}
        title={t("market.loadFailed")}
        description={t("market.emptyDescription")}
        action={
          <Button variant="outline" size="sm" onClick={() => void refetch()} className="gap-1">
            <RefreshCw className="h-3.5 w-3.5" /> {t("refresh", { ns: "common" })}
          </Button>
        }
      />
    );
  }

  if (skills.length === 0) {
    return <EmptyState icon={Zap} title={t("market.emptyTitle")} description={t("market.emptyDescription")} />;
  }

  return (
    <div className="space-y-4">
      {kits.length > 0 && (
        <section aria-label={t("market.kitsTitle")}>
          <h3 className="mb-2 flex items-center gap-1.5 text-sm font-semibold text-muted-foreground">
            <Layers className="h-4 w-4" /> {t("market.kitsTitle")}
          </h3>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {kits.map((kit) => (
              <MarketKitCard
                key={kit.slug}
                kit={kit}
                busy={kitBusy === kit.slug}
                disabled={kitBusy !== null}
                selected={activeKitSlug === kit.slug}
                canManage={canManage}
                onSelect={(slug) => setActiveKitSlug((cur) => (cur === slug ? null : slug))}
                onInstallMissing={(kit) => void installKitMissing(kit)}
              />
            ))}
          </div>
        </section>
      )}

      <div className="flex flex-col gap-2 md:flex-row md:items-center">
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder={t("market.searchPlaceholder")}
          className="w-full md:max-w-sm"
        />
        <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={t("market.categories")}>
          <button
            type="button"
            className={cn(
              "rounded-full px-3 py-1 text-xs font-medium transition-colors min-h-9",
              category === "all"
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:text-foreground",
            )}
            onClick={() => setCategory("all")}
          >
            {t("market.all")}
          </button>
          {categories.map((cat) => (
            <button
              key={cat}
              type="button"
              className={cn(
                "rounded-full px-3 py-1 text-xs font-medium transition-colors min-h-9",
                category === cat
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted text-muted-foreground hover:text-foreground",
              )}
              onClick={() => setCategory(cat)}
            >
              {cat}
            </button>
          ))}
        </div>
        {!canManage && (
          <p className="text-xs text-muted-foreground md:ml-auto">{t("market.adminRequired")}</p>
        )}
      </div>

      {activeKit && (
        <div className="flex items-center gap-2 text-sm">
          <span className="rounded-full bg-primary/10 px-3 py-1 font-medium text-primary">
            {t("market.kitSelected", { name: activeKit.name })}
          </span>
          <button
            type="button"
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground min-h-9"
            onClick={() => setActiveKitSlug(null)}
          >
            <X className="h-3.5 w-3.5" /> {t("market.kitShowAll")}
          </button>
        </div>
      )}

      {filtered.length === 0 ? (
        <EmptyState icon={Zap} title={t("market.noMatchTitle")} description={t("market.noMatchDescription")} />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((skill) => (
            <MarketCard
              key={skill.slug}
              skill={skill}
              canManage={canManage}
              busyAction={busy?.slug === skill.slug ? busy.action : null}
              onInstall={(slug) => void run(slug, "install", () => install([slug]))}
              onUninstall={(slug) => void run(slug, "uninstall", () => uninstall(slug))}
              onUpdate={(slug) => void run(slug, "update", () => update(slug))}
            />
          ))}
        </div>
      )}
    </div>
  );
}
