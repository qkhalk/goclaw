import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Network } from "lucide-react";
import { EmptyState } from "@/components/shared/empty-state";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { KGEntitiesTab } from "@/pages/memory/kg-entities-tab";

export function KnowledgeGraphPage() {
  const { t } = useTranslation("memory");
  const { agents } = useAgents();
  const [agentId, setAgentId] = useState("");
  const [userIdFilter, setUserIdFilter] = useState("");

  return (
    <div className="flex h-full flex-col p-4 sm:p-6">
      {/* Header + filters in one row */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="mr-auto">
          <h1 className="text-lg font-semibold">{t("kg.pageTitle")}</h1>
          <p className="text-xs text-muted-foreground">{t("kg.pageDescription")}</p>
        </div>
        <select
          id="kg-agent"
          value={agentId}
          onChange={(e) => { setAgentId(e.target.value); setUserIdFilter(""); }}
          className="h-8 rounded-md border bg-background px-2 text-sm"
        >
          <option value="">{t("filters.selectAgent")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.display_name || a.agent_key}
            </option>
          ))}
        </select>
        {agentId && (
          <input
            id="kg-scope"
            value={userIdFilter}
            onChange={(e) => setUserIdFilter(e.target.value)}
            placeholder={t("filters.allScope")}
            className="h-8 rounded-md border bg-background px-2 text-sm w-[160px]"
          />
        )}
      </div>

      {/* Content */}
      <div className="mt-3 min-h-0 flex-1">
        {!agentId ? (
          <EmptyState
            icon={Network}
            title={t("kg.selectAgentTitle")}
            description={t("kg.selectAgentDescription")}
          />
        ) : (
          <KGEntitiesTab agentId={agentId} userId={userIdFilter || undefined} />
        )}
      </div>
    </div>
  );
}
