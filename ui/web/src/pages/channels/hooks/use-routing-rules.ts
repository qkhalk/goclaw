import { useCallback, useState, useEffect } from "react";
import { useWs } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { Methods } from "@/api/protocol";

/** Match constraints for a routing rule; unset fields are wildcards. */
export interface RoutingRuleMatch {
  channel?: string;
  accountId?: string;
  peerKind?: string;
  peerId?: string;
  guildId?: string;
}

export interface RoutingRule {
  id: string;
  tenantId: string;
  priority: number;
  match: RoutingRuleMatch;
  targetAgentId: string;
  enabled: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface RoutingRuleInput {
  id?: string;
  priority: number;
  match: RoutingRuleMatch;
  targetAgentId: string;
  enabled: boolean;
  [key: string]: unknown;
}

export function useRoutingRules() {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [rules, setRules] = useState<RoutingRule[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!connected) return;
    setLoading(true);
    try {
      const res = await ws.call<{ rules: RoutingRule[] }>(Methods.ROUTING_RULES_LIST);
      setRules(res.rules ?? []);
    } catch {
      setRules([]);
    } finally {
      setLoading(false);
    }
  }, [ws, connected]);

  useEffect(() => {
    load();
  }, [load]);

  const saveRule = useCallback(
    async (input: RoutingRuleInput) => {
      await ws.call(Methods.ROUTING_RULES_SET, input);
      await load();
    },
    [ws, load],
  );

  const deleteRule = useCallback(
    async (id: string) => {
      await ws.call(Methods.ROUTING_RULES_DELETE, { id });
      await load();
    },
    [ws, load],
  );

  return { rules, loading, refresh: load, saveRule, deleteRule };
}
