import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useHttp } from "@/hooks/use-ws";

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
  /** Tenant-wide shared (enterprise "company drive") — admin-set. */
  shared: boolean;
  created_at: string;
}

export type CloudBindingScopeType = "tenant" | "user" | "group";

/** One per-scope account assignment (tenant default / user / group). */
export interface CloudBinding {
  id: string;
  scope_type: CloudBindingScopeType;
  scope_key: string;
  provider: string;
  account_id: string;
  created_by: string;
}

export type CloudProvider = "google" | "onedrive";

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
    queryKey: ["cloud", "status"],
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
    queryKey: ["cloud", "settings", provider],
    enabled,
    queryFn: () => http.get<CloudSettings>(`/v1/cloud/settings?provider=${provider}`),
  });

  const saveSettings = useCallback(
    async (client_id: string, client_secret: string) => {
      await http.put(`/v1/cloud/settings?provider=${provider}`, { client_id, client_secret });
      await queryClient.invalidateQueries({ queryKey: ["cloud"] });
    },
    [http, provider, queryClient],
  );

  return { settings: query.data, saveSettings };
}

export function useCloudAccounts() {
  const http = useHttp();
  const queryClient = useQueryClient();
  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: ["cloud"] }),
    [queryClient],
  );

  const query = useQuery({
    queryKey: ["cloud", "accounts"],
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
    queryKey: ["cloud", "bindings"],
    enabled,
    queryFn: async () =>
      (await http.get<{ bindings: CloudBinding[] }>("/v1/cloud/bindings")).bindings,
  });

  const upsertBinding = useCallback(
    async (input: { scope_type: CloudBindingScopeType; scope_key: string; provider: string; account_id: string }) => {
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
