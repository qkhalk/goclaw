import { useCallback, useRef, useState } from "react";
import { Events, Methods } from "@/api/protocol";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import { extractMarkdown, MIN_USEFUL_CHARS } from "@/lib/dom-extract";

/**
 * State machine for the browser panel (client-side browsing). The gateway
 * targets this client with a browser.panel.invoke event when the agent calls
 * web_browse; the panel renders the sanitized relay document in a sandboxed
 * same-origin iframe (assets load from the origin site directly), extracts
 * the page text locally, and posts it back via browser.panel.result.
 */
export type BrowserPanelStatus = "idle" | "loading" | "ready" | "error";

export interface BrowserPanelState {
  browseId: string | null;
  /** Originally requested URL (shown as title context). */
  url: string;
  /** Signed same-origin relay URL the iframe loads. */
  relayUrl: string;
  /** Origin URL after redirects (shown in the URL bar). */
  finalUrl: string;
  title: string;
  status: BrowserPanelStatus;
  /** Bumped by reload() so the panel can remount the iframe. */
  reloadNonce: number;
}

const initialState: BrowserPanelState = {
  browseId: null,
  url: "",
  relayUrl: "",
  finalUrl: "",
  title: "",
  status: "idle",
  reloadNonce: 0,
};

interface InvokePayload {
  browseId?: string;
  url?: string;
  relayUrl?: string;
  finalUrl?: string;
  deadlineMs?: number;
}

export function useBrowserPanel(onInvoke: () => void) {
  const ws = useWs();
  const [state, setState] = useState<BrowserPanelState>(initialState);
  // Ref mirrors state for the iframe onLoad callback, which must read the
  // browse request the iframe was loaded with without re-binding onLoad.
  const stateRef = useRef(state);
  stateRef.current = state;

  useWsEvent(Events.BROWSER_PANEL_INVOKE, (payload) => {
    const p = (payload ?? {}) as InvokePayload;
    if (!p.browseId || !p.relayUrl) return;
    setState({
      browseId: p.browseId,
      url: p.url ?? p.finalUrl ?? "",
      relayUrl: p.relayUrl,
      finalUrl: p.finalUrl ?? p.url ?? "",
      title: "",
      status: "loading",
      reloadNonce: 0,
    });
    onInvoke();
  });

  /** Re-mount the iframe with the same relay URL (manual reload). */
  const reload = useCallback(() => {
    setState((s) =>
      s.relayUrl
        ? { ...s, status: "loading", reloadNonce: s.reloadNonce + 1 }
        : s,
    );
  }, []);

  /** iframe onLoad: extract markdown from the same-origin document. */
  const handleIframeLoad = useCallback(
    (iframe: HTMLIFrameElement | null) => {
      const current = stateRef.current;
      if (!current.browseId || current.status !== "loading" || !iframe) return;
      let doc: Document | null = null;
      try {
        doc = iframe.contentDocument;
      } catch {
        doc = null;
      }
      let payload: Record<string, unknown>;
      if (doc && doc.body) {
        const content = extractMarkdown(doc);
        if (content.length >= MIN_USEFUL_CHARS) {
          payload = {
            browseId: current.browseId,
            content,
            title: doc.title || current.title,
            finalUrl: current.finalUrl,
          };
        } else {
          payload = {
            browseId: current.browseId,
            error: "page content too thin — likely rendered via JavaScript",
            title: doc.title || current.title,
            finalUrl: current.finalUrl,
          };
        }
      } else {
        payload = {
          browseId: current.browseId,
          error: "document not readable (load failure or blocked)",
          finalUrl: current.finalUrl,
        };
      }
      ws.call(Methods.BROWSER_PANEL_RESULT, payload).catch(() => {
        // Server waiters time out on their own; nothing to recover here.
      });
      setState((s) => (s.browseId === current.browseId ? { ...s, status: "ready" } : s));
    },
    [ws],
  );

  /** Mark the load as failed (iframe error / user dismissal of blocked page). */
  const markError = useCallback((message: string) => {
    const current = stateRef.current;
    if (!current.browseId) return;
    ws.call(Methods.BROWSER_PANEL_RESULT, {
      browseId: current.browseId,
      error: message,
      finalUrl: current.finalUrl,
    }).catch(() => {});
    setState((s) => ({ ...s, status: "error" }));
  }, [ws]);

  const reset = useCallback(() => setState(initialState), []);

  return { state, handleIframeLoad, markError, reload, reset };
}
