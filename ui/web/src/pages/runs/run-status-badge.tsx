import { Loader2, CircleDashed, CheckCircle2, XCircle, Ban, Archive } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { RunStatus } from "./types";

const STATUS_ICONS: Record<string, typeof Loader2> = {
  pending: CircleDashed,
  running: Loader2,
  compacting: Archive,
  completed: CheckCircle2,
  failed: XCircle,
  cancelled: Ban,
};

const STATUS_CLASSES: Record<string, string> = {
  pending: "text-muted-foreground",
  running: "text-blue-500",
  compacting: "text-amber-500",
  completed: "text-emerald-500",
  failed: "text-red-500",
  cancelled: "text-muted-foreground",
};

/** Status badge for a durable agent run (agent_runs.status values). */
export function RunStatusBadge({ status, label }: { status: string; label?: string }) {
  const Icon = STATUS_ICONS[status] ?? CircleDashed;
  const spinning = status === "running";
  return (
    <Badge variant="outline" className={cn("shrink-0 gap-1 text-2xs px-1.5 py-0", STATUS_CLASSES[status])}>
      <Icon className={cn("h-3 w-3", spinning && "animate-spin")} />
      {label ?? status}
    </Badge>
  );
}

export type { RunStatus };
