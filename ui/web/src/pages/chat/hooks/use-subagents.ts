import { useCallback, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useWs } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { Methods } from "@/api/protocol";
import { queryKeys } from "@/lib/query-keys";
import { toast } from "@/stores/use-toast-store";
import i18next from "i18next";
import { userFriendlyError } from "@/lib/error-utils";

/**
 * Durable subagent task rows returned by WS `subagents.list`
 * (internal/gateway/methods/subagents.go — subagentTaskJSON, camelCase).
 */
export interface SubagentTask {
  taskId: string;
  label: string;
  status: SubagentTaskStatus;
  model?: string;
  createdAt: string;
  completedAt?: string;
  archivedAt?: string;
  summary?: string;
  error?: string;
}

/**
 * Status vocabulary — mirrors internal/store/subagent_store.go terminal set
 * and internal/channels/telegram/commands_subagents.go:19-34 icons so the
 * web and Telegram surfaces read the same states.
 */
export type SubagentTaskStatus =
  | "queued"
  | "running"
  | "waiting"
  | "waiting_child"
  | "completed"
  | "failed"
  | "cancelled";

/** Non-terminal statuses — the query keeps polling while any row is active. */
export const SUBAGENT_ACTIVE_STATUSES: ReadonlySet<string> = new Set([
  "queued",
  "running",
  "waiting",
  "waiting_child",
]);

/** Terminal statuses (store.IsTerminalSubagentTaskStatus) — archivable/cancellable boundary. */
export const SUBAGENT_TERMINAL_STATUSES: ReadonlySet<string> = new Set([
  "completed",
  "failed",
  "cancelled",
]);

interface SubagentsListResponse {
  tasks?: SubagentTask[];
  count?: number;
}

/** Mutations shared by the sheet rows, the pill and the announce card. */
export interface SubagentActions {
  archiveTask: (taskId: string) => Promise<void>;
  cancelTask: (taskId: string) => Promise<void>;
  archiveCompleted: () => Promise<void>;
  pendingArchive: string | null;
  pendingCancel: string | null;
  pendingArchiveAll: boolean;
}

/**
 * Optimistic subagent mutations over WS `subagents.archive` /
 * `subagents.archive_completed` / `subagents.cancel`. Each writes the
 * expected outcome into the shared react-query cache immediately and rolls
 * back to the previous snapshot on error, then refetches server truth.
 *
 * Safe to call with an empty agentId (the announce card may render before an
 * agent is resolved) — callers disable their buttons in that case.
 */
export function useSubagentActions(agentId: string): SubagentActions {
  const ws = useWs();
  const queryClient = useQueryClient();
  const [pendingArchive, setPendingArchive] = useState<string | null>(null);
  const [pendingCancel, setPendingCancel] = useState<string | null>(null);
  const [pendingArchiveAll, setPendingArchiveAll] = useState(false);

  const key = queryKeys.subagents.list(agentId);

  const archiveTask = useCallback(
    async (taskId: string) => {
      const prev = queryClient.getQueryData<SubagentTask[]>(key);
      setPendingArchive(taskId);
      queryClient.setQueryData<SubagentTask[]>(key, (old) =>
        (old ?? []).filter((t) => t.taskId !== taskId),
      );
      try {
        await ws.call(Methods.SUBAGENTS_ARCHIVE, { taskId });
        toast.success(i18next.t("chat:subagents.toast.archived"));
      } catch (err) {
        queryClient.setQueryData(key, prev);
        toast.error(i18next.t("chat:subagents.toast.archiveFailed"), userFriendlyError(err));
        throw err;
      } finally {
        setPendingArchive(null);
        void queryClient.invalidateQueries({ queryKey: key });
      }
    },
    [queryClient, key, ws],
  );

  const cancelTask = useCallback(
    async (taskId: string) => {
      const prev = queryClient.getQueryData<SubagentTask[]>(key);
      setPendingCancel(taskId);
      // Optimistic: the gateway answers only after the durable row flips, so
      // showing "cancelled" right away matches what the refetch will confirm.
      queryClient.setQueryData<SubagentTask[]>(key, (old) =>
        (old ?? []).map((t) =>
          t.taskId === taskId ? { ...t, status: "cancelled" as const } : t,
        ),
      );
      try {
        await ws.call(Methods.SUBAGENTS_CANCEL, { taskId });
        toast.success(i18next.t("chat:subagents.toast.cancelled"));
      } catch (err) {
        queryClient.setQueryData(key, prev);
        toast.error(i18next.t("chat:subagents.toast.cancelFailed"), userFriendlyError(err));
        throw err;
      } finally {
        setPendingCancel(null);
        void queryClient.invalidateQueries({ queryKey: key });
      }
    },
    [queryClient, key, ws],
  );

  const archiveCompleted = useCallback(async () => {
    const prev = queryClient.getQueryData<SubagentTask[]>(key);
    const finished = (prev ?? []).filter((t) => SUBAGENT_TERMINAL_STATUSES.has(t.status));
    setPendingArchiveAll(true);
    queryClient.setQueryData<SubagentTask[]>(key, (old) =>
      (old ?? []).filter((t) => !SUBAGENT_TERMINAL_STATUSES.has(t.status)),
    );
    try {
      const res = await ws.call<{ archived?: number; count?: number }>(
        Methods.SUBAGENTS_ARCHIVE_COMPLETED,
        { agentId },
      );
      // Go responds {agentId, archived:n}; the plan text said {count} — accept
      // both, falling back to the optimistic local count if neither arrives.
      const count = res.archived ?? res.count ?? finished.length;
      toast.success(i18next.t("chat:subagents.toast.archivedCount", { count }));
    } catch (err) {
      queryClient.setQueryData(key, prev);
      toast.error(i18next.t("chat:subagents.toast.archiveFailed"), userFriendlyError(err));
      throw err;
    } finally {
      setPendingArchiveAll(false);
      void queryClient.invalidateQueries({ queryKey: key });
    }
  }, [queryClient, key, ws, agentId]);

  return { archiveTask, cancelTask, archiveCompleted, pendingArchive, pendingCancel, pendingArchiveAll };
}

/**
 * Live subagent task list for one agent (Paseo-style tracker pill + sheet).
 *
 * Polling: 5s only while at least one task is queued/running/waiting — fully
 * terminal lists never poll (plan 260922-0554 phase 6). The announce run
 * arrival in use-chat-messages invalidates this key so fresh completions
 * show up without waiting for the next tick.
 */
export function useSubagents(agentId: string) {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const actions = useSubagentActions(agentId);

  const query = useQuery({
    queryKey: queryKeys.subagents.list(agentId),
    queryFn: async () => {
      const res = await ws.call<SubagentsListResponse>(Methods.SUBAGENTS_LIST, {
        agentId,
        includeArchived: false,
      });
      return res.tasks ?? [];
    },
    enabled: !!agentId && connected,
    staleTime: 3_000,
    refetchInterval: (q) => {
      const tasks = q.state.data as SubagentTask[] | undefined;
      return tasks?.some((t) => SUBAGENT_ACTIVE_STATUSES.has(t.status)) ? 5_000 : false;
    },
  });

  const tasks = query.data ?? [];
  const activeCount = tasks.filter((t) => SUBAGENT_ACTIVE_STATUSES.has(t.status)).length;
  const terminalCount = tasks.filter((t) => SUBAGENT_TERMINAL_STATUSES.has(t.status)).length;

  return {
    tasks,
    activeCount,
    terminalCount,
    loading: query.isLoading,
    error: query.error instanceof Error ? query.error.message : query.error ? String(query.error) : null,
    refetch: query.refetch,
    ...actions,
  };
}
