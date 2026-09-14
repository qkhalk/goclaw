import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { Clock, Folder, PackageOpen, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { EmptyState } from "@/components/shared/empty-state";
import { useAuthStore } from "@/stores/use-auth-store";
import { clearRecents, readRecents, type CloudRecentEntry } from "@/lib/cloud-recent";
import { useCloudAccounts } from "../hooks/use-cloud";
import { parentPath } from "./paths";

/** "Gần đây" pseudo-view (?view=recent on /cloud): the device-local recent
 * list (localStorage, per user — no backend). Click opens the owning account
 * at the entry's location; clear-all wipes the list. */
export function RecentView() {
  const { t } = useTranslation("cloud");
  const navigate = useNavigate();
  const userId = useAuthStore((s) => s.userId);
  const { accounts } = useCloudAccounts();
  const [entries, setEntries] = useState<CloudRecentEntry[]>(() => readRecents(userId));
  const [clearOpen, setClearOpen] = useState(false);

  // userId may hydrate after first render (auth store) — re-read the list for
  // the resolved user so we don't keep showing the anonymous bucket.
  useEffect(() => {
    setEntries(readRecents(userId));
  }, [userId]);

  function refresh() {
    setEntries(readRecents(userId));
  }

  function open(entry: CloudRecentEntry) {
    const account = accounts.find((a) => a.id === entry.accountId);
    if (!account) return;
    const target = entry.isDir ? entry.path : parentPath(entry.path);
    navigate(`/cloud/${account.provider}/${account.id}?path=${encodeURIComponent(target)}`);
  }

  function handleClear() {
    clearRecents(userId);
    refresh();
    setClearOpen(false);
  }

  if (entries.length === 0) {
    return (
      <div className="p-4">
        <EmptyState icon={Clock} title={t("recent.empty")} />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3 p-3 md:p-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t("recent.description")}</p>
        <Button
          variant="outline"
          size="sm"
          className="min-h-11 sm:min-h-8"
          onClick={() => setClearOpen(true)}
        >
          <Trash2 className="mr-1.5 h-4 w-4" />
          {t("recent.clear")}
        </Button>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full min-w-[600px] text-sm">
          <thead>
            <tr className="border-b text-left text-xs text-muted-foreground">
              <th className="py-1.5 pr-2 font-medium">{t("starred.name")}</th>
              <th className="px-2 py-1.5 font-medium">{t("drive.accounts")}</th>
              <th className="px-2 py-1.5 font-medium">{t("starred.location")}</th>
              <th className="px-2 py-1.5 font-medium">{t("recent.opened_at")}</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => {
              const account = accounts.find((a) => a.id === e.accountId);
              const missing = !account;
              return (
                <tr
                  key={`${e.accountId}:${e.path}`}
                  className={missing ? "border-b opacity-50 last:border-0" : "cursor-pointer border-b last:border-0 hover:bg-muted/40"}
                  onClick={() => !missing && open(e)}
                  title={missing ? t("starred.account_gone") : undefined}
                >
                  <td className="py-2 pr-2">
                    <div className="flex max-w-[280px] items-center gap-2">
                      {e.isDir ? (
                        <Folder className="h-4 w-4 shrink-0 fill-sky-100 text-sky-500 dark:fill-sky-950" />
                      ) : (
                        <PackageOpen className="h-4 w-4 shrink-0 text-muted-foreground" />
                      )}
                      <span className="min-w-0 flex-1 truncate font-medium">{e.name}</span>
                    </div>
                  </td>
                  <td className="px-2 py-2 text-xs text-muted-foreground">
                    {account?.email ?? e.email}
                  </td>
                  <td className="max-w-[220px] truncate px-2 py-2 text-xs text-muted-foreground" dir="ltr">
                    {e.path}
                  </td>
                  <td className="px-2 py-2 text-xs text-muted-foreground">
                    {new Date(e.at).toLocaleString()}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <ConfirmDialog
        open={clearOpen}
        onOpenChange={setClearOpen}
        title={t("recent.clear_confirm_title")}
        description={t("recent.clear_confirm")}
        confirmLabel={t("recent.clear")}
        onConfirm={handleClear}
      />
    </div>
  );
}
