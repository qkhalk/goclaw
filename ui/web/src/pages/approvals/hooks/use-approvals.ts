import { useState, useEffect, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { useWsEvent } from "@/hooks/use-ws-event";
import { Methods, Events } from "@/api/protocol";
import { toast } from "@/stores/use-toast-store";
import i18n from "@/i18n";

export type ApprovalScope = "once" | "session" | "always";

export interface PendingApproval {
  id: string;
  command: string;
  agentId: string;
  createdAt: number;
  toolClass?: string;
  toolName?: string;
  risk?: string;
  sessionKey?: string;
  expiresAt?: number;
}

export interface ApprovalHistoryItem {
  id: string;
  command: string;
  actionType: string;
  status: string;
  decision: string;
  requesterId?: string;
  agentId?: string;
  sessionKey?: string;
  grantExpiresAt?: number;
  createdAt: number;
  decidedAt?: number;
}

export function useApprovals() {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [pending, setPending] = useState<PendingApproval[]>([]);
  const [history, setHistory] = useState<ApprovalHistoryItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!connected) return;
    setLoading(true);
    setError(null);
    try {
      const res = await ws.call<{ pending: PendingApproval[] }>(Methods.APPROVALS_LIST);
      setPending(res.pending ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load approvals");
    } finally {
      setLoading(false);
    }
  }, [ws, connected]);

  const loadHistory = useCallback(async () => {
    if (!connected) return;
    try {
      // exec.approval.history is not part of the shared protocol constant
      // table; use the wire name directly.
      const res = await ws.call<{ history: ApprovalHistoryItem[]; total: number }>(
        "exec.approval.history",
        { limit: 50, offset: 0 },
      );
      setHistory(res.history ?? []);
    } catch {
      // History is best-effort; an unavailable store should not break the page.
      setHistory([]);
    }
  }, [ws, connected]);

  useEffect(() => {
    load();
  }, [load]);

  // Listen for new approval requests
  useWsEvent(Events.EXEC_APPROVAL_REQUESTED, load);

  // Listen for resolved approvals
  useWsEvent(Events.EXEC_APPROVAL_RESOLVED, () => {
    load();
    loadHistory();
  });

  const approve = useCallback(
    async (id: string, scope: ApprovalScope = "once") => {
      try {
        await ws.call(Methods.APPROVALS_APPROVE, { id, scope });
        setPending((prev) => prev.filter((a) => a.id !== id));
        toast.success(
          i18n.t("approvals:toast.approved"),
          scope === "always"
            ? i18n.t("approvals:toast.approvedAlways")
            : scope === "session"
              ? i18n.t("approvals:toast.approvedSession")
              : i18n.t("approvals:toast.approvedOnce"),
        );
      } catch (err) {
        toast.error(i18n.t("approvals:toast.approveFailed"), err instanceof Error ? err.message : i18n.t("approvals:toast.unknownError"));
        throw err;
      }
    },
    [ws],
  );

  const deny = useCallback(
    async (id: string) => {
      try {
        await ws.call(Methods.APPROVALS_DENY, { id });
        setPending((prev) => prev.filter((a) => a.id !== id));
        toast.success(i18n.t("approvals:toast.denied"), i18n.t("approvals:toast.deniedDesc"));
      } catch (err) {
        toast.error(i18n.t("approvals:toast.denyFailed"), err instanceof Error ? err.message : i18n.t("approvals:toast.unknownError"));
        throw err;
      }
    },
    [ws],
  );

  return { pending, history, loading, error, refresh: load, refreshHistory: loadHistory, approve, deny };
}
