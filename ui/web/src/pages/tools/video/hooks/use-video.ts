import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useRef } from "react";
import { useHttp } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";

/** One video render job (mirrors store.VideoRenderJob, snake_case wire). */
export interface VideoRenderJob {
  id: string;
  tenant_id: string;
  user_id: string;
  agent_id: string;
  session_key: string;
  status: "queued" | "rendering" | "done" | "failed" | "cancelled";
  engine: string;
  storyboard_json: string;
  output_path: string;
  output_size_bytes: number;
  error: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
  expires_at?: string;
}

/** WS payload of video.job.updated. */
export interface VideoJobEvent {
  jobId: string;
  status: VideoRenderJob["status"];
  progress: number;
  outputPath?: string;
  error?: string;
}

export function useVideoJobs(enabled: boolean) {
  const http = useHttp();
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: ["video", "jobs"],
    enabled,
    refetchInterval: (q) =>
      // Poll while any job is queued/rendering (WS events may not reach all roles).
      (q.state.data ?? []).some((j) => j.status === "queued" || j.status === "rendering")
        ? 5_000
        : false,
    queryFn: async () =>
      (await http.get<{ jobs: VideoRenderJob[] }>("/v1/video/jobs", { limit: "50" })).jobs ?? [],
  });

  const refresh = useCallback(
    () => queryClient.invalidateQueries({ queryKey: ["video", "jobs"] }),
    [queryClient],
  );

  // Live progress updates push a patch into the cached list, then refetch to
  // converge (timestamps/output fields are not in the event payload).
  const progressById = useRef(new Map<string, number>());
  useWsEvent("video.job.updated", (payload) => {
    const ev = payload as VideoJobEvent;
    progressById.current.set(ev.jobId, ev.progress);
    queryClient.setQueryData<{ jobs: VideoRenderJob[] }>(["video", "jobs"], (old) => {
      if (!old) return old;
      return {
        jobs: old.jobs.map((j) =>
          j.id === ev.jobId ? { ...j, status: ev.status, output_path: ev.outputPath ?? j.output_path, error: ev.error ?? j.error } : j,
        ),
      };
    });
    void refresh();
  });

  return { jobs: query.data ?? [], loading: query.isLoading, refresh, progressById };
}

export function useVideoCancel() {
  const http = useHttp();
  const queryClient = useQueryClient();
  return useCallback(
    async (id: string) => {
      const res = await http.delete<{ jobId: string; status: string }>(`/v1/video/jobs/${id}`);
      await queryClient.invalidateQueries({ queryKey: ["video", "jobs"] });
      return res;
    },
    [http, queryClient],
  );
}

/** Server-side storyboard validation before submit: POST /v1/video/jobs
 * runs the same Validate() as the render_video tool. Errors come back as
 * {error} JSON. */
export async function submitRenderJob(http: ReturnType<typeof useHttp>, storyboard: unknown) {
  return http.post<{ jobId: string; status: string }>("/v1/video/jobs", { storyboard });
}
