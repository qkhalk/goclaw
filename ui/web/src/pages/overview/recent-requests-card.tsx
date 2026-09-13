import { useQuery } from "@tanstack/react-query";
import { ArrowRight, ArrowDown, ArrowUp } from "lucide-react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ROUTES } from "@/lib/constants";
import { formatRelativeTime, formatTokens } from "@/lib/format";
import { useHttp } from "@/hooks/use-ws";

/** One recent LLM API call, matching GET /v1/usage/recent-requests. */
interface RecentLLMRequest {
  span_id: string;
  trace_id: string;
  model: string;
  provider: string;
  input_tokens: number;
  output_tokens: number;
  status: string;
  error?: string;
  start_time: string;
  duration_ms: number;
  cost_usd?: number;
}

const REFRESH_INTERVAL = 30_000;

/** 9router-style recent requests: one row per LLM API call — Model |
 * In/Out (colored) | When. */
export function RecentRequestsCard() {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const { data, isLoading } = useQuery({
    queryKey: ["usage", "recent-requests"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => http.get<{ requests: RecentLLMRequest[] }>("/v1/usage/recent-requests", { limit: "8" }),
  });
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
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="pb-2 font-medium">{t("recentRequests.columns.model")}</th>
                  <th className="pb-2 px-4 font-medium text-right">{t("recentRequests.columns.inOut")}</th>
                  <th className="pb-2 pl-4 font-medium text-right">{t("recentRequests.columns.when")}</th>
                </tr>
              </thead>
              <tbody>
                {requests.map((r) => (
                  <tr key={r.span_id} className="border-b last:border-0">
                    <td className="py-2.5 pr-4">
                      <div className="flex min-w-0 items-center gap-2">
                        {r.status === "error" && (
                          <span
                            className="h-2 w-2 shrink-0 rounded-full bg-red-500"
                            title={r.error || r.status}
                          />
                        )}
                        <div className="min-w-0">
                          <p className="truncate font-mono text-xs font-medium" title={r.model}>
                            {r.model || "--"}
                          </p>
                          {(r.cost_usd ?? 0) > 0 && (
                            <p className="text-[11px] leading-tight text-muted-foreground">
                              ${r.cost_usd!.toFixed(4)}
                            </p>
                          )}
                        </div>
                      </div>
                    </td>
                    <td
                      className="whitespace-nowrap px-4 py-2.5 text-right tabular-nums"
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
                    <td className="whitespace-nowrap py-2.5 pl-4 text-right text-muted-foreground">
                      {formatRelativeTime(r.start_time)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
