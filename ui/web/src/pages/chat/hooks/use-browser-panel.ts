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

/**
 * relay — sanitized same-origin document (default; script-free, agent-operable
 * via [eN] refs). live — sandboxed preview of the real URL for JavaScript-
 * rendered pages whose static relay would be blank: scripts run inside the
 * frame but it gets an opaque origin (no allow-same-origin), so the page is
 * isolated from the dashboard and the panel cannot read its DOM — preview
 * only. The panel auto-switches to live when a relay extraction is too thin.
 */
export type BrowserPanelMode = "relay" | "live";

export interface BrowserPanelState {
  /** Current page's original URL ("" = nothing open). */
  url: string;
  finalUrl: string;
  title: string;
  relayUrl: string;
  status: BrowserPanelStatus;
  mode: BrowserPanelMode;
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
  mode: "relay",
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
  const modeRef = useRef<BrowserPanelMode>("relay");
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

  /** Switch relay↔live. modeRef updates synchronously so an iframe onLoad
   *  arriving before React re-renders still reads the right mode. */
  const setMode = useCallback(
    (mode: BrowserPanelMode) => {
      modeRef.current = mode;
      publishState({ mode, status: "loading", note: "" });
    },
    [publishState],
  );

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
      modeRef.current = "relay";
      publishState({
        url: entry.url,
        finalUrl: entry.finalUrl,
        title: entry.title,
        relayUrl: entry.relayUrl,
        status: "loading",
        mode: "relay",
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
      // Live preview frames are cross-origin (opaque origin) — no DOM access,
      // so refs and extraction are impossible. Navigation actions still work:
      // they re-open through the relay pipeline.
      if (modeRef.current === "live" && (action === "click" || action === "type" || action === "extract")) {
        postResult(payload.browseId, {
          error:
            "the panel is showing a live preview of a JavaScript-rendered page — refs and static extraction are unavailable; open a static page instead, or ask the user to interact with the preview",
        });
        publishState({ status: "ready" });
        return;
      }
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
      if (modeRef.current === "live") {
        // Live preview: the real page runs in a sandboxed cross-origin frame.
        // contentDocument is null by design — no interception, no extraction.
        if (pending) {
          pendingRef.current = null;
          postResult(pending.id, {
            error:
              "page rendered live (JavaScript) — static extraction unavailable in live preview mode",
          });
        }
        publishState({ status: "ready", ...syncNav() });
        return;
      }
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
      // Thin relay extraction = JS-rendered page: the static document is a
      // blank shell, so switch the panel to the live preview instead. The
      // agent already got the honest "too thin" error above.
      if (markdown.length < MIN_USEFUL_CHARS && /^https?:/i.test(cur.finalUrl)) {
        setMode("live");
      }
    },
    [openURL, postResult, publishState, setMode, syncNav],
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

  /** User toggled static↔live. Back to static re-opens through the gateway:
   *  the signed relay URL may be past its TTL, and a fresh load also restores
   *  ref extraction for agent actions. */
  const toggleMode = useCallback(() => {
    if (!stateRef.current.finalUrl) return;
    if (modeRef.current === "live") {
      modeRef.current = "relay";
      openURL(stateRef.current.url || stateRef.current.finalUrl, { note: "" });
    } else {
      setMode("live");
    }
  }, [openURL, setMode]);

  const reset = useCallback(() => {
    historyRef.current = { entries: [], index: -1 };
    pendingRef.current = null;
    iframeRef.current = null;
    modeRef.current = "relay";
    setState(initialState);
  }, []);

  return { state, handleIframeLoad, goBack, goForward, reload, reset, openURL, toggleMode };
}
