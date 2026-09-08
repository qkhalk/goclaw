import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useParams, Link } from "react-router";
import {
  ArrowLeft, Bot, Brain, Wrench, Zap, Bookmark, Play, Pause,
  CheckCircle2, XCircle, Ban, ChevronDown, ChevronRight,
  MessageSquare, Loader2, ExternalLink,
} from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { formatDate, formatDuration, formatTokens, computeDurationMs } from "@/lib/format";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/stores/use-ui-store";
import { useRun, useRunEvents } from "./hooks/use-runs";
import { RunStatusBadge } from "./run-status-badge";
import { parseItemContent, type LlmCompletedContent, type CheckpointContent, type RunTimelineItem } from "./types";

// Keyed wrapper: useRunEvents accumulates pages in per-instance refs, so the
// replay body must remount whenever the route param changes to another run.
export function RunReplayPage() {
  const { runId = "" } = useParams<{ runId: string }>();
  return <RunReplayInner key={runId} runId={runId} />;
}

function RunReplayInner({ runId }: { runId: string }) {
  const { t } = useTranslation("runs");
  const tz = useUiStore((s) => s.timezone);
  const { run, loading: runLoading } = useRun(runId);
  const { items, loading: eventsLoading, hasMore, loadMore } = useRunEvents(runId);

  const summary = useMemo(() => {
    if (!run) return null;
    return {
      duration: formatDuration(computeDurationMs(run.started_at, run.completed_at ?? undefined)),
    };
  }, [run]);

  if (runLoading || eventsLoading) {
    return (
      <div className="flex flex-col gap-4">
        <PageHeader title={runId.slice(0, 8) || "…"} />
        <TableSkeleton rows={6} />
      </div>
    );
  }

  if (!run) {
    return (
      <div className="flex flex-col gap-4">
        <PageHeader title={t("detail.notFound")} />
        <EmptyState icon={Ban} title={t("detail.notFound")} />
        <div>
          <Button variant="outline" size="sm" asChild>
            <Link to="/runs"><ArrowLeft className="h-3.5 w-3.5" /> {t("detail.backToList")}</Link>
          </Button>
        </div>
      </div>
    );
  }

  const fields: Array<{ label: string; value: string; mono?: boolean }> = [
    { label: t("detail.runId"), value: run.run_id, mono: true },
    { label: t("detail.sessionKey"), value: run.session_key || "—" },
    { label: t("detail.agent"), value: run.agent_id?.slice(0, 8) ?? "—" },
    { label: t("detail.user"), value: run.user_id || "—" },
    { label: t("detail.channel"), value: run.channel || "—" },
    { label: t("detail.chatId"), value: run.chat_id || "—" },
    { label: t("detail.startedAt"), value: formatDate(run.started_at, tz) },
    { label: t("detail.completedAt"), value: run.completed_at ? formatDate(run.completed_at, tz) : "—" },
    { label: t("detail.heartbeatAt"), value: formatDate(run.heartbeat_at, tz) },
    { label: t("detail.duration"), value: summary?.duration ?? "—" },
    { label: t("detail.attempt"), value: String(run.attempt) },
  ];

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={run.run_id.slice(0, 8)}
        actions={
          <div className="flex gap-2">
            <Button variant="outline" size="sm" asChild>
              <Link to="/runs"><ArrowLeft className="h-3.5 w-3.5" /> {t("detail.backToList")}</Link>
            </Button>
            {run.session_key && (
              <Button variant="outline" size="sm" asChild>
                <Link to={`/sessions/${encodeURIComponent(run.session_key)}`}>
                  <ExternalLink className="h-3.5 w-3.5" /> {t("detail.openSession")}
                </Link>
              </Button>
            )}
          </div>
        }
      />
      <div className="-mt-3">
        <RunStatusBadge status={run.status} label={t(`status.${run.status}`, { defaultValue: run.status })} />
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-2">
        {fields.map((f) => (
          <div key={f.label} className="rounded-lg border px-3 py-2 min-w-0">
            <div className="text-xs text-muted-foreground">{f.label}</div>
            <div className={cn("text-sm truncate", f.mono && "font-mono text-xs")} title={f.value}>{f.value}</div>
          </div>
        ))}
      </div>

      {run.error && (
        <div className="rounded-lg border border-red-500/40 bg-red-500/5 px-4 py-3 text-sm text-red-600 dark:text-red-400 break-words">
          <span className="font-medium">{t("detail.error")}: </span>{run.error}
        </div>
      )}

      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-muted-foreground">
          {t("detail.timeline")} · {t("detail.events", { count: items.length })}
        </h2>
      </div>

      {items.length === 0 ? (
        <EmptyState icon={MessageSquare} title={t("empty.noEvents")} />
      ) : (
        <ol className="relative flex flex-col gap-1 border-l ml-3">
          {items.map((item, idx) => (
            <TimelineRow key={item.id} item={item} prevTime={idx > 0 ? items[idx - 1]?.created_at : undefined} />
          ))}
        </ol>
      )}

      {hasMore && (
        <div className="flex justify-center">
          <Button variant="outline" size="sm" onClick={loadMore}>
            <ChevronDown className="h-3.5 w-3.5" /> {t("detail.loadMore")}
          </Button>
        </div>
      )}
    </div>
  );
}

