import { useTranslation } from "react-i18next";
import { Loader2, X } from "lucide-react";import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { cn } from "@/lib/utils";
import { formatFileSize } from "@/lib/format";
import type { CloudUploads, UploadItem } from "./use-cloud-uploads";

function statusLabel(item: UploadItem, t: (key: string) => string): string {
  switch (item.status) {
    case "done":
      return t("files.upload_done");
    case "failed":
      return t("files.upload_failed");
    case "canceled":
      return t("files.cancel_upload");
    default:
      return `${formatFileSize(item.loaded)} / ${formatFileSize(item.size)}`;
  }
}

/** Floating per-file upload progress panel (aggregate + rows + cancel). */
export function UploadPanel({
  uploads,
  className,
}: {
  uploads: CloudUploads;
  className?: string;
}) {
  const { t } = useTranslation("cloud");
  if (uploads.items.length === 0) return null;

  const activeCount = uploads.items.filter(
    (it) => it.status === "queued" || it.status === "uploading",
  ).length;

  return (
    <div
      className={cn(
        "fixed right-3 bottom-20 z-40 w-80 max-w-[calc(100vw-1.5rem)] rounded-lg border bg-background/95 p-3 shadow-lg backdrop-blur md:bottom-4",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <p className="flex items-center gap-2 text-sm font-medium">
          {activeCount > 0 && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          {t("files.uploads_title")}
          <span className="text-xs font-normal text-muted-foreground">
            {activeCount > 0 ? `(${activeCount})` : ""}
          </span>
        </p>
        <div className="flex items-center gap-1">
          {activeCount > 0 ? (
            <Button
              variant="ghost"
              size="xs"
              className="min-h-11 sm:min-h-6"
              onClick={() => uploads.cancel()}
            >
              {t("files.cancel_upload")}
            </Button>
          ) : (
            <Button variant="ghost" size="xs" className="min-h-11 sm:min-h-6" onClick={uploads.clearFinished}>
              {t("files.clear_finished")}
            </Button>
          )}
        </div>
      </div>

      <ul className="mt-2 max-h-56 space-y-2 overflow-y-auto overscroll-contain">
        {uploads.items.map((it) => (
          <li key={it.id} className="space-y-1">
            <div className="flex items-center justify-between gap-2 text-xs">
              <span className="min-w-0 flex-1 truncate" title={it.name}>
                {it.name}
              </span>
              <span
                className={cn(
                  "shrink-0 tabular-nums",
                  it.status === "failed" && "text-destructive",
                  it.status === "done" && "text-emerald-600 dark:text-emerald-400",
                  it.status === "canceled" && "text-muted-foreground",
                )}
              >
                {statusLabel(it, (k) => t(k as never))}
              </span>
              {(it.status === "queued" || it.status === "uploading") && (
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={t("files.cancel_upload")}
                  onClick={() => uploads.cancel(it.id)}
                >
                  <X className="h-3 w-3" />
                </Button>
              )}
            </div>
            <Progress
              value={it.status === "done" ? 100 : it.size > 0 ? Math.round((it.loaded / it.size) * 100) : 0}
              className={cn("h-1.5", it.status === "failed" && "[&>[data-slot=progress-indicator]]:bg-destructive")}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}
