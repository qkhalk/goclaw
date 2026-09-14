import { useTranslation } from "react-i18next";
import {
  ArrowDown,
  ArrowUp,
  LayoutGrid,
  List,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { RefreshButton } from "@/components/ui/refresh-button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { DriveBreadcrumbs } from "./drive-breadcrumbs";
import type { SortDir, SortKey, SortSpec, ViewMode } from "./paths";

const SORT_KEYS: SortKey[] = ["name", "size", "modified"];

/** Drive top bar: breadcrumbs/title row + tools row (search / sort / grid-list
 * toggle). Purely controlled — state lives in the page (search & sort are
 * ephemeral UI state, not URL state). The mobile rail hamburger is rendered by
 * the shell around this component. */
export function DriveTopBar({
  // Breadcrumb mode (account view)
  path,
  onNavigatePath,
  rootLabel,
  // Title mode (home / provider view)
  title,
  subtitle,
  // Tools (account view)
  showTools,
  search,
  onSearchChange,
  sort,
  onSortChange,
  viewMode,
  onViewModeChange,
  onRefresh,
  refreshing,
  // Page-level extras (gear, connect buttons, upload actions…)
  right,
}: {
  path?: string;
  onNavigatePath?: (path: string) => void;
  rootLabel?: string;
  title?: React.ReactNode;
  subtitle?: React.ReactNode;
  showTools?: boolean;
  search?: string;
  onSearchChange?: (v: string) => void;
  sort?: SortSpec;
  onSortChange?: (s: SortSpec) => void;
  viewMode?: ViewMode;
  onViewModeChange?: (m: ViewMode) => void;
  onRefresh?: () => void;
  refreshing?: boolean;
  right?: React.ReactNode;
}) {
  const { t } = useTranslation("cloud");
  const isAccountView = showTools === true;

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2">
      <div className="flex items-center gap-1">
        {path !== undefined && onNavigatePath ? (
          <DriveBreadcrumbs path={path} rootLabel={rootLabel ?? ""} onNavigatePath={onNavigatePath} />
        ) : (
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-base font-semibold md:text-lg">{title}</h1>
            {subtitle && <p className="truncate text-xs text-muted-foreground">{subtitle}</p>}
          </div>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          {right}
          {onRefresh && (
            <RefreshButton
              onRefresh={onRefresh}
              refreshing={refreshing}
              label={t("refresh")}
            />
          )}
        </div>
      </div>

      {isAccountView && (
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-0 flex-1 sm:max-w-xs">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search ?? ""}
              onChange={(e) => onSearchChange?.(e.target.value)}
              placeholder={t("drive.search_placeholder")}
              className="pl-8 text-base md:text-sm"
              autoComplete="off"
              data-cloud-search
            />
          </div>

          {sort && onSortChange && (
            <div className="flex items-center gap-1">
              <Select
                value={sort.key}
                onValueChange={(v) => onSortChange({ ...sort, key: v as SortKey })}
              >
                <SelectTrigger size="sm" className="w-[130px]" aria-label={t("drive.sort_label")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SORT_KEYS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(`drive.sort.${k}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="icon-sm"
                aria-label={t(`drive.sort.${sort.dir}`)}
                title={t(`drive.sort.${sort.dir}`)}
                onClick={() => onSortChange({ ...sort, dir: (sort.dir === "asc" ? "desc" : "asc") as SortDir })}
              >
                {sort.dir === "asc" ? <ArrowUp className="h-4 w-4" /> : <ArrowDown className="h-4 w-4" />}
              </Button>
            </div>
          )}

          {viewMode && onViewModeChange && (
            <div className="ml-auto flex items-center gap-0.5 rounded-md border p-0.5">
              <Button
                variant={viewMode === "grid" ? "secondary" : "ghost"}
                size="icon-sm"
                aria-label={t("drive.view.grid")}
                title={t("drive.view.grid")}
                onClick={() => onViewModeChange("grid")}
              >
                <LayoutGrid className="h-4 w-4" />
              </Button>
              <Button
                variant={viewMode === "list" ? "secondary" : "ghost"}
                size="icon-sm"
                aria-label={t("drive.view.list")}
                title={t("drive.view.list")}
                onClick={() => onViewModeChange("list")}
              >
                <List className="h-4 w-4" />
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
