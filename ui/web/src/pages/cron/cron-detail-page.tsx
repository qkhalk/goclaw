import { useState, useEffect, useCallback } from "react";
import { ArrowLeft, Play, Trash2, Power, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Pagination } from "@/components/shared/pagination";
import { formatDate, formatTokens } from "@/lib/format";
import type { CronJob, CronRunLogEntry } from "./hooks/use-cron";

function formatScheduleDetail(job: CronJob): string {
  const s = job.schedule;
  if (s.kind === "every" && s.everyMs) {
    const sec = s.everyMs / 1000;
    if (sec < 60) return `Every ${sec} seconds`;
    if (sec < 3600) return `Every ${Math.round(sec / 60)} minutes`;
    return `Every ${(sec / 3600).toFixed(1)} hours`;
  }
  if (s.kind === "cron" && s.expr) return s.expr;
  if (s.kind === "at" && s.atMs) return `Once at ${new Date(s.atMs).toLocaleString()}`;
  return s.kind;
}

function formatDuration(ms?: number): string {
  if (!ms) return "-";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}

interface CronDetailPageProps {
  job: CronJob;
  onBack: () => void;
  onRun: (id: string) => Promise<void>;
  onToggle: (id: string, enabled: boolean) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
  getRunLog: (id: string, limit?: number, offset?: number) => Promise<{ entries: CronRunLogEntry[]; total: number }>;
  onRefresh: () => void;
}

