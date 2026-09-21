// RemoteBrowserView — the "server browser" mode of the chat panel: the
// gateway's headless Chrome (Rod) opens and drives the page server-side and
// streams the rendered view back as screenshots. This is the only mode that
// can access + operate JS-heavy or framing-protected external sites that the
// relay (script-free) and live iframe (cross-origin) cannot; the same
// browser backs the agent's `browser` tool, so what the user sees here is
// the page the agent reads and clicks.
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Loader2, MonitorSmartphone, RotateCw } from "lucide-react";
import { useWs } from "@/hooks/use-ws";
import { Events } from "@/api/protocol";

interface RemoteCapture {
  targetId?: string;
  screenshot?: string;
  url?: string;
  title?: string;
  snapshot?: string;
}

export function RemoteBrowserView() {
  const { t } = useTranslation("chat");
  const ws = useWs();
  const [targetId, setTargetId] = useState("");
  const [shot, setShot] = useState("");
  const [url, setUrl] = useState("");
  const [title, setTitle] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [draft, setDraft] = useState("");

  const open = async (raw: string) => {
    const target = raw.trim();
    if (!target || loading) return;
    setLoading(true);
    setError("");
    try {
      const res = await ws.call<RemoteCapture>(Events.BROWSER_REMOTE_OPEN, { url: target });
      setTargetId(res.targetId ?? "");
      setShot(res.screenshot ?? "");
      setUrl(res.url ?? target);
      setTitle(res.title ?? "");
      setDraft(res.url ?? target);
      if (!res.screenshot && !res.snapshot) {
        setError(t("browserPanel.remote.noCapture"));
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const act = async (action: string) => {
    if (!targetId || loading) return;
    setLoading(true);
    setError("");
    try {
      const res = await ws.call<RemoteCapture>(Events.BROWSER_REMOTE_ACT, { targetId, action });
      setShot(res.screenshot ?? shot);
      setUrl(res.url ?? url);
      setTitle(res.title ?? title);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const refresh = async () => {
    if (!targetId || loading) return;
    setLoading(true);
    try {
      const res = await ws.call<RemoteCapture>(Events.BROWSER_REMOTE_SCREENSHOT, { targetId });
      setShot(res.screenshot ?? shot);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1.5">
        <button
          type="button"
          onClick={() => act("back")}
          disabled={!targetId || loading}
          title={t("browserPanel.back")}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={refresh}
          disabled={!targetId || loading}
          title={t("browserPanel.reload")}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40"
        >
          <RotateCw className="h-4 w-4" />
        </button>
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") open(draft);
          }}
          placeholder={t("browserPanel.remote.placeholder")}
          spellCheck={false}
          className="min-w-0 flex-1 rounded-md border bg-muted px-2 py-1 text-base text-muted-foreground md:text-xs focus:outline-none focus:ring-1 focus:ring-primary/50"
        />
      </div>

      <div className="relative flex min-h-0 flex-1 overflow-auto overscroll-contain bg-muted/30">
        {loading && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60">
            <Loader2 className="h-5 w-5 animate-spin text-primary" />
          </div>
        )}
        {shot ? (
          <img src={shot} alt={title || url} className="h-auto min-h-full w-full object-contain" />
        ) : (
          <div className="flex h-full w-full flex-col items-center justify-center gap-2 p-6 text-center text-sm text-muted-foreground">
            <MonitorSmartphone className="h-6 w-6" />
            <p>{error || t("browserPanel.remote.empty")}</p>
          </div>
        )}
      </div>

      <div className="shrink-0 border-t px-3 py-1.5 text-xs text-muted-foreground safe-bottom">
        {url ? `${title ? title + " · " : ""}${url}` : t("browserPanel.remote.note")}
      </div>
    </div>
  );
}
