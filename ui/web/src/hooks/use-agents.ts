import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { queryKeys } from "@/lib/query-keys";
import type { AgentData } from "@/types/agent";

/**
 * Shared agents fetch for the chat surface. The selector, the top bar and the
 * empty-state CTA all read from this cache, so /v1/agents is requested once
 * per screen instead of once per component.
 */
export function useAgents() {
  const http = useHttp();
  const connected = useAuthStore((s) => s.connected);

  return useQuery({
    queryKey: queryKeys.agents.chatList,
    queryFn: async () => {
      const res = await http.get<{ agents: AgentData[] }>("/v1/agents");
      return res.agents ?? [];
    },
    enabled: connected,
    staleTime: 30_000,
  });
}