export function CronDetailPage({ job, onBack, onRun, onToggle, onDelete, getRunLog, onRefresh }: CronDetailPageProps) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmToggle, setConfirmToggle] = useState(false);
  const [runLog, setRunLog] = useState<CronRunLogEntry[]>([]);
  const [runLogTotal, setRunLogTotal] = useState(0);
  const [runLogLoading, setRunLogLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const isRunning = running || job.state?.lastStatus === "running";
  const [runLogPage, setRunLogPage] = useState(1);
  const [runLogPageSize, setRunLogPageSize] = useState(10);
  const loadRunLog = useCallback(async (page?: number, pageSize?: number) => {
    const p = page ?? runLogPage;
    const ps = pageSize ?? runLogPageSize;
    setRunLogLoading(true);
    try {
      const { entries, total } = await getRunLog(job.id, ps, (p - 1) * ps);
      setRunLog(entries);
      setRunLogTotal(total);
    } finally {
      setRunLogLoading(false);
    }
  }, [job.id, getRunLog, runLogPage, runLogPageSize]);

  const runLogTotalPages = Math.ceil(runLogTotal / runLogPageSize);

  useEffect(() => {
    loadRunLog();
  }, [loadRunLog]);

  // Poll cron list while job is running to detect completion
  useEffect(() => {
    if (!isRunning) return;
    const interval = setInterval(onRefresh, 3000);
    return () => clearInterval(interval);
  }, [isRunning, onRefresh]);

  // Clear local running state when backend status changes from running
  useEffect(() => {
    if (running && job.state?.lastStatus && job.state.lastStatus !== "running") {
      setRunning(false);
      loadRunLog(1);
    }
  }, [running, job.state?.lastStatus, loadRunLog]);

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b p-4">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" onClick={onBack}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h3 className="flex items-center gap-2 font-medium">
              {job.name}
              <Badge variant={job.enabled ? "success" : "secondary"}>
                {job.enabled ? "enabled" : "disabled"}
              </Badge>
            </h3>
            <div className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="outline">{job.schedule.kind}</Badge>
              {job.agentId && <Badge variant="secondary">{job.agentId}</Badge>}
              {job.state?.lastStatus && (
                <Badge variant={
                  job.state.lastStatus === "ok" ? "success"
                    : job.state.lastStatus === "running" ? "outline"
                    : "destructive"
                }>
                  {job.state.lastStatus === "running" ? "running..." : job.state.lastStatus}
                </Badge>
              )}
            </div>
          </div>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            className="gap-1"
            disabled={isRunning}
            onClick={async () => {
              setRunning(true);
              await onRun(job.id);
            }}
          >
            {isRunning ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
            {isRunning ? "Running..." : "Run Now"}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="gap-1"
            onClick={() => setConfirmToggle(true)}
          >
            <Power className="h-3.5 w-3.5" /> {job.enabled ? "Disable" : "Enable"}
          </Button>
          <Button variant="destructive" size="sm" className="gap-1" onClick={() => setConfirmDelete(true)}>
            <Trash2 className="h-3.5 w-3.5" /> Delete
          </Button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-3xl space-y-6">
          {/* Job Info */}
          <div className="grid grid-cols-2 gap-4 rounded-md border p-4 text-sm">
            <InfoRow label="Schedule" value={formatScheduleDetail(job)} />
            {job.schedule.tz && <InfoRow label="Timezone" value={job.schedule.tz} />}
            {job.state?.nextRunAtMs && (
              <InfoRow label="Next Run" value={formatDate(new Date(job.state.nextRunAtMs))} />
            )}
            {job.state?.lastRunAtMs && (
              <InfoRow label="Last Run" value={formatDate(new Date(job.state.lastRunAtMs))} />
            )}
            <InfoRow label="Created" value={formatDate(new Date(job.createdAtMs))} />
            <InfoRow label="Updated" value={formatDate(new Date(job.updatedAtMs))} />
            {job.deleteAfterRun && <InfoRow label="Auto-delete" value="Yes (one-time)" />}
          </div>

          {/* Payload */}
          <div className="rounded-md border p-4 text-sm">
            <h4 className="mb-2 font-medium">Payload</h4>
            <div className="space-y-2">
              <div className="rounded bg-muted p-3 font-mono text-xs whitespace-pre-wrap">
                {job.payload?.message || "(empty)"}
              </div>
              <div className="flex gap-4 text-xs text-muted-foreground">
                {job.payload?.deliver && <span>Deliver: direct</span>}
                {job.payload?.channel && <span>Channel: {job.payload.channel}</span>}
                {job.payload?.to && <span>To: {job.payload.to}</span>}
              </div>
            </div>
          </div>

          {/* Last Error */}
          {job.state?.lastError && (
            <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm">
              <h4 className="mb-1 font-medium text-destructive">Last Error</h4>
              <p className="text-xs text-destructive/80">{job.state.lastError}</p>
            </div>
          )}

          {/* Run History */}
          <div>
            <div className="mb-3 flex items-center justify-between">
              <h4 className="font-medium">Run History</h4>
              <Button variant="ghost" size="sm" onClick={() => loadRunLog()} className="text-xs">
                Refresh
              </Button>
            </div>
            {runLogLoading && runLog.length === 0 ? (
              <div className="flex items-center justify-center py-8">
                <div className="h-5 w-5 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
              </div>
            ) : runLog.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">No run history yet.</p>
            ) : (
              <>
                <div className="space-y-2">
                  {runLog.map((entry, i) => (
                    <div key={i} className="rounded-md border p-3 text-sm">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <span className="text-muted-foreground">{formatDate(new Date(entry.ts))}</span>
                          {entry.durationMs != null && entry.durationMs > 0 && (
                            <span className="text-xs text-muted-foreground">({formatDuration(entry.durationMs)})</span>
                          )}
                        </div>
                        <div className="flex items-center gap-2">
                          {(entry.inputTokens != null && entry.inputTokens > 0) && (
                            <span className="text-xs text-muted-foreground">
                              {formatTokens(entry.inputTokens)} in / {formatTokens(entry.outputTokens ?? 0)} out
                            </span>
                          )}
                          <Badge variant={entry.status === "ok" || entry.status === "success" ? "success" : "destructive"}>
                            {entry.status || "unknown"}
                          </Badge>
                        </div>
                      </div>
                      {entry.summary && (
                        <p className="mt-1 line-clamp-3 text-muted-foreground">{entry.summary}</p>
                      )}
                      {entry.error && (
                        <p className="mt-1 text-destructive">{entry.error}</p>
                      )}
                    </div>
                  ))}
                </div>
                <Pagination
                  page={runLogPage}
                  pageSize={runLogPageSize}
                  total={runLogTotal}
                  totalPages={runLogTotalPages}
                  onPageChange={(p) => { setRunLogPage(p); loadRunLog(p); }}
                  onPageSizeChange={(s) => { setRunLogPageSize(s); setRunLogPage(1); loadRunLog(1, s); }}
                  pageSizes={[10, 20, 50]}
                />
              </>
            )}
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title="Delete Cron Job"
        description={`Delete "${job.name}"? This cannot be undone.`}
        confirmLabel="Delete"
        variant="destructive"
        onConfirm={async () => {
          await onDelete(job.id);
          setConfirmDelete(false);
        }}
      />

      <ConfirmDialog
        open={confirmToggle}
        onOpenChange={setConfirmToggle}
        title={job.enabled ? "Disable Cron Job" : "Enable Cron Job"}
        description={
          job.enabled
            ? `Disable "${job.name}"? It will stop running until re-enabled.`
            : `Enable "${job.name}"? It will start running on schedule.`
        }
        confirmLabel={job.enabled ? "Disable" : "Enable"}
        variant={job.enabled ? "destructive" : "default"}
        onConfirm={async () => {
          await onToggle(job.id, !job.enabled);
          setConfirmToggle(false);
        }}
      />
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span className="text-muted-foreground">{label}</span>
      <div className="mt-0.5 font-medium">{value}</div>
    </div>
  );
}
