import { useCallback, useEffect, useRef, useState } from "react";
import { useWs } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { Methods } from "@/api/protocol";
import type { TaskNode } from "@/types/workspace";

/** Flat task augmented with client-built child links. */
export interface TaskTreeNode extends TaskNode {
  children: TaskTreeNode[];
}

/**
 * Builds a nested tree from a flat TaskNode list using parentId.
 * Nodes whose parent is missing (deleted or cross-workspace) render at root level defensively.
 */
function buildTree(flat: TaskNode[]): TaskTreeNode[] {
  const byId = new Map<string, TaskTreeNode>();
  for (const task of flat) byId.set(task.id, { ...task, children: [] });

  const roots: TaskTreeNode[] = [];
  for (const node of byId.values()) {
    const parent = node.parentId ? byId.get(node.parentId) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  return roots;
}

/**
 * Loads the task graph for a workspace via tasks.tree.
 * Initial load + manual refresh; deliberately polling-free.
 */
export function useTaskTree(workspaceId?: string | null) {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [tasks, setTasks] = useState<TaskTreeNode[]>([]);
  const [loading, setLoading] = useState(true);
  const loadSeqRef = useRef(0);

  const load = useCallback(async () => {
    if (!connected) return;
    const seq = ++loadSeqRef.current;
    setLoading(true);
    try {
      const res = await ws.call<{ tasks: TaskNode[] }>(
        Methods.TASKS_TREE,
        { workspaceId: workspaceId ?? undefined },
      );
      if (seq === loadSeqRef.current) setTasks(buildTree(res.tasks ?? []));
    } catch {
      // List failures degrade to an empty view; the panel shows its empty state.
      if (seq === loadSeqRef.current) setTasks([]);
    } finally {
      if (seq === loadSeqRef.current) setLoading(false);
    }
  }, [ws, workspaceId, connected]);

  useEffect(() => {
    load();
  }, [load]);

  return { tasks, loading, refresh: load };
}
