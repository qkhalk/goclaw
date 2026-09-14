import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight, Download, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { useHttp } from "@/hooks/use-ws";
import { formatFileSize } from "@/lib/format";
import { toast } from "@/stores/use-toast-store";
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
/** Video/audio blob cap — mirrors the download endpoint's server cap
 * (cloud.fetch_size_cap_mb, default 100 MB, 413 above it). */
const MEDIA_CAP = 100 << 20;

/** Size cap for a previewable kind (the cap shown in the too-large message). */
function capFor(kind: PreviewKind): number {
  switch (kind) {
    case "text":
      return TEXT_CAP;
    case "video":
    case "audio":
      return MEDIA_CAP;
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

/** Right-side preview Sheet for one file (images / text / pdf / video / audio
 * via the download endpoint + object URLs, revoked on every switch/close).
 * Video/audio stream through an authed blob fetch (a plain <video src> could
 * not send the Bearer header). Prev/next walks the current folder's files;
 * oversized and unsupported files fall back to a download button. */
export function PreviewSheet({
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

  // Load the preview body whenever the file changes; revoke the object URL on
  // every switch/close (no leak across a long preview session).
  useEffect(() => {
    if (!file || blocked) return;
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
  }, [accountId, name, kind, blocked]);

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

  return (
    <Sheet open onOpenChange={(open) => !open && onClose()}>
      <SheetContent className="flex w-full flex-col gap-0 sm:max-w-xl">
        <SheetHeader>
          <SheetTitle className="min-w-0 truncate pr-8">{file.entry.name}</SheetTitle>
          <p className="text-xs text-muted-foreground">{formatFileSize(file.entry.size)}</p>
        </SheetHeader>

        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain p-4">
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
          ) : loading ? (
            <p className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
              {t("preview.loading")}
            </p>
          ) : kind === "text" && textContent !== null ? (
            <pre className="min-w-0 whitespace-pre-wrap break-words rounded-md bg-muted/40 p-3 text-xs leading-relaxed">
              {textContent}
            </pre>
          ) : kind === "image" && url ? (
            <img src={url} alt={file.entry.name} className="mx-auto max-h-full max-w-full rounded-md object-contain" />
          ) : kind === "video" && url ? (
            <video
              src={url}
              controls
              preload="metadata"
              className="mx-auto max-h-full max-w-full rounded-md bg-black object-contain"
            />
          ) : kind === "audio" && url ? (
            <div className="flex flex-1 items-center justify-center">
              <audio src={url} controls preload="metadata" className="w-full max-w-xs" />
            </div>
          ) : kind === "pdf" && url ? (
            <iframe src={url} title={file.entry.name} className="h-full min-h-[60vh] w-full rounded-md border" />
          ) : (
            <p className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
              {t("preview.cannot_preview")}
            </p>
          )}
        </div>

        <SheetFooter className="mt-0">
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
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
