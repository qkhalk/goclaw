import { useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Building2, Clock, Cloud, HardDrive, Inbox, Loader2, Star } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";
import { formatFileSize } from "@/lib/format";
import { ROUTES } from "@/lib/routes";
import { useCloudAccounts, useCloudStarred, type CloudAccount, type CloudProvider } from "../hooks/use-cloud";
import { accountCanMail, MailboxPreview } from "./mailbox-preview";

/** Connectable providers (backend mirror: cloud.SupportedProviders). */
export const CLOUD_PROVIDERS: { id: CloudProvider; name: string; icon: typeof Cloud }[] = [
  { id: "google", name: "Google Drive", icon: Cloud },
  { id: "onedrive", name: "Microsoft OneDrive", icon: HardDrive },
];

/** rclone quota for one account (GET /v1/cloud/accounts/{id}/about). */
interface CloudAbout {
  total: number;
  used: number;
  free: number;
}

/** Drive-style left rail: quick views, providers section (each provider a nav
 * item with its accounts nested below), quota card of the open account,
 * mailbox preview. Rendered in a desktop aside and in a mobile Sheet
 * (DriveShell). */
export function DriveRail({
  accountId,
  onNavigate,
}: {
  accountId?: string;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation("cloud");
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const { provider: routeProvider } = useParams();
  const { accounts } = useCloudAccounts();
  const starred = useCloudStarred();
  const [mailAccountId, setMailAccountId] = useState<string | null>(null);
  const activeView = params.get("view") ?? "";

  /** Starred / recent pseudo-views live on /cloud itself (?view=…). */
  function openView(view: "starred" | "recent") {
    onNavigate?.();
    navigate(`${ROUTES.CLOUD}?view=${view}`);
  }

  function openProvider(id: string) {
    onNavigate?.();
    navigate(ROUTES.CLOUD_PROVIDER.replace(":provider", id));
  }

  const byProvider = useMemo(() => {
    const map = new Map<CloudProvider, CloudAccount[]>();
    for (const p of CLOUD_PROVIDERS) map.set(p.id, []);
    for (const a of accounts) {
      const list = map.get(a.provider as CloudProvider);
      if (list) list.push(a);
    }
    return map;
  }, [accounts]);

  function openAccount(provider: string, id: string) {
    onNavigate?.();
    navigate(`${ROUTES.CLOUD_PROVIDER.replace(":provider", provider)}/${id}`);
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto p-3">
      <nav className="flex flex-col gap-1">
        <RailLink
          icon={Star}
          label={t("starred.title")}
          active={activeView === "starred"}
          badge={starred.items.length > 0 ? starred.items.length : undefined}
          onClick={() => openView("starred")}
        />
        <RailLink
          icon={Clock}
          label={t("recent.title")}
          active={activeView === "recent"}
          onClick={() => openView("recent")}
        />
      </nav>

      <nav className="flex flex-col gap-1">
        <p className="mb-1 px-2 text-xs font-medium text-muted-foreground">
          {t("drive.providers")}
        </p>
        {CLOUD_PROVIDERS.map((p) => {
          const items = byProvider.get(p.id) ?? [];
          const providerActive = routeProvider === p.id;
          return (
            <div key={p.id}>
              <button
                type="button"
                onClick={() => openProvider(p.id)}
                className={cn(
                  "flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-muted/60",
                  providerActive && "bg-muted font-medium",
                )}
              >
                <p.icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{p.name}</span>
                {items.length > 0 && (
                  <span className="shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground">
                    {items.length}
                  </span>
                )}
              </button>
              {items.length > 0 && (
                <ul className="flex flex-col gap-0.5 pl-6">
                  {items.map((a) => {
                    const active = a.id === accountId;
                    return (
                      <li key={a.id}>
                        <button
                          type="button"
                          onClick={() => openAccount(a.provider, a.id)}
                          className={cn(
                            "flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-muted/60",
                            active && "bg-muted font-medium",
                          )}
                          title={a.shared ? t("drive.shared_tag") : undefined}
                        >
                          {a.shared ? (
                            <Building2 className="h-4 w-4 shrink-0 text-amber-500" aria-label={t("drive.shared_tag")} />
                          ) : (
                            <p.icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                          )}
                          <span className="min-w-0 flex-1 truncate">{a.email}</span>
                          {accountCanMail(a) && (
                            <span
                              role="button"
                              tabIndex={0}
                              aria-label={t("drive.mail")}
                              className="rounded p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                              onClick={(e) => {
                                e.stopPropagation();
                                setMailAccountId(a.id);
                              }}
                              onKeyDown={(e) => {
                                if (e.key === "Enter" || e.key === " ") {
                                  e.stopPropagation();
                                  setMailAccountId(a.id);
                                }
                              }}
                            >
                              <Inbox className="h-3.5 w-3.5" />
                            </span>
                          )}
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          );
        })}
      </nav>

      {accountId && <RailQuotaCard accountId={accountId} />}

      <Dialog open={mailAccountId !== null} onOpenChange={(open) => !open && setMailAccountId(null)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("drive.mail")}</DialogTitle>
          </DialogHeader>
          {mailAccountId && <MailboxPreview accountId={mailAccountId} />}
        </DialogContent>
      </Dialog>

    </div>
  );
}

/** One rail link (starred / recent pseudo-views). */
function RailLink({
  icon: Icon,
  label,
  active,
  badge,
  onClick,
}: {
  icon: typeof Star;
  label: string;
  active: boolean;
  badge?: number;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex min-h-11 w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-muted/60",
        active && "bg-muted font-medium",
      )}
    >
      <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {badge !== undefined && (
        <span className="shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground">
          {badge}
        </span>
      )}
    </button>
  );
}

/** Storage quota card for the currently open account (red >90% / amber >75% / emerald). */
function RailQuotaCard({ accountId }: { accountId: string }) {
  const { t } = useTranslation("cloud");
  const http = useHttp();

  const about = useQuery({
    queryKey: queryKeys.cloud.about(accountId),
    staleTime: 60_000,
    queryFn: () => http.get<CloudAbout>(`/v1/cloud/accounts/${accountId}/about`),
  });

  const pct = useMemo(() => {
    if (!about.data || about.data.total <= 0) return null;
    return Math.min(100, Math.round((about.data.used / about.data.total) * 100));
  }, [about.data]);

  return (
    <div className="mt-auto rounded-lg border p-3">
      <p className="text-xs font-medium text-muted-foreground">{t("drive.storage")}</p>
      {about.isError ? (
        <p className="mt-1 text-xs text-muted-foreground">{t("drive.storage_unavailable")}</p>
      ) : about.data && pct !== null ? (
        <div className="mt-1.5">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {t("detail.used_of", {
                used: formatFileSize(about.data.used),
                total: formatFileSize(about.data.total),
              })}
            </span>
            <span className="tabular-nums">{pct}%</span>
          </div>
          <div className="mt-1 h-2 w-full overflow-hidden rounded-full bg-muted">
            <div
              className={`h-full rounded-full transition-all ${pct > 90 ? "bg-red-500" : pct > 75 ? "bg-amber-500" : "bg-emerald-500"}`}
              style={{ width: `${pct}%` }}
            />
          </div>
        </div>
      ) : (
        <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          {t("detail.loading_quota")}
        </div>
      )}
    </div>
  );
}
