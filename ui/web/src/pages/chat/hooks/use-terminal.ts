import { useCallback, useEffect, useRef, useState } from "react";
import { Events, Methods } from "@/api/protocol";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";

/** terminal.list row — mirrors store.TerminalSession JSON tags. */
export interface TerminalSessionInfo {
  id: string;
  workspaceId: string;
  cwd: string;
  shell: string;
  status: "running" | "exited" | "closed" | string;
  exitCode?: number | null;
  createdAt: string;
  updatedAt: string;
}

export type TerminalPhase =
  /** No workspace selected yet. */
  | "idle"
  /** Looking up an existing session / waiting for the PTY to come up. */
  | "connecting"
  /** PTY is live and streaming. */
  | "ready"
  /** Backend reported the shell process exited (terminal.exit). */
  | "ended"
  /** RPC failed or no session could be created. */
  | "failed";

export interface UseTerminalResult {
  phase: TerminalPhase;
  sessionId: string | null;
  /** RPC failure detail for the failed state. */
  error: string | null;
  /** Ring-buffer replay (base64) from terminal.attach; consumed once by the view. */
  consumeReplay: () => string | null;
  /** Create a fresh PTY for this workspace (new-tab button). */
  create: () => Promise<boolean>;
  /** Close the current PTY and start a new one in its place. */
  renew: () => Promise<void>;
  /** Kill the PTY and drop back to the pre-create idle state. */
  close: () => Promise<void>;
  /** Forward raw keystrokes to the live PTY; lazily spawns on first input. */
  handleInput: (data: string) => void;
}

/**
 * Web terminal state machine for the chat console side panel
 * (Paseo plan Phase 4 / §25). One PTY per workspace tab:
 *
 * - attach on open: terminal.list → newest running session → terminal.attach
 *   (live flag + ring-buffer replay) else mark ended;
 * - lazy create on first keystroke when nothing is attached;
 * - output via terminal.output events (base64 chunks), exit via terminal.exit.
 *
 * Closing the panel does NOT kill the PTY — sessions keep running server-side
 * and the next open re-attaches to the newest running one.
 */
