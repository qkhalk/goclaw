import { useState, useEffect, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { useWsEvent } from "@/hooks/use-ws-event";
import { Methods, Events } from "@/api/protocol";

export interface ComputeNode {
  id: string;
  name: string;
  platform: string;
  capabilities: string[];
  trust: "pending" | "trusted" | "revoked";
  online: boolean;
  tenantId?: string;
  lastSeenAt?: string;
  createdAt: string;
  revokedAt?: string;
}

export interface CreateKeyResult {
  created: boolean;
  node: ComputeNode;
  key: string;
}

export function useComputeNodes() {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [nodes, setNodes] = useState<ComputeNode[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!connected) return;
    setLoading(true);
    try {
      const res = await ws.call<{ nodes: ComputeNode[] }>(Methods.NODES_LIST);
      setNodes(res.nodes ?? []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [ws, connected]);

  useEffect(() => {
    load();
  }, [load]);

  // Live refresh when a node registers / its trust changes / it is revoked.
  useWsEvent(Events.NODE_STATE, () => {
    load();
  });

  const createKey = useCallback(
    async (name: string): Promise<CreateKeyResult> => {
      const res = await ws.call<CreateKeyResult>(Methods.NODES_LIST, {
        create: true,
        name,
      });
      await load();
      return res;
    },
    [ws, load],
  );

  const trustNode = useCallback(
    async (nodeId: string, trust: "trusted" | "pending") => {
      await ws.call(Methods.NODES_LIST, { nodeId, trust });
      await load();
    },
    [ws, load],
  );

  const revokeNode = useCallback(
    async (nodeId: string) => {
      await ws.call(Methods.NODES_REVOKE, { nodeId });
      await load();
    },
    [ws, load],
  );

  return { nodes, loading, load, createKey, trustNode, revokeNode };
}
