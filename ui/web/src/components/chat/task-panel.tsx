import { X, MessageSquare, Paperclip } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ActiveTeamTask } from "@/types/chat";

interface TaskPanelProps {
  tasks: ActiveTeamTask[];
  open: boolean;
  onClose: () => void;
}

export function TaskPanel({ tasks, open, onClose }: TaskPanelProps) {
  if (!open) return null;

  return (
    <div className={cn(
      "flex h-full w-72 shrink-0 flex-col border-l bg-background",
      "max-sm:fixed max-sm:inset-y-0 max-sm:right-0 max-sm:z-50 max-sm:w-full max-sm:max-w-[85vw] max-sm:shadow-xl",
    )}>
      {/* Header */}
      <div className="flex items-center justify-between border-b px-3 py-2">
        <span className="text-sm font-medium">
          Tasks ({tasks.length})
        </span>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Task list */}
      <div className="flex-1 overflow-y-auto overscroll-contain p-2 space-y-1.5">
        {tasks.length === 0 ? (
          <p className="px-2 py-4 text-center text-xs text-muted-foreground">No active tasks</p>
        ) : (
          tasks.map((task) => <TaskCard key={task.taskId} task={task} />)
        )}
      </div>
    </div>
  );
}

function TaskCard({ task }: { task: ActiveTeamTask }) {
  return (
    <div className="rounded-md border bg-card p-2 text-xs">
      <div className="flex items-start gap-1.5">
        <span className="shrink-0 font-mono text-muted-foreground">#{task.taskNumber}</span>
        <span className="font-medium leading-tight line-clamp-2">{task.subject}</span>
      </div>

      <div className="mt-1.5 flex items-center gap-2 text-muted-foreground">
        <span className="truncate">{task.ownerDisplayName || task.ownerAgentKey || "unassigned"}</span>

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

      {/* Progress bar */}
      {task.progressPercent != null && (
        <div className="mt-1.5">
          <div className="flex items-center justify-between text-[10px] text-muted-foreground">
            <span className="truncate">{task.progressStep || "In progress"}</span>
            <span className="shrink-0 ml-1">{task.progressPercent}%</span>
          </div>
          <div className="mt-0.5 h-1 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-primary transition-all duration-300"
              style={{ width: `${Math.min(task.progressPercent, 100)}%` }}
            />
          </div>
        </div>
      )}
    </div>
  );
}
