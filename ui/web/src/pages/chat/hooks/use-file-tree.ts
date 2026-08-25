import { useCallback, useEffect, useRef, useState } from "react";
import { Methods } from "@/api/protocol";
import { useWs } from "@/hooks/use-ws";
import type { FileEntry } from "@/types/workspace";

export interface UseFileTreeResult {
  /** Lazily loaded directory listings keyed by dir path ("" = root). */
  nodes: Record<string, FileEntry[]>;
  /** Paths of directories currently expanded. */
  expanded: Set<string>;
  /** Per-dir server-side truncation flags from workspace.files.list. */
  truncated: Record<string, boolean>;
  /** Per-path in-flight markers for spinners. */
  loading: Record<string, boolean>;
  error: string | null;
  /** True once the root listing (path "") has been fetched. */
  rootLoaded: boolean;
  loadDir: (path: string, opts?: { force?: boolean }) => Promise<void>;
  toggle: (path: string) => void;
}

/**
 * Lazy-loading directory tree for the chat file explorer (Paseo plan §24).
 * Each expand fetches one level via workspace.files.list and caches it per
 * workspaceId; switching workspaces resets the whole cache.
 */
export function useFileTree(workspaceId: string | null): UseFileTreeResult {
  const ws = useWs();
  const [nodes, setNodes] = useState<Record<string, FileEntry[]>>({});
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [truncated, setTruncated] = useState<Record<string, boolean>>({});
  const [loading, setLoading] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);

  // Workspace whose responses are still relevant; results arriving after a
  // workspace switch are discarded instead of poisoning the fresh cache.
  const workspaceRef = useRef(workspaceId);
  const inflight = useRef(new Set<string>());

  // Reset cache when the workspace changes.
  useEffect(() => {
    if (workspaceRef.current !== workspaceId) {
      workspaceRef.current = workspaceId;
      setNodes({});
      setExpanded(new Set());
      setTruncated({});
      setLoading({});
      setError(null);
      inflight.current.clear();
    }
  }, [workspaceId]);

  const loadDir = useCallback(
    async (path: string, opts?: { force?: boolean }) => {
      const activeWorkspace = workspaceRef.current;
      if (!activeWorkspace || inflight.current.has(path)) return;
      if (!opts?.force && nodes[path]) return; // cached

      inflight.current.add(path);
      setLoading((prev) => ({ ...prev, [path]: true }));
      try {
        const res = await ws.call<{ entries: FileEntry[]; truncated: boolean }>(
          Methods.WORKSPACE_FILES_LIST,
          { workspaceId: activeWorkspace, path },
        );
        if (workspaceRef.current !== activeWorkspace) return; // stale response
        setNodes((prev) => ({ ...prev, [path]: res.entries ?? [] }));
        setTruncated((prev) => ({ ...prev, [path]: Boolean(res.truncated) }));
        setError(null);
      } catch (err) {
        if (workspaceRef.current === activeWorkspace) {
          setError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        inflight.current.delete(path);
        setLoading((prev) => {
          const next = { ...prev };
          delete next[path];
          return next;
        });
      }
    },
    [ws, nodes],
  );

  // Fetch the root listing as soon as a workspace is available.
  useEffect(() => {
    if (workspaceId && nodes[""] === undefined && !inflight.current.has("")) {
      void loadDir("");
    }
  }, [workspaceId, nodes, loadDir]);

  const toggle = useCallback(
    (path: string) => {
      const isExpanded = expanded.has(path);
      setExpanded((prev) => {
        const next = new Set(prev);
        if (isExpanded) {
          next.delete(path);
        } else {
          next.add(path);
        }
        return next;
      });
      if (!isExpanded && !nodes[path]) {
        void loadDir(path);
      }
    },
    [expanded, nodes, loadDir],
  );

  return {
    nodes,
    expanded,
    truncated,
    loading,
    error,
    rootLoaded: nodes[""] !== undefined,
    loadDir,
    toggle,
  };
}
