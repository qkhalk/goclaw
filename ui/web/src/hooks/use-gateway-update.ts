import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import type { HttpClient } from "@/api/http-client";
import { useAuthStore } from "@/stores/use-auth-store";
import { queryKeys } from "@/lib/query-keys";

export interface GatewayUpdateCheck {
  current: string;
  latest?: string;
  available: boolean;
  version?: string;
  asset?: string;
  release_url?: string;
  reason?: string;
}

/** Poll the gateway for a newer stable release. Only owners/admins may call
 * the check endpoint, so the query stays disabled for other roles and while
 * the WS is disconnected. Re-checked every 30 minutes (and on reconnect via
 * the enabled flag flipping). */
export function useGatewayUpdateCheck() {
  const http = useHttp();
  const connected = useAuthStore((s) => s.connected);
  const role = useAuthStore((s) => s.role);
  const canCheck = connected && (role === "owner" || role === "admin");

  return useQuery({
    queryKey: queryKeys.system.gatewayUpdateCheck,
    queryFn: () => http.get<GatewayUpdateCheck>("/v1/system/gateway/upgrade/check"),
    enabled: canCheck,
    staleTime: 15 * 60_000,
    refetchInterval: 30 * 60_000,
    refetchOnWindowFocus: true,
    retry: 1,
  });
}

/** Trigger the host upgrade script for the given tag. The gateway replaces
 * its binary and restarts — the WS drops and reconnects on the new version. */
export async function applyGatewayUpdate(http: HttpClient, tag: string): Promise<void> {
  await http.post("/v1/system/gateway/upgrade", { tag });
}
