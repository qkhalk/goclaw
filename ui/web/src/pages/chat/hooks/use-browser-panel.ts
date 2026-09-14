import { useCallback, useRef, useState } from "react";
import { Events, Methods } from "@/api/protocol";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import {
  extractPageWithRefs,
  refElement,
  MIN_USEFUL_CHARS,
  MAX_EXTRACT_CHARS,
} from "@/lib/dom-extract";

/**
 * Client-side mini-browser behind the chat browser panel (ZCode-style).
 *
 * The gateway targets this client with a browser.panel.invoke event:
 * without `action` it means "load this relayed page and report the text";
 * with an action (click/type/extract/back/reload) it means "operate the page
 * currently shown". The panel renders sanitized same-origin relay documents
 * (assets load from the origin site directly), intercepts the user's own
 * clicks/URL-bar entries into new relay loads, extracts text locally, and
 * posts everything back via browser.panel.result. The server only ever does
 * one GET per navigation — all rendering/interaction happens here.
 */
export type BrowserPanelStatus = "idle" | "loading" | "ready" | "error";

export interface BrowserPanelState {
  /** Current page's original URL ("" = nothing open). */
  url: string;
  finalUrl: string;
  title: string;
  relayUrl: string;
  status: BrowserPanelStatus;
  canBack: boolean;
  canForward: boolean;
  /** Last action note for the status bar. */
  note: string;
}

interface HistoryEntry {
  browseId: string;
  url: string;
  relayUrl: string;
  finalUrl: string;
  title: string;
}

interface InvokePayload {
  browseId?: string;
  action?: string;
  ref?: string;
  text?: string;
  maxChars?: number;
  url?: string;
  relayUrl?: string;
  finalUrl?: string;
}

type Pending =
  | { id: string; kind: "open" }
  | { id: string; kind: "action"; action: string; ref?: string; text?: string; maxChars?: number }
  | null;

const initialState: BrowserPanelState = {
  url: "",
  finalUrl: "",
  title: "",
  relayUrl: "",
  status: "idle",
  canBack: false,
  canForward: false,
  note: "",
};

