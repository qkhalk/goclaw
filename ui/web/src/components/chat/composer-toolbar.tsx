import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { BrainCog, Cpu } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useProviders } from "@/pages/providers/hooks/use-providers";
import { useProviderModels } from "@/pages/providers/hooks/use-provider-models";

/** Per-message overrides sent with chat.send; empty fields = agent defaults. */
export interface ComposerOverrides {
  providerName?: string;
  model?: string;
  thinkingLevel?: string;
}

interface ComposerToolbarProps {
  value: ComposerOverrides;
  onChange: (next: ComposerOverrides) => void;
  disabled?: boolean;
}

const FALLBACK_LEVELS = ["off", "low", "medium", "high"];

/**
 * Paseo-style composer toolbar: connected-provider picker, model picker fed by
 * the selected provider's /models endpoint, and a per-model thinking picker.
 * Thinking options come from the model's reasoning capability (levels differ
 * per model); models without capability info get the generic ladder — the
 * backend still guards unsupported levels per provider.
 */
export function ComposerToolbar({ value, onChange, disabled }: ComposerToolbarProps) {
  const { t } = useTranslation("chat");
  const { providers } = useProviders(!disabled);

  const enabledProviders = useMemo(
    () => providers.filter((p) => p.enabled),
    [providers],
  );

  const selectedProvider = useMemo(
    () => enabledProviders.find((p) => p.name === value.providerName),
    [enabledProviders, value.providerName],
  );

  const { models, loading: modelsLoading } = useProviderModels(selectedProvider?.id);

  const thinkingLevels = useMemo(() => {
    const model = models.find((m) => m.id === value.model);
    const levels = model?.reasoning?.levels;
    return levels && levels.length > 0 ? levels : FALLBACK_LEVELS;
  }, [models, value.model]);

  const levelLabel = (level: string) => t(`thinkingLevels.${level}`, { defaultValue: level });

  return (
    <div className="flex min-w-0 items-center gap-1">
      {/* Provider picker — only connected (enabled) providers */}
      <Select
        value={value.providerName ?? ""}
        onValueChange={(v) => onChange({ providerName: v || undefined, model: undefined })}
        disabled={disabled || enabledProviders.length === 0}
      >
        <SelectTrigger
          size="sm"
          className="h-7 max-w-[150px] gap-1 rounded-lg border-none bg-muted/60 px-2 text-xs font-medium text-muted-foreground shadow-none focus:ring-1 [&>svg]:h-3.5 [&>svg]:w-3.5"
          title={t("composer.provider")}
        >
          <Cpu className="h-3.5 w-3.5 shrink-0" />
          <SelectValue placeholder={t("composer.providerDefault")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="agent-default" hidden>
            {t("composer.providerDefault")}
          </SelectItem>
          {enabledProviders.map((p) => (
            <SelectItem key={p.id} value={p.name} className="text-sm">
              {p.display_name || p.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Model picker — models of the selected provider */}
      {selectedProvider && (
        <Select
          value={value.model ?? ""}
          onValueChange={(v) => onChange({ ...value, model: v || undefined })}
          disabled={disabled || modelsLoading}
        >
          <SelectTrigger
            size="sm"
            className="h-7 max-w-[190px] gap-1 rounded-lg border-none bg-muted/60 px-2 text-xs font-medium text-muted-foreground shadow-none focus:ring-1 [&>svg]:h-3.5 [&>svg]:w-3.5"
            title={t("composer.model")}
          >
            <SelectValue placeholder={modelsLoading ? t("composer.loadingModels") : t("composer.modelDefault")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="agent-default" hidden>
              {t("composer.modelDefault")}
            </SelectItem>
            {models.map((m) => (
              <SelectItem key={m.id} value={m.id} className="text-sm">
                {m.name || m.id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      {/* Thinking picker — levels follow the selected model's capability */}
      <Select
        value={value.thinkingLevel ?? "adaptive"}
        onValueChange={(v) => onChange({ ...value, thinkingLevel: v })}
        disabled={disabled}
      >
        <SelectTrigger
          size="sm"
          className="h-7 max-w-[150px] gap-1 rounded-lg border-none bg-muted/60 px-2 text-xs font-medium text-muted-foreground shadow-none focus:ring-1 [&>svg]:h-3.5 [&>svg]:w-3.5"
          title={t("composer.thinking")}
        >
          <BrainCog className="h-3.5 w-3.5 shrink-0" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="adaptive" className="text-sm">
            {levelLabel("adaptive")}
          </SelectItem>
          <SelectItem value="auto" className="text-sm">
            {levelLabel("auto")}
          </SelectItem>
          {thinkingLevels.map((level) => (
            <SelectItem key={level} value={level} className="text-sm">
              {levelLabel(level)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
