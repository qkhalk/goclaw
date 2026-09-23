import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Layers, Loader2, RefreshCw, X, Zap } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/shared/empty-state";
import { SearchInput } from "@/components/shared/search-input";
import { Pagination } from "@/components/shared/pagination";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuthStore } from "@/stores/use-auth-store";
import { usePagination } from "@/hooks/use-pagination";
import { cn } from "@/lib/utils";
import { useSkillMarket, type MarketKit } from "../hooks/use-skill-market";
import { MarketKitCard } from "./market-kit-card";
import { MarketRow } from "./market-row";

type StatusFilter = "all" | "installed" | "notInstalled";

/**
 * Market tab of /skills — bundled-skill catalog grouped by kit, rendered as a
 * compact paginated table (same pattern as the Core/Custom tabs). Kits are
 * the navigation layer: pick a kit or sub-kit to narrow the table; search,
 * category and installed-state filters combine on top.
 */
export function MarketTab() {
  const { t } = useTranslation("skills");
  const role = useAuthStore((s) => s.role);
  const canManage = role === "admin" || role === "owner";
  const { skills, kits, loading, error, refetch, install, uninstall, update } = useSkillMarket();

  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [activeKitSlug, setActiveKitSlug] = useState<string | null>(null);
  const [busy, setBusy] = useState<{ slug: string; action: "install" | "uninstall" | "update" } | null>(null);
  const [kitBusy, setKitBusy] = useState<string | null>(null);

  const categories = useMemo(
    () => [...new Set(skills.map((s) => s.category).filter(Boolean))].sort(),
    [skills],
  );

  const activeKit = useMemo(() => {
    for (const kit of kits) {
      if (kit.slug === activeKitSlug) return kit;
      for (const sub of kit.subKits ?? []) {
        if (sub.slug === activeKitSlug) return sub;
      }
    }
    return null;
  }, [kits, activeKitSlug]);
  const activeKitSlugs = useMemo(() => new Set(activeKit?.skills ?? []), [activeKit]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return skills.filter((skill) => {
      if (activeKit && !activeKitSlugs.has(skill.slug)) return false;
      if (category !== "all" && skill.category !== category) return false;
      if (status === "installed" && !skill.installed) return false;
      if (status === "notInstalled" && skill.installed) return false;
      if (!q) return true;
      return (
        skill.name.toLowerCase().includes(q) ||
        skill.slug.toLowerCase().includes(q) ||
        (skill.description ?? "").toLowerCase().includes(q)
      );
    });
  }, [skills, query, category, status, activeKit, activeKitSlugs]);

  const { pageItems, pagination, setPage, setPageSize, resetPage } = usePagination(filtered);
  useEffect(() => {
    resetPage();
  }, [query, category, status, activeKitSlug, resetPage]);

  const run = async (slug: string, action: "install" | "uninstall" | "update", op: () => Promise<unknown>) => {
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

  const parentKit = kits.find((k) => k.slug === activeKitSlug) ?? null;

  return (
    <div className="space-y-4">
      {kits.length > 0 && (
        <section aria-label={t("market.kitsTitle")}>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            {kits.map((kit) => (
              <MarketKitCard
                key={kit.slug}
                kit={kit}
                featured={kit.subKits != null && kit.subKits.length > 0}
                className="md:col-span-2"
                busy={kitBusy === kit.slug}
                disabled={kitBusy !== null}
                selected={activeKitSlug === kit.slug}
                canManage={canManage}
                onSelect={(slug) => setActiveKitSlug((cur) => (cur === slug ? null : slug))}
                onInstallMissing={(kit) => void installKitMissing(kit)}
              />
            ))}
          </div>
          {parentKit?.subKits && parentKit.subKits.length > 0 && (
            <div className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
              {parentKit.subKits.map((sub) => (
                <MarketKitCard
                  key={sub.slug}
                  kit={sub}
                  busy={kitBusy === sub.slug}
                  disabled={kitBusy !== null}
                  selected={activeKitSlug === sub.slug}
                  canManage={canManage}
                  onSelect={(slug) => setActiveKitSlug((cur) => (cur === slug ? parentKit.slug : slug))}
                  onInstallMissing={(kit) => void installKitMissing(kit)}
                />
              ))}
            </div>
          )}
        </section>
      )}

      <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
        <SearchInput
          value={query}
          onChange={setQuery}
          placeholder={t("market.searchPlaceholder")}
          className="w-full lg:max-w-xs"
        />
        <div className="flex flex-wrap items-center gap-2">
          <div role="group" aria-label={t("market.filterStatus")} className="flex rounded-md border p-0.5">
            {(
              [
                ["all", t("market.all")],
                ["installed", t("market.filterInstalled")],
                ["notInstalled", t("market.filterNotInstalled")],
              ] as [StatusFilter, string][]
            ).map(([value, label]) => (
              <button
                key={value}
                type="button"
                className={cn(
                  "rounded px-2.5 py-1.5 text-xs font-medium transition-colors min-h-9",
                  status === value
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
                onClick={() => setStatus(value)}
              >
                {label}
              </button>
            ))}
          </div>
          <Select value={category} onValueChange={setCategory}>
            <SelectTrigger className="h-9 w-full min-h-9 sm:w-[190px]" aria-label={t("market.categories")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent className="max-h-72">
              <SelectItem value="all">{t("market.allCategories")}</SelectItem>
              {categories.map((cat) => (
                <SelectItem key={cat} value={cat}>
                  {cat}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2 lg:ml-auto">
          {activeKit && (
            <button
              type="button"
              className="flex items-center gap-1.5 rounded-full bg-primary/10 py-1 pl-3 pr-2 text-xs font-medium text-primary min-h-9 hover:bg-primary/15"
              onClick={() => setActiveKitSlug(null)}
            >
              <Layers className="h-3.5 w-3.5" />
              {activeKit.name}
              <X className="h-3.5 w-3.5" />
            </button>
          )}
          <span className="whitespace-nowrap text-xs text-muted-foreground">
            {t("market.results", { count: filtered.length, total: skills.length })}
          </span>
          {!canManage && <p className="text-xs text-muted-foreground">{t("market.adminRequired")}</p>}
        </div>
      </div>

      {filtered.length === 0 ? (
        <EmptyState icon={Zap} title={t("market.noMatchTitle")} description={t("market.noMatchDescription")} />
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full min-w-[640px] text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="px-4 py-3 text-left font-medium">{t("columns.name")}</th>
                <th className="w-36 px-4 py-3 text-left font-medium">{t("columns.category")}</th>
                <th className="w-32 px-4 py-3 text-left font-medium">{t("columns.status")}</th>
                <th className="w-44 px-4 py-3 text-right font-medium">{t("columns.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {pageItems.map((skill) => (
                <MarketRow
                  key={skill.slug}
                  skill={skill}
                  canManage={canManage}
                  busyAction={busy?.slug === skill.slug ? busy.action : null}
                  onInstall={(slug) => void run(slug, "install", () => install([slug]))}
                  onUninstall={(slug) => void run(slug, "uninstall", () => uninstall(slug))}
                  onUpdate={(slug) => void run(slug, "update", () => update(slug))}
                />
              ))}
            </tbody>
          </table>
          <Pagination
            page={pagination.page}
            pageSize={pagination.pageSize}
            total={pagination.total}
            totalPages={pagination.totalPages}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        </div>
      )}
    </div>
  );
}
