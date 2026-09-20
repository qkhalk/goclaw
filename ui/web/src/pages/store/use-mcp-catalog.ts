import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import i18next from "i18next";
import { useHttp } from "@/hooks/use-ws";

/**
 * MCP tool-server catalog for the Tool Store page.
 *
 * Backend contract (all under /v1/mcp):
 * - GET  /catalog              → installable entries + installed package state
 * - POST /install              → catalog {name} or custom repo payload → {job_id}
 * - GET  /install/{job_id}     → job progress (polled while status === "running")
 * - DEL /installed/{name}      → uninstall
 */

/** Installed package record embedded in a catalog entry once installed. */
export interface McpInstalledPackage {
  name: string;
  display_name: string;
  source: string;
  repo: string;
  ref: string;
  commit_sha: string;
  runtime: string;
  entry: string;
  install_dir: string;
  status: "installed" | "installing" | "failed";
  error?: string;
  tool_count: number;
  created_at: string;
  updated_at: string;
}

/** One entry of GET /v1/mcp/catalog. */
export interface McpCatalogEntry {
  name: string;
  display_name: string;
  description: string;
  category: string;
  runtime: string;
  entry: string;
  repo: string;
  ram_note?: string;
  installed: boolean;
  package: McpInstalledPackage | null;
}

export interface McpCatalog {
  entries: McpCatalogEntry[];
  default_repo: string;
}

/** GET /v1/mcp/install/{job_id} — polled install job progress. */
export interface McpInstallJob {
  id: string;
  name: string;
  status: "running" | "done" | "error";
  step: "" | "preflight" | "clone" | "deps" | "smoke" | "register";
  progress: number;
  log: string[];
  error: string;
  started_at: string;
  finished_at: string;
}

/** Custom (any-repo) install body for POST /v1/mcp/install. */
export interface McpCustomInstallInput {
  name: string;
  display_name?: string;
  repo: string;
  ref?: string;
  subdir?: string;
  runtime: "node" | "python";
  entry: string;
}

export type McpInstallResult = { ok: true } | { ok: false; error: string };

export interface McpActionError {
  /** Slug of the catalog entry the failed action targeted. */
  for: string;
  message: string;
}

// Local query keys. Prefixed with "mcp" so MCP-page mutations (which invalidate
// the ["mcp"] prefix) also refresh the catalog; the catalog never invalidates
// the MCP page's server list on its own.
const MCP_CATALOG_KEY = ["mcp", "catalog"] as const;
const mcpJobKey = (jobId: string | null) => ["mcp", "install-job", jobId] as const;
const JOB_POLL_INTERVAL_MS = 1500;

const tr = (key: string) => i18next.t(`tools:store.${key}`);

export function useMcpCatalog() {
  const http = useHttp();
  const queryClient = useQueryClient();
  const [jobId, setJobId] = useState<string | null>(null);
  /** Entry name between POST /install acceptance and the first job poll response. */
  const [pendingName, setPendingName] = useState<string | null>(null);
  const [actionError, setActionError] = useState<McpActionError | null>(null);

  const catalogQuery = useQuery({
    queryKey: MCP_CATALOG_KEY,
    queryFn: async () => {
      const res = await http.get<McpCatalog>("/v1/mcp/catalog");
      return { entries: res.entries ?? [], default_repo: res.default_repo ?? "" };
    },
    staleTime: 30_000,
  });

  const jobQuery = useQuery({
    queryKey: mcpJobKey(jobId),
    enabled: jobId !== null,
    refetchInterval: (query) =>
      query.state.data?.status === "running" ? JOB_POLL_INTERVAL_MS : false,
    queryFn: async () => {
      if (jobId === null) throw new Error("No active MCP install job");
      return http.get<McpInstallJob>(`/v1/mcp/install/${encodeURIComponent(jobId)}`);
    },
  });

  const job = jobId === null ? null : jobQuery.data ?? null;
  const entries = catalogQuery.data?.entries ?? [];
  /** True from POST acceptance until the job reaches a terminal status. */
  const installing = jobId !== null && (job === null || job.status === "running");
  const activeName = job?.name ?? pendingName;

  // Terminal job states are reflected on the cards by a catalog refetch.
  const jobStatus = job?.status;
  useEffect(() => {
    if (jobStatus === "done" || jobStatus === "error") {
      void queryClient.invalidateQueries({ queryKey: MCP_CATALOG_KEY });
    }
  }, [jobStatus, queryClient]);

  // A permanently failing job poll (e.g. expired job id) must not spin forever.
  const jobError = jobId !== null ? jobQuery.error : null;
  useEffect(() => {
    if (!jobError) return;
    setActionError({
      for: pendingName ?? "",
      message: jobError instanceof Error && jobError.message ? jobError.message : tr("mcp_job_failed"),
    });
    setJobId(null);
    setPendingName(null);
  }, [jobError, pendingName]);

  const install = useCallback(
    async (name: string): Promise<McpInstallResult> => {
      if (installing) {
        const message = tr("mcp_in_progress");
        setActionError({ for: name, message });
        return { ok: false, error: message };
      }
      if (entries.find((e) => e.name === name)?.installed) {
        const message = tr("mcp_already_installed");
        setActionError({ for: name, message });
        return { ok: false, error: message };
      }
      try {
        const res = await http.post<{ job_id: string }>("/v1/mcp/install", { name });
        setActionError(null);
        setPendingName(name);
        setJobId(res.job_id);
        return { ok: true };
      } catch (err) {
        // 409/400 bodies carry an already-localized message — surface it as-is.
        const message = err instanceof Error && err.message ? err.message : tr("mcp_job_failed");
        setActionError({ for: name, message });
        return { ok: false, error: message };
      }
    },
    [http, installing, entries],
  );

  const installCustom = useCallback(
    async (input: McpCustomInstallInput): Promise<McpInstallResult> => {
      if (installing) return { ok: false, error: tr("mcp_in_progress") };
      try {
        const res = await http.post<{ job_id: string }>("/v1/mcp/install", input);
        setActionError(null);
        setPendingName(input.name);
        setJobId(res.job_id);
        return { ok: true };
      } catch (err) {
        // Dialog-local error path — the message is rendered inside the dialog.
        return { ok: false, error: err instanceof Error && err.message ? err.message : tr("mcp_job_failed") };
      }
    },
    [http, installing],
  );

  const uninstall = useCallback(
    async (name: string): Promise<void> => {
      try {
        await http.delete<{ status: string }>(`/v1/mcp/installed/${encodeURIComponent(name)}`);
        setActionError(null);
      } catch (err) {
        // 404 (already gone) or transport failure — the refetch below reconciles the cards.
        setActionError({
          for: name,
          message: err instanceof Error && err.message ? err.message : tr("mcp_job_failed"),
        });
      } finally {
        void queryClient.invalidateQueries({ queryKey: MCP_CATALOG_KEY });
      }
    },
    [http, queryClient],
  );

  const dismissJob = useCallback(() => {
    setJobId(null);
    setPendingName(null);
  }, []);

  const clearActionError = useCallback(() => setActionError(null), []);

  return {
    entries,
    loading: catalogQuery.isLoading,
    error: catalogQuery.error,
    refetch: catalogQuery.refetch,
    /** Current install job (any status) or null. */
    job,
    /** A job was accepted and has not reached a terminal status yet. */
    installing,
    /** Entry slug the active job (or pending POST) belongs to. */
    activeName,
    install,
    installCustom,
    uninstall,
    actionError,
    clearActionError,
    dismissJob,
  };
}
