import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { SkillWithGrant } from "@/types/skill";

export function useAgentSkills(agentId: string) {
  const http = useHttp();
  const queryClient = useQueryClient();

  const { data: skills = [], isLoading: loading } = useQuery({
    queryKey: queryKeys.skills.agentGrants(agentId),
    queryFn: () =>
      http
        .get<{ skills: SkillWithGrant[] }>(`/v1/agents/${agentId}/skills`)
        .then((r) => r.skills ?? []),
  });

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.skills.agentGrants(agentId) }),
    [queryClient, agentId],
  );

  const grantSkill = useCallback(
    async (skillId: string) => {
      await http.post(`/v1/skills/${skillId}/grants/agent`, { agent_id: agentId });
      await invalidate();
    },
    [http, agentId, invalidate],
  );

  const revokeSkill = useCallback(
    async (skillId: string) => {
      await http.delete(`/v1/skills/${skillId}/grants/agent/${agentId}`);
      await invalidate();
    },
    [http, agentId, invalidate],
  );

  return { skills, loading, grantSkill, revokeSkill };
}
