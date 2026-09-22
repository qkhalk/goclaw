import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { BrainCog, Cpu, ShieldCheck } from "lucide-react";
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
  permissionMode?: string;
  /**
   * Dev mode flag (chat.send per-message). Not persisted inside the composer
   * overrides object — state lives in the chat page, keyed per session.
   */
  devMode?: boolean;
}

/** Permission modes mirrored from tools.PermMode* (chat.send permissionMode). */
export const PERMISSION_MODES = [
  { value: "plan", labelKey: "permissionModes.plan" },
  { value: "full_access", labelKey: "permissionModes.fullAccess" },
  { value: "write_approval", labelKey: "permissionModes.writeApproval" },
  { value: "always_ask", labelKey: "permissionModes.alwaysAsk" },
] as const;

interface ComposerToolbarProps {
  value: ComposerOverrides;
  onChange: (next: ComposerOverrides) => void;
  disabled?: boolean;
  /**
   * Provider name of the agent this composer talks to. When no provider
   * override is picked, the model list is fed from this provider so a model
   * can be selected alone (chat.send applies model-only overrides on the
   * agent's own provider).
   */
  defaultProviderName?: string;
  /**
   * Hide the per-run permission-mode picker. Designer composers omit it —
   * those agents run a locked design-only tool surface, so a per-run gating
   * override is noise (spec: provider, model, thinking only).
   */
  showPermissionMode?: boolean;
}

const FALLBACK_LEVELS = ["off", "low", "medium", "high"];
/** Sentinel item value for "back to agent default" (Radix forbids item value=""). */
const AGENT_DEFAULT = "agent-default";

/**
 * Paseo-style composer toolbar: connected-provider picker, model picker fed by
 * the selected provider's /models endpoint, and a per-model thinking picker.
 * Thinking options come from the model's reasoning capability (levels differ
 * per model); models without capability info get the generic ladder — the
 * backend still guards unsupported levels per provider.
 *
 * SelectContent uses position="popper": the stock item-aligned mode renders the
 * highlighted option on top of the trigger instead of a panel, which reads as
 * a broken button on the compact composer chips (and never aligns when nothing
 * is selected). Popper also flips upward automatically for the bottom-docked
 * composer.
 */
export function ComposerToolbar({ value, onChange, disabled, defaultProviderName, showPermissionMode = true }: ComposerToolbarProps) {
  const { t } = useTranslation("chat");
  const { providers } = useProviders(!disabled);

  const enabledProviders = useMemo(
    () => providers.filter((p) => p.enabled),
    [providers],
  );

  // With a provider override the model list follows that provider; without
  // one it follows the agent's own provider (model-only override), so the
  // model picker stays usable in both states.
  const selectedProvider = useMemo(
    () =>
      enabledProviders.find((p) => p.name === value.providerName) ??
      enabledProviders.find((p) => p.name === defaultProviderName),
    [enabledProviders, value.providerName, defaultProviderName],
  );

  const { models, loading: modelsLoading } = useProviderModels(selectedProvider?.id);

  const thinkingLevels = useMemo(() => {
    const model = models.find((m) => m.id === value.model);
    const levels = model?.reasoning?.levels;
    return levels && levels.length > 0 ? levels : FALLBACK_LEVELS;
  }, [models, value.model]);

  const levelLabel = (level: string) => t(`thinkingLevels.${level}`, { defaultValue: level });

  // The agent's own provider is a real selectable value (not a "Provider của
  // agent" placeholder), so the pill always shows an actual provider name.
  // The sentinel entry stays only as a fallback when the agent's provider is
  // unknown or disconnected.
  const agentProvider = enabledProviders.find((p) => p.name === defaultProviderName);

  return (
    <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
      {/* Provider picker — only connected (enabled) providers */}
      <Select
        value={value.providerName ?? agentProvider?.name ?? AGENT_DEFAULT}
        onValueChange={(v) => {
          if (!v || v === AGENT_DEFAULT || v === defaultProviderName) {
            onChange({ providerName: undefined, model: undefined });
          } else {
            onChange({ providerName: v, model: undefined });
          }
        }}
        disabled={disabled || enabledProviders.length === 0}
      >
        <SelectTrigger
          size="sm"
          className="h-7 max-w-[140px] gap-1 rounded-lg border-none bg-muted/60 px-2 text-xs font-medium text-muted-foreground shadow-none focus:ring-1 [&>svg]:h-3.5 [&>svg]:w-3.5 [&>span]:truncate"
          title={t("composer.provider")}
        >
          <Cpu className="h-3.5 w-3.5 shrink-0" />
          <SelectValue placeholder={t("composer.providerDefault")} />
        </SelectTrigger>
        <SelectContent position="popper" sideOffset={6} className="w-56">
          {!agentProvider && (
            <SelectItem value={AGENT_DEFAULT} className="text-sm">
              {t("composer.providerDefault")}
            </SelectItem>
          )}
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
          value={value.model ?? AGENT_DEFAULT}
          onValueChange={(v) => onChange({ ...value, model: v === AGENT_DEFAULT ? undefined : v })}
          disabled={disabled || modelsLoading}
        >
          <SelectTrigger
            size="sm"
            className="h-7 max-w-[170px] gap-1 rounded-lg border-none bg-muted/60 px-2 text-xs font-medium text-muted-foreground shadow-none focus:ring-1 [&>svg]:h-3.5 [&>svg]:w-3.5 [&>span]:truncate"
            title={t("composer.model")}
          >
            <SelectValue placeholder={modelsLoading ? t("composer.loadingModels") : t("composer.modelDefault")} />
          </SelectTrigger>
          <SelectContent position="popper" sideOffset={6} className="w-64">
            <SelectItem value={AGENT_DEFAULT} className="text-sm">
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
          className="h-7 max-w-[120px] gap-1 rounded-lg border-none bg-muted/60 px-2 text-xs font-medium text-muted-foreground shadow-none focus:ring-1 [&>svg]:h-3.5 [&>svg]:w-3.5 [&>span]:truncate"
          title={t("composer.thinking")}
        >
          <BrainCog className="h-3.5 w-3.5 shrink-0" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent position="popper" sideOffset={6} className="w-52">
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

      {/* Permission mode picker — per-run tool gating override (agent default
          when unset). plan = read-only, full_access = no gates, the ask-modes
          route writes/exec through the approvals queue. Designer composers
          hide it via showPermissionMode={false}. */}
      {showPermissionMode && (
        <Select
          value={value.permissionMode ?? AGENT_DEFAULT}
          onValueChange={(v) =>
            onChange({ ...value, permissionMode: v === AGENT_DEFAULT ? undefined : v })
          }
          disabled={disabled}
        >
          <SelectTrigger
            size="sm"
            className="h-7 max-w-[150px] gap-1 rounded-lg border-none bg-muted/60 px-2 text-xs font-medium text-muted-foreground shadow-none focus:ring-1 [&>svg]:h-3.5 [&>svg]:w-3.5"
            title={t("composer.permissionMode")}
          >
            <ShieldCheck className="h-3.5 w-3.5 shrink-0" />
            <SelectValue placeholder={t("permissionModes.default")} />
          </SelectTrigger>
          <SelectContent position="popper" sideOffset={6} className="w-56">
            <SelectItem value={AGENT_DEFAULT} className="text-sm">
              {t("permissionModes.default")}
            </SelectItem>
            {PERMISSION_MODES.map((m) => (
              <SelectItem key={m.value} value={m.value} className="text-sm">
                {t(m.labelKey)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}
