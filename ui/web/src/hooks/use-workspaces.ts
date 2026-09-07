import { useState, useEffect, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";
import type { WorkspaceInfo } from "@/types/workspace";
import { useAuthStore } from "@/stores/use-auth-store";

/**
 * Manages the workspace list for the chat console.
 * Loads all workspaces once the WS connection is up; supports manual refresh.
 */
export function useWorkspaces() {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [workspaces, setWorkspaces] = useState<WorkspaceInfo[]>([]);
  const [loading, setLoading] = useState(true);

  /** Active workspaces first, then alphabetical by name. */
  const sortWorkspaces = useCallback(
    (list: WorkspaceInfo[]): WorkspaceInfo[] =>
      [...list].sort((a, b) => {
        if (a.status !== b.status) {
          return a.status === "active" ? -1 : 1;
        }
        return a.name.localeCompare(b.name);
      }),
    [],
  );

  const refresh = useCallback(async () => {
    if (!connected) return;
    setLoading(true);
    try {
      const res = await ws.call<{ workspaces: WorkspaceInfo[] }>(
        Methods.WORKSPACE_LIST,
        {},
      );
      setWorkspaces(sortWorkspaces(res.workspaces ?? []));
    } catch (err) {
      console.error("[useWorkspaces] load failed:", err);
    } finally {
      setLoading(false);
    }
  }, [ws, connected, sortWorkspaces]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  /** Creates a workspace via workspace.create; throws the WS error on failure. */
  const create = useCallback(
    async (name: string): Promise<WorkspaceInfo | null> => {
      if (!connected) return null;
      const res = await ws.call<{ workspace: WorkspaceInfo }>(
        Methods.WORKSPACE_CREATE,
        { name },
      );
      await refresh();
      return res.workspace ?? null;
    },
    [ws, connected, refresh],
  );

  return { workspaces, loading, refresh, create };
}
