import { useTranslation } from "react-i18next";
import { GitFork, Library, Moon } from "lucide-react";
import type { AgentData } from "@/types/agent";
import { useKGStats } from "@/pages/memory/hooks/use-knowledge-graph";
import { ROUTES } from "@/lib/routes";
import { CapabilityCard } from "./capability-card";
import { LiveStrip, LiveChip } from "./live-strip";

interface KnowledgeTabProps {
  agent: AgentData;
  agentId: string;
}

export function KnowledgeTab({ agentId, agent }: KnowledgeTabProps) {
  const { t } = useTranslation("v3-capabilities");
  const { stats, loading: kgLoading } = useKGStats(agentId);

  // DreamingConfig fields stay undefined when the operator hasn't overridden
  // them — surface that as "default" instead of guessing the backend value.
  const dreaming = agent.memory_config?.dreaming;
  const dreamingValue =
    dreaming?.enabled === true
      ? t("live.on")
      : dreaming?.enabled === false
        ? t("live.off")
        : t("live.default");

  return (
    <div className="space-y-3 pt-2">
      <LiveStrip to={ROUTES.KNOWLEDGE_GRAPH}>
        <LiveChip
          icon={GitFork}
          label={t("live.kg")}
          value={
            kgLoading
              ? "…"
              : stats
                ? t("live.kgValue", {
                    entities: stats.entity_count,
                    relations: stats.relation_count,
                  })
                : t("live.noneYet")
          }
        />
        <LiveChip
          icon={Moon}
          label={t("live.dreaming")}
          value={dreamingValue}
          tone={dreaming?.enabled === false ? "off" : dreaming?.enabled === true ? "on" : undefined}
        />
      </LiveStrip>

      <CapabilityCard
        icon={GitFork}
        title={t("knowledge.kgTitle")}
        description={t("knowledge.kgDesc")}
      />
      <CapabilityCard
        icon={Library}
        title={t("knowledge.vaultTitle")}
        description={t("knowledge.vaultDesc")}
      />
      <CapabilityCard
        icon={Moon}
        title={t("knowledge.dreamingTitle")}
        description={t("knowledge.dreamingDesc")}
      />
    </div>
  );
}
