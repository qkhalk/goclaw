import { useTranslation } from "react-i18next";
import {
  Workflow,
  Brain,
  Scissors,
  Wrench,
  Eye,
  Save,
  Flag,
  ChevronRight,
  Zap,
  Database,
  Search,
  ScissorsLineDashed,
} from "lucide-react";
import type { AgentData } from "@/types/agent";
import { useV3Flags } from "@/hooks/use-v3-flags";
import { LiveStrip, LiveChip } from "./live-strip";

interface PipelineTabProps {
  agent: AgentData;
  agentId: string;
}

export function PipelineTab({ agentId, agent }: PipelineTabProps) {
  const { t } = useTranslation("v3-capabilities");
  const { flags, loading: flagsLoading } = useV3Flags(agentId);

  const iterationStages = [
    { icon: Brain, key: "think" },
    { icon: Scissors, key: "prune" },
    { icon: Wrench, key: "tools" },
    { icon: Eye, key: "observe" },
    { icon: Save, key: "checkpoint" },
  ] as const;

  const flag = (enabled: boolean | undefined) =>
    flagsLoading || flags == null
      ? "…"
      : enabled
        ? t("live.on")
        : t("live.off");

  return (
    <div className="space-y-4 pt-2">
      <LiveStrip>
        <LiveChip
          icon={Workflow}
          label={t("tabs.pipeline")}
          value={flag(flags?.v3_pipeline_enabled)}
          tone={flags?.v3_pipeline_enabled ? "on" : "off"}
        />
        <LiveChip
          icon={Database}
          label={t("tabs.memory")}
          value={flag(flags?.v3_memory_enabled)}
          tone={flags?.v3_memory_enabled ? "on" : "off"}
        />
        <LiveChip
          icon={Search}
          label={t("live.retrieval")}
          value={flag(flags?.v3_retrieval_enabled)}
          tone={flags?.v3_retrieval_enabled ? "on" : "off"}
        />
        <LiveChip
          icon={ScissorsLineDashed}
          label={t("live.pruning")}
          value={agent.context_pruning?.mode ?? t("live.default")}
        />
      </LiveStrip>

      <div>
        <div className="flex items-center gap-2 mb-1">
          <Zap className="h-4 w-4 text-blue-500" />
          <h4 className="text-sm font-medium">{t("pipeline.title")}</h4>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("pipeline.description")}
        </p>
      </div>

      {/* Setup */}
      <div className="rounded-lg border p-3 space-y-1">
        <div className="flex items-center gap-2">
          <Flag className="h-3.5 w-3.5 text-green-500" />
          <span className="text-xs font-medium text-green-700 dark:text-green-400">
            {t("pipeline.setup")}
          </span>
        </div>
        <p className="text-xs text-muted-foreground leading-relaxed">
          {t("pipeline.setupDesc")}
        </p>
      </div>

      {/* Iteration loop */}
      <div className="rounded-lg border border-dashed border-blue-300 dark:border-blue-700 p-3 space-y-2">
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-blue-600 dark:text-blue-400">
            {t("pipeline.iteration")}
          </span>
          <span className="text-2xs text-muted-foreground">
            {t("pipeline.iterationDesc")}
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          {iterationStages.map(({ icon: Icon, key }, i) => (
            <div key={key} className="flex items-center gap-1">
              <div className="flex items-center gap-1.5 rounded-md bg-muted/50 px-2.5 py-1.5">
                <Icon className="h-3.5 w-3.5 text-blue-500" />
                <div className="min-w-0">
                  <p className="text-xs-plus font-medium">
                    {t(`pipeline.${key}`)}
                  </p>
                  <p className="text-2xs text-muted-foreground">
                    {t(`pipeline.${key}Desc`)}
                  </p>
                </div>
              </div>
              {i < iterationStages.length - 1 && (
                <ChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Finalize */}
      <div className="rounded-lg border p-3 space-y-1">
        <div className="flex items-center gap-2">
          <Flag className="h-3.5 w-3.5 text-amber-500" />
          <span className="text-xs font-medium text-amber-700 dark:text-amber-400">
            {t("pipeline.finalize")}
          </span>
        </div>
        <p className="text-xs text-muted-foreground leading-relaxed">
          {t("pipeline.finalizeDesc")}
        </p>
      </div>
    </div>
  );
}
