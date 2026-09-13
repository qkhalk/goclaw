import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ChevronRight, Folder, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";
import type { CloudFileEntry } from "../hooks/use-cloud";
import { childPath, normalizePath, parentPath, pathCrumbs } from "./paths";

/** Simple folder browser used as a destination picker: lists the folders of
 * one level, navigate by clicking, go up via breadcrumbs, then confirm with
 * "Use this folder". */
export function FolderPickerDialog({
  open,
  onOpenChange,
  accountId,
  title,
  confirmLabel,
  initialPath,
  onPick,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accountId: string;
  title: string;
  confirmLabel: string;
  /** Folder the browser starts in. */
  initialPath: string;
  onPick: (folderPath: string) => void;
}) {
  const { t } = useTranslation("cloud");
  const { t: tc } = useTranslation("common");
  const http = useHttp();
  const [browsePath, setBrowsePath] = useState(initialPath);

  // Re-open → restart at the initial folder.
  useEffect(() => {
    if (open) setBrowsePath(normalizePath(initialPath));
  }, [open, initialPath]);

  const files = useQuery({
    queryKey: queryKeys.cloud.files(accountId, browsePath),
    enabled: open,
    staleTime: 30_000,
    queryFn: () =>
      http.get<{ path: string; entries: CloudFileEntry[] }>(
        `/v1/cloud/accounts/${accountId}/files?path=${encodeURIComponent(browsePath)}&limit=200`,
      ),
  });

  const folders = useMemo(
    () => (files.data?.entries ?? []).filter((e) => e.is_dir).sort((a, b) => a.name.localeCompare(b.name)),
    [files.data],
  );

  const crumbs = useMemo(() => pathCrumbs(browsePath), [browsePath]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>

        {/* Breadcrumb of the browse position */}
        <div className="flex min-h-[28px] flex-wrap items-center gap-0.5 text-xs text-muted-foreground">
          <button
            type="button"
            className={cn("rounded px-1 py-0.5 hover:bg-muted", browsePath === "/" && "font-medium text-foreground")}
            onClick={() => setBrowsePath("/")}
          >
            {t("detail.root")}
          </button>
          {crumbs.map((c) => (
            <span key={c.path} className="flex items-center gap-0.5">
              <ChevronRight className="h-3 w-3" />
              <button
                type="button"
                className={cn(
                  "max-w-[140px] truncate rounded px-1 py-0.5 hover:bg-muted",
                  c.path === browsePath && "font-medium text-foreground",
                )}
                onClick={() => setBrowsePath(c.path)}
              >
                {c.name}
              </button>
            </span>
          ))}
        </div>

        <div className="max-h-64 min-h-[120px] overflow-y-auto rounded-md border p-1 overscroll-contain">
          {files.isLoading ? (
            <div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {t("detail.loading_files")}
            </div>
          ) : files.isError ? (
            <p className="p-3 text-sm text-muted-foreground">{t("detail.files_unavailable")}</p>
          ) : (
            <>
              {browsePath !== "/" && (
                <button
                  type="button"
                  className="flex min-h-11 w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-muted/60"
                  onClick={() => setBrowsePath(parentPath(browsePath))}
                >
                  <Folder className="h-4 w-4 shrink-0 text-muted-foreground" />
                  ..
                </button>
              )}
              {folders.map((f) => (
                <button
                  key={f.name}
                  type="button"
                  className="flex min-h-11 w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-muted/60"
                  onClick={() => setBrowsePath((p) => childPath(p, f.name))}
                >
                  <Folder className="h-4 w-4 shrink-0 text-sky-500" />
                  <span className="min-w-0 flex-1 truncate">{f.name}</span>
                  <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                </button>
              ))}
              {!files.isLoading && !files.isError && folders.length === 0 && browsePath === "/" && (
                <p className="p-3 text-sm text-muted-foreground">{t("detail.no_files")}</p>
              )}
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc("cancel")}
          </Button>
          <Button onClick={() => onPick(browsePath)}>{confirmLabel}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
