// Browser panel (client-side browsing): renders the sanitized relay document
// in a sandboxed same-origin iframe with ZCode-style navigation chrome —
// back/forward/reload, an editable URL bar, open-in-new-tab. The user's
// browser loads every subresource directly from the origin site; link clicks
// and URL-bar entries navigate through the gateway's sanitized relay
// (browser.panel.open), and agent actions operate the live page via the
// use-browser-panel hook.
import { ArrowLeft, ArrowRight, ExternalLink, Globe, RotateCw, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { BrowserPanelState } from "@/pages/chat/hooks/use-browser-panel";

interface BrowserPanelProps {
  open: boolean;
  onClose: () => void;
  state: BrowserPanelState;
  onIframeLoad: (iframe: HTMLIFrameElement | null) => void;
  onBack: () => void;
  onForward: () => void;
  onReload: () => void;
  onURLSubmit: (url: string) => void;
}

export function BrowserPanel({ open, onClose, state, onIframeLoad, onBack, onForward, onReload, onURLSubmit }: BrowserPanelProps) {
  const { t } = useTranslation("chat");
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const [urlDraft, setUrlDraft] = useState(state.url);

  // Keep the URL bar in sync while the user is not editing it.
  useEffect(() => {
    setUrlDraft(state.url);
  }, [state.url]);

  if (!open) return null;

  const showIframe = state.relayUrl !== "" && state.status !== "error";
  const displayTitle = state.title || state.finalUrl || t("browserPanel.title");

  const submitURL = () => {
    const trimmed = urlDraft.trim();
    if (!trimmed || trimmed === state.url) return;
    const withScheme = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
    onURLSubmit(withScheme);
  };

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      {/* Header: title + actions */}
      <div className="flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2 safe-top">
        <span className="flex min-w-0 items-center gap-2 text-sm font-medium">
          <Globe className="h-4 w-4 shrink-0 text-primary" />
          <span className="truncate">{displayTitle}</span>
        </span>
        <button
          type="button"
          onClick={() => window.open(state.finalUrl, "_blank", "noopener")}
          disabled={!state.finalUrl}
          title={t("browserPanel.openNewTab")}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
        >
          <ExternalLink className="h-4 w-4" />
        </button>
      </div>

      {/* Navigation toolbar: back / forward / reload / URL bar (ZCode-style) */}
      <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1.5">
        <button
          type="button"
          onClick={onBack}
          disabled={!state.canBack}
          title={t("browserPanel.back")}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onForward}
          disabled={!state.canForward}
          title={t("browserPanel.forward")}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40"
        >
          <ArrowRight className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onReload}
          disabled={!state.url}
          title={t("browserPanel.reload")}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40"
        >
          <RotateCw className="h-4 w-4" />
        </button>
        <input
          type="text"
          value={urlDraft}
          onChange={(e) => setUrlDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitURL();
          }}
          onBlur={() => setUrlDraft(state.url)}
          placeholder={t("browserPanel.urlPlaceholder")}
          spellCheck={false}
          className="min-w-0 flex-1 rounded-md border bg-muted px-2 py-1 text-xs text-muted-foreground md:text-xs focus:outline-none focus:ring-1 focus:ring-primary/50"
          aria-label={t("browserPanel.urlBar")}
        />
        <button
          type="button"
          onClick={onClose}
          title={t("browserPanel.close")}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Content */}
      <div className="relative flex min-h-0 flex-1 overflow-hidden overscroll-contain">
        {showIframe ? (
          <iframe
            ref={iframeRef}
            src={state.relayUrl}
            title={displayTitle}
            sandbox="allow-same-origin"
            referrerPolicy="no-referrer"
            className="h-full w-full border-0 bg-white"
            onLoad={() => onIframeLoad(iframeRef.current)}
          />
        ) : (
          <div className="flex h-full items-center justify-center p-4 text-center text-sm text-muted-foreground">
            {state.status === "error"
              ? t("browserPanel.error")
              : t("browserPanel.empty")}
          </div>
        )}
        {state.status === "loading" && (
          <div className="pointer-events-none absolute inset-x-0 top-0 h-0.5 overflow-hidden">
            <div className="h-full w-1/3 animate-pulse bg-primary" />
          </div>
        )}
      </div>

      {/* Status bar */}
      <div className="shrink-0 border-t px-3 py-1.5 text-xs text-muted-foreground safe-bottom">
        {state.note
          ? state.note
          : state.status === "loading"
            ? t("browserPanel.loading")
            : state.status === "ready"
              ? t("browserPanel.staticNote")
              : state.status === "error"
                ? t("browserPanel.errorHint")
                : t("browserPanel.empty")}
      </div>
    </div>
  );
}
