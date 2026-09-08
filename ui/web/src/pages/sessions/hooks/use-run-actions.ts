import { useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";
import { toast } from "@/stores/use-toast-store";
import i18next from "i18next";
import { userFriendlyError } from "@/lib/error-utils";

/**
 * Run-level lifecycle actions (durable agent_runs): pause a running run,
 * resume / wake a paused one. Backed by runs.pause / runs.resume / runs.wake.
 */
export function useRunActions() {
  const ws = useWs();

  const runAction = useCallback(
    async (method: string, runId: string, successKey: string, failKey: string) => {
      if (!ws.isConnected || !runId) return;
      try {
        await ws.call(method, { runId });
        toast.success(i18next.t(successKey));
      } catch (err) {
        toast.error(i18next.t(failKey), userFriendlyError(err));
      }
    },
    [ws],
  );

  const pauseRun = useCallback(
    (runId: string) => runAction(Methods.RUNS_PAUSE, runId, "sessions:toast.runPaused", "sessions:toast.runActionFailed"),
    [runAction],
  );
  const resumeRun = useCallback(
    (runId: string) => runAction(Methods.RUNS_RESUME, runId, "sessions:toast.runResumed", "sessions:toast.runActionFailed"),
    [runAction],
  );
  const wakeRun = useCallback(
    (runId: string) => runAction(Methods.RUNS_WAKE, runId, "sessions:toast.runWoken", "sessions:toast.runActionFailed"),
    [runAction],
  );

  return { pauseRun, resumeRun, wakeRun };
}
