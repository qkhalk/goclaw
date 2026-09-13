import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowRightLeft, Check, FolderInput, Loader2, TriangleAlert, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";
import { toast } from "@/stores/use-toast-store";
import {
  useCloudAccounts,
  useCloudTransfer,
  type CloudAccount,
  type CloudFileEntry,
} from "../hooks/use-cloud";
import { childPath, pathDisplayName } from "./paths";
import { FolderPickerDialog } from "./folder-picker-dialog";

/** Sentinel for "no target account picked" (Radix Select forbids empty values). */
const NONE = "__none__";

type TransferItemStatus = "pending" | "running" | "done" | "background" | "error";

interface TransferItemState {
  name: string;
  isDir: boolean;
  status: TransferItemStatus;
  jobId?: string;
  error?: string;
}

const JOB_POLL_MS = 3000;
const JOB_POLL_MAX = 240; // polls (~12 min) — the rclone job keeps running either way

/** Cross-account copy/move: pick a target account + destination folder, then
 * transfer the selected entries one by one (provider rate-limit friendly).
 * Folders queue an async rclone job polled here; closing the dialog mid-run
 * abandons the polling, never the transfer itself. */
export function TransferDialog({
  open,
  onOpenChange,
  sourceAccount,
  sources,
  sourcePath,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Account the entries come from. */
  sourceAccount: CloudAccount;
  /** Selected entries (names are unique within sourcePath). */
  sources: CloudFileEntry[];
  /** Encoded folder the entries live in. */
  sourcePath: string;
}) {
  const { t } = useTranslation("cloud");
  const { t: tc } = useTranslation("common");
  const http = useHttp();
  const queryClient = useQueryClient();
  const { accounts } = useCloudAccounts();
  const doTransfer = useCloudTransfer();

  const targets = useMemo(
    () => accounts.filter((a) => a.id !== sourceAccount.id),
    [accounts, sourceAccount.id],
  );
  const [targetId, setTargetId] = useState<string>(NONE);
  const [mode, setMode] = useState<"copy" | "move">("copy");
  const [targetPath, setTargetPath] = useState<string>("/");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [items, setItems] = useState<TransferItemState[]>([]);
  const [running, setRunning] = useState(false);
  const [transferring, setTransferring] = useState(false);
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  // Re-open → reset to a fresh form (a previous run's per-item log is dropped).
  useEffect(() => {
    if (open) {
      setTargetId(NONE);
      setMode("copy");
      setTargetPath("/");
      setItems([]);
      setRunning(false);
      setTransferring(false);
    }
  }, [open]);

  const target = targets.find((a) => a.id === targetId);
  const hasFolder = sources.some((s) => s.is_dir);

  function statusIcon(item: TransferItemState) {
    switch (item.status) {
      case "running":
        return <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />;
      case "done":
        return <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />;
      case "background":
        return <Loader2 className="h-3.5 w-3.5 animate-spin text-sky-500" />;
      case "error":
        return <TriangleAlert className="h-3.5 w-3.5 text-destructive" />;
      default:
        return <span className="h-3.5 w-3.5" />;
    }
  }

  function patchItem(name: string, update: Partial<TransferItemState>) {
    if (!aliveRef.current) return;
    setItems((prev) => prev.map((it) => (it.name === name ? { ...it, ...update } : it)));
  }

  /** Poll an async rclone transfer job to completion. */
  async function pollJob(jobId: string): Promise<{ finished: boolean; success: boolean; error?: string }> {
    for (let i = 0; i < JOB_POLL_MAX; i++) {
      if (!aliveRef.current) return { finished: false, success: false };
      const res = await http.get<{
        job_id: string;
        finished: boolean;
        success: boolean;
        error?: string;
      }>(`/v1/cloud/transfers/${jobId}`);
      if (res.finished) return res;
      await new Promise((r) => setTimeout(r, JOB_POLL_MS));
    }
    return { finished: false, success: false };
  }

  async function handleRun() {
    if (!target) return;
    setRunning(true);
    setTransferring(true);
    setItems(sources.map((s) => ({ name: s.name, isDir: s.is_dir, status: "pending" as const })));
    let failures = 0;
    let background = 0;
    try {
      for (const source of sources) {
        if (!aliveRef.current) return;
        patchItem(source.name, { status: "running" });
        const src = childPath(sourcePath, source.name);
        const dst = childPath(targetPath, source.name);
        try {
          const res = await doTransfer({
            source_account_id: sourceAccount.id,
            source_path: src,
            target_account_id: target.id,
            target_path: dst,
            mode,
          });
          if (res.job_id) {
            background++;
            patchItem(source.name, { status: "background", jobId: res.job_id });
            const job = await pollJob(res.job_id);
            if (job.finished && job.success) {
              patchItem(source.name, { status: "done" });
            } else if (job.finished) {
              failures++;
              patchItem(source.name, { status: "error", error: job.error });
            }
          } else {
            patchItem(source.name, { status: "done" });
          }
        } catch (e) {
          failures++;
          patchItem(source.name, { status: "error", error: e instanceof Error ? e.message : String(e) });
        }
      }
      if (aliveRef.current) {
        if (failures > 0) {
          toast.error(t("transfer.title"), t("transfer.some_failed", { count: failures }));
        } else {
          toast.success(t("transfer.done"));
        }
        if (background > 0) {
          toast.info(t("transfer.background_running"));
        }
      }
      // Target content + quota changed.
      await queryClient.invalidateQueries({ queryKey: queryKeys.cloud.allFiles(target.id) });
      await queryClient.invalidateQueries({ queryKey: queryKeys.cloud.about(target.id) });
    } finally {
      setTransferring(false);
      if (aliveRef.current) setRunning(false);
    }
  }

  const itemStatus = (status: TransferItemStatus) => t(`transfer.status.${status}`);

  return (
    <>
      <Dialog open={open} onOpenChange={(o) => !transferring && onOpenChange(o)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("transfer.title")}</DialogTitle>
          </DialogHeader>

          <div className="space-y-3">
            {/* Read-only source summary */}
            <div className="rounded-md border p-2 text-sm">
              <p className="truncate text-xs text-muted-foreground">
                {sourceAccount.email} · {pathDisplayName(sourcePath) || "/"}
              </p>
              <p className="mt-1 truncate font-medium">
                {sources.length === 1 ? sources[0]?.name : t("files.selected_count", { count: sources.length })}
              </p>
            </div>

            <div className="space-y-1.5">
              <p className="text-sm font-medium">{t("transfer.target_account")}</p>
              <Select
                value={targetId}
                onValueChange={(v) => {
                  setTargetId(v);
                  setTargetPath("/");
                }}
                disabled={transferring}
              >
                <SelectTrigger className="w-full" dir="ltr">
                  <SelectValue placeholder={t("transfer.target_account")} />
                </SelectTrigger>
                <SelectContent>
                  {targets.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      <span className="flex items-center gap-2">
                        <ArrowRightLeft className="h-3.5 w-3.5 text-muted-foreground" />
                        {a.email}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <p className="text-sm font-medium">{t("transfer.target_folder")}</p>
              <div className="flex items-center gap-2">
                <span className="min-w-0 flex-1 truncate rounded-md border px-2 py-1.5 text-sm text-muted-foreground" dir="ltr">
                  {targetPath}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="min-h-11 shrink-0 sm:min-h-8"
                  disabled={!target || transferring}
                  onClick={() => setPickerOpen(true)}
                >
                  <FolderInput className="mr-1.5 h-4 w-4" />
                  {t("transfer.choose_folder")}
                </Button>
              </div>
            </div>

            <div className="space-y-1.5">
              <p className="text-sm font-medium">{t("transfer.mode.label")}</p>
              <div className="flex gap-2">
                {(["copy", "move"] as const).map((m) => (
                  <button
                    key={m}
                    type="button"
                    disabled={transferring}
                    onClick={() => setMode(m)}
                    className={cn(
                      "min-h-11 flex-1 rounded-md border px-3 py-1.5 text-sm transition-colors sm:min-h-9",
                      mode === m ? "border-primary/60 bg-primary/5 font-medium" : "hover:bg-muted/40",
                    )}
                  >
                    {t(`transfer.mode.${m}`)}
                  </button>
                ))}
              </div>
              {mode === "move" && hasFolder && (
                <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
                  <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
                  {t("transfer.move_folder_hint")}
                </p>
              )}
            </div>

            {items.length > 0 && (
              <ul className="max-h-40 space-y-1.5 overflow-y-auto rounded-md border p-2 overscroll-contain">
                {items.map((it) => (
                  <li key={it.name} className="flex items-center gap-2 text-xs">
                    {statusIcon(it)}
                    <span className="min-w-0 flex-1 truncate">{it.name}</span>
                    <span
                      className={cn(
                        "shrink-0",
                        it.status === "error" && "text-destructive",
                        it.status === "done" && "text-emerald-600 dark:text-emerald-400",
                      )}
                      title={it.error}
                    >
                      {it.status === "pending" ? "…" : itemStatus(it.status)}
                    </span>
                    {it.status === "error" && <X className="h-3 w-3 text-destructive" />}
                  </li>
                ))}
              </ul>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {transferring ? t("transfer.hide") : tc("cancel")}
            </Button>
            <Button
              disabled={!target || transferring || sources.length === 0}
              onClick={() => void handleRun()}
            >
              {transferring && <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />}
              {running ? t("transfer.running") : t("transfer.start")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {target && pickerOpen && (
        <FolderPickerDialog
          open
          onOpenChange={setPickerOpen}
          accountId={target.id}
          title={t("transfer.target_folder")}
          confirmLabel={t("transfer.use_folder")}
          initialPath={targetPath}
          onPick={(p) => {
            setTargetPath(p);
            setPickerOpen(false);
          }}
        />
      )}
    </>
  );
}
