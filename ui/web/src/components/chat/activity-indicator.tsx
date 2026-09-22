import { useTranslation } from "react-i18next";
import { Brain, Wrench, Pencil, Archive, RefreshCw, Users } from "lucide-react";
import type { RunActivity } from "@/types/chat";

/** LLM call metadata captured from llm.started / llm.completed agent events. */
export interface RunLlmMeta {
  provider?: string;
  model?: string;
  /** Effective reasoning effort actually sent (post downgrade/resolve). */
  effort?: string;
  durationMs?: string;
  isError?: boolean;
}

type PhaseT = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Compose the "provider · model · effort" meta line from whatever fields are
 * available yet (llm.started arrives before effort; announce/legacy frames may
 * miss the provider). Returns null when nothing is known.
 */
export function llmMetaText(meta: RunLlmMeta | null | undefined, t: PhaseT): string | null {
  if (!meta) return null;
  const { provider, model, effort } = meta;
  if (provider && model && effort) return t("phase.metaFull", { provider, model, effort });
  if (model && effort) return t("phase.metaModelEffort", { model, effort });
  if (provider && model) return t("phase.metaProviderModel", { provider, model });
  if (model) return t("phase.metaModel", { model });
  if (provider && effort) return t("phase.metaProviderEffort", { provider, effort });
  if (provider) return t("phase.metaProvider", { provider });
  return null;
}

interface ActivityIndicatorProps {
  activity: RunActivity | null;
  isRunning: boolean;
  /** Latest llm.started/llm.completed metadata for the current run. */
  llmMeta?: RunLlmMeta | null;
}

/** The single run-phase indicator on the chat screen (bottom of the thread). */
export function ActivityIndicator({ activity, isRunning, llmMeta }: ActivityIndicatorProps) {
  const { t } = useTranslation("chat");
  if (!isRunning && activity?.phase !== "leader_processing") return null;

  const metaLine = llmMetaText(llmMeta, t);

  if (!activity) {
    return (
      <div className="flex flex-col gap-0.5">
        <div className="flex items-center gap-1 px-1 py-1">
          <span className="flex gap-1">
            <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:0ms]" />
            <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:150ms]" />
            <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:300ms]" />
          </span>
        </div>
        {metaLine && <div className="px-1 text-2xs text-muted-foreground">{metaLine}</div>}
      </div>
    );
  }

  const config = getPhaseConfig(activity, t);

  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <config.icon className={`h-4 w-4 ${config.animation} ${config.color}`} />
        <span className={config.color}>{config.label}</span>
        {activity.phase !== "retrying" && activity.iteration && activity.iteration > 1 && (
          <span className="text-muted-foreground">· {t("phase.step", { n: activity.iteration })}</span>
        )}
      </div>
      {metaLine && <div className="pl-6 text-2xs text-muted-foreground">{metaLine}</div>}
    </div>
  );
}


function getPhaseConfig(activity: RunActivity, t: PhaseT) {
  switch (activity.phase) {
    case "thinking":
      return { icon: Brain, animation: "animate-pulse", color: "text-amber-500", label: t("phase.thinking") };
    case "tool_exec":
      return {
        icon: Wrench,
        animation: "animate-wobble",
        color: "text-blue-500",
        label: activity.tool ? t("phase.tool", { tool: activity.tool }) : t("phase.tools"),
      };
    case "streaming":
      return { icon: Pencil, animation: "", color: "text-foreground", label: t("phase.streaming") };
    case "compacting":
      return { icon: Archive, animation: "animate-pulse", color: "text-amber-500", label: t("phase.compacting") };
    case "retrying":
      return {
        icon: RefreshCw,
        animation: "animate-spin",
        color: "text-amber-500",
        label: t("phase.retrying", { attempt: activity.retryAttempt ?? 0, max: activity.retryMax ?? 0 }),
      };
    case "leader_processing":
      return { icon: Users, animation: "animate-pulse", color: "text-emerald-500", label: t("phase.leaderProcessing") };
    default:
      return { icon: Brain, animation: "animate-pulse", color: "text-muted-foreground", label: t("phase.working") };
  }
}
