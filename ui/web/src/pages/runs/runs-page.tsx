import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { RefreshCw, ChevronLeft, ChevronRight, Inbox } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { formatDate, formatDuration, computeDurationMs } from "@/lib/format";
import { useUiStore } from "@/stores/use-ui-store";
import { useRuns, RUN_STATUS_FILTERS } from "./hooks/use-runs";
import { RunStatusBadge } from "./run-status-badge";
import type { RunRecord } from "./types";

export function RunsPage() {
  const { t } = useTranslation("runs");
  const { t: tc } = useTranslation("common");
  const navigate = useNavigate();
  const tz = useUiStore((s) => s.timezone);

  const [status, setStatus] = useState("");
  const [page, setPage] = useState(0);
  const pageSize = 25;

  const { runs, hasMore, loading, fetching, refetch } = useRuns({ status, page, pageSize });

  const onRefresh = useCallback(() => void refetch(), [refetch]);

  const statusLabel = useCallback(
    (s: string) => t(`status.${s}`, { defaultValue: s }),
    [t],
  );

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={fetching}>
            <RefreshCw className={"h-3.5 w-3.5" + (fetching ? " animate-spin" : "")} /> {tc("refresh")}
          </Button>
        }
      />

      <div className="flex flex-wrap gap-1.5">
        <Button
          variant={status === "" ? "secondary" : "ghost"}
          size="sm"
          className="h-7 text-xs"
          onClick={() => { setStatus(""); setPage(0); }}
        >
          {t("filters.allStatuses")}
        </Button>
        {RUN_STATUS_FILTERS.map((s) => (
          <Button
            key={s}
            variant={status === s ? "secondary" : "ghost"}
            size="sm"
            className="h-7 text-xs"
            onClick={() => { setStatus(s); setPage(0); }}
          >
            {statusLabel(s)}
          </Button>
        ))}
      </div>

      {loading ? (
        <TableSkeleton rows={8} />
      ) : runs.length === 0 ? (
        <EmptyState
          icon={Inbox}
          title={status ? t("empty.noRunsFiltered") : t("empty.noRuns")}
          description={status ? undefined : t("empty.noRunsHint")}
        />
      ) : (
        <div className="rounded-lg border overflow-x-auto">
          <table className="w-full min-w-[720px] text-sm">
            <thead>
              <tr className="border-b bg-muted/40 text-left text-muted-foreground">
                <th className="px-4 py-2.5 font-medium">{t("table.runId")}</th>
                <th className="px-4 py-2.5 font-medium">{t("table.status")}</th>
                <th className="px-4 py-2.5 font-medium">{t("table.session")}</th>
                <th className="px-4 py-2.5 font-medium">{t("table.channel")}</th>
                <th className="px-4 py-2.5 font-medium">{t("table.agent")}</th>
                <th className="px-4 py-2.5 font-medium text-right">{t("table.started")}</th>
                <th className="px-4 py-2.5 font-medium text-right">{t("table.duration")}</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <RunRow key={run.run_id} run={run} onOpen={() => navigate(`/runs/${encodeURIComponent(run.run_id)}`)} tz={tz} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {!loading && (page > 0 || hasMore) && (
        <div className="flex items-center justify-end gap-2">
          <Button variant="outline" size="sm" disabled={page === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-xs text-muted-foreground">{page + 1}</span>
          <Button variant="outline" size="sm" disabled={!hasMore} onClick={() => setPage((p) => p + 1)}>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}
    </div>
  );
}

function RunRow({ run, onOpen, tz }: { run: RunRecord; onOpen: () => void; tz?: string }) {
  const { t } = useTranslation("runs");
  return (
    <tr
      className="border-b last:border-0 cursor-pointer hover:bg-muted/40 transition-colors"
      onClick={onOpen}
    >
      <td className="px-4 py-2.5 font-mono text-xs">{run.run_id.slice(0, 8)}</td>
      <td className="px-4 py-2.5">
        <RunStatusBadge status={run.status} label={t(`status.${run.status}`, { defaultValue: run.status })} />
      </td>
      <td className="px-4 py-2.5 max-w-[220px] truncate text-xs">{run.session_key}</td>
      <td className="px-4 py-2.5 text-muted-foreground text-xs">{run.channel || "—"}</td>
      <td className="px-4 py-2.5 font-mono text-xs text-muted-foreground">{run.agent_id?.slice(0, 8) || "—"}</td>
      <td className="px-4 py-2.5 text-right whitespace-nowrap text-muted-foreground">
        <div>{formatDate(run.started_at, tz)}</div>
        <div className="text-xs">
          {t("table.attempt")} {run.attempt}
        </div>
      </td>
      <td className="px-4 py-2.5 text-right whitespace-nowrap">
        {formatDuration(computeDurationMs(run.started_at, run.completed_at ?? undefined))}
      </td>
    </tr>
  );
}
