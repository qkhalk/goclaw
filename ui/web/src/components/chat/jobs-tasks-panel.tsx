import { useTranslation } from "react-i18next";
import { RefreshCw, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/format";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import type { AgentJob, TaskNode } from "@/types/workspace";
import { useJobs } from "@/pages/chat/hooks/use-jobs";
import { useTaskTree, type TaskTreeNode } from "@/pages/chat/hooks/use-task-tree";

interface JobsTasksPanelProps {
  workspaceId?: string | null;
  open: boolean;
  onClose: () => void;
}

/** Local status → badge class map (queued gray / running blue / waiting amber / done green / bad red). */
const JOB_STATUS_CLASS: Record<AgentJob["status"], string> = {
  queued: "bg-muted text-muted-foreground",
  starting: "bg-blue-500/15 text-blue-600 dark:text-blue-400",
  running: "bg-blue-500/15 text-blue-600 dark:text-blue-400",
  waiting_input: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  waiting_approval: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  paused: "bg-muted text-muted-foreground",
  completed: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  failed: "bg-red-500/15 text-red-600 dark:text-red-400",
  cancelled: "bg-red-500/15 text-red-600 dark:text-red-400",
};

const TERMINAL_JOB_STATUSES = new Set<AgentJob["status"]>([
  "completed",
  "failed",
  "cancelled",
]);

export type TaskStatusTone = "done" | "active" | "blocked" | "failed" | "pending";

function taskStatusTone(status: string): TaskStatusTone {
  const s = status.toLowerCase();
  if (s === "done" || s === "completed") return "done";
  if (s === "in_progress" || s === "running" || s === "active") return "active";
  if (s === "blocked" || s === "waiting_input" || s === "waiting_approval") return "blocked";
  if (s === "failed" || s === "cancelled") return "failed";
  return "pending";
}

const TASK_DOT_CLASS: Record<TaskStatusTone, string> = {
  pending: "bg-muted-foreground/40",
  active: "bg-blue-500",
  blocked: "bg-amber-500",
  done: "bg-emerald-500",
  failed: "bg-red-500",
};

export function JobsTasksPanel({ workspaceId, open, onClose }: JobsTasksPanelProps) {
  const { t } = useTranslation("chat");
  const { jobs, loading: jobsLoading, refresh: refreshJobs, cancel } = useJobs(workspaceId);
  const { tasks, loading: tasksLoading, refresh: refreshTasks } = useTaskTree(workspaceId);

  if (!open) return null;

  return (
    <div className={cn(
      "flex h-full w-72 shrink-0 flex-col border-l bg-background",
      "max-sm:fixed max-sm:inset-y-0 max-sm:right-0 max-sm:z-50 max-sm:w-full max-sm:max-w-[85vw] max-sm:shadow-xl",
    )}>
      {/* Header */}
      <div className="flex items-center justify-between border-b px-3 py-2">
        <span className="text-sm font-medium">{t("jobsPanel.title")}</span>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {!workspaceId ? (
        <div className="flex flex-1 items-center justify-center p-4">
          <p className="text-center text-xs text-muted-foreground">{t("jobsPanel.noWorkspace")}</p>
        </div>
      ) : (
        <Tabs defaultValue="jobs" className="min-h-0 flex-1 gap-0">
          <div className="flex items-center gap-1 border-b px-2 pt-2">
            <TabsList>
              <TabsTrigger value="jobs">
                {t("jobsPanel.tabJobs")}
                <span className="text-2xs tabular-nums text-muted-foreground">({jobs.length})</span>
              </TabsTrigger>
              <TabsTrigger value="tasks">
                {t("jobsPanel.tabTasks")}
                <span className="text-2xs tabular-nums text-muted-foreground">({tasks.length})</span>
              </TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="jobs" className="min-h-0 data-[state=inactive]:hidden">
            <JobsTab
              jobs={jobs}
              loading={jobsLoading}
              onRefresh={refreshJobs}
              onCancel={cancel}
            />
          </TabsContent>

          <TabsContent value="tasks" className="min-h-0 data-[state=inactive]:hidden">
            <TasksTab
              tasks={tasks}
              loading={tasksLoading}
              onRefresh={refreshTasks}
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

interface JobsTabProps {
  jobs: AgentJob[];
  loading: boolean;
  onRefresh: () => void;
  onCancel: (jobId: string) => void;
}

function JobsTab({ jobs, loading, onRefresh, onCancel }: JobsTabProps) {
  const { t } = useTranslation("chat");

  return (
    <>
      <div className="flex justify-end px-3 pt-2">
        <Button
          variant="ghost"
          size="icon"
          disabled={loading}
          onClick={onRefresh}
          title={t("jobsPanel.refresh")}
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </Button>
      </div>

      <div className="overscroll-contain flex-1 space-y-2 overflow-y-auto p-2">
        {jobs.length === 0 ? (
          <p className="px-2 py-4 text-center text-xs text-muted-foreground">
            {loading ? t("jobsPanel.loading") : t("jobsPanel.empty")}
          </p>
        ) : (
          jobs.map((job) => (
            <JobRow key={job.id} job={job} onCancel={onCancel} />
          ))
        )}
      </div>
    </>
  );
}

function JobRow({ job, onCancel }: { job: AgentJob; onCancel: (jobId: string) => void }) {
  const { t } = useTranslation("chat");
  const terminal = TERMINAL_JOB_STATUSES.has(job.status);

  return (
    <div className="rounded-lg border bg-card p-2.5 text-xs shadow-sm">
      <div className="flex items-start gap-1.5">
        <p className="min-w-0 flex-1 font-medium leading-snug">{job.title || job.kind}</p>
        <span
          className={cn(
            "shrink-0 rounded-full px-1.5 py-0.5 text-2xs font-medium whitespace-nowrap",
            JOB_STATUS_CLASS[job.status],
          )}
        >
          {t(`jobsPanel.status.${job.status}`)}
        </span>
      </div>
      <div className="mt-1.5 flex items-center gap-1.5 text-muted-foreground">
        <span>{formatRelativeTime(job.updatedAt)}</span>
        {!terminal && (
          <Button
            variant="outline"
            size="sm"
            className="ml-auto h-6 px-2 text-xs"
            onClick={() => onCancel(job.id)}
          >
            {t("jobsPanel.cancel")}
          </Button>
        )}
      </div>
      {job.error && (
        <p className="mt-1 line-clamp-2 leading-snug text-destructive">{job.error}</p>
      )}
    </div>
  );
}

interface TasksTabProps {
  tasks: TaskTreeNode[];
  loading: boolean;
  onRefresh: () => void;
}

function TasksTab({ tasks, loading, onRefresh }: TasksTabProps) {
  const { t } = useTranslation("chat");

  return (
    <>
      <div className="flex justify-end px-3 pt-2">
        <Button
          variant="ghost"
          size="icon"
          disabled={loading}
          onClick={onRefresh}
          title={t("taskTree.refresh")}
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </Button>
      </div>

      <div className="overscroll-contain flex-1 space-y-1 overflow-y-auto p-2">
        {tasks.length === 0 ? (
          <p className="px-2 py-4 text-center text-xs text-muted-foreground">
            {loading ? t("taskTree.loading") : t("taskTree.empty")}
          </p>
        ) : (
          tasks.map((node) => <TaskRow key={node.id} node={node} depth={0} />)
        )}
      </div>
    </>
  );
}

function TaskRow({ node, depth }: { node: TaskTreeNode; depth: number }) {
  const tone = taskStatusTone(node.status);

  return (
    <div>
      <div
        className="flex items-center gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-muted"
        style={{ paddingLeft: `${depth * 12 + 8}px` }}
      >
        <span
          className={cn("h-2 w-2 shrink-0 rounded-full", TASK_DOT_CLASS[tone])}
          aria-hidden
        />
        <span className="min-w-0 flex-1 truncate" title={node.title}>
          {node.title}
        </span>
        {node.priority > 0 && (
          <span className="shrink-0 rounded bg-muted px-1 py-0.5 text-2xs tabular-nums text-muted-foreground">
            P{node.priority}
          </span>
        )}
      </div>
      {node.children.map((child) => (
        <TaskRow key={child.id} node={child} depth={depth + 1} />
      ))}
    </div>
  );
}
