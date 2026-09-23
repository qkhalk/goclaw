import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Users, TrendingUp, Link2, Lightbulb } from "lucide-react";
import { useAgentLinks, type AgentLinkData } from "@/pages/teams/hooks/use-agent-links";
import { useV3Flags } from "@/hooks/use-v3-flags";
import { useEvolutionSuggestions } from "@/hooks/use-evolution-suggestions";
import { CapabilityCard } from "./capability-card";
import { LiveStrip, LiveChip } from "./live-strip";

interface OrchestrationTabProps {
  agentId: string;
  /** Switch the underlying agent-detail page to its Evolution tab. */
  onOpenEvolution?: () => void;
}

export function OrchestrationTab({
  agentId,
  onOpenEvolution,
}: OrchestrationTabProps) {
  const { t } = useTranslation("v3-capabilities");
  const { listLinks } = useAgentLinks();
  const { flags, loading: flagsLoading } = useV3Flags(agentId);
  const { suggestions, loading: suggestionsLoading } = useEvolutionSuggestions(
    agentId,
    "pending",
  );

  const [links, setLinks] = useState<AgentLinkData[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    listLinks(agentId)
      .then((res) => {
        if (!cancelled) setLinks(res);
      })
      .catch(() => {
        if (!cancelled) setLinks([]);
      });
    return () => {
      cancelled = true;
    };
  }, [agentId, listLinks]);

  const linkCount = links?.length ?? 0;
  const linkedNames = (links ?? [])
    .map((l) => (l.source_agent_id === agentId ? l.target_display_name : l.source_display_name))
    .slice(0, 3);

  const flag = (enabled: boolean | undefined) =>
    flagsLoading || flags == null ? "…" : enabled ? t("live.on") : t("live.off");

  return (
    <div className="space-y-3 pt-2">
      <LiveStrip
        onAction={onOpenEvolution ? () => onOpenEvolution() : undefined}
        actionLabel={onOpenEvolution ? t("live.openEvolution") : undefined}
        note={
          links == null
            ? undefined
            : linkCount > 0
              ? linkedNames.join(" · ")
              : t("live.linksEmpty")
        }
      >
        <LiveChip
          icon={Link2}
          label={t("live.links")}
          value={
            links == null
              ? "…"
              : t("live.linksValue", { total: linkCount })
          }
        />
        <LiveChip
          icon={TrendingUp}
          label={t("live.evolutionMetrics")}
          value={flag(flags?.self_evolution_metrics)}
          tone={flags?.self_evolution_metrics ? "on" : "off"}
        />
        <LiveChip
          icon={Lightbulb}
          label={t("live.pendingSuggestions")}
          value={
            suggestionsLoading || flags?.self_evolution_suggestions === false
              ? "—"
              : String(suggestions.length)
          }
        />
      </LiveStrip>

      <CapabilityCard
        icon={Users}
        title={t("orchestration.delegateTitle")}
        description={t("orchestration.delegateDesc")}
      />
      <CapabilityCard
        icon={TrendingUp}
        title={t("orchestration.evolutionTitle")}
        description={t("orchestration.evolutionDesc")}
      />
    </div>
  );
}
