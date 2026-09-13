import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ChevronRight,
  File,
  Folder,
  Inbox,
  Loader2,
  Mail,
  Mailbox,
  FolderTree,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { useHttp } from "@/hooks/use-ws";
import { formatRelativeTime } from "@/lib/format";

/** rclone quota for one account (GET /v1/cloud/accounts/{id}/about). */
interface AccountAbout {
  total: number;
  used: number;
  free: number;
}

/** One remote entry (GET /v1/cloud/accounts/{id}/files). */
interface FileEntry {
  name: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

/** One inbox message (GET /v1/cloud/accounts/{id}/mail). */
interface MailSummary {
  id: string;
  from: string;
  subject: string;
  date: string;
  snippet: string;
}

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  const v = n / 1024 ** i;
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

export type DetailTab = "files" | "mail" | null;

/** Per-account detail panel: storage quota + file browser (+ mailbox preview
 * for Gmail-capable accounts). Everything is read-only. */
export function AccountDetail({
  accountId,
  provider,
  canMail,
  tab,
  onTabChange,
}: {
  accountId: string;
  provider: string;
  canMail: boolean;
  tab: DetailTab;
  onTabChange: (t: DetailTab) => void;
}) {
  const { t } = useTranslation("cloud");
  const http = useHttp();

  const about = useQuery({
    queryKey: ["cloud", "about", accountId],
    staleTime: 60_000,
    queryFn: () => http.get<AccountAbout>(`/v1/cloud/accounts/${accountId}/about`),
  });

  const pct = useMemo(() => {
    if (!about.data || about.data.total <= 0) return null;
    return Math.min(100, Math.round((about.data.used / about.data.total) * 100));
  }, [about.data]);

  return (
    <div className="space-y-3 border-t pt-3">
      {/* Capability shortcuts */}
      <div className="flex flex-wrap gap-2">
        <Button
          variant={tab === "files" ? "secondary" : "outline"}
          size="sm"
          className="min-h-9"
          onClick={() => onTabChange(tab === "files" ? null : "files")}
        >
          <FolderTree className="mr-1.5 h-4 w-4" />
          {t("detail.files_btn")}
        </Button>
        {canMail && (
          <Button
            variant={tab === "mail" ? "secondary" : "outline"}
            size="sm"
            className="min-h-9"
            onClick={() => onTabChange(tab === "mail" ? null : "mail")}
          >
            <Inbox className="mr-1.5 h-4 w-4" />
            {t("detail.mail_btn")}
          </Button>
        )}
      </div>

      {/* Quota bar */}
      {about.isError ? (
        <p className="text-xs text-muted-foreground">{t("detail.quota_unavailable")}</p>
      ) : about.data && pct !== null ? (
        <div>
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {t("detail.used_of", {
                used: formatBytes(about.data.used),
                total: formatBytes(about.data.total),
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
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          {t("detail.loading_quota")}
        </div>
      )}

      {tab === "files" && <FilesBrowser accountId={accountId} provider={provider} />}
      {tab === "mail" && canMail && <MailboxPreview accountId={accountId} />}
    </div>
  );
}

/** Read-only remote file browser with breadcrumb navigation. */
function FilesBrowser({ accountId, provider }: { accountId: string; provider: string }) {
  const { t } = useTranslation("cloud");
  const http = useHttp();
  const [path, setPath] = useState("/");

  const files = useQuery({
    queryKey: ["cloud", "files", accountId, path],
    staleTime: 30_000,
    queryFn: () =>
      http.get<{ path: string; entries: FileEntry[] }>(
        `/v1/cloud/accounts/${accountId}/files?path=${encodeURIComponent(path)}&limit=200`,
      ),
  });

  const crumbs = useMemo(
    () =>
      path
        .split("/")
        .filter(Boolean)
        .map((seg, i, arr) => ({ name: decodeURIComponent(seg), path: "/" + arr.slice(0, i + 1).join("/") })),
    [path],
  );

  const entries = useMemo(() => {
    const list = [...(files.data?.entries ?? [])];
    list.sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1));
    return list;
  }, [files.data]);

  return (
    <div className="rounded-lg border bg-muted/10 p-3">
      {/* Breadcrumb */}
      <div className="flex flex-wrap items-center gap-1 text-xs">
        <button
          type="button"
          className="rounded px-1 py-0.5 hover:bg-muted"
          onClick={() => setPath("/")}
        >
          {t("detail.root")}
        </button>
        {crumbs.map((c) => (
          <span key={c.path} className="flex items-center gap-1">
            <ChevronRight className="h-3 w-3 text-muted-foreground" />
            <button
              type="button"
              className="rounded px-1 py-0.5 hover:bg-muted"
              onClick={() => setPath(c.path)}
            >
              {c.name}
            </button>
          </span>
        ))}
      </div>

      <div className="mt-2 overflow-x-auto">
        {files.isLoading ? (
          <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t("detail.loading_files")}
          </div>
        ) : files.isError ? (
          <p className="py-4 text-sm text-muted-foreground">{t("detail.files_unavailable")}</p>
        ) : entries.length === 0 ? (
          <p className="py-4 text-sm text-muted-foreground">{t("detail.no_files")}</p>
        ) : (
          <table className="w-full min-w-[540px] text-sm">
            <thead>
              <tr className="border-b text-left text-muted-foreground">
                <th className="pb-2 pr-4 font-medium">{t("detail.col_name")}</th>
                <th className="pb-2 px-4 font-medium text-right">{t("detail.col_size")}</th>
                <th className="pb-2 pl-4 font-medium">{t("detail.col_modified")}</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e) =>
                e.is_dir ? (
                  <tr
                    key={e.name}
                    className="cursor-pointer border-b last:border-0 hover:bg-muted/40"
                    onClick={() =>
                      setPath((path === "/" ? "" : path) + "/" + encodeURIComponent(e.name))
                    }
                  >
                    <td className="py-2 pr-4">
                      <span className="flex items-center gap-2">
                        <Folder className="h-4 w-4 shrink-0 text-sky-500" />
                        <span className="truncate">{e.name}</span>
                      </span>
                    </td>
                    <td className="py-2 px-4 text-right text-muted-foreground">—</td>
                    <td className="py-2 pl-4 text-muted-foreground">{formatRelativeTime(e.mod_time)}</td>
                  </tr>
                ) : (
                  <tr key={e.name} className="border-b last:border-0">
                    <td className="py-2 pr-4">
                      <span className="flex items-center gap-2">
                        <File className="h-4 w-4 shrink-0 text-muted-foreground" />
                        <span className="truncate">{e.name}</span>
                      </span>
                    </td>
                    <td className="py-2 px-4 text-right tabular-nums text-muted-foreground">
                      {formatBytes(e.size)}
                    </td>
                    <td className="py-2 pl-4 text-muted-foreground">{formatRelativeTime(e.mod_time)}</td>
                  </tr>
                ),
              )}
            </tbody>
          </table>
        )}
      </div>
      <p className="mt-2 text-[11px] text-muted-foreground">
        {t("detail.readonly_note", { provider })}
      </p>
    </div>
  );
}

