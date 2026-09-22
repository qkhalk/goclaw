import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, ArrowDown, ArrowUp } from "lucide-react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ROUTES } from "@/lib/constants";
import { formatRelativeTime, formatTokens } from "@/lib/format";
import { useHttp } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import { Events } from "@/api/protocol";

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

/**
 * Live row synthesized from the agent event stream. The gateway broadcasts an
 * `agent` WS event with payload.type "llm.completed" for every think-stage LLM
 * call (provider, model, duration_ms, is_error, input_tokens, output_tokens —
 * numeric fields serialized as strings by the agent loop). Admin connections
 * receive all agent events, so the dashboard can prepend rows live and
 * reconcile them against the HTTP list on the next refetch.
 */
interface LiveLLMRequest extends RecentLLMRequest {
  /** Completion time (epoch ms) — used to dedup against fetched rows. */
  doneAt: number;
}

const REFRESH_INTERVAL = 30_000;
/** Rows kept in the card: fetched history (limit) + live rows, capped. */
const MAX_ROWS = 30;
const FETCH_LIMIT = 30;
/** Live rows that were never reconciled with a refetch are dropped after this. */
const LIVE_TTL_MS = 90_000;
/** A live row matching a fetched row (same model, completion time within window) is a dup. */
const DEDUP_WINDOW_MS = 10_000;
/** How long a freshly-arrived live row keeps its highlight background. */
const HIGHLIGHT_MS = 4_000;

function toNum(v: unknown): number {
  const n = typeof v === "number" ? v : parseInt(String(v ?? ""), 10);
  return Number.isFinite(n) ? n : 0;
}

