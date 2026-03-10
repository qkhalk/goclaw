import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import type { CompactionConfig } from "@/types/agent";
import { ConfigSection, InfoLabel, numOrUndef } from "./config-section";

interface CompactionSectionProps {
  enabled: boolean;
  value: CompactionConfig;
  onToggle: (v: boolean) => void;
  onChange: (v: CompactionConfig) => void;
}

export function CompactionSection({ enabled, value, onToggle, onChange }: CompactionSectionProps) {
  const { t } = useTranslation("agents");
  const s = "configSections.compaction";
  return (
    <ConfigSection
      title={t(`${s}.title`)}
      description={t(`${s}.description`)}
      enabled={enabled}
      onToggle={onToggle}
    >
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <InfoLabel tip={t(`${s}.maxHistoryShareTip`)}>{t(`${s}.maxHistoryShare`)}</InfoLabel>
          <Input
            type="number"
            step="0.05"
            placeholder="0.75"
            value={value.maxHistoryShare ?? ""}
            onChange={(e) => onChange({ ...value, maxHistoryShare: numOrUndef(e.target.value) })}
          />
        </div>
        <div className="space-y-2">
          <InfoLabel tip={t(`${s}.minMessagesTip`)}>{t(`${s}.minMessages`)}</InfoLabel>
          <Input
            type="number"
            placeholder="50"
            value={value.minMessages ?? ""}
            onChange={(e) => onChange({ ...value, minMessages: numOrUndef(e.target.value) })}
          />
        </div>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <InfoLabel tip={t(`${s}.keepLastMessagesTip`)}>{t(`${s}.keepLastMessages`)}</InfoLabel>
          <Input
            type="number"
            placeholder="4"
            value={value.keepLastMessages ?? ""}
            onChange={(e) => onChange({ ...value, keepLastMessages: numOrUndef(e.target.value) })}
          />
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Switch
          checked={value.memoryFlush?.enabled ?? true}
          onCheckedChange={(v) =>
            onChange({ ...value, memoryFlush: { ...value.memoryFlush, enabled: v } })
          }
        />
        <InfoLabel tip={t(`${s}.memoryFlushTip`)}>{t(`${s}.memoryFlush`)}</InfoLabel>
      </div>
    </ConfigSection>
  );
}
