import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { parentPath } from "../drive/paths";

/** One connected cloud account (tokens never returned by the API). */
export interface CloudAccount {
  id: string;
  provider: string;
  email: string;
  display_name: string;
  scopes: string;
  token_expires_at?: string;
  status: "active" | "expired" | "revoked" | "error";
  status_message: string;
  /** Owner user id — only the owner can re-grant a shared account. */
  user_id: string;
  /** Tenant-wide shared (enterprise "company drive") — admin-set. */
  shared: boolean;
  /** True when the stored OAuth grant includes the provider's write scope.
   * False for accounts connected before the write upgrade — read-only until
   * the owner re-grants. */
  can_write?: boolean;
  created_at: string;
}

export type CloudBindingScopeType = "tenant" | "user" | "group";

/** One per-scope account assignment (tenant default / user / group).
 * enabled=false keeps the rule configured but excluded from resolution;
 * priority breaks ties within a scope tier (lower wins, default 100). */
export interface CloudBinding {
  id: string;
  scope_type: CloudBindingScopeType;
  scope_key: string;
  provider: string;
  account_id: string;
  created_by: string;
  enabled: boolean;
  priority: number;
}

export type CloudProvider = "google" | "onedrive";

/** One tenant-level one-way folder sync pair (source → target, additive
 * mirror — files deleted at the source are never deleted at the target). */
export interface CloudSyncPair {
  id: string;
  tenant_id: string;
  source_account_id: string;
  source_path: string;
  target_account_id: string;
  target_path: string;
  /** 0 = manual ("run now") only. */
  interval_minutes: number;
  enabled: boolean;
  last_run_at?: string;
  /** ok | error | running (absent = never ran). */
  last_status?: "ok" | "error" | "running";
  last_error?: string;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

/** One starred file/folder of the caller (user-level metadata). */
export interface CloudStarred {
  id: string;
  tenant_id: string;
  user_id: string;
  account_id: string;
  path: string;
  name: string;
  is_dir: boolean;
  starred_at: string;
}

/** One remote entry (GET /v1/cloud/accounts/{id}/files). */
export interface CloudFileEntry {
  name: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

export interface CloudStatus {
  enabled: boolean;
  edition: string;
  providers: { google?: { configured: boolean }; onedrive?: { configured: boolean } };
}

export interface CloudStartResponse {
  auth_url: string;
  redirect_uri: string;
  /** "callback" = browser lands back on the server; "paste" = user pastes the
   * loopback redirect URL back (embedded shared client, zero config). */
  mode: "callback" | "paste";
}

export function useCloudStatus() {
  const http = useHttp();
  return useQuery({
    queryKey: queryKeys.cloud.status,
    staleTime: 30_000,
    queryFn: () => http.get<CloudStatus>("/v1/cloud/status"),
  });
}

/** Admin view of one provider's OAuth client config (secret never returned). */
export interface CloudSettings {
  client_id: string;
  secret_set: boolean;
  redirect_uri: string;
}

export function useCloudSettings(provider: CloudProvider, enabled: boolean) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: queryKeys.cloud.settings(provider),
    enabled,
    queryFn: () => http.get<CloudSettings>(`/v1/cloud/settings?provider=${provider}`),
  });

  const saveSettings = useCallback(
    async (client_id: string, client_secret: string) => {
      await http.put(`/v1/cloud/settings?provider=${provider}`, { client_id, client_secret });
      await queryClient.invalidateQueries({ queryKey: queryKeys.cloud.all });
    },
    [http, provider, queryClient],
  );

  return { settings: query.data, saveSettings };
}

export function useCloudAccounts() {
  const http = useHttp();
  const queryClient = useQueryClient();
  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.cloud.all }),
    [queryClient],
  );

  const query = useQuery({
    queryKey: queryKeys.cloud.accounts,
    queryFn: async () =>
      (await http.get<{ accounts: CloudAccount[] }>("/v1/cloud/accounts")).accounts,
  });

  const disconnect = useCallback(
    async (id: string) => {
      await http.delete(`/v1/cloud/accounts/${id}`);
      await invalidate();
    },
    [http, invalidate],
  );

  const startConnect = useCallback(
    async (provider: CloudProvider) =>
      http.post<CloudStartResponse>(`/v1/cloud/oauth/${provider}/start`),
    [http],
  );

  /** Finish the paste-back flow: submit the address-bar URL the browser
   * landed on after consent (loopback redirect, nothing listening). */
  const completeConnect = useCallback(
    async (provider: CloudProvider, url: string) => {
      const res = await http.post<{ email: string }>(`/v1/cloud/oauth/${provider}/complete`, { url });
      await invalidate();
      return res;
    },
    [http, invalidate],
  );

  /** Toggle the tenant-wide shared flag (admin). */
  const setShared = useCallback(
    async (id: string, shared: boolean) => {
      await http.put(`/v1/cloud/accounts/${id}/shared`, { shared });
      await invalidate();
    },
    [http, invalidate],
  );

  return { accounts: query.data ?? [], loading: query.isLoading, refresh: invalidate, disconnect, startConnect, completeConnect, setShared };
}

