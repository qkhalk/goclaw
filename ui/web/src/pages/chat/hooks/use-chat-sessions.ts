import { useState, useEffect, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import { Methods, Events } from "@/api/protocol";
import type { SessionInfo } from "@/types/session";
import { sessionOrigin } from "@/lib/session-origin";
import { useAuthStore } from "@/stores/use-auth-store";
import { toast } from "@/stores/use-toast-store";
import i18next from "i18next";
import { userFriendlyError } from "@/lib/error-utils";
import { uniqueId } from "@/lib/utils";

/**
 * Session row with the soft-archive column (sessions.archived_at, phase 8).
 * Kept local so types/session.ts stays untouched while the backend contract
 * lands concurrently.
 */
export type ArchivedSessionInfo = SessionInfo & { archivedAt?: string };

interface SessionsListResponse {
  sessions: SessionInfo[];
}

/**
 * Manages the session list for the chat sidebar.
 * Loads sessions for the selected agent, supports creating new sessions,
 * and soft-archives/restores finished conversations (WS sessions.archive /
 * sessions.restore) out of the main list into the "Archived" section.
 */
export function useChatSessions(agentId: string) {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [archived, setArchived] = useState<ArchivedSessionInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Studio sessions (video/pptx designer chats) never belong in the
  // /chat sidebar — they live on their own tool pages. The backend
  // filters by agentId when one is selected; this covers the unscoped
  // default list (agentId === "").
  const usableSessions = useCallback((rows: SessionInfo[]): SessionInfo[] =>
    rows.filter(
      (s: SessionInfo) => sessionOrigin(s.key) !== "video" && sessionOrigin(s.key) !== "pptx",
    ), []);

  const loadSessions = useCallback(async () => {
    if (!connected) return;
    setLoading(true);
    setError(null);
    try {
      const res = await ws.call<SessionsListResponse>(
        Methods.SESSIONS_LIST,
        { agentId, channel: "ws" },
      );
      // sessions.list excludes archived server-side once the backend filter
      // lands; the client-side `archivedAt` guard keeps behavior correct
      // against older servers that still return both.
      const usable = usableSessions(res.sessions ?? []).filter(
        (s) => !((s as ArchivedSessionInfo).archivedAt),
      );
      const sorted = usable.sort(
        (a: SessionInfo, b: SessionInfo) =>
          new Date(b.updated).getTime() - new Date(a.updated).getTime(),
      );
      setSessions(sorted);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setLoading(false);
    }
  }, [ws, agentId, connected, usableSessions]);

  // Archived section: same list call with includeArchived=true (contract may
  // land concurrently — unknown params are ignored server-side, in which case
  // no rows carry archivedAt and the section stays hidden).
  const loadArchived = useCallback(async () => {
    if (!connected) return;
    try {
      const res = await ws.call<SessionsListResponse>(Methods.SESSIONS_LIST, {
        agentId,
        channel: "ws",
        includeArchived: true,
        limit: 100,
      });
      const rows = usableSessions(res.sessions ?? []).filter(
        (s) => !!(s as ArchivedSessionInfo).archivedAt,
      ) as ArchivedSessionInfo[];
      rows.sort((a, b) => new Date(b.updated).getTime() - new Date(a.updated).getTime());
      setArchived(rows);
    } catch {
      // Archived section is additive — leave it as-is on failure.
    }
  }, [ws, agentId, connected, usableSessions]);

  useEffect(() => {
    loadSessions();
    loadArchived();
  }, [loadSessions, loadArchived]);

  const buildNewSessionKey = useCallback(() => {
    const convId = uniqueId();
    return `agent:${agentId}:ws:direct:${convId}`;
  }, [agentId]);

  const deleteSession = useCallback(async (key: string) => {
    if (!connected) return;
    try {
      await ws.call(Methods.SESSIONS_DELETE, { key });
      await loadSessions();
      await loadArchived();
      toast.success(i18next.t("sessions:toast.deleted"));
    } catch (err) {
      toast.error(i18next.t("sessions:toast.deleteFailed"), userFriendlyError(err));
      throw err;
    }
  }, [ws, connected, loadSessions, loadArchived]);

  /** Soft-archive: moves the session out of the main list (messages intact). */
  const archiveSession = useCallback(async (key: string) => {
    if (!connected) return;
    // Optimistic remove — restore-on-error below.
    const prevSessions = sessions;
    setSessions((cur) => cur.filter((s) => s.key !== key));
    try {
      await ws.call(Methods.SESSIONS_ARCHIVE, { sessionKey: key });
      toast.success(i18next.t("chat:sessions.archived"));
      await loadSessions();
      await loadArchived();
    } catch (err) {
      setSessions(prevSessions);
      toast.error(i18next.t("chat:sessions.archiveFailed"), userFriendlyError(err));
      throw err;
    }
  }, [ws, connected, sessions, loadSessions, loadArchived]);

  /** Bring an archived session back into the main list (messages intact). */
  const restoreSession = useCallback(async (key: string) => {
    if (!connected) return;
    const prevArchived = archived;
    setArchived((cur) => cur.filter((s) => s.key !== key));
    try {
      await ws.call(Methods.SESSIONS_RESTORE, { sessionKey: key });
      toast.success(i18next.t("chat:sessions.restored"));
      await loadSessions();
      await loadArchived();
    } catch (err) {
      setArchived(prevArchived);
      toast.error(i18next.t("chat:sessions.restoreFailed"), userFriendlyError(err));
      throw err;
    }
  }, [ws, connected, archived, loadSessions, loadArchived]);

  // Update session label in-place when backend generates a title.
  const handleSessionUpdated = useCallback((payload: unknown) => {
    const event = payload as { sessionKey?: string; label?: string };
    if (!event?.sessionKey || !event?.label) return;
    setSessions((prev) =>
      prev.map((s) =>
        s.key === event.sessionKey ? { ...s, label: event.label } : s,
      ),
    );
  }, []);
  useWsEvent(Events.SESSION_UPDATED, handleSessionUpdated);

  return {
    sessions,
    archivedSessions: archived,
    loading,
    error,
    refresh: loadSessions,
    buildNewSessionKey,
    deleteSession,
    archiveSession,
    restoreSession,
  };
}
