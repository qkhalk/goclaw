import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { useCloudAccounts, type CloudFileEntry } from "../hooks/use-cloud";
import { DriveGrid } from "./drive-grid";
import { DriveTable } from "./drive-table";
import type { SortSpec, ViewMode } from "./paths";

/** File area of the Drive shell: lists the current folder (?path=), with
 * client-side search filter and dirs-first sorting. Phase 3 = read-only
 * browsing; file operations are layered on by the file-ops phase. */
export function DriveFileArea({
  accountId,
  provider,
  path,
  onOpenFolder,
  search,
  sort,
  viewMode,
}: {
  accountId: string;
  provider: string;
  path: string;
  onOpenFolder: (entry: CloudFileEntry) => void;
  search: string;
  sort: SortSpec;
  viewMode: ViewMode;
}) {
  const { t } = useTranslation("cloud");
  const http = useHttp();
  const { accounts } = useCloudAccounts();
  const account = accounts.find((a) => a.id === accountId);
  const readOnlyNote = account?.can_write === false;

  const files = useQuery({
    queryKey: queryKeys.cloud.files(accountId, path),
    staleTime: 30_000,
    queryFn: () =>
      http.get<{ path: string; entries: CloudFileEntry[] }>(
        `/v1/cloud/accounts/${accountId}/files?path=${encodeURIComponent(path)}&limit=200`,
      ),
  });

  const entries = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = (files.data?.entries ?? []).filter((e) => !q || e.name.toLowerCase().includes(q));
    list.sort((a, b) => {
      if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
      let cmp = 0;
      switch (sort.key) {
        case "size":
          cmp = a.size - b.size;
          break;
        case "modified":
          cmp = new Date(a.mod_time).getTime() - new Date(b.mod_time).getTime();
          break;
        default:
          cmp = a.name.localeCompare(b.name);
      }
      return sort.dir === "asc" ? cmp : -cmp;
    });
    return list;
  }, [files.data, search, sort]);

  return (
    <div className="flex flex-col gap-3 p-3 md:p-4">
      {files.isLoading ? (
        <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          {t("detail.loading_files")}
        </div>
      ) : files.isError ? (
        <p className="py-8 text-sm text-muted-foreground">{t("detail.files_unavailable")}</p>
      ) : entries.length === 0 ? (
        <p className="py-8 text-sm text-muted-foreground">{t("detail.no_files")}</p>
      ) : viewMode === "grid" ? (
        <DriveGrid entries={entries} onOpenFolder={onOpenFolder} />
      ) : (
        <DriveTable entries={entries} onOpenFolder={onOpenFolder} />
      )}

      {readOnlyNote && (
        <p className="text-[11px] text-muted-foreground">
          {t("detail.readonly_note", { provider })}
        </p>
      )}
    </div>
  );
}
