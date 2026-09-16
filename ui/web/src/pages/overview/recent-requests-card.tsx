import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, ArrowDown, ArrowUp } from "lucide-react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useWsEvent } from "@/hooks/use-ws-event";
import { Events } from "@/api/protocol";
import { ROUTES } from "@/lib/constants";
import { formatRelativeTime, formatTokens } from "@/lib/format";
import { useHttp } from "@/hooks/use-ws";
import type { RecentLLMRequest } from "./types";

const REFRESH_INTERVAL = 15_000;
const REQUEST_LIMIT = 30;

/** 9router-style recent requests: compact fixed-height card, one row per LLM
 * API call — status dot | Model | In/Out (colored) | When. Live: refetches on
 * trace status changes (a request starting/finishing anywhere) with a 15s
 * polling fallback; scrolls internally with a sticky header through up to 30
 * rows so it never stretches the overview grid row. */
export function RecentRequestsCard() {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["usage", "recent-requests"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => http.get<{ requests: RecentLLMRequest[] }>("/v1/usage/recent-requests", { limit: String(REQUEST_LIMIT) }),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["usage", "recent-requests"] });
  // trace.status fires on every span status write — request started/completed;
  // trace.updated covers the final row payload.
  useWsEvent(Events.TRACE_STATUS, invalidate);
  useWsEvent(Events.TRACE_UPDATED, invalidate);

  const requests = data?.requests ?? [];

  return (
    <Card className="flex h-full min-h-0 flex-col overflow-hidden">
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="text-base">{t("recentRequests.title")}</CardTitle>
        {requests.length > 0 && (
          <Link
            to={ROUTES.TRACES}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            {t("recentRequests.viewAll")} <ArrowRight className="h-3 w-3" />
          </Link>
        )}
      </CardHeader>
      <CardContent className="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-3">
        {isLoading ? (
          <div className="space-y-2.5">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        ) : requests.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("recentRequests.noRequests")}
          </p>
        ) : (
          // Table sits directly in the scrolling CardContent: an intermediate
          // overflow-x wrapper would become the sticky thead's scrollport and
          // break sticking. overflow-y-auto computes overflow-x to auto, so
          // the min-w table still scrolls horizontally.
          <table className="w-full min-w-[300px] text-sm">
            <thead className="sticky top-0 z-10 bg-card">
              <tr className="border-b text-left text-muted-foreground">
                <th className="pb-2 font-medium">{t("recentRequests.columns.model")}</th>
                <th className="px-3 pb-2 font-medium text-right">{t("recentRequests.columns.inOut")}</th>
                <th className="pb-2 pl-3 font-medium text-right">{t("recentRequests.columns.when")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/50">
              {requests.map((r) => (
                <tr key={r.span_id} className="hover:bg-muted/30 transition-colors">
                  <td className="py-2 pr-3">
                    <div className="flex min-w-0 items-center gap-2">
                      <span
                        className={`h-1.5 w-1.5 shrink-0 rounded-full ${r.status === "error" ? "bg-red-500" : "bg-emerald-500"}`}
                        title={r.error || r.status}
                      />
                      <p
                        className="truncate font-mono text-xs font-medium"
                        title={
                          r.cost_usd && r.cost_usd > 0
                            ? `${r.model} · $${r.cost_usd.toFixed(4)}`
                            : r.model
                        }
                      >
                        {r.model || "--"}
                      </p>
                    </div>
                  </td>
                  <td
                    className="whitespace-nowrap px-3 py-2 text-right tabular-nums"
                    title={`${r.input_tokens.toLocaleString()} in / ${r.output_tokens.toLocaleString()} out`}
                  >
                    <span className="inline-flex items-center gap-1 text-rose-500 dark:text-rose-400">
                      <ArrowUp className="h-3 w-3" />
                      {formatTokens(r.input_tokens)}
                    </span>
                    <span className="mx-1.5 text-muted-foreground/60">/</span>
                    <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                      <ArrowDown className="h-3 w-3" />
                      {formatTokens(r.output_tokens)}
                    </span>
                  </td>
                  <td className="whitespace-nowrap py-2 pl-3 text-right text-muted-foreground">
                    {formatRelativeTime(r.start_time)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </CardContent>
    </Card>
  );
}