export function useCloudBindings(enabled: boolean) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: ["cloud", "bindings"] }),
    [queryClient],
  );

  const query = useQuery({
    queryKey: queryKeys.cloud.bindings,
    enabled,
    queryFn: async () =>
      (await http.get<{ bindings: CloudBinding[] }>("/v1/cloud/bindings")).bindings,
  });

  const upsertBinding = useCallback(
    async (input: {
      scope_type: CloudBindingScopeType;
      scope_key: string;
      provider: string;
      account_id: string;
      enabled?: boolean;
      priority?: number;
    }) => {
      await http.put("/v1/cloud/bindings", input);
      await invalidate();
    },
    [http, invalidate],
  );

  const deleteBinding = useCallback(
    async (id: string) => {
      await http.delete(`/v1/cloud/bindings/${id}`);
      await invalidate();
    },
    [http, invalidate],
  );

  return { bindings: query.data ?? [], loading: query.isLoading, upsertBinding, deleteBinding };
}

/** Sync-pair CRUD + manual run for the settings sheet. Tenant-admin only
 * (every endpoint answers 403 for members). */
export function useCloudSyncPairs(enabled: boolean) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.cloud.syncPairs }),
    [queryClient],
  );

  const query = useQuery({
    queryKey: queryKeys.cloud.syncPairs,
    enabled,
    queryFn: async () =>
      (await http.get<{ pairs: CloudSyncPair[] }>("/v1/cloud/sync-pairs")).pairs,
  });

  const createPair = useCallback(
    async (input: {
      source_account_id: string;
      source_path: string;
      target_account_id: string;
      target_path: string;
      interval_minutes?: number;
      enabled?: boolean;
    }) => {
      await http.post("/v1/cloud/sync-pairs", input);
      await invalidate();
    },
    [http, invalidate],
  );

  const updatePair = useCallback(
    async (id: string, input: Partial<Omit<CloudSyncPair, "id" | "tenant_id" | "created_at" | "updated_at">>) => {
      await http.put(`/v1/cloud/sync-pairs/${id}`, input);
      await invalidate();
    },
    [http, invalidate],
  );

  const deletePair = useCallback(
    async (id: string) => {
      await http.delete(`/v1/cloud/sync-pairs/${id}`);
      await invalidate();
    },
    [http, invalidate],
  );

  const runPair = useCallback(async (id: string) => {
    await http.post(`/v1/cloud/sync-pairs/${id}/run`);
  }, [http]);

  return { pairs: query.data ?? [], loading: query.isLoading, createPair, updatePair, deletePair, runPair };
}

/** Cross-account transfer (POST /v1/cloud/transfer). Returns the async job id
 * for folders ({job_id}) or 0-ish for synchronous file transfers. */
export function useCloudTransfer() {
  const http = useHttp();
  return useCallback(
    async (input: {
      source_account_id: string;
      source_path: string;
      target_account_id: string;
      target_path: string;
      mode?: "copy" | "move";
    }) => http.post<{ ok?: boolean; job_id?: string }>("/v1/cloud/transfer", input),
    [http],
  );
}

/** Poll one async transfer job (GET /v1/cloud/transfers/{jobId}) until it
 * finishes. Polling stops on error or completion. */
export function useCloudTransferJob(jobId: string | null) {
  const http = useHttp();
  return useQuery({
    queryKey: queryKeys.cloud.transferJob(jobId ?? ""),
    enabled: !!jobId,
    refetchInterval: (query) => {
      const job = query.state.data as CloudTransferJob | undefined;
      return job && !job.finished ? 3000 : false;
    },
    queryFn: () => http.get<CloudTransferJob>(`/v1/cloud/transfers/${jobId}`),
  });
}

/** Shape of GET /v1/cloud/transfers/{jobId}. */
export interface CloudTransferJob {
  job_id: string;
  finished: boolean;
  success: boolean;
  error?: string;
}

