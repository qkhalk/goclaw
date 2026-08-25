// Web terminal side panel for the chat console (Paseo plan Phase 4 / §25).
// xterm.js view over a server-side PTY streamed via terminal.output events;
// input flows back through terminal.input, resize through terminal.resize.
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle, Loader2, Plus, X } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { Events, Methods } from "@/api/protocol";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import { cn } from "@/lib/utils";
import {
  type TerminalPhase,
  type UseTerminalResult,
  useTerminal,
} from "@/pages/chat/hooks/use-terminal";

interface TerminalPanelProps {
  open: boolean;
  onClose: () => void;
  workspaceId: string | null;
}

/** Decode a base64 chunk into bytes xterm can write directly (binary-safe). */
function decodeB64(data: string): Uint8Array {
  const bin = atob(data);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

export function TerminalPanel({ open, onClose, workspaceId }: TerminalPanelProps) {
  const { t } = useTranslation("chat");
  const term: UseTerminalResult = useTerminal(workspaceId);

  if (!open) return null;

  return (
    <div
      className={cn(
        "flex h-full w-72 shrink-0 flex-col border-l bg-background",
        "max-sm:fixed max-sm:inset-y-0 max-sm:right-0 max-sm:z-50 max-sm:w-full max-sm:max-w-[85vw] max-sm:shadow-xl",
      )}
    >
      {/* Header — mirrors file-explorer-panel */}
      <div className="flex items-center justify-between border-b px-3 py-2">
        <span className="text-sm font-medium">{t("terminal.title")}</span>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={() => void term.renew()}
            disabled={!workspaceId}
            title={t("terminal.newTab")}
            aria-label={t("terminal.newTab")}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
          >
            <Plus className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={() => {
              void term.close();
              onClose();
            }}
            title={t("terminal.closeTab")}
            aria-label={t("terminal.closeTab")}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Body */}
      {!workspaceId ? (
        <p className="px-3 py-6 text-center text-xs text-muted-foreground">
          {t("terminal.noWorkspace")}
        </p>
      ) : (
        <TerminalView
          key={term.sessionId ?? "none"}
          phase={term.phase}
          error={term.error}
          consumeReplay={term.consumeReplay}
          onInput={term.handleInput}
        />
      )}
    </div>
  );
}

/**
 * xterm viewport + status strip. Keyed by session id so a fresh PTY
 * (new tab) starts from a blank screen instead of stale buffer contents.
 * On mount with a live session it replays the ring buffer returned by
 * terminal.attach, then keeps writing terminal.output chunks as they arrive.
 */
function TerminalView({
  phase,
  error,
  consumeReplay,
  onInput,
}: {
  phase: TerminalPhase;
  error: string | null;
  consumeReplay: () => string | null;
  onInput: (data: string) => void;
}) {
  const { t } = useTranslation("chat");
  const ws = useWs();
  const containerRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  // Latest live session; drops output events from superseded sessions.
  const sessionIdRef = useRef<string | null>(null);
  const resizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Stream output chunks into the viewport. terminal.output carries base64
  // because arbitrary binary can appear; atob → bytes keeps it lossless.
  useWsEvent(Events.TERMINAL_OUTPUT, (payload) => {
    const p = payload as { terminalId?: string; data?: string } | null;
    if (!p?.terminalId || !p.data) return;
    if (sessionIdRef.current !== null && p.terminalId !== sessionIdRef.current) return;
    xtermRef.current?.write(decodeB64(p.data));
  });

  useEffect(() => {
    const el = containerRef.current;
    if (!el || phase !== "ready") return;

    const xterm = new Terminal({
      cursorBlink: true,
      fontSize: 12,
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
      scrollback: 5000,
    });
    const fit = new FitAddon();
    xterm.loadAddon(fit);
    xterm.open(el);
    xtermRef.current = xterm;

    // Ring-buffer snapshot from terminal.attach (one-shot, may be empty).
    const replayed = consumeReplay();
    if (replayed) xterm.write(decodeB64(replayed));

    try {
      fit.fit();
    } catch {
      // Container may still be transitioning; the ResizeObserver below refits.
    }

    // Keystrokes stream straight to the PTY.
    const dataDisposable = xterm.onData((data) => onInput(data));

    const reportSize = () => {
      const id = sessionIdRef.current;
      if (!id) return;
      void ws
        .call(Methods.TERMINAL_RESIZE, { terminalId: id, cols: xterm.cols, rows: xterm.rows })
        .catch(() => {});
    };

    const ro = new ResizeObserver(() => {
      try {
        fit.fit();
      } catch {
        return;
      }
      // Debounced so window/panel drags don't spam terminal.resize RPCs.
      clearTimeout(resizeTimerRef.current);
      resizeTimerRef.current = setTimeout(reportSize, 250);
    });
    ro.observe(el);

    return () => {
      ro.disconnect();
      dataDisposable.dispose();
      clearTimeout(resizeTimerRef.current);
      xterm.dispose();
      xtermRef.current = null;
    };
  }, [ws, phase, consumeReplay, onInput]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {phase === "ready" ? (
        <div ref={containerRef} className="min-h-0 flex-1 overflow-hidden" />
      ) : (
        <StatusStrip phase={phase} error={error} />
      )}
      {(phase === "connecting" || phase === "ended") && !error && (
        <p className="border-t px-3 py-1 text-xs text-muted-foreground">
          {phase === "connecting" ? t("terminal.connecting") : ""}
        </p>
      )}
    </div>
  );
}

/** Non-ready states rendered in place of the xterm viewport. */
function StatusStrip({ phase, error }: { phase: TerminalPhase; error: string | null }) {
  const { t } = useTranslation("chat");
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center px-3">
      {error ? (
        <p className="flex items-start gap-1.5 text-xs text-destructive">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span className="min-w-0 break-words">{t("terminal.failed", { message: error })}</span>
        </p>
      ) : phase === "connecting" ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          {t("terminal.connecting")}
        </p>
      ) : phase === "ended" ? (
        <p className="text-xs text-muted-foreground">{t("terminal.ended")}</p>
      ) : null}
    </div>
  );
}
