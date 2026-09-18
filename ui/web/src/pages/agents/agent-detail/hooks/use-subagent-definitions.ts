import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { toast } from "@/stores/use-toast-store";
import i18next from "i18next";
import { userFriendlyError } from "@/lib/error-utils";
import type { SubagentDefinition } from "@/types/agent";

/**
 * CRUD access to an agent's named subagent definitions. Definitions live in
 * the agent's subagents_config JSONB under "definitions" (mirrors Go
 * config.SubagentDefinition); the backend exposes them as a nested collection
 * under /v1/agents/{id}/subagent-definitions.
 */

export function useSubagentDefinitions(agentId: string) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const queryKey = ["agents", agentId, "subagent-definitions"] as const;

  const query = useQuery({
    queryKey,
    enabled: Boolean(agentId),
    staleTime: 30_000,
    queryFn: async () => {
      const res = await http.get<{ definitions: SubagentDefinition[] }>(
        `/v1/agents/${agentId}/subagent-definitions`,
      );
      return res.definitions ?? [];
    },
  });

  const invalidate = useCallback(
    async () => {
      await queryClient.invalidateQueries({
        queryKey: ["agents", agentId, "subagent-definitions"] as const,
      });
    },
    [queryClient, agentId],
  );

  const createDefinition = useCallback(
    async (def: SubagentDefinition) => {
      try {
        await http.post(`/v1/agents/${agentId}/subagent-definitions`, def);
        await invalidate();
        toast.success(i18next.t("agents:subagentDefs.toast.created"));
      } catch (err) {
        toast.error(i18next.t("agents:subagentDefs.toast.createFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [http, agentId, invalidate],
  );

  const updateDefinition = useCallback(
    async (name: string, def: SubagentDefinition) => {
      try {
        await http.put(`/v1/agents/${agentId}/subagent-definitions/${encodeURIComponent(name)}`, def);
        await invalidate();
        toast.success(i18next.t("agents:subagentDefs.toast.updated"));
      } catch (err) {
        toast.error(i18next.t("agents:subagentDefs.toast.updateFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [http, agentId, invalidate],
  );

  const deleteDefinition = useCallback(
    async (name: string) => {
      try {
        await http.delete(`/v1/agents/${agentId}/subagent-definitions/${encodeURIComponent(name)}`);
        await invalidate();
        toast.success(i18next.t("agents:subagentDefs.toast.deleted"));
      } catch (err) {
        toast.error(i18next.t("agents:subagentDefs.toast.deleteFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [http, agentId, invalidate],
  );

  return {
    definitions: query.data ?? [],
    loading: query.isPending,
    refresh: invalidate,
    createDefinition,
    updateDefinition,
    deleteDefinition,
  };
}
