import { useTranslation } from "react-i18next";
import {
  Brain,
  MessageSquare,
  BookOpen,
  Network,
  ArrowDown,
} from "lucide-react";
import { useEpisodicSummaries } from "@/pages/memory/hooks/use-episodic";
import { useKGStats } from "@/pages/memory/hooks/use-knowledge-graph";
import { formatRelativeTime } from "@/lib/format";
import { ROUTES } from "@/lib/routes";
import { CapabilityCard } from "./capability-card";
import { LiveStrip, LiveChip } from "./live-strip";

interface MemoryTabProps {
  agentId: string;
}

export function MemoryTab({ agentId }: MemoryTabProps) {
  const { t } = useTranslation("v3-capabilities");
  const { summaries, loading: episodicLoading } = useEpisodicSummaries(
    agentId,
    { limit: 1 },
  );
  const { stats, loading: kgLoading } = useKGStats(agentId);

  const latest = summaries[0];

  return (
    <div className="space-y-3 pt-2">
      <LiveStrip to={ROUTES.MEMORY}>
        <LiveChip
          icon={BookOpen}
          label={t("live.episodicLatest")}
          value={
            episodicLoading
              ? "…"
              : latest
                ? formatRelativeTime(latest.created_at)
                : t("live.noneYet")
          }
        />
        <LiveChip
          icon={Network}
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
      </LiveStrip>

      <div>
        <div className="flex items-center gap-2 mb-1">
          <Brain className="h-4 w-4 text-blue-500" />
          <h4 className="text-sm font-medium">{t("memory.title")}</h4>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("memory.description")}
        </p>
      </div>

      <CapabilityCard
        icon={MessageSquare}
        title={t("memory.l0Title")}
        description={t("memory.l0Desc")}
      />

      <div className="flex justify-center">
        <div className="flex items-center gap-1.5 text-2xs text-muted-foreground">
          <ArrowDown className="h-3 w-3" />
          <span>session.completed</span>
        </div>
      </div>

      <CapabilityCard
        icon={BookOpen}
        title={t("memory.l1Title")}
        description={t("memory.l1Desc")}
      />

      <div className="flex justify-center">
        <div className="flex items-center gap-1.5 text-2xs text-muted-foreground">
          <ArrowDown className="h-3 w-3" />
          <span>episodic.created</span>
        </div>
      </div>

      <CapabilityCard
        icon={Network}
        title={t("memory.l2Title")}
        description={t("memory.l2Desc")}
      />

      <p className="text-xs-plus text-muted-foreground italic">
        {t("memory.eventFlow")}
      </p>
    </div>
  );
}
