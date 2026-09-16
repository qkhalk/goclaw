import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight, Download, TriangleAlert, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useHttp } from "@/hooks/use-ws";
import { formatFileSize } from "@/lib/format";
import { toast } from "@/stores/use-toast-store";
import { queryKeys } from "@/lib/query-keys";
import type { CloudFileEntry } from "../hooks/use-cloud";
import { opErrorToast } from "./op-error";
import { rawPath } from "./paths";

/** Previewable kinds, decided by EXTENSION (the list endpoint returns no MIME
 * type; extension mapping keeps the backend untouched). */
type PreviewKind = "image" | "text" | "pdf" | "video" | "audio" | "unsupported";

const IMAGE_EXT = new Set(["png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "ico"]);
const TEXT_EXT = new Set(["txt", "md", "markdown", "json", "csv", "log", "xml", "yml", "yaml", "ini", "conf", "ts", "tsx", "js", "jsx", "py", "go", "css", "html", "sql", "sh"]);
const PDF_EXT = new Set(["pdf"]);
const VIDEO_EXT = new Set(["mp4", "webm", "mov", "m4v"]);
const AUDIO_EXT = new Set(["mp3", "m4a", "wav"]);

const IMAGE_CAP = 25 << 20; // 25 MB — never auto-load anything bigger
const TEXT_CAP = 1 << 20; // 1 MB — text renders as <pre>

/** Streaming kinds use short-lived signed URLs (<video>/<audio>/<iframe> tags
 * cannot send Bearer headers), so the browser streams via HTTP byte-range —
 * the server's cloud.fetch_size_cap_mb (413) still bounds them. */
const STREAM_CAP = 100 << 20;

function capFor(kind: PreviewKind): number {
  switch (kind) {
    case "text":
      return TEXT_CAP;
    case "video":
    case "audio":
    case "pdf":
      return STREAM_CAP;
    default:
      return IMAGE_CAP;
  }
}

function extOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i >= 0 ? name.slice(i + 1).toLowerCase() : "";
}

export function previewKind(entry: CloudFileEntry): PreviewKind {
  const ext = extOf(entry.name);
  if (IMAGE_EXT.has(ext)) return "image";
  if (TEXT_EXT.has(ext)) return "text";
  if (PDF_EXT.has(ext)) return "pdf";
  if (VIDEO_EXT.has(ext)) return "video";
  if (AUDIO_EXT.has(ext)) return "audio";
  return "unsupported";
}

/** One previewable file: the list entry plus its encoded remote path. */
export interface PreviewFile {
  entry: CloudFileEntry;
  /** Full encoded remote path (childPath form). */
  path: string;
}

/** Front-center preview overlay for one file (replaces the side Sheet):
 * images/text load via the authed download endpoint (blob object URLs),
 * video/audio/pdf stream through short-lived signed URLs with HTTP
 * byte-range seeking. Prev/next walks the current folder's files; oversized
 * and unsupported files fall back to a download button. */
export function PreviewModal({
  accountId,
  files,
  index,
  onIndexChange,
  onClose,
}: {
  accountId: string;
  /** Files of the current folder (folders excluded), in display order. */
  files: PreviewFile[];
  index: number;
  onIndexChange: (index: number) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation("cloud");
  const http = useHttp();
  const file = files[index];
  const [url, setUrl] = useState<string | null>(null);
  const [textContent, setTextContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const urlRef = useRef<string | null>(null);

  const name = file?.entry.name ?? "";
  const kind = file ? previewKind(file.entry) : "unsupported";
  const tooLarge = !!file && file.entry.size > capFor(kind);
  const blocked = kind === "unsupported" || tooLarge;
  const streams = kind === "video" || kind === "audio" || kind === "pdf";

  // Signed streaming URL (video/audio/pdf) — expires, so it is fetched per
  // file switch and re-fetched below the TTL while the preview stays open
  // (seeking after expiry would otherwise error with no recovery).
  const signed = useQuery({
    queryKey: queryKeys.cloud.thumb(accountId, file?.path ?? ""),
    enabled: !!file && streams && !tooLarge,
    staleTime: 60_000,
    refetchInterval: 5 * 60_000, // token TTL is 10 min — refresh before expiry
    queryFn: async () => {
      const res = await http.post<{ url: string }>(
        `/v1/cloud/accounts/${accountId}/files/sign`,
        { path: rawPath(file!.path) },
      );
      return res.url;
    },
  });

  // Blob load for image/text kinds; revoked on every switch/close.
  useEffect(() => {
    if (!file || blocked || streams) return;
    let cancelled = false;
    setLoading(true);
    setFailed(false);
    setTextContent(null);
    (async () => {
      try {
        const blob = await http.fetchBlob(`/v1/cloud/accounts/${accountId}/files/download`, {
          path: rawPath(file.path),
        });
        if (cancelled) return;
        if (kind === "text") {
          const text = await blob.text();
          if (!cancelled) setTextContent(text);
        } else {
          const objURL = URL.createObjectURL(blob);
          urlRef.current = objURL;
          if (!cancelled) setUrl(objURL);
          else URL.revokeObjectURL(objURL);
        }
      } catch {
        if (!cancelled) setFailed(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
      if (urlRef.current) {
        URL.revokeObjectURL(urlRef.current);
        urlRef.current = null;
      }
      setUrl(null);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accountId, name, kind, blocked, streams]);

  // Esc / arrow keys at the overlay level (like the image lightbox).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft" && index > 0) onIndexChange(index - 1);
      else if (e.key === "ArrowRight" && index < files.length - 1) onIndexChange(index + 1);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [index, files.length, onIndexChange, onClose]);

  if (!file) return null;

  async function download() {
    if (!file) return;
    try {
      const blob = await http.fetchBlob(`/v1/cloud/accounts/${accountId}/files/download`, {
        path: rawPath(file.path),
      });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = file.entry.name;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (e) {
      opErrorToast(e, t);
    }
  }

  const streamURL = streams && signed.data ? `${signed.data}#t=0.001` : null;

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      role="dialog"
      aria-modal="true"
      aria-label={name}
    >
      <div className="flex max-h-[92dvh] w-full max-w-4xl flex-col overflow-hidden rounded-lg border bg-background shadow-2xl">
        {/* Header */}
        <div className="flex min-w-0 items-center gap-2 border-b px-4 py-2.5">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{file.entry.name}</p>
            <p className="text-xs text-muted-foreground">{formatFileSize(file.entry.size)}</p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 shrink-0"
            aria-label={t("preview.close")}
            onClick={onClose}
          >
            <X className="h-4 w-4" />
          </Button>
        </div>

        {/* Body */}
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain bg-black/5 p-4">
          {blocked || failed ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
              <TriangleAlert className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">
                {tooLarge
                  ? t("preview.too_large", {
                      cap: kind === "text" ? "1 MB" : kind === "image" ? "25 MB" : "100 MB",
                    })
                  : failed
                    ? t("preview.error")
                    : t("preview.cannot_preview")}
              </p>
              <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => void download()}>
                <Download className="mr-1.5 h-4 w-4" />
                {t("preview.download")}
              </Button>
            </div>
          ) : loading || (streams && !streamURL && !signed.isError) ? (
            <p className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
              {t("preview.loading")}
            </p>
          ) : kind === "text" && textContent !== null ? (
            <pre className="min-w-0 whitespace-pre-wrap break-words rounded-md bg-muted/40 p-3 text-xs leading-relaxed">
              {textContent}
            </pre>
          ) : kind === "image" && url ? (
            <img src={url} alt={file.entry.name} className="mx-auto max-h-full max-w-full rounded-md object-contain" />
          ) : kind === "video" && streamURL ? (
            <video
              src={streamURL}
              controls
              preload="metadata"
              playsInline
              className="mx-auto max-h-full w-full max-w-3xl rounded-md bg-black object-contain"
            />
          ) : kind === "audio" && streamURL ? (
            <div className="flex flex-1 items-center justify-center">
              <audio src={signed.data} controls preload="metadata" className="w-full max-w-xs" />
            </div>
          ) : kind === "pdf" && streamURL ? (
            <iframe src={signed.data} title={file.entry.name} className="h-full min-h-[65vh] w-full rounded-md border bg-white" />
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
              <TriangleAlert className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">{t("preview.error")}</p>
              <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => void download()}>
                <Download className="mr-1.5 h-4 w-4" />
                {t("preview.download")}
              </Button>
            </div>
          )}
        </div>

        {/* Footer nav */}
        <div className="flex items-center gap-1 border-t px-3 py-2">
          <Button
            variant="ghost"
            size="sm"
            className="min-h-11 sm:min-h-8"
            disabled={index <= 0}
            onClick={() => onIndexChange(index - 1)}
          >
            <ChevronLeft className="mr-1 h-4 w-4" />
            {t("preview.prev")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="min-h-11 sm:min-h-8"
            disabled={index >= files.length - 1}
            onClick={() => onIndexChange(index + 1)}
          >
            {t("preview.next")}
            <ChevronRight className="ml-1 h-4 w-4" />
          </Button>
          <div className="flex-1" />
          <Button
            variant="outline"
            size="sm"
            className="min-h-11 sm:min-h-8"
            onClick={() => {
              void download();
              toast.success(t("preview.download_started"));
            }}
          >
            <Download className="mr-1.5 h-4 w-4" />
            {t("preview.download")}
          </Button>
        </div>
      </div>
    </div>
  );
}