export function useTerminal(workspaceId: string | null): UseTerminalResult {
  const ws = useWs();
  const [phase, setPhase] = useState<TerminalPhase>("idle");
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Refs mirror state so event handlers and callbacks stay stable across
  // renders; pendingRef dedupes concurrent creates (double-click, races).
  const phaseRef = useRef<TerminalPhase>("idle");
  const sessionIdRef = useRef<string | null>(null);
  const workspaceRef = useRef(workspaceId);
  const pendingRef = useRef(false);
  const replayRef = useRef<string | null>(null);

  // Workspace switch resets everything and re-attaches to the newest running
  // session for that workspace, if any.
  useEffect(() => {
    workspaceRef.current = workspaceId;
    sessionIdRef.current = null;
    pendingRef.current = false;
    replayRef.current = null;
    setSessionId(null);
    setError(null);
    if (!workspaceId) {
      phaseRef.current = "idle";
      setPhase("idle");
      return;
    }
    phaseRef.current = "connecting";
    setPhase("connecting");

    let cancelled = false;
    const seqWorkspace = workspaceId;
    void (async () => {
      try {
        const res = await ws.call<{ sessions: TerminalSessionInfo[] }>(
          Methods.TERMINAL_LIST,
          { workspaceId: seqWorkspace },
        );
        if (cancelled || workspaceRef.current !== seqWorkspace) return;
        const running = (res.sessions ?? [])
          .filter((s) => s.status === "running")
          .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))[0];
        if (!running) {
          // Nothing to attach yet — wait for a lazy create on first input.
          phaseRef.current = "idle";
          setPhase("idle");
          return;
        }
        sessionIdRef.current = running.id;
        setSessionId(running.id);
        const att = await ws.call<{ live: boolean; data?: string }>(Methods.TERMINAL_ATTACH, {
          terminalId: running.id,
        });
        if (cancelled || workspaceRef.current !== seqWorkspace) return;
        replayRef.current = att.live ? (att.data ?? null) : null;
        phaseRef.current = att.live ? "ready" : "ended";
        setPhase(phaseRef.current);
      } catch (err) {
        if (cancelled || workspaceRef.current !== seqWorkspace) return;
        setError(err instanceof Error ? err.message : String(err));
        phaseRef.current = "failed";
        setPhase("failed");
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [workspaceId, ws]);

  // One-shot handoff of the attach replay buffer to the viewport.
  const consumeReplay = useCallback(() => {
    const r = replayRef.current;
    replayRef.current = null;
    return r;
  }, []);

  /**
   * Create a fresh PTY for this workspace (new-tab button + lazy first-input
   * path). Resolves true once the backend confirms a running session.
   */
  const create = useCallback(async (): Promise<boolean> => {
    const target = workspaceRef.current;
    if (!target || pendingRef.current || !phaseRefIsCreatable(phaseRef.current)) return false;
    pendingRef.current = true;
    setError(null);
    replayRef.current = null;
    phaseRef.current = "connecting";
    setPhase("connecting");
    try {
      const res = await ws.call<{ terminal: { id: string } }>(Methods.TERMINAL_CREATE, {
        workspaceId: target,
      });
      if (workspaceRef.current !== target) return false;
      sessionIdRef.current = res.terminal.id;
      setSessionId(res.terminal.id);
      phaseRef.current = "ready";
      setPhase("ready");
      return true;
    } catch (err) {
      if (workspaceRef.current !== target) return false;
      setError(err instanceof Error ? err.message : String(err));
      phaseRef.current = "failed";
      setPhase("failed");
      return false;
    } finally {
      pendingRef.current = false;
    }
  }, [ws]);

  /** Close the current PTY and start a new one in its place (new-tab action). */
  const renew = useCallback(async () => {
    const target = workspaceRef.current;
    if (!target || pendingRef.current) return;
    const old = sessionIdRef.current;
    if (old) {
      try {
        await ws.call(Methods.TERMINAL_CLOSE, { terminalId: old });
      } catch {
        // Best-effort: the PTY may already be gone server-side.
      }
    }
    sessionIdRef.current = null;
    setSessionId(null);
    phaseRef.current = "idle"; // allow create() to spawn the replacement
    await create();
  }, [ws, create]);

  /** Kill the PTY and drop back to the pre-create idle state. */
  const close = useCallback(async () => {
    const old = sessionIdRef.current;
    sessionIdRef.current = null;
    replayRef.current = null;
    setSessionId(null);
    setError(null);
    phaseRef.current = "idle";
    setPhase("idle");
    if (!old) return;
    try {
      await ws.call(Methods.TERMINAL_CLOSE, { terminalId: old });
    } catch {
      // Server-side cleanup failures are non-fatal here.
    }
  }, [ws]);

  // terminal.exit flips an attached session into the ended state.
  useWsEvent(Events.TERMINAL_EXIT, (payload) => {
    const p = payload as { terminalId?: string } | null;
    if (p?.terminalId && p.terminalId === sessionIdRef.current) {
      phaseRef.current = "ended";
      setPhase("ended");
    }
  });

  // Keystrokes stream straight to the live PTY; with nothing attached yet,
  // the first keystroke lazily spawns a session and forwards the keypress.
  const handleInput = useCallback(
    (data: string) => {
      const send = (id: string) =>
        ws.call(Methods.TERMINAL_INPUT, { terminalId: id, data: encodeB64(data) }).catch(() => {});
      const id = sessionIdRef.current;
      if (id) {
        void send(id);
        return;
      }
      if (!pendingRef.current && phaseRefIsCreatable(phaseRef.current)) {
        void create().then((ok) => {
          if (!ok || !data) return;
          const created = sessionIdRef.current;
          if (created) void send(created);
        });
      }
    },
    [ws, create],
  );

  return { phase, sessionId, error, consumeReplay, create, renew, close, handleInput };
}

/** UTF-8-safe base64 (plain btoa chokes on multi-byte pastes). */
function encodeB64(data: string): string {
  const bytes = new TextEncoder().encode(data);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

function phaseRefIsCreatable(phase: TerminalPhase): boolean {
  // "ended"/"failed"/"idle" sessions may be replaced by a fresh PTY;
  // "connecting"/"ready" ones must not double-spawn.
  return phase === "idle" || phase === "ended" || phase === "failed";
}
