import { useTranslation } from "react-i18next";
import type { AgentData, SubagentsConfig, ToolPolicyConfig } from "@/types/agent";
import { SubagentsSection, ToolPolicySection, FileCloudPermissionsSection } from "../config-sections";
import { ConfigGroupHeader } from "@/components/shared/config-group-header";

interface CapabilitiesSectionProps {
  agent: AgentData;
  onUpdate: (updates: Record<string, unknown>) => Promise<void>;

  subEnabled: boolean;
  sub: SubagentsConfig;
  onSubToggle: (v: boolean) => void;
  onSubChange: (v: SubagentsConfig) => void;

  toolsEnabled: boolean;
  tools: ToolPolicyConfig;
  onToolsToggle: (v: boolean) => void;
  onToolsChange: (v: ToolPolicyConfig) => void;
}

export function CapabilitiesSection({
  agent, onUpdate,
  subEnabled, sub, onSubToggle, onSubChange,
  toolsEnabled, tools, onToolsToggle, onToolsChange,
}: CapabilitiesSectionProps) {
  const { t } = useTranslation("agents");

  return (
    <section className="space-y-4 rounded-lg border p-3 sm:p-4">
      <ConfigGroupHeader
        title={t("detail.capabilities")}
        description={t("configGroups.capabilitiesDesc")}
      />
      <div className="space-y-4">
        <SubagentsSection
          enabled={subEnabled}
          value={sub}
          onToggle={(v) => { onSubToggle(v); if (!v) onSubChange({}); }}
          onChange={onSubChange}
        />
        <ToolPolicySection
          enabled={toolsEnabled}
          value={tools}
          onToggle={(v) => { onToolsToggle(v); if (!v) onToolsChange({}); }}
          onChange={onToolsChange}
        />
        <FileCloudPermissionsSection agent={agent} onUpdate={onUpdate} />
      </div>
    </section>
  );
}