function deltaChip(deltaMs: number | null): string | null {
  if (deltaMs === null || deltaMs < 1000) return null;
  return `+${formatDuration(deltaMs)}`;
}

function TimelineRow({ item, prevTime }: { item: RunTimelineItem; prevTime?: string }) {
  const tz = useUiStore((s) => s.timezone);
  const delta = deltaChip(computeDurationMs(prevTime ?? undefined, item.created_at));
  return (
    <li className="pl-4 py-1.5 relative">
      <span className="absolute -left-[5px] top-4 h-2 w-2 rounded-full bg-border" aria-hidden />
      <div className="flex items-start gap-2">
        <TimelineItemBody item={item} />
        {delta && <span className="ml-auto shrink-0 text-2xs text-muted-foreground whitespace-nowrap pt-0.5">{delta}</span>}
        <span className="shrink-0 text-2xs text-muted-foreground whitespace-nowrap pt-0.5">
          {formatDate(item.created_at, tz)}
        </span>
      </div>
    </li>
  );
}

function TimelineItemBody({ item }: { item: RunTimelineItem }) {
  switch (item.item_type) {
    case "run.status":
      return <RunStatusItem item={item} />;
    case "checkpoint":
      return <CheckpointItem item={item} />;
    case "tool.started":
    case "tool.call":
      return <ToolItem item={item} pending />;
    case "tool.result":
      return <ToolItem item={item} pending={false} />;
    case "thinking":
      return <TextItem icon={Brain} muted title={item.title} body={item.content || item.preview} />;
    case "chunk":
    case "assistant.message":
      return <TextItem icon={Bot} title={item.title} body={item.content || item.preview} />;
    case "activity":
    default:
      return <ActivityItem item={item} />;
  }
}

function RunStatusItem({ item }: { item: RunTimelineItem }) {
  const { t } = useTranslation("runs");
  const status = item.status || item.preview || "";
  const Icon =
    status === "completed" ? CheckCircle2
    : status === "failed" ? XCircle
    : status === "cancelled" ? Ban
    : status === "paused" || status === "compacting" ? Pause
    : Play;
  const color =
    status === "completed" ? "text-emerald-500"
    : status === "failed" ? "text-red-500"
    : status === "cancelled" ? "text-muted-foreground"
    : "text-blue-500";
  return (
    <div className="flex items-center gap-2 min-w-0">
      <Icon className={cn("h-3.5 w-3.5 shrink-0", color)} />
      <span className="text-xs font-medium">{t(`timelineStatus.${status}`, { defaultValue: status || item.title })}</span>
    </div>
  );
}

