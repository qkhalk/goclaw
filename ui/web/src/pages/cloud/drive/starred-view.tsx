import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { Folder, Star } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { useCloudAccounts, useCloudStarred } from "../hooks/use-cloud";
import { parentPath } from "./paths";

/** "Đã gắn dấu sao" pseudo-view (?view=starred on /cloud): every starred
 * file/folder across the user's accounts; click opens the owning account at
 * the item's location. Unstar stays available from the row. */
export function StarredView() {
  const { t } = useTranslation("cloud");
  const navigate = useNavigate();
  const { accounts, loading: accountsLoading } = useCloudAccounts();
  const { items, loading, remove } = useCloudStarred();

  function open(accountId: string, path: string, isDir: boolean) {
    const account = accounts.find((a) => a.id === accountId);
    if (!account) return;
    const target = isDir ? path : parentPath(path);
    navigate(`/cloud/${account.provider}/${account.id}?path=${encodeURIComponent(target)}`);
  }

  if (loading || accountsLoading) {
    return (
      <div className="p-4">
        <TableSkeleton rows={3} />
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="p-4">
        <EmptyState icon={Star} title={t("starred.empty")} />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3 p-3 md:p-4">
      <div className="overflow-x-auto">
        <table className="w-full min-w-[600px] text-sm">
          <thead>
            <tr className="border-b text-left text-xs text-muted-foreground">
              <th className="py-1.5 pr-2 font-medium">{t("starred.name")}</th>
              <th className="px-2 py-1.5 font-medium">{t("drive.accounts")}</th>
              <th className="px-2 py-1.5 font-medium">{t("starred.location")}</th>
              <th className="px-2 py-1.5 font-medium">{t("starred.starred_at")}</th>
              <th className="w-12 px-2 py-1.5" />
            </tr>
          </thead>
          <tbody>
            {items.map((s) => {
              const account = accounts.find((a) => a.id === s.account_id);
              const missing = !account;
              return (
                <tr
                  key={s.id}
                  className={missing ? "border-b opacity-50 last:border-0" : "cursor-pointer border-b last:border-0 hover:bg-muted/40"}
                  onClick={() => !missing && open(s.account_id, s.path, s.is_dir)}
                  title={missing ? t("starred.account_gone") : undefined}
                >
                  <td className="py-2 pr-2">
                    <div className="flex max-w-[280px] items-center gap-2">
                      {s.is_dir ? (
                        <Folder className="h-4 w-4 shrink-0 fill-sky-100 text-sky-500 dark:fill-sky-950" />
                      ) : (
                        <Star className="h-4 w-4 shrink-0 fill-amber-400 text-amber-500" />
                      )}
                      <span className="min-w-0 flex-1 truncate font-medium">{s.name}</span>
                    </div>
                  </td>
                  <td className="px-2 py-2 text-xs text-muted-foreground">
                    {account?.email ?? t("sync.account_missing")}
                  </td>
                  <td className="max-w-[220px] truncate px-2 py-2 text-xs text-muted-foreground" dir="ltr">
                    {s.path}
                  </td>
                  <td className="px-2 py-2 text-xs text-muted-foreground">
                    {new Date(s.starred_at).toLocaleString()}
                  </td>
                  <td className="px-2 py-2 text-right">
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t("starred.remove")}
                      title={t("starred.remove")}
                      className="text-muted-foreground hover:text-destructive"
                      onClick={(e) => {
                        e.stopPropagation();
                        void remove(s.id).catch(() => undefined);
                      }}
                    >
                      <Star className="h-4 w-4 fill-current" />
                    </Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
