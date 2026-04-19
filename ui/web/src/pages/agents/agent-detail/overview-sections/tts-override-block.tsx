import { useState } from "react";
import { PlayCircleIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { toast } from "@/stores/use-toast-store";
import { VoicePicker } from "@/components/voice-picker";
import { DynamicParamForm } from "@/components/dynamic-param-form";
import type { ParamValue } from "@/components/dynamic-param-form";
import { useTtsCapabilities } from "@/api/tts-capabilities";
import { PROVIDER_MODEL_CATALOG } from "@/data/tts-providers";
import type { TtsProviderId } from "@/data/tts-providers";
import type { SynthesizeParams } from "@/pages/tts/hooks/use-tts-config";

interface Props {
  globalProvider: string;
  voiceId: string;
  modelId: string;
  onVoiceChange: (v: string) => void;
  onModelChange: (v: string) => void;
  /** Whether agent-level override is enabled (checkbox driven by parent) */
  overrideEnabled: boolean;
  onOverrideChange: (v: boolean) => void;
  synthesize: (params: SynthesizeParams) => Promise<Blob>;
  /** Generic tts_params stored in agents.other_config.tts_params (e.g. {speed:1.2}) */
  ttsParams: Record<string, ParamValue>;
  onTtsParamsChange: (params: Record<string, ParamValue>) => void;
}

/**
 * Adapter table: maps generic agent override key → provider-native capability key.
 * Must mirror internal/audio/agent_params_adapter.go AdaptAgentParams switch.
 * Used for bidirectional conversion at the UI boundary (generic↔native).
 *
 * Finding #9: This mirrors the Go adapter but is NOT the source of truth for
 * which params are shown — that comes from agent_overridable===true in
 * capabilities. This table is only used to convert stored generic keys to
 * native keys for form state, and back on save.
 */
const GENERIC_TO_NATIVE: Record<string, Record<string, string>> = {
  openai: { speed: "speed" },
  elevenlabs: { speed: "voice_settings.speed", style: "voice_settings.style" },
  edge: {},
  minimax: { speed: "speed", emotion: "emotion" },
  gemini: {},
};

/** Invert the generic→native table to native→generic for a given provider. */
function buildNativeToGeneric(provider: string): Record<string, string> {
  const map = GENERIC_TO_NATIVE[provider] ?? {};
  const inv: Record<string, string> = {};
  for (const [generic, native] of Object.entries(map)) {
    inv[native] = generic;
  }
  return inv;
}

/**
 * Convert stored generic params (e.g. {speed: 1.2}) to capability-native form
 * state (e.g. {voice_settings.speed: 1.2} for ElevenLabs).
 * Called at load time.
 */
export function genericToNativeFormState(
  genericParams: Record<string, ParamValue>,
  provider: string,
): Record<string, ParamValue> {
  const map = GENERIC_TO_NATIVE[provider] ?? {};
  const out: Record<string, ParamValue> = {};
  for (const [generic, val] of Object.entries(genericParams)) {
    const native = map[generic];
    if (native !== undefined) {
      out[native] = val;
    }
  }
  return out;
}

/**
 * Convert capability-native form state back to generic keys for storage.
 * Called at save time (inside handleSave in prompt-settings-section.tsx).
 */
export function nativeFormStateToGeneric(
  nativeState: Record<string, ParamValue>,
  provider: string,
): Record<string, ParamValue> {
  const nativeToGeneric = buildNativeToGeneric(provider);
  const out: Record<string, ParamValue> = {};
  for (const [native, val] of Object.entries(nativeState)) {
    const generic = nativeToGeneric[native];
    if (generic !== undefined) {
      out[generic] = val;
    }
  }
  return out;
}

/**
 * Rendered inside the TTS subsection of PromptSettingsSection when global TTS is configured.
 * Manages: inheritance chip, override checkbox, VoicePicker, model Select, inline test button.
 * Also renders the fine-tune (tts_params) section — filtered to agent_overridable params only
 * (Finding #9: single source of truth from capabilities API).
 *
 * Key design: agent storage uses GENERIC keys (speed, emotion, style). The DynamicParamForm
 * uses CAPABILITY-NATIVE keys (voice_settings.speed for ElevenLabs). A bidirectional adapter
 * converts at load (generic→native) and save (native→generic) boundaries.
 */
export function TtsOverrideBlock({
  globalProvider,
  voiceId,
  modelId,
  onVoiceChange,
  onModelChange,
  overrideEnabled,
  onOverrideChange,
  synthesize,
  ttsParams,
  onTtsParamsChange,
}: Props) {
  const { t } = useTranslation("tts");
  const [testing, setTesting] = useState(false);

  // Fetch capabilities for the current provider to find agent_overridable params.
  const { data: allCaps } = useTtsCapabilities();
  const providerCaps = allCaps?.find((c) => c.provider === globalProvider);
  const overridableParams = (providerCaps?.params ?? []).filter(
    (p) => p.agent_overridable === true,
  );

  // Form state uses CAPABILITY-NATIVE keys. Convert from stored generic keys on each render.
  // This is a pure derivation — form state is owned by this component transiently,
  // and serialised back to generic keys via nativeFormStateToGeneric on change.
  const nativeFormState = genericToNativeFormState(ttsParams, globalProvider);

  const handleParamChange = (nativeKey: string, val: ParamValue) => {
    const updated = { ...nativeFormState, [nativeKey]: val };
    // Convert native form state → generic keys for parent storage.
    const generic = nativeFormStateToGeneric(updated, globalProvider);
    onTtsParamsChange(generic);
  };

  const providerLabel = globalProvider.charAt(0).toUpperCase() + globalProvider.slice(1);
  const models = PROVIDER_MODEL_CATALOG[globalProvider as TtsProviderId] ?? [];
  const hasModels = models.length > 0;

  const canTest = overrideEnabled && !!voiceId && (hasModels ? !!modelId : true) && !!globalProvider;

  const handleTest = async () => {
    if (!canTest) return;
    setTesting(true);
    try {
      const blob = await synthesize({
        text: t("test.sample_text"),
        provider: globalProvider,
        voice_id: voiceId,
        model_id: modelId || undefined,
      });
      const url = URL.createObjectURL(blob);
      const audio = new Audio(url);
      audio.onended = () => URL.revokeObjectURL(url);
      audio.onerror = () => URL.revokeObjectURL(url);
      await audio.play();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Test failed");
    } finally {
      setTesting(false);
    }
  };

  const handleOverrideChange = (checked: boolean) => {
    onOverrideChange(checked);
    if (!checked) {
      onVoiceChange("");
      onModelChange("");
      onTtsParamsChange({});
    }
  };

  return (
    <div className="space-y-3">
      {/* Inheritance info chip */}
      <p className="text-xs text-muted-foreground bg-muted/50 rounded px-2 py-1 inline-block">
        {t("override.inherits", {
          provider: providerLabel,
          voice: globalProvider === "elevenlabs" ? t("voice_label") : "–",
          model: models[0]?.value ?? "–",
        })}
      </p>

      {/* Override checkbox */}
      <label className="flex items-center gap-2 cursor-pointer select-none">
        <input
          type="checkbox"
          className="size-4 rounded accent-primary"
          checked={overrideEnabled}
          onChange={(e) => handleOverrideChange(e.target.checked)}
        />
        <span className="text-sm">{t("override.label")}</span>
      </label>

      {overrideEnabled && (
        <div className="space-y-2 pl-6">
          {/* Voice picker — provider-aware */}
          <div className="space-y-1">
            <Label className="text-xs text-muted-foreground">{t("voice_label")}</Label>
            <VoicePicker
              provider={(globalProvider as TtsProviderId) || undefined}
              value={voiceId || undefined}
              onChange={onVoiceChange}
            />
          </div>

          {/* Model select — catalog-driven; hidden for providers with no models (edge) */}
          {hasModels && (
            <div className="space-y-1">
              <Label className="text-xs text-muted-foreground">{t("model_label")}</Label>
              <Select value={modelId} onValueChange={onModelChange}>
                <SelectTrigger className="w-full text-base md:text-sm">
                  <SelectValue placeholder={t("model_placeholder")} />
                </SelectTrigger>
                <SelectContent>
                  {models.map((m) => (
                    <SelectItem key={m.value} value={m.value}>
                      {m.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Fine-tune section — agent_overridable params only (Finding #9).
              Hidden entirely for providers with no overridable params (edge, gemini). */}
          {overridableParams.length > 0 && (
            <div className="space-y-2 border-t pt-2">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                {t("override.params.title")}
              </p>
              <DynamicParamForm
                schema={overridableParams}
                value={nativeFormState}
                onChange={handleParamChange}
              />
            </div>
          )}

          {/* Inline test button */}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!canTest || testing}
            onClick={handleTest}
            className="min-h-[44px] sm:min-h-9 gap-1.5"
          >
            <PlayCircleIcon className="size-4" />
            {testing ? "..." : t("test.button")}
          </Button>
        </div>
      )}
    </div>
  );
}