function CheckpointItem({ item }: { item: RunTimelineItem }) {
  const { t } = useTranslation("runs");
  const cp = parseItemContent<CheckpointContent>(item);
  return (
    <div className="flex items-center gap-2 min-w-0 flex-wrap">
      <Bookmark className="h-3.5 w-3.5 shrink-0 text-violet-500" />
      <span className="text-xs font-medium">{t("itemType.checkpoint")}</span>
      {cp?.iteration != null && (
        <Badge variant="secondary" className="text-2xs px-1.5 py-0">
          {t("detail.iteration", { count: Number(cp.iteration) })}
        </Badge>
      )}
      {(cp?.status || item.status) && (
        <Badge variant="outline" className="text-2xs px-1.5 py-0">{cp?.status ?? item.status}</Badge>
      )}
    </div>
  );
}

function ToolItem({ item, pending }: { item: RunTimelineItem; pending: boolean }) {
  const { t } = useTranslation("runs");
  const failed = item.status === "failed";
  return (
    <div className="flex items-center gap-2 min-w-0">
      {pending ? (
        <Loader2 className="h-3.5 w-3.5 shrink-0 text-blue-500 animate-spin" />
      ) : failed ? (
        <XCircle className="h-3.5 w-3.5 shrink-0 text-red-500" />
      ) : (
        <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-emerald-500" />
      )}
      <Wrench className="h-3 w-3 shrink-0 text-muted-foreground" />
      <span className="text-xs font-medium truncate">{item.tool_name || item.title}</span>
      {item.status && !pending && (
        <Badge variant={failed ? "destructive" : "outline"} className="text-2xs px-1.5 py-0 shrink-0">
          {t(`timelineStatus.${item.status}`, { defaultValue: item.status })}
        </Badge>
      )}
      {(() => {
        const body = item.content ?? item.preview;
        return !pending && body ? <CollapsibleText text={body} /> : null;
      })()}
    </div>
  );
}

function TextItem({
  icon: Icon, title, body, muted,
}: { icon: typeof Bot; title?: string; body?: string; muted?: boolean }) {
  const [open, setOpen] = useState(!muted);
  const text = body ?? title ?? "";
  if (!text) return null;
  return (
    <div className="min-w-0">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-2 text-left min-w-0 w-full"
      >
        <Icon className={cn("h-3.5 w-3.5 shrink-0", muted ? "text-amber-500" : "text-blue-500")} />
        <span className={cn("text-xs font-medium truncate", muted && "italic text-muted-foreground")}>
          {title ?? text.slice(0, 120)}
        </span>
        {open ? <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />}
      </button>
      {open && body && (
        <div className={cn(
          "mt-1 whitespace-pre-wrap break-words rounded-md bg-muted/40 px-3 py-2 text-xs max-h-60 overflow-y-auto overscroll-contain",
          muted && "italic text-muted-foreground",
        )}>
          {body}
        </div>
      )}
    </div>
  );
}

function ActivityItem({ item }: { item: RunTimelineItem }) {
  const { t } = useTranslation("runs");
  const llm = parseItemContent<LlmCompletedContent>(item);
  return (
    <div className="flex items-center gap-2 min-w-0 flex-wrap">
      <Zap className="h-3.5 w-3.5 shrink-0 text-amber-500" />
      <span className="text-xs truncate">{item.title || t("itemType.activity")}</span>
      {llm?.duration_ms != null && Number(llm.duration_ms) > 0 && (
        <Badge variant="outline" className="text-2xs px-1.5 py-0">
          {t("detail.llmDuration", { ms: formatDuration(Number(llm.duration_ms)) })}
        </Badge>
      )}
      {(llm?.prompt_tokens != null || llm?.completion_tokens != null) && (
        <Badge variant="secondary" className="text-2xs px-1.5 py-0">
          {formatTokens((Number(llm?.prompt_tokens) || 0) + (Number(llm?.completion_tokens) || 0))}
        </Badge>
      )}
      {item.preview && !llm && <span className="text-2xs text-muted-foreground truncate">{item.preview}</span>}
    </div>
  );
}

function CollapsibleText({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="min-w-0 flex-1">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="text-2xs text-muted-foreground hover:text-foreground shrink-0"
      >
        {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
      </button>
      {open && (
        <div className="mt-1 whitespace-pre-wrap break-words rounded-md bg-muted/40 px-3 py-2 text-xs max-h-60 overflow-y-auto overscroll-contain">
          {text}
        </div>
      )}
    </div>
  );
}