/** Gmail inbox preview (read-only, latest N). */
function MailboxPreview({ accountId }: { accountId: string }) {
  const { t } = useTranslation("cloud");
  const http = useHttp();

  const mail = useQuery({
    queryKey: ["cloud", "mail", accountId],
    staleTime: 60_000,
    queryFn: () =>
      http.get<{ email: string; messages: MailSummary[] }>(
        `/v1/cloud/accounts/${accountId}/mail?max=10`,
      ),
  });

  return (
    <div className="rounded-lg border bg-muted/10 p-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Mailbox className="h-4 w-4 shrink-0" />
        {mail.data?.email || t("detail.mailbox")}
      </div>
      {mail.isLoading ? (
        <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          {t("detail.loading_mail")}
        </div>
      ) : mail.isError ? (
        <p className="py-4 text-sm text-muted-foreground">{t("detail.mail_unavailable")}</p>
      ) : (mail.data?.messages ?? []).length === 0 ? (
        <p className="py-4 text-sm text-muted-foreground">{t("detail.no_mail")}</p>
      ) : (
        <ul className="mt-2 divide-y">
          {mail.data!.messages.map((m) => (
            <li key={m.id} className="py-2">
              <div className="flex items-start justify-between gap-2">
                <p className="min-w-0 flex-1 truncate text-sm font-medium">{m.subject || t("detail.no_subject")}</p>
                <span className="shrink-0 text-[11px] text-muted-foreground">{m.date}</span>
              </div>
              <p className="truncate text-xs text-muted-foreground">
                <Mail className="mr-1 inline h-3 w-3" />
                {m.from}
              </p>
              <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground/80">{m.snippet}</p>
            </li>
          ))}
        </ul>
      )}
      <p className="mt-2 text-[11px] text-muted-foreground">{t("detail.mail_readonly_note")}</p>
    </div>
  );
}
