import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CheckCircle2, Download, Eraser, HelpCircle, Loader2, ShieldCheck, Trash2, XCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { DropZone } from "@/components/shared/drop-zone";
import { useAuthStore } from "@/stores/use-auth-store";
import { cn } from "@/lib/utils";

// Browser-only SDK: heavy work (Canvas + TypedArrays) stays on the client —
// images are never uploaded. Lazily imported to keep it out of the main chunk.
type RemoveFn = typeof import("@pilio/gemini-watermark-remover/browser")["removeWatermarkFromImage"];

interface Item {
  id: string;
  file: File;
  srcUrl: string;
  outUrl?: string;
  outName?: string;
  status: "pending" | "processing" | "done" | "skipped" | "failed";
  error?: string;
  applied?: boolean;
  confidence?: number | null;
}

const MAX_SIZE = 50 * 1024 * 1024; // 50 MB per image

export function WatermarkToolPage() {
  const { t } = useTranslation("toolbox");
  const userId = useAuthStore((s) => s.userId);
  const [items, setItems] = useState<Item[]>([]);
  const [running, setRunning] = useState(false);
  const [showPrivacy, setShowPrivacy] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const cancelRef = useRef(false);
  const removeFnRef = useRef<RemoveFn | null>(null);

  const ensureRemoveFn = useCallback(async () => {
    if (!removeFnRef.current) {
      const mod = await import("@pilio/gemini-watermark-remover/browser");
      removeFnRef.current = mod.removeWatermarkFromImage;
    }
    return removeFnRef.current;
  }, []);

  const addFiles = useCallback(
    (files: File[]) => {
      const accepted = files.filter(
        (f) => f.type.startsWith("image/") && f.size <= MAX_SIZE,
      );
      if (accepted.length === 0) return;
      setItems((prev) => [
        ...prev,
        ...accepted.map((file) => ({
          id: `${userId}:${file.name}:${file.lastModified}:${Math.random().toString(36).slice(2)}`,
          file,
          srcUrl: URL.createObjectURL(file),
          status: "pending" as const,
        })),
      ]);
    },
    [userId],
  );

  const processAll = useCallback(async () => {
    setRunning(true);
    cancelRef.current = false;
    try {
      const remove = await ensureRemoveFn();
      for (const item of items) {
        if (cancelRef.current) break;
        if (item.status !== "pending") continue;
        setItems((prev) =>
          prev.map((it) => (it.id === item.id ? { ...it, status: "processing" } : it)),
        );
        try {
          const img = new Image();
          img.src = item.srcUrl;
          await img.decode();
          const { canvas, meta } = await remove(img);
          // The engine returns HTMLCanvasElement in browsers; cast for toBlob.
          const blob = await new Promise<Blob | null>((resolve) =>
            (canvas as HTMLCanvasElement).toBlob(resolve, "image/png"),
          );
          if (!blob) throw new Error("encode failed");
          const applied = meta?.applied ?? false;
          setItems((prev) =>
            prev.map((it) =>
              it.id === item.id
                ? {
                    ...it,
                    status: applied ? "done" : "skipped",
                    applied,
                    confidence: meta?.selectionDebug
                      ? null
                      : (meta?.detection?.adaptiveConfidence ?? null),
                    outUrl: applied ? URL.createObjectURL(blob) : it.outUrl,
                    outName: applied ? item.file.name.replace(/\.[^.]+$/, "") + "-no-watermark.png" : it.outName,
                    error: applied ? undefined : (meta?.skipReason ?? undefined),
                  }
                : it,
            ),
          );
        } catch (e) {
          setItems((prev) =>
            prev.map((it) =>
              it.id === item.id
                ? { ...it, status: "failed", error: e instanceof Error ? e.message : String(e) }
                : it,
            ),
          );
        }
      }
    } finally {
      setRunning(false);
    }
  }, [ensureRemoveFn, items]);

  function downloadAll() {
    for (const item of items) {
      if (item.status === "done" && item.outUrl && item.outName) {
        const a = document.createElement("a");
        a.href = item.outUrl;
        a.download = item.outName;
        document.body.appendChild(a);
        a.click();
        a.remove();
      }
    }
  }

  function clearAll() {
    for (const item of items) {
      URL.revokeObjectURL(item.srcUrl);
      if (item.outUrl) URL.revokeObjectURL(item.outUrl);
    }
    setItems([]);
  }

  function removeItem(id: string) {
    setItems((prev) => {
      const target = prev.find((it) => it.id === id);
      if (target) {
        URL.revokeObjectURL(target.srcUrl);
        if (target.outUrl) URL.revokeObjectURL(target.outUrl);
      }
      return prev.filter((it) => it.id !== id);
    });
  }

  const doneCount = items.filter((it) => it.status === "done").length;

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
      <PageHeader title={t("watermark.title")} description={t("watermark.description")} />

      <div className="flex items-start gap-2 rounded-lg border border-green-500/30 bg-green-500/5 p-3 text-sm">
        <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-600" />
        <p className="text-muted-foreground">{t("watermark.privacy")}</p>
        <button
          type="button"
          onClick={() => setShowPrivacy((v) => !v)}
          className="ml-auto shrink-0 text-muted-foreground hover:text-foreground"
          aria-label={t("watermark.usage_note")}
        >
          <HelpCircle className="h-4 w-4" />
        </button>
      </div>
      {showPrivacy && (
        <p className="-mt-4 px-1 text-xs text-muted-foreground">
          {t("watermark.usage_note")} {t("watermark.credits")}
        </p>
      )}

      <DropZone onDrop={addFiles} title={t("watermark.drop_title")}>
        <div
          role="button"
          tabIndex={0}
          onClick={() => inputRef.current?.click()}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") inputRef.current?.click();
          }}
          className="flex min-h-40 cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border border-dashed p-6 text-center transition-colors hover:bg-muted/40"
        >
          <Eraser className="h-8 w-8 text-muted-foreground" />
          <p className="text-sm font-medium">{t("watermark.drop_title")}</p>
          <p className="text-xs text-muted-foreground">{t("watermark.drop_hint")}</p>
        </div>
      </DropZone>
      <input
        ref={inputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        multiple
        className="hidden"
        onChange={(e) => {
          addFiles(Array.from(e.target.files ?? []));
          e.target.value = "";
        }}
      />

      {items.length > 0 && (
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={processAll} disabled={running} className="min-h-11 sm:min-h-9">
              {running ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {t("watermark.processing")}
                </>
              ) : (
                <>
                  <Eraser className="mr-2 h-4 w-4" />
                  {t("watermark.processed")} ({doneCount}/{items.length})
                </>
              )}
            </Button>
            <Button
              variant="outline"
              onClick={downloadAll}
              disabled={doneCount === 0}
              className="min-h-11 sm:min-h-9"
            >
              <Download className="mr-2 h-4 w-4" />
              {t("watermark.download_all")}
            </Button>
            <Button variant="ghost" onClick={clearAll} disabled={running} className="min-h-11 sm:min-h-9">
              <Trash2 className="mr-2 h-4 w-4" />
              {t("watermark.clear")}
            </Button>
          </div>

          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {items.map((item) => (
              <li key={item.id} className="flex flex-col gap-2 rounded-lg border p-3">
                <div className="flex items-center gap-2">
                  <p className="min-w-0 flex-1 truncate text-sm font-medium" title={item.file.name}>
                    {item.file.name}
                  </p>
                  <StatusChip item={item} />
                  <Button
                    variant="ghost" size="icon-sm" aria-label={t("watermark.remove")}
                    disabled={running} onClick={() => removeItem(item.id)}
                  >
                    <XCircle className="h-4 w-4" />
                  </Button>
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <figure className="flex flex-col gap-1">
                    <img
                      src={item.srcUrl}
                      alt={t("watermark.before")}
                      className="aspect-video w-full rounded-md border object-contain"
                    />
                    <figcaption className="text-center text-xs text-muted-foreground">
                      {t("watermark.before")}
                    </figcaption>
                  </figure>
                  <figure className="flex flex-col gap-1">
                    {item.outUrl ? (
                      <img
                        src={item.outUrl}
                        alt={t("watermark.after")}
                        className="aspect-video w-full rounded-md border object-contain"
                      />
                    ) : (
                      <div className="flex aspect-video w-full items-center justify-center rounded-md border bg-muted/30">
                        {item.status === "processing" && (
                          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                        )}
                      </div>
                    )}
                    <figcaption className="text-center text-xs text-muted-foreground">
                      {t("watermark.after")}
                    </figcaption>
                  </figure>
                </div>
                {item.status === "done" && item.outUrl && (
                  <Button variant="outline" size="sm" asChild className="min-h-11 sm:min-h-9">
                    <a href={item.outUrl} download={item.outName}>
                      <Download className="mr-2 h-4 w-4" />
                      {t("watermark.download")}
                    </a>
                  </Button>
                )}
                {item.error && (
                  <p className="truncate text-xs text-destructive" title={item.error}>
                    {item.error}
                  </p>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function StatusChip({ item }: { item: Item }) {
  const { t } = useTranslation("toolbox");
  const map = {
    pending: null,
    processing: (
      <span className="flex items-center gap-1 text-xs text-blue-600">
        <Loader2 className="h-3 w-3 animate-spin" />
        {t("watermark.processing")}
      </span>
    ),
    done: (
      <span className={cn("flex items-center gap-1 text-xs text-green-600")}>
        <CheckCircle2 className="h-3 w-3" />
        {t("watermark.processed")}
      </span>
    ),
    skipped: (
      <span className="flex items-center gap-1 text-xs text-muted-foreground">
        <HelpCircle className="h-3 w-3" />
        {t("watermark.skipped")}
      </span>
    ),
    failed: (
      <span className="flex items-center gap-1 text-xs text-destructive">
        <XCircle className="h-3 w-3" />
        {t("watermark.failed")}
      </span>
    ),
  } as const;
  return map[item.status];
}
