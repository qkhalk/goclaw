import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import type { ChannelInstanceData } from "@/types/channel";

export interface GroupWriterGroupInfo {
  group_id: string;
  writer_count: number;
}

export interface GroupFileWriterData {
  user_id: string;
  display_name?: string;
  username?: string;
}

export function useChannelDetail(instanceId: string | undefined) {
  const http = useHttp();
  const queryClient = useQueryClient();

  const { data, isLoading: loading } = useQuery({
    queryKey: queryKeys.channels.detail(instanceId ?? ""),
    queryFn: async () => {
      return http.get<ChannelInstanceData>(`/v1/channels/instances/${instanceId}`);
    },
    enabled: !!instanceId,
  });

  const instance = data ?? null;

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: queryKeys.channels.detail(instanceId ?? "") });
    queryClient.invalidateQueries({ queryKey: queryKeys.channels.all });
  }, [queryClient, instanceId]);

  const updateInstance = useCallback(
    async (updates: Record<string, unknown>) => {
      if (!instanceId) return;
      await http.put(`/v1/channels/instances/${instanceId}`, updates);
      await invalidate();
    },
    [instanceId, http, invalidate],
  );

  // Writers API
  const listWriterGroups = useCallback(
    async (): Promise<GroupWriterGroupInfo[]> => {
      if (!instanceId) return [];
      const res = await http.get<{ groups: GroupWriterGroupInfo[] }>(`/v1/channels/instances/${instanceId}/writers/groups`);
      return res.groups ?? [];
    },
    [instanceId, http],
  );

  const listWriters = useCallback(
    async (groupId: string): Promise<GroupFileWriterData[]> => {
      if (!instanceId) return [];
      const res = await http.get<{ writers: GroupFileWriterData[] }>(`/v1/channels/instances/${instanceId}/writers`, { group_id: groupId });
      return res.writers ?? [];
    },
    [instanceId, http],
  );

  const addWriter = useCallback(
    async (groupId: string, userId: string, displayName?: string, username?: string) => {
      if (!instanceId) return;
      await http.post(`/v1/channels/instances/${instanceId}/writers`, {
        group_id: groupId,
        user_id: userId,
        display_name: displayName ?? "",
        username: username ?? "",
      });
    },
    [instanceId, http],
  );

  const removeWriter = useCallback(
    async (groupId: string, userId: string) => {
      if (!instanceId) return;
      await http.delete(`/v1/channels/instances/${instanceId}/writers/${userId}?group_id=${encodeURIComponent(groupId)}`);
    },
    [instanceId, http],
  );

  return {
    instance,
    loading,
    updateInstance,
    listWriterGroups,
    listWriters,
    addWriter,
    removeWriter,
    refresh: invalidate,
  };
}
