import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Loader2, Mail, Mailbox } from "lucide-react";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { CloudAccount } from "../hooks/use-cloud";

/** One inbox message (GET /v1/cloud/accounts/{id}/mail). */
export interface CloudMailSummary {
  id: string;
  from: string;
  subject: string;
  date: string;
  snippet: string;
}

/** True when the account's OAuth grant includes Gmail (google only). */
export function accountCanMail(a: Pick<CloudAccount, "provider" | "scopes">): boolean {
  if (a.provider !== "google") return false;
  try {
    const scopes: string[] = JSON.parse(a.scopes || "[]");
    return scopes.some((sc) => sc.includes("/auth/gmail"));
  } catch {
    return false;
  }
}

/** Gmail inbox preview (read-only, latest N) — opened from the rail. */
export function MailboxPreview({ accountId }: { accountId: string }) {
  const { t } = useTranslation("cloud");
  const http = useHttp();

  const mail = useQuery({
    queryKey: queryKeys.cloud.mail(accountId),
    staleTime: 60_000,
    queryFn: () =>
      http.get<{ email: string; messages: CloudMailSummary[] }>(
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
