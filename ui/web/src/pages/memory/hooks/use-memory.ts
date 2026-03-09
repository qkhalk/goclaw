import { useCallback, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { toast } from "@/stores/use-toast-store";
import type {
  MemoryDocument,
  MemoryDocumentDetail,
  MemoryChunk,
  MemorySearchResult,
} from "@/types/memory";

export interface MemoryDocFilters {
  agentId?: string;
  userId?: string;
}

export function useMemoryDocuments(filters: MemoryDocFilters) {
  const http = useHttp();
  const queryClient = useQueryClient();

  const queryKey = queryKeys.memory.list({ ...filters });

  const { data, isLoading, isFetching } = useQuery({
    queryKey,
    queryFn: async () => {
      // No agent selected → list all memory across all agents
      if (!filters.agentId) {
        const res = await http.get<MemoryDocument[]>("/v1/memory/documents");
        return res ?? [];
      }
      const params: Record<string, string> = {};
      if (filters.userId) params.user_id = filters.userId;
      const res = await http.get<MemoryDocument[]>(
        `/v1/agents/${filters.agentId}/memory/documents`,
        params,
      );
      return res ?? [];
    },
    placeholderData: (prev) => prev,
  });

  const documents = data ?? [];

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.memory.all }),
    [queryClient],
  );

  const getDocument = useCallback(
    async (path: string, userId?: string) => {
      const params: Record<string, string> = {};
      if (userId) params.user_id = userId;
      return http.get<MemoryDocumentDetail>(
        `/v1/agents/${filters.agentId}/memory/documents/${path}`,
        params,
      );
    },
    [http, filters.agentId],
  );

  const createDocument = useCallback(
    async (path: string, content: string, userId?: string) => {
      try {
        await http.put(`/v1/agents/${filters.agentId}/memory/documents/${path}`, {
          content,
          user_id: userId || "",
        });
        await invalidate();
        toast.success("Document created", path);
      } catch (err) {
        toast.error("Failed to create document", err instanceof Error ? err.message : "Unknown error");
        throw err;
      }
    },
    [http, filters.agentId, invalidate],
  );

  const updateDocument = useCallback(
    async (path: string, content: string, userId?: string) => {
      try {
        await http.put(`/v1/agents/${filters.agentId}/memory/documents/${path}`, {
          content,
          user_id: userId || "",
        });
        await invalidate();
        toast.success("Document updated", path);
      } catch (err) {
        toast.error("Failed to update document", err instanceof Error ? err.message : "Unknown error");
        throw err;
      }
    },
    [http, filters.agentId, invalidate],
  );

  const deleteDocument = useCallback(
    async (path: string, userId?: string) => {
      try {
        const qs = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
        await http.delete(`/v1/agents/${filters.agentId}/memory/documents/${path}${qs}`);
        await invalidate();
        toast.success("Document deleted", path);
      } catch (err) {
        toast.error("Failed to delete document", err instanceof Error ? err.message : "Unknown error");
        throw err;
      }
    },
    [http, filters.agentId, invalidate],
  );

  const getChunks = useCallback(
    async (path: string, userId?: string) => {
      const params: Record<string, string> = { path };
      if (userId) params.user_id = userId;
      return http.get<MemoryChunk[]>(
        `/v1/agents/${filters.agentId}/memory/chunks`,
        params,
      );
    },
    [http, filters.agentId],
  );

  const indexDocument = useCallback(
    async (path: string, userId?: string) => {
      try {
        await http.post(`/v1/agents/${filters.agentId}/memory/index`, {
          path,
          user_id: userId || "",
        });
        toast.success("Document indexed", path);
      } catch (err) {
        toast.error("Failed to index document", err instanceof Error ? err.message : "Unknown error");
        throw err;
      }
    },
    [http, filters.agentId],
  );

  const indexAll = useCallback(
    async (userId?: string) => {
      try {
        await http.post(`/v1/agents/${filters.agentId}/memory/index-all`, {
          user_id: userId || "",
        });
        toast.success("All documents indexed");
      } catch (err) {
        toast.error("Failed to index all", err instanceof Error ? err.message : "Unknown error");
        throw err;
      }
    },
    [http, filters.agentId],
  );

  return {
    documents,
    loading: isLoading,
    fetching: isFetching,
    refresh: invalidate,
    getDocument,
    createDocument,
    updateDocument,
    deleteDocument,
    getChunks,
    indexDocument,
    indexAll,
  };
}

export function useMemorySearch(agentId: string) {
  const http = useHttp();
  const [results, setResults] = useState<MemorySearchResult[]>([]);
  const [searching, setSearching] = useState(false);

  const search = useCallback(
    async (query: string, userId?: string, maxResults?: number, minScore?: number) => {
      setSearching(true);
      try {
        const res = await http.post<{ results: MemorySearchResult[]; count: number }>(
          `/v1/agents/${agentId}/memory/search`,
          {
            query,
            user_id: userId || "",
            max_results: maxResults || 10,
            min_score: minScore || 0,
          },
        );
        setResults(res.results ?? []);
        return res.results ?? [];
      } catch (err) {
        toast.error("Search failed", err instanceof Error ? err.message : "Unknown error");
        setResults([]);
        return [];
      } finally {
        setSearching(false);
      }
    },
    [http, agentId],
  );

  return { results, searching, search };
}