/** Starred items of the caller (user-level, provider-agnostic metadata). */
export function useCloudStarred() {
  const http = useHttp();
  const queryClient = useQueryClient();
  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.cloud.starred }),
    [queryClient],
  );

  const query = useQuery({
    queryKey: queryKeys.cloud.starred,
    queryFn: async () => (await http.get<{ items: CloudStarred[] }>("/v1/cloud/starred")).items,
  });

  const add = useCallback(
    async (input: { account_id: string; path: string; name: string; is_dir: boolean }) => {
      await http.put("/v1/cloud/starred", input);
      await invalidate();
    },
    [http, invalidate],
  );

  const remove = useCallback(
    async (id: string) => {
      await http.delete(`/v1/cloud/starred/${id}`);
      await invalidate();
    },
    [http, invalidate],
  );

  /** Toggle by account+path: removes when already starred, adds otherwise. */
  const toggle = useCallback(
    async (input: { account_id: string; path: string; name: string; is_dir: boolean }) => {
      const items = queryClient.getQueryData<CloudStarred[]>(queryKeys.cloud.starred) ?? [];
      const existing = items.find((s) => s.account_id === input.account_id && s.path === input.path);
      if (existing) {
        await remove(existing.id);
        return false;
      }
      await add(input);
      return true;
    },
    [queryClient, remove, add],
  );

  const isStarred = useCallback(
    (accountId: string, path: string) =>
      (query.data ?? []).some((s) => s.account_id === accountId && s.path === path),
    [query.data],
  );

  return { items: query.data ?? [], loading: query.isLoading, add, remove, toggle, isStarred };
}

/** File-operation mutations for one account, consuming the Phase 4 backend:
 * mkdir / move(rename) / copy / delete / upload (multipart with progress) /
 * download. Mutations invalidate the account's file listing + quota (uploads
 * and deletes change both); mkdir is targeted — only the parent folder's
 * listing, refetched in the background without blocking the caller. */
export function useCloudFileOps(accountId: string) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const invalidate = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.cloud.allFiles(accountId) });
    await queryClient.invalidateQueries({ queryKey: queryKeys.cloud.about(accountId) });
  }, [queryClient, accountId]);

  const mkdir = useCallback(
    async (path: string) => {
      await http.post(`/v1/cloud/accounts/${accountId}/folders`, { path });
      // Mkdir only adds one entry to the PARENT listing: invalidate just that
      // key and let react-query refetch it in the background (no await, no
      // quota refetch, no purge of every cached folder of the account) — this
      // keeps "New folder" as fast as the POST round trip.
      void queryClient.invalidateQueries({
        queryKey: queryKeys.cloud.files(accountId, parentPath(path)),
      });
    },
    [http, accountId, queryClient],
  );

  /** Rename or move within the account (PATCH — same endpoint). */
  const move = useCallback(
    async (from: string, to: string) => {
      await http.patch(`/v1/cloud/accounts/${accountId}/files`, { from, to });
      await invalidate();
    },
    [http, accountId, invalidate],
  );

  const copy = useCallback(
    async (from: string, to: string) => {
      await http.post(`/v1/cloud/accounts/${accountId}/files/copy`, { from, to });
      await invalidate();
    },
    [http, accountId, invalidate],
  );

  /** PERMANENT delete (no trash on the provider side). */
  const remove = useCallback(
    async (path: string, isDir: boolean) => {
      await http.delete(
        `/v1/cloud/accounts/${accountId}/files?path=${encodeURIComponent(path)}&isDir=${isDir}`,
      );
      await invalidate();
    },
    [http, accountId, invalidate],
  );

  /** Upload one file into folder `dir` with real progress (XHR). */
  const upload = useCallback(
    (file: File, dir: string, opts?: { onProgress?: (loaded: number, total: number) => void; signal?: AbortSignal }) => {
      const fd = new FormData();
      fd.append("file", file);
      fd.append("path", dir);
      return http
        .uploadWithProgress<{ path: string; filename: string; size: number }>(
          `/v1/cloud/accounts/${accountId}/files`,
          fd,
          opts,
        )
        .then(async (res) => {
          await invalidate();
          return res;
        });
    },
    [http, accountId, invalidate],
  );

  /** Download one file as a Blob (auth headers required — no direct href). */
  const download = useCallback(
    (path: string) => http.fetchBlob(`/v1/cloud/accounts/${accountId}/files/download`, { path }),
    [http, accountId],
  );

  return { mkdir, move, copy, remove, upload, download, invalidate };
}
