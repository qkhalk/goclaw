import { useState, useRef, useLayoutEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { ChevronUp, Loader2, MessageSquare, Paperclip, Users } from "lucide-react";
import { usePortalDropdownClose } from "@/hooks/use-portal-dropdown-close";
import type { ActiveTeamTask } from "@/types/chat";

/**
 * Paseo-style tracker pill docked above the composer: the single in-chat
 * surface for team tasks. Replaces the old right TaskPanel column, the inline
 * TeamActivityPanel and the top-bar chip (which all rendered the same data).
 */
export function TeamTasksPill({ tasks }: { tasks: ActiveTeamTask[] }) {
  const { t } = useTranslation("chat");
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});

  // Pop the panel above the pill (the pill sits at the bottom of the screen).
  useLayoutEffect(() => {
    if (!open || !containerRef.current) return;
    const rect = containerRef.current.getBoundingClientRect();
    setDropdownStyle({
      position: "fixed",
      bottom: window.innerHeight - rect.top + 6,
      left: Math.max(rect.left, 8),
      width: Math.min(rect.width + 160, 384),
      zIndex: 9999,
    });
  }, [open]);

  usePortalDropdownClose({
    open,
    onClose: () => setOpen(false),
    ignore: [containerRef, dropdownRef],
  });

  if (tasks.length === 0) return null;

  return (
    <div ref={containerRef} className="relative mx-3 mb-1 flex shrink-0">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 rounded-full border bg-muted/60 px-3 py-1 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground max-sm:min-h-[44px]"
        aria-expanded={open}
        title={t("teamTasks.panelTitle", { n: tasks.length })}
      >
        <Users className="h-3.5 w-3.5 shrink-0" />
        <span>{t("teamTasks.pill", { n: tasks.length })}</span>
        <Loader2 className="h-3 w-3 animate-spin" />
        <ChevronUp className="h-3.5 w-3.5 shrink-0" />
      </button>

      {open && createPortal(
        <div
          ref={dropdownRef}
          style={dropdownStyle}
          className="pointer-events-auto max-h-[60vh] overflow-y-auto overscroll-contain rounded-lg border bg-popover p-2 shadow-md"
        >
          {tasks.length === 0 ? (
            <p className="px-2 py-4 text-center text-xs text-muted-foreground">
              {t("teamTasks.empty")}
            </p>
          ) : (
            <div className="space-y-2">
              {tasks.map((task) => <TaskCard key={task.taskId} task={task} />)}
            </div>
          )}
        </div>,
        document.body,
      )}
    </div>
  );
}

function TaskCard({ task }: { task: ActiveTeamTask }) {
  const { t } = useTranslation("chat");
  const pct = task.progressPercent;

  return (
    <div className="rounded-lg border bg-card p-2.5 text-xs shadow-sm">
      {/* Header: number + owner + counters */}
      <div className="flex items-center gap-1.5 text-muted-foreground">
        <span className="font-mono">#{task.taskNumber}</span>
        <span className="truncate">{task.ownerDisplayName || task.ownerAgentKey || t("teamTasks.unassigned")}</span>
        <span className="ml-auto flex items-center gap-2">
          {(task.commentCount ?? 0) > 0 && (
            <span className="flex items-center gap-0.5">
              <MessageSquare className="h-3 w-3" /> {task.commentCount}
            </span>
          )}
          {(task.attachmentCount ?? 0) > 0 && (
            <span className="flex items-center gap-0.5">
              <Paperclip className="h-3 w-3" /> {task.attachmentCount}
            </span>
          )}
        </span>
      </div>

      {/* Progress bar — above the step message */}
      {pct != null && (
        <div className="mt-1.5">
          <div className="flex items-center gap-1.5">
            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-primary transition-all duration-300"
                style={{ width: `${Math.min(pct, 100)}%` }}
              />
            </div>
            <span className="shrink-0 text-2xs tabular-nums text-muted-foreground">{pct}%</span>
          </div>
        </div>
      )}

      {/* Step message — full text, no truncation */}
      {task.progressStep && (
        <p className="mt-1 text-xs-plus leading-snug text-muted-foreground">
          {task.progressStep}
        </p>
      )}

      {/* Subject — full text */}
      <p className="mt-1.5 font-medium leading-snug">{task.subject}</p>
    </div>
  );
}
