import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { formatRelativeTime } from "@/lib/format";
import type { TeamEventEntry } from "@/stores/use-team-event-store";
import { useAgentResolver } from "./use-agent-resolver";
import { DelegationEventCard } from "./delegation-event-cards";
import { TaskEventCard } from "./task-event-cards";
import { MessageEventCard } from "./message-event-card";
import { AgentEventCard } from "./agent-event-cards";
import { TeamCrudEventCard } from "./team-crud-event-cards";
import { EventDetailDialog } from "./event-detail-dialog";

interface EventCardProps {
  entry: TeamEventEntry;
}

export function EventCard({ entry }: EventCardProps) {
  const { event } = entry;
  const { resolveAgent } = useAgentResolver();
  const [showDetail, setShowDetail] = useState(false);

  let content: React.ReactNode;
  if (event.startsWith("delegation.")) {
    content = <DelegationEventCard entry={entry} resolveAgent={resolveAgent} />;
  } else if (event.startsWith("team.task.")) {
    content = <TaskEventCard entry={entry} resolveAgent={resolveAgent} />;
  } else if (event === "team.message.sent") {
    content = <MessageEventCard entry={entry} resolveAgent={resolveAgent} />;
  } else if (event === "agent") {
    content = <AgentEventCard entry={entry} resolveAgent={resolveAgent} />;
  } else if (
    event.startsWith("team.created") ||
    event.startsWith("team.updated") ||
    event.startsWith("team.deleted") ||
    event.startsWith("team.member.") ||
    event.startsWith("agent_link.")
  ) {
    content = <TeamCrudEventCard entry={entry} resolveAgent={resolveAgent} />;
  } else {
    content = (
      <pre className="overflow-x-auto text-xs text-muted-foreground">
        {JSON.stringify(entry.payload, null, 2)}
      </pre>
    );
  }

  return (
    <>
      <button
        type="button"
        onClick={() => setShowDetail(true)}
        className="w-full cursor-pointer overflow-hidden rounded-lg border bg-card px-3 py-2 text-left transition-colors hover:border-primary/20"
      >
        <div className="flex min-w-0 items-start gap-2">
          <EventTypeBadge event={event} payload={entry.payload} />
          <div className="min-w-0 flex-1">{content}</div>
          <span className="mt-0.5 shrink-0 text-xs text-muted-foreground">
            {formatRelativeTime(new Date(entry.timestamp))}
          </span>
        </div>
      </button>

      {showDetail && (
        <EventDetailDialog entry={entry} onClose={() => setShowDetail(false)} />
      )}
    </>
  );
}

function EventTypeBadge({ event, payload }: { event: string; payload: unknown }) {
  // For agent events, show the subtype (run.started, tool.call) instead of generic "agent"
  let label = event;
  if (event === "agent") {
    const p = payload as { type?: string };
    if (p?.type) label = p.type;
  }

  const variant =
    label.includes("failed") || label.includes("cancelled") || label.includes("deleted")
      ? "destructive"
      : label.includes("completed") || label.includes("created") || label.includes("added")
        ? "success"
        : label.includes("started") || label.includes("progress") || label.includes("claimed")
          ? "info"
          : "secondary";

  return (
    <Badge variant={variant} className="mt-0.5 shrink-0 font-mono text-xs">
      {label}
    </Badge>
  );
}
