import { useCallback, useEffect, useRef, useState } from "react";
import { useWs } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { toast } from "@/stores/use-toast-store";
import i18next from "i18next";
import { userFriendlyError } from "@/lib/error-utils";
import { Methods } from "@/api/protocol";
import type { AgentJob } from "@/types/workspace";

/**
 * Loads agent jobs for a workspace via jobs.list.
 * Initial load + manual refresh + jobs.cancel mutation; deliberately polling-free.
 */
export function useJobs(workspaceId?: string | null) {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [jobs, setJobs] = useState<AgentJob[]>([]);
  const [loading, setLoading] = useState(true);
  const loadSeqRef = useRef(0);

  const load = useCallback(async () => {
    if (!connected) return;
    const seq = ++loadSeqRef.current;
    setLoading(true);
    try {
      const res = await ws.call<{ jobs: AgentJob[] }>(
        Methods.JOBS_LIST,
        { workspaceId: workspaceId ?? undefined },
      );
      if (seq === loadSeqRef.current) setJobs(res.jobs ?? []);
    } catch {
      // List failures degrade to an empty view; the panel shows its empty state.
      if (seq === loadSeqRef.current) setJobs([]);
    } finally {
      if (seq === loadSeqRef.current) setLoading(false);
    }
  }, [ws, workspaceId, connected]);

  useEffect(() => {
    load();
  }, [load]);

  const cancel = useCallback(
    async (jobId: string) => {
      try {
        await ws.call(Methods.JOBS_CANCEL, { jobId });
        toast.success(i18next.t("chat:jobsPanel.toast.cancelled"));
        await load();
      } catch (err) {
        toast.error(i18next.t("chat:jobsPanel.toast.cancelFailed"), userFriendlyError(err));
      }
    },
    [ws, load],
  );

  return { jobs, loading, refresh: load, cancel };
}