export function useBrowserPanel(onInvoke: () => void) {
  const ws = useWs();
  const [state, setState] = useState<BrowserPanelState>(initialState);
  // Refs mirror state/mutable session data for callbacks that must read the
  // latest values without re-binding (iframe onLoad, WS event handler).
  const stateRef = useRef(state);
  stateRef.current = state;
  const historyRef = useRef<{ entries: HistoryEntry[]; index: number }>({ entries: [], index: -1 });
  const pendingRef = useRef<Pending>(null);
  const iframeRef = useRef<HTMLIFrameElement | null>(null);

  const publishState = useCallback((patch: Partial<BrowserPanelState>) => {
    setState((s) => ({ ...s, ...patch }));
  }, []);

  const syncNav = useCallback(() => {
    const h = historyRef.current;
    return { canBack: h.index > 0, canForward: h.index < h.entries.length - 1 };
  }, []);

  /** Post an extraction/error back to the server for a pending invocation. */
  const postResult = useCallback(
    (correlationId: string, payload: Record<string, unknown>) => {
      if (!correlationId) return; // user-initiated load — no waiter to reply to
      ws.call(Methods.BROWSER_PANEL_RESULT, { browseId: correlationId, ...payload }).catch(() => {
        // Server waiters time out on their own; nothing to recover here.
      });
    },
    [ws],
  );

  /** Extract the current iframe document (tagging interactive refs). */
  const extractCurrent = useCallback(() => {
    const doc = iframeRef.current?.contentDocument ?? null;
    if (!doc || !doc.body) return null;
    const { markdown, refs } = extractPageWithRefs(doc);
    return { doc, markdown, refs };
  }, []);

  /** Load a relay URL into the iframe (component re-keys on relayUrl). */
  const loadEntry = useCallback(
    (entry: HistoryEntry, push: boolean) => {
      const h = historyRef.current;
      if (push) {
        h.entries = h.entries.slice(0, h.index + 1);
        h.entries.push(entry);
        h.index = h.entries.length - 1;
      } else {
        h.entries[h.index] = entry;
      }
      pendingRef.current = { id: pendingRef.current?.id ?? "", kind: "open" };
      publishState({
        url: entry.url,
        finalUrl: entry.finalUrl,
        title: entry.title,
        relayUrl: entry.relayUrl,
        status: "loading",
        note: "",
        ...syncNav(),
      });
    },
    [publishState, syncNav],
  );

  /** Navigate to a URL through the gateway's browser.panel.open pipeline. */
  const openURL = useCallback(
    (url: string, opts?: { replyTo?: string; maxChars?: number; note?: string }) => {
      const replyTo = opts?.replyTo ?? "";
      pendingRef.current = replyTo ? { id: replyTo, kind: "open" } : pendingRef.current;
      publishState({ status: "loading", note: opts?.note ?? "" });
      ws.call<{ browseId: string; relayUrl: string; finalUrl: string; title: string }>(
        Methods.BROWSER_PANEL_OPEN,
        { url },
      )
        .then((res) => {
          if (!res || !res.relayUrl) throw new Error("empty relay response");
          loadEntry(
            {
              browseId: res.browseId,
              url,
              relayUrl: res.relayUrl,
              finalUrl: res.finalUrl || url,
              title: res.title || "",
            },
            true,
          );
        })
        .catch((err) => {
          publishState({ status: "error", note: String(err?.message ?? err) });
          pendingRef.current = null;
          if (replyTo) postResult(replyTo, { error: `navigation failed: ${String(err?.message ?? err)}` });
        });
    },
    [ws, publishState, loadEntry, postResult],
  );

  /** Run an agent action against the currently displayed page. */
  const runAction = useCallback(
    (payload: InvokePayload & { browseId: string }) => {
      const action = payload.action ?? "extract";
      const maxChars = typeof payload.maxChars === "number" ? payload.maxChars : undefined;
      const finish = (note: string) => {
        const ex = extractCurrent();
        pendingRef.current = null;
        if (!ex) {
          postResult(payload.browseId, { error: "no readable page in the panel" });
          publishState({ status: "error" });
          return;
        }
        const cur = stateRef.current;
        let content = ex.markdown;
        let truncated = false;
        if (maxChars !== undefined && content.length > maxChars) {
          content = content.slice(0, maxChars);
          truncated = true;
        }
        if (content.length > MAX_EXTRACT_CHARS) {
          content = content.slice(0, MAX_EXTRACT_CHARS);
          truncated = true;
        }
        const thin = content.length < MIN_USEFUL_CHARS;
        publishState({ status: "ready", note });
        if (thin) {
          postResult(payload.browseId, {
            error: "page content too thin — likely rendered via JavaScript",
            title: ex.doc.title || cur.title,
            finalUrl: cur.finalUrl,
          });
          return;
        }
        postResult(payload.browseId, {
          content,
          title: ex.doc.title || cur.title,
          finalUrl: cur.finalUrl,
          note,
          truncated,
        });
      };

      if (action === "extract") {
        finish("extracted current page");
        return;
      }
      if (action === "back" || action === "reload" || action === "forward") {
        const h = historyRef.current;
        let target: string | null = null;
        if (action === "reload") target = stateRef.current.url;
        else if (action === "back" && h.index > 0) {
          h.index -= 1;
          target = h.entries[h.index]?.url ?? null;
        } else if (action === "forward" && h.index < h.entries.length - 1) {
          h.index += 1;
          target = h.entries[h.index]?.url ?? null;
        }
        if (!target) {
          finish("nothing to navigate to");
          return;
        }
        // Re-open through the gateway: fresh signed relay regardless of TTL.
        pendingRef.current = { id: payload.browseId, kind: "open" };
        publishState({ status: "loading" });
        ws.call<{ browseId: string; relayUrl: string; finalUrl: string; title: string }>(
          Methods.BROWSER_PANEL_OPEN,
          { url: target },
        )
          .then((res) => {
            if (!res || !res.relayUrl) throw new Error("empty relay response");
            loadEntry(
              {
                browseId: res.browseId,
                url: target!,
                relayUrl: res.relayUrl,
                finalUrl: res.finalUrl || target!,
                title: res.title || "",
              },
              false,
            );
          })
          .catch((err) => {
            pendingRef.current = null;
            publishState({ status: "error", note: String(err?.message ?? err) });
            postResult(payload.browseId, { error: `navigation failed: ${String(err?.message ?? err)}` });
          });
        return;
      }
      // click / type — resolve the ref in the live document. NB: iframe
      // elements belong to the iframe's own realm, so `instanceof` against
      // our window's constructors is always false — compare tag names and
      // take prototypes/events from the element's own window.
      const doc = iframeRef.current?.contentDocument ?? null;
      const el = doc ? refElement(doc, payload.ref ?? "") : null;
      if (!el) {
        finish(`ref ${payload.ref} not found on the current page (page may have changed)`);
        return;
      }
      const win = el.ownerDocument.defaultView;
      if (action === "type") {
        const value = payload.text ?? "";
        const proto = !win
          ? null
          : el.tagName === "TEXTAREA"
            ? win.HTMLTextAreaElement.prototype
            : el.tagName === "SELECT"
              ? win.HTMLSelectElement.prototype
              : win.HTMLInputElement.prototype;
        try {
          const setter = proto ? Object.getOwnPropertyDescriptor(proto, "value")?.set : undefined;
          setter?.call(el, value);
          el.dispatchEvent(new win!.Event("input", { bubbles: true }));
          el.dispatchEvent(new win!.Event("change", { bubbles: true }));
        } catch {
          // Non-form element tagged role=textbox — value setting not applicable.
        }
        finish(`typed into [${payload.ref}]`);
        return;
      }
      // click
      if (el.tagName === "A" && el.getAttribute("href")) {
        // .href resolves absolutely; the property read is realm-safe.
        const href = (el as HTMLAnchorElement).href;
        // The click's answer is the DESTINATION page: replyTo keeps the
        // invoke's browseId attached through openURL → loadEntry, so the
        // post-load extraction replies to this action instead of dropping.
        openURL(href, { replyTo: payload.browseId, note: "" });
        return;
      }
      el.dispatchEvent(new (win ?? window).MouseEvent("click", { bubbles: true, cancelable: true }));
      finish(`clicked [${payload.ref}] (static relay — pages run no scripts, so most buttons have no effect)`);
    },
    [extractCurrent, openURL, postResult, publishState, ws, loadEntry],
  );

  useWsEvent(Events.BROWSER_PANEL_INVOKE, (payload) => {
    const p = (payload ?? {}) as InvokePayload;
    if (!p.browseId) return;
    if (p.action) {
      // Agent operating the current page (click/type/extract/back/reload).
      onInvoke();
      runAction({ ...p, browseId: p.browseId });
      return;
    }
    // Open flow: the gateway already prepared the relay document.
    if (!p.relayUrl) return;
    pendingRef.current = { id: p.browseId, kind: "open" };
    const h = historyRef.current;
    h.entries = h.entries.slice(0, h.index + 1);
    h.entries.push({
      browseId: p.browseId,
      url: p.url ?? p.finalUrl ?? "",
      relayUrl: p.relayUrl,
      finalUrl: p.finalUrl ?? p.url ?? "",
      title: "",
    });
    h.index = h.entries.length - 1;
    publishState({
      url: p.url ?? p.finalUrl ?? "",
      relayUrl: p.relayUrl,
      finalUrl: p.finalUrl ?? p.url ?? "",
      title: "",
      status: "loading",
      note: "",
      ...syncNav(),
    });
    onInvoke();
  });

  /** iframe onLoad: attach interception, extract, reply to the pending op. */
  const handleIframeLoad = useCallback(
    (iframe: HTMLIFrameElement | null) => {
      iframeRef.current = iframe;
      const pending = pendingRef.current;
      const doc = iframe?.contentDocument ?? null;
      if (!iframe || !doc || !doc.body) {
        if (pending) {
          pendingRef.current = null;
          postResult(pending.id, { error: "document not readable (load failure or blocked)" });
        }
        publishState({ status: "error", note: "" });
        return;
      }

      // User-interaction interception (capture phase): links and form submits
      // become relay navigations; the static page itself stays script-free.
      // Tag-name checks — iframe elements live in another realm.
      doc.addEventListener("click", (ev) => {
        const target = (ev.target as Element | null)?.closest("a");
        const href = target?.getAttribute("href") ?? "";
        if (target && href && !(target as HTMLAnchorElement).href.startsWith("#")) {
          ev.preventDefault();
          ev.stopPropagation();
          openURL((target as HTMLAnchorElement).href, { note: "" });
        }
      }, true);
      doc.addEventListener("submit", (ev) => {
        ev.preventDefault();
        publishState({ note: "" /* forms stay inert in the static relay */ });
      }, true);

      const { markdown } = extractPageWithRefs(doc);
      const cur = stateRef.current;
      const title = doc.title || cur.title;
      publishState({ title, status: "ready", ...syncNav() });

      if (pending) {
        pendingRef.current = null;
        const thin = markdown.length < MIN_USEFUL_CHARS;
        if (thin) {
          postResult(pending.id, {
            error: "page content too thin — likely rendered via JavaScript",
            title,
            finalUrl: cur.finalUrl,
          });
        } else {
          postResult(pending.id, {
            content: markdown.slice(0, MAX_EXTRACT_CHARS),
            title,
            finalUrl: cur.finalUrl,
          });
        }
      }
    },
    [openURL, postResult, publishState, syncNav],
  );

  const goBack = useCallback(() => {
    const h = historyRef.current;
    if (h.index <= 0) return;
    const target = h.entries[h.index - 1];
    if (target) openURL(target.url, { note: "" });
  }, [openURL]);

  const goForward = useCallback(() => {
    const h = historyRef.current;
    if (h.index >= h.entries.length - 1) return;
    const target = h.entries[h.index + 1];
    if (target) openURL(target.url, { note: "" });
  }, [openURL]);

  const reload = useCallback(() => {
    if (stateRef.current.url) openURL(stateRef.current.url, { note: "" });
  }, [openURL]);

  const reset = useCallback(() => {
    historyRef.current = { entries: [], index: -1 };
    pendingRef.current = null;
    iframeRef.current = null;
    setState(initialState);
  }, []);

  return { state, handleIframeLoad, goBack, goForward, reload, reset, openURL };
}
