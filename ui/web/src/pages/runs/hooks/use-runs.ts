import { useCallback, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";
import { queryKeys } from "@/lib/query-keys";
import type { RunRecord, RunTimelineItem } from "../types";

export const RUN_STATUS_FILTERS = [
  "running",
  "pending",
  "compacting",
  "completed",
  "failed",
  "cancelled",
] as const;

/** True for statuses where the run may still make progress. */
export function isRunActive(status: string): boolean {
  return status === "running" || status === "pending" || status === "compacting";
}

interface RunsListResponse {
  runs: RunRecord[];
  limit: number;
  offset: number;
}

export interface RunsListFilters {
  status?: string;
  page?: number;
  pageSize?: number;
}

/** List durable run records (runs.list). Pages via limit/offset; no server total. */
export function useRuns(filters: RunsListFilters = {}) {
  const ws = useWs();
  const { status = "", page = 0, pageSize = 25 } = filters;

  const params = useMemo(
    () => ({ status, limit: pageSize, offset: page * pageSize }),
    [status, page, pageSize],
  );

  const query = useQuery({
    queryKey: queryKeys.runs.list(params),
    queryFn: async () => {
      const res = await ws.call<RunsListResponse>(Methods.RUNS_LIST, params);
      return res.runs ?? [];
    },
    placeholderData: (prev) => prev,
    // Poll while any listed run is still in flight so live runs advance.
    refetchInterval: (q) => {
      const runs = q.state.data as RunRecord[] | undefined;
      return runs?.some((r) => isRunActive(r.status)) ? 5000 : false;
    },
  });

  const runs = query.data ?? [];
  // runs.list has no total count: "has more" = full page came back.
  const hasMore = runs.length === pageSize;

  return { runs, hasMore, loading: query.isLoading, fetching: query.isFetching, refetch: query.refetch };
}

interface RunsGetResponse {
  run: RunRecord;
}

interface RunsEventsResponse {
  runId: string;
  afterSeq: number;
  items: RunTimelineItem[];
  limit: number;
  nextAfter: number;
}

/** One durable run record (runs.get). */
export function useRun(runId: string) {
  const ws = useWs();

  const query = useQuery({
    queryKey: queryKeys.runs.detail(runId),
    queryFn: async () => {
      const res = await ws.call<RunsGetResponse>(Methods.RUNS_GET, { runId });
      return res.run;
    },
    enabled: !!runId,
    refetchInterval: (q) => {
      const run = q.state.data as RunRecord | undefined;
      return run && isRunActive(run.status) ? 5000 : false;
    },
  });

  return { run: query.data ?? null, loading: query.isLoading, error: query.error, refetch: query.refetch };
}

const RUN_EVENTS_PAGE = 500;

/**
 * Archived timeline items for one run (runs.events), accumulated page by page
 * through the afterSeq cursor. Loads the first page immediately; call
 * loadMore() to pull older→newer pages until exhausted.
 */
export function useRunEvents(runId: string) {
  const ws = useWs();
  const queryClient = useQueryClient();
  const itemsRef = useRef<RunTimelineItem[]>([]);
  const cursorRef = useRef(0);
  const exhaustedRef = useRef(false);

  const query = useQuery({
    queryKey: queryKeys.runs.events(runId),
    queryFn: async () => {
      const res = await ws.call<RunsEventsResponse>(Methods.RUNS_EVENTS, {
        runId,
        afterSeq: cursorRef.current,
        limit: RUN_EVENTS_PAGE,
      });
      const fresh = res.items ?? [];
      if (fresh.length === 0 || res.nextAfter <= cursorRef.current) {
        exhaustedRef.current = true;
      } else {
        cursorRef.current = res.nextAfter;
      }
      // Merge by seq so a refetch of the first page never duplicates items.
      const bySeq = new Map<number, RunTimelineItem>();
      for (const item of [...itemsRef.current, ...fresh]) bySeq.set(item.seq, item);
      itemsRef.current = [...bySeq.values()].sort((a, b) => a.seq - b.seq);
      return itemsRef.current;
    },
    enabled: !!runId,
    staleTime: 0,
  });

  const loadMore = useCallback(() => {
    if (exhaustedRef.current) return;
    void queryClient.invalidateQueries({ queryKey: queryKeys.runs.events(runId) });
  }, [queryClient, runId]);

  const items = query.data ?? [];

  return {
    items,
    loading: query.isLoading,
    hasMore: !exhaustedRef.current && items.length > 0,
    loadMore,
    refetch: query.refetch,
  };
}