function formatDuration(ms: number): string {
  if (ms <= 0) return "--";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

/** 9router-style recent requests: one row per LLM API call — Model |
 * In/Out (colored) | When. React-query loads history + polls as a fallback;
 * the `agent` WS event stream (llm.completed subtype) prepends rows live. */
export function RecentRequestsCard() {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const { data, isLoading } = useQuery({
    queryKey: ["usage", "recent-requests"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () =>
      http.get<{ requests: RecentLLMRequest[] }>("/v1/usage/recent-requests", {
        limit: String(FETCH_LIMIT),
      }),
  });
  const fetched = useMemo(() => data?.requests ?? [], [data]);

  const [liveRows, setLiveRows] = useState<LiveLLMRequest[]>([]);
  const [freshIds, setFreshIds] = useState<Set<string>>(new Set());
  const highlightTimers = useRef(new Map<string, number>());

  useEffect(() => {
    const timers = highlightTimers.current;
    return () => {
      for (const timer of timers.values()) window.clearTimeout(timer);
      timers.clear();
    };
  }, []);

  const handleAgentEvent = useCallback((payload: unknown) => {
    const ev = payload as {
      type?: string;
      runId?: string;
      payload?: Record<string, unknown>;
    };
    if (ev?.type !== "llm.completed") return;
    const p = ev.payload ?? {};
    const durationMs = toNum(p.duration_ms);
    const doneAt = Date.now();
    const id = `live-${ev.runId ?? "run"}-${doneAt}-${Math.random()
      .toString(36)
      .slice(2, 8)}`;
    const row: LiveLLMRequest = {
      span_id: id,
      trace_id: "",
      model: String(p.model ?? "") || "--",
      provider: String(p.provider ?? ""),
      input_tokens: toNum(p.input_tokens),
      output_tokens: toNum(p.output_tokens),
      status: p.is_error ? "error" : "ok",
      // The fetched list keys rows by span start; reconstruct it so relative
      // times and dedup matching line up with the persisted span.
      start_time: new Date(doneAt - durationMs).toISOString(),
      duration_ms: durationMs,
      doneAt,
    };
    setLiveRows((prev) => [row, ...prev].slice(0, MAX_ROWS));
    setFreshIds((prev) => new Set(prev).add(id));
    const timer = window.setTimeout(() => {
      setFreshIds((prev) => {
        if (!prev.has(id)) return prev;
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
      highlightTimers.current.delete(id);
    }, HIGHLIGHT_MS);
    // A previous timer for this id cannot exist (id is unique).
    highlightTimers.current.set(id, timer);
  }, []);
  useWsEvent(Events.AGENT, handleAgentEvent);

  // Expire live rows on a timer, not just on re-renders: if fetched data
  // stays referentially identical (structural sharing) or tracing is down,
  // the memo below never re-runs and stale "live" rows would linger forever.
  useEffect(() => {
    const iv = window.setInterval(() => {
      setLiveRows((prev) => {
        const now = Date.now();
        const next = prev.filter((r) => now - r.doneAt <= LIVE_TTL_MS);
        return next.length === prev.length ? prev : next;
      });
    }, 30_000);
    return () => window.clearInterval(iv);
  }, []);

  // Fetched history + live rows, dropping live rows once the poll has caught
  // up with them (same model + completion time within the dedup window).
  const requests = useMemo(() => {
    if (liveRows.length === 0) return fetched;
    const completionTimesByModel = new Map<string, number[]>();
    for (const r of fetched) {
      const done = Date.parse(r.start_time) + (r.duration_ms || 0);
      const times = completionTimesByModel.get(r.model);
      if (times) times.push(done);
      else completionTimesByModel.set(r.model, [done]);
    }
    const now = Date.now();
    const pending = liveRows.filter((lr) => {
      if (now - lr.doneAt > LIVE_TTL_MS) return false;
      const times = completionTimesByModel.get(lr.model);
      if (!times) return true;
      return !times.some((done) => Math.abs(done - lr.doneAt) < DEDUP_WINDOW_MS);
    });
    if (pending.length === 0) return fetched;
    return [...pending, ...fetched].slice(0, MAX_ROWS);
  }, [liveRows, fetched]);

  return (
    <Card className="h-full">
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          {t("recentRequests.title")}
          <span
            className="inline-flex h-2 w-2 shrink-0 animate-pulse rounded-full bg-emerald-500"
            aria-hidden="true"
          />
        </CardTitle>
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
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        ) : requests.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("recentRequests.noRequests")}
          </p>
        ) : (
          // ~12 rows visible, then internal scroll up to MAX_ROWS of history.
          <div className="max-h-[480px] overflow-auto overscroll-contain">
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10 bg-card">
                <tr className="border-b text-left text-muted-foreground">
                  <th className="pb-2 font-medium">
                    {t("recentRequests.columns.model")}
                  </th>
                  <th className="px-4 pb-2 text-right font-medium">
                    {t("recentRequests.columns.inOut")}
                  </th>
                  <th className="pb-2 pl-4 text-right font-medium">
                    {t("recentRequests.columns.when")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {requests.map((r) => {
                  const isFresh = freshIds.has(r.span_id);
                  return (
                    <tr
                      key={r.span_id}
                      className={`border-b transition-colors duration-700 last:border-0 ${
                        isFresh ? "bg-emerald-500/10 dark:bg-emerald-500/15" : ""
                      }`}
                    >
                      <td className="py-2.5 pr-4">
                        <div className="flex min-w-0 items-center gap-2">
                          {r.status === "error" && (
                            <span
                              className="h-2 w-2 shrink-0 rounded-full bg-red-500"
                              title={r.error || r.status}
                            />
                          )}
                          <div className="min-w-0">
                            <p
                              className="truncate font-mono text-xs font-medium"
                              title={r.model}
                            >
                              {r.model || "--"}
                            </p>
                            <p className="text-[11px] leading-tight text-muted-foreground">
                              {(r.cost_usd ?? 0) > 0 && (
                                <span>${r.cost_usd!.toFixed(4)} · </span>
                              )}
                              {formatDuration(r.duration_ms)}
                            </p>
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
                        <span className="mx-1.5 text-muted-foreground/60">
                          /
                        </span>
                        <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                          <ArrowDown className="h-3 w-3" />
                          {formatTokens(r.output_tokens)}
                        </span>
                      </td>
                      <td
                        className="whitespace-nowrap py-2.5 pl-4 text-right text-muted-foreground"
                        title={new Date(r.start_time).toLocaleString()}
                      >
                        {formatRelativeTime(r.start_time)}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
