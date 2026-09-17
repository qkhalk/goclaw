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

/** 9router-style recent requests: one compact row per LLM API call — status
 * dot | Model | In/Out (colored) | When. Live: refetches on trace status
 * changes (a request starting/finishing anywhere) with a 15s polling
 * fallback. The scroll body is pinned to the routing graph's height
 * (h-[320px] sm:h-[480px]) so the card is exactly as tall as the Model
 * Routing card beside it — longer lists scroll inside with a sticky header. */
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
    <Card>
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
      <CardContent>
        {/* Same height math as the routing graph container next to this card
            (320px / 480px at sm) — equal cards by construction. */}
        <div className="h-[320px] w-full overflow-y-auto overscroll-contain sm:h-[480px]">
        {isLoading ? (
          <div className="space-y-1.5">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-4 w-full" />
            ))}
          </div>
        ) : requests.length === 0 ? (
          <p className="py-6 text-center text-xs text-muted-foreground">
            {t("recentRequests.noRequests")}
          </p>
        ) : (
          // Table sits directly in the scrolling CardContent: an intermediate
          // overflow-x wrapper would become the sticky thead's scrollport and
          // break sticking. overflow-y-auto computes overflow-x to auto, so
          // the min-w table still scrolls horizontally.
          <table className="w-full min-w-[300px] text-xs">
            <thead className="sticky top-0 z-10 bg-card">
              <tr className="border-b text-left text-muted-foreground">
                <th className="pb-1.5 text-[11px] font-medium">{t("recentRequests.columns.model")}</th>
                <th className="px-2.5 pb-1.5 text-[11px] font-medium text-right">{t("recentRequests.columns.inOut")}</th>
                <th className="pb-1.5 pl-2.5 text-[11px] font-medium text-right">{t("recentRequests.columns.when")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/50">
              {requests.map((r) => (
                <tr key={r.span_id} className="hover:bg-muted/30 transition-colors">
                  <td className="py-1.5 pr-2.5">
                    <div className="flex min-w-0 items-center gap-1.5">
                      <span
                        className={`h-1.5 w-1.5 shrink-0 rounded-full ${r.status === "error" ? "bg-red-500" : "bg-emerald-500"}`}
                        title={r.error || r.status}
                      />
                      <p
                        className="truncate font-mono text-[11px] font-medium"
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
                    className="whitespace-nowrap px-2.5 py-1.5 text-right tabular-nums"
                    title={`${r.input_tokens.toLocaleString()} in / ${r.output_tokens.toLocaleString()} out`}
                  >
                    <span className="inline-flex items-center gap-0.5 text-rose-500 dark:text-rose-400">
                      <ArrowUp className="h-2.5 w-2.5" />
                      {formatTokens(r.input_tokens)}
                    </span>
                    <span className="mx-1 text-muted-foreground/60">/</span>
                    <span className="inline-flex items-center gap-0.5 text-emerald-600 dark:text-emerald-400">
                      <ArrowDown className="h-2.5 w-2.5" />
                      {formatTokens(r.output_tokens)}
                    </span>
                  </td>
                  <td className="whitespace-nowrap py-1.5 pl-2.5 text-right text-[11px] text-muted-foreground">
                    {formatRelativeTime(r.start_time)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        </div>
      </CardContent>
    </Card>
  );
}
