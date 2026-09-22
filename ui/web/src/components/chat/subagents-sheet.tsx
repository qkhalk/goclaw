import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Archive,
  CornerDownLeft,
  Hourglass,
  RefreshCw,
  Square,
  XCircle,
  CheckCircle2,
  Bot,
} from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatRelativeTime } from "@/lib/format";
import { cn } from "@/lib/utils";
import {
  useSubagents,
  SUBAGENT_TERMINAL_STATUSES,
  type SubagentTask,
} from "@/pages/chat/hooks/use-subagents";

/**
 * Status icon + color vocabulary — mirrors the Telegram surface
 * (internal/channels/telegram/commands_subagents.go:19-34) so both surfaces
 * read the same states at a glance. Lucide glyphs stand in for the emojis:
 * ⏳ queued/waiting, 🔄 running, ↪️ waiting_child, ✅ completed,
 * ❌ failed, ⏹ cancelled.
 */
export function subagentStatusIcon(status: string) {
  switch (status) {
    case "queued":
    case "waiting":
      return { icon: Hourglass, className: "text-amber-500" };
    case "waiting_child":
      return { icon: CornerDownLeft, className: "text-amber-500" };
    case "completed":
      return { icon: CheckCircle2, className: "text-emerald-500" };
    case "failed":
      return { icon: XCircle, className: "text-destructive" };
    case "cancelled":
      return { icon: Square, className: "text-muted-foreground" };
    default: // running
      return { icon: RefreshCw, className: "text-blue-500 animate-spin" };
  }
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation("chat");
  const { icon: Icon, className } = subagentStatusIcon(status);
  const labelKey =
    status === "waiting_child"
      ? "subagents.statuses.waitingChild"
      : `subagents.statuses.${status}`;
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-2xs font-medium text-muted-foreground">
      <Icon className={cn("h-3 w-3", className, status === "queued" || status === "waiting" ? "animate-pulse" : undefined)} />
      {t(labelKey, { defaultValue: status })}
    </span>
  );
}

interface SubagentsSheetProps {
  agentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Paseo-style subagent tracker panel: rows with status/model/times/summary
 * plus Cancel (confirm) and Archive actions, and a footer "archive all
 * finished" when ≥1 terminal task exists. Full-screen on mobile via the
 * shared Sheet (max-sm:inset-0).
 */
export function SubagentsSheet({ agentId, open, onOpenChange }: SubagentsSheetProps) {
  const { t } = useTranslation("chat");
  const { t: tc } = useTranslation("common");
  const {
    tasks,
    terminalCount,
    loading,
    error,
    refetch,
    archiveTask,
    cancelTask,
    archiveCompleted,
    pendingArchive,
    pendingCancel,
    pendingArchiveAll,
  } = useSubagents(agentId);
  const [cancelTarget, setCancelTarget] = useState<SubagentTask | null>(null);

  const busy = pendingArchive !== null || pendingCancel !== null || pendingArchiveAll;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="gap-0 p-0 sm:max-w-md">
        <SheetHeader className="px-4 pt-4 pb-3">
          <SheetTitle className="flex items-center gap-2">
            <Bot className="h-4 w-4" />
            {t("subagents.title")}
          </SheetTitle>
          <SheetDescription>{t("subagents.description")}</SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-4">
          {loading ? (
            <div className="space-y-2 py-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="h-16 animate-pulse rounded-lg bg-muted" />
              ))}
            </div>
          ) : error ? (
            <div className="flex flex-col items-center gap-3 py-8 text-center">
              <p className="text-sm text-muted-foreground">{error}</p>
              <Button variant="outline" size="sm" onClick={() => void refetch()}>
                {tc("retry")}
              </Button>
            </div>
          ) : tasks.length === 0 ? (
            <div className="py-8 text-center text-sm leading-relaxed text-muted-foreground">
              <p className="font-medium text-foreground">{t("subagents.emptyTitle")}</p>
              <p className="mt-1">{t("subagents.empty")}</p>
            </div>
          ) : (
            <div className="space-y-2 py-2">
              {tasks.map((task) => (
                <SubagentRow
                  key={task.taskId}
                  task={task}
                  busy={busy}
                  pendingArchive={pendingArchive}
                  pendingCancel={pendingCancel}
                  onArchive={() => void archiveTask(task.taskId).catch(() => { /* toast shown */ })}
                  onCancel={() => setCancelTarget(task)}
                />
              ))}
            </div>
          )}
        </div>

        {terminalCount > 0 && (
          <SheetFooter className="px-4 py-3">
            <Button
              variant="outline"
              className="w-full gap-2 sm:w-auto"
              disabled={busy}
              onClick={() => void archiveCompleted().catch(() => { /* toast shown */ })}
            >
              <Archive className="h-4 w-4" />
              {pendingArchiveAll ? tc("loading") : t("subagents.archive_all", { n: terminalCount })}
            </Button>
          </SheetFooter>
        )}
      </SheetContent>

      {/* Cancel needs explicit confirmation — it stops a live run. */}
      <Dialog open={!!cancelTarget} onOpenChange={(o) => !o && setCancelTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("subagents.cancelConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("subagents.cancelConfirmBody")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCancelTarget(null)}>
              {tc("cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (cancelTarget) {
                  void cancelTask(cancelTarget.taskId).catch(() => { /* toast shown */ });
                  setCancelTarget(null);
                }
              }}
            >
              {t("subagents.cancel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Sheet>
  );
}

interface SubagentRowProps {
  task: SubagentTask;
  busy: boolean;
  pendingArchive: string | null;
  pendingCancel: string | null;
  onArchive: () => void;
  onCancel: () => void;
}

function SubagentRow({ task, busy, pendingArchive, pendingCancel, onArchive, onCancel }: SubagentRowProps) {
  const { t } = useTranslation("chat");
  const { t: tc } = useTranslation("common");
  const isTerminal = SUBAGENT_TERMINAL_STATUSES.has(task.status);

  return (
    <div className="rounded-lg border bg-card p-3 shadow-sm">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="truncate text-sm font-medium">{task.label}</span>
            <StatusBadge status={task.status} />
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-2xs text-muted-foreground">
            {task.model && <span className="truncate font-mono">{task.model}</span>}
            <span>{formatRelativeTime(task.createdAt)}</span>
            {task.completedAt && (
              <>
                <span>→</span>
                <span>{formatRelativeTime(task.completedAt)}</span>
              </>
            )}
          </div>
        </div>
      </div>

      {task.summary && !task.error && (
        <p className="mt-1.5 line-clamp-2 text-xs leading-snug text-muted-foreground">{task.summary}</p>
      )}
      {task.error && (
        <p className="mt-1.5 line-clamp-2 text-xs leading-snug text-destructive">{task.error}</p>
      )}

      <div className="mt-2 flex flex-wrap justify-end gap-2">
        {!isTerminal && (
          <Button
            variant="outline"
            size="sm"
            className="min-h-[44px] sm:min-h-8"
            disabled={busy}
            onClick={onCancel}
          >
            {pendingCancel === task.taskId ? tc("loading") : t("subagents.cancel")}
          </Button>
        )}
        {isTerminal && (
          <Button
            variant="outline"
            size="sm"
            className="min-h-[44px] sm:min-h-8"
            disabled={busy}
            onClick={onArchive}
          >
            <Archive className="h-3.5 w-3.5" />
            {pendingArchive === task.taskId ? tc("loading") : t("subagents.archive")}
          </Button>
        )}
      </div>
    </div>
  );
}
