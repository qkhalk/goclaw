// Browser panel (client-side browsing): renders the sanitized relay document
// in a sandboxed same-origin iframe. The user's browser loads every
// subresource directly from the origin site — the server only relayed the
// single HTML document (web_browse). Extraction posts back from the hook.
import { ExternalLink, Globe, RotateCw, X } from "lucide-react";
import { useRef } from "react";
import { useTranslation } from "react-i18next";
import type { BrowserPanelState } from "@/pages/chat/hooks/use-browser-panel";

interface BrowserPanelProps {
  open: boolean;
  onClose: () => void;
  state: BrowserPanelState;
  onIframeLoad: (iframe: HTMLIFrameElement | null) => void;
  onReload: () => void;
}

export function BrowserPanel({ open, onClose, state, onIframeLoad, onReload }: BrowserPanelProps) {
  const { t } = useTranslation("chat");
  const iframeRef = useRef<HTMLIFrameElement | null>(null);

  if (!open) return null;

  const showIframe = state.relayUrl !== "" && state.status !== "error";
  const displayTitle = state.title || state.finalUrl || t("browserPanel.title");

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      {/* Header: title + actions */}
      <div className="flex items-center justify-between gap-2 border-b px-3 py-2 safe-top">
        <span className="flex min-w-0 items-center gap-2 text-sm font-medium">
          <Globe className="h-4 w-4 shrink-0 text-primary" />
          <span className="truncate">{displayTitle}</span>
        </span>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={() => window.open(state.finalUrl, "_blank", "noopener")}
            disabled={!state.finalUrl}
            title={t("browserPanel.openNewTab")}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
          >
            <ExternalLink className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={onReload}
            disabled={!state.relayUrl}
            title={t("browserPanel.reload")}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
          >
            <RotateCw className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={onClose}
            title={t("browserPanel.close")}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* URL bar (read-only): shows where assets actually come from */}
      {state.finalUrl && (
        <div className="border-b px-3 py-1.5">
          <input
            type="text"
            readOnly
            value={state.finalUrl}
            onFocus={(e) => e.target.select()}
            className="w-full rounded-md border bg-muted px-2 py-1 text-xs text-muted-foreground md:text-xs"
            aria-label={t("browserPanel.urlBar")}
          />
        </div>
      )}

      {/* Content */}
      <div className="relative flex-1 overflow-hidden overscroll-contain">
        {showIframe ? (
          <iframe
            ref={iframeRef}
            key={`${state.relayUrl}#${state.reloadNonce}`}
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
      <div className="border-t px-3 py-1.5 text-xs text-muted-foreground safe-bottom">
        {state.status === "loading" && t("browserPanel.loading")}
        {state.status === "ready" && t("browserPanel.ready")}
        {state.status === "error" && t("browserPanel.errorHint")}
        {state.status === "idle" && t("browserPanel.empty")}
      </div>
    </div>
  );
}
