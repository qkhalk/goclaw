import { useTranslation } from "react-i18next";
import {
  ArrowLeftRight,
  Braces,
  Film,
  Layers,
  Mic,
  Plus,
  Sparkles,
  Type,
  type LucideIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import { TRANSITION_TYPES, type SceneTransition } from "../scene-transition";
import type { Layer, Scene } from "../../hooks/use-timeline";

// ── Rail sections ──

export type RailSection =
  | "media"
  | "voice"
  | "text"
  | "transitions"
  | "effects"
  | "layers";

const SECTIONS: Array<{ id: RailSection; icon: LucideIcon; labelKey: string }> = [
  { id: "media", icon: Film, labelKey: "video.studio.rail.media" },
  { id: "voice", icon: Mic, labelKey: "video.studio.rail.voice" },
  { id: "text", icon: Type, labelKey: "video.studio.rail.text" },
  { id: "transitions", icon: ArrowLeftRight, labelKey: "video.studio.rail.transitions" },
  { id: "effects", icon: Sparkles, labelKey: "video.studio.rail.effects" },
  { id: "layers", icon: Layers, labelKey: "video.studio.rail.layers" },
];

/** Layer cap mirrors SceneCard / useTimeline (8 per scene). */
const MAX_LAYERS = 8;

interface MediaRailProps {
  scenes: Scene[];
  selectedIndex: number;
  active: RailSection;
  onActiveChange: (s: RailSection) => void;
  onSelectScene: (index: number) => void;
  onAddScene: () => void;
  /** Patch the SELECTED scene (quick toggles / transition). */
  onUpdateScene: (patch: Partial<Scene>) => void;
  /** Insert a layer into the SELECTED scene. */
  onAddLayer: (layer: Layer) => void;
  layerCount: number;
  /** Switch the inspector to its Layers tab. */
  onOpenLayersTab: () => void;
  onOpenJson: () => void;
  /** Scroll the scene editor to the narration field. */
  onScrollToNarration: () => void;
}

/**
 * Filmora-style left rail: slim icon strip + a quick-insert panel. This is a
 * NAVIGATION surface over existing handlers only — it never re-implements
 * scene editing (the full editors live in the inspector's Scene tab).
 */
export function MediaRail({
  scenes,
  selectedIndex,
  active,
  onActiveChange,
  onSelectScene,
  onAddScene,
  onUpdateScene,
  onAddLayer,
  layerCount,
  onOpenLayersTab,
  onOpenJson,
  onScrollToNarration,
}: MediaRailProps) {
  const { t } = useTranslation("toolbox");
  const scene = scenes[selectedIndex];
  const layersFull = layerCount >= MAX_LAYERS;

  return (
    <div className="flex flex-col border-white/[0.06] max-lg:border-t lg:h-full lg:flex-row lg:border-r">
      {/* Icon rail (vertical on desktop, horizontal strip on mobile) */}
      <nav
        aria-label={t("video.studio.rail.label")}
        className="flex shrink-0 items-center gap-1 border-white/[0.06] px-2 py-1 max-lg:overflow-x-auto max-lg:overscroll-contain lg:flex-col lg:overflow-y-auto lg:border-b lg:px-1.5 lg:py-2"
      >
        {SECTIONS.map(({ id, icon: Icon, labelKey }) => (
          <button
            key={id}
            type="button"
            onClick={() => onActiveChange(id)}
            aria-pressed={active === id}
            title={t(labelKey)}
            className={cn(
              "flex h-11 w-11 shrink-0 items-center justify-center rounded-lg transition-colors lg:h-10 lg:w-10",
              active === id
                ? "bg-primary/15 text-primary"
                : "text-zinc-400 hover:bg-white/5 hover:text-zinc-100",
            )}
          >
            <Icon className="h-[18px] w-[18px]" />
          </button>
        ))}
        <span aria-hidden className="mx-1 hidden h-px w-6 bg-white/10 lg:block" />
        <button
          type="button"
          onClick={onOpenJson}
          title={t("video.json_mode")}
          aria-label={t("video.json_mode")}
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-zinc-400 transition-colors hover:bg-white/5 hover:text-zinc-100 lg:h-10 lg:w-10"
        >
          <Braces className="h-[18px] w-[18px]" />
        </button>
      </nav>

      {/* Quick panel */}
      <div className="min-w-0 overflow-y-auto overscroll-contain border-white/[0.06] max-lg:border-t lg:h-full lg:w-60 lg:shrink-0 lg:border-l">
        <div className="p-3">
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-500">
            {t(SECTIONS.find((s) => s.id === active)?.labelKey ?? "video.studio.rail.media")}
          </p>
          {active === "media" && (
            <MediaPanel
              scenes={scenes}
              selectedIndex={selectedIndex}
              onSelectScene={onSelectScene}
              onAddScene={onAddScene}
            />
          )}
          {active === "voice" && (
            <VoicePanel
              scene={scene}
              scenes={scenes}
              onToggleMute={(v) => onUpdateScene({ mute: v })}
              onEditNarration={onScrollToNarration}
              onSelectScene={onSelectScene}
            />
          )}
          {active === "text" && (
            <TextPanel
              disabled={layersFull}
              onAddText={() =>
                onAddLayer({ kind: "text", text: t("video.layer.new_text"), y: 0.2, font_size: 64 })
              }
              onAddCard={() =>
                onAddLayer({
                  kind: "card",
                  y: 0.3,
                  w: 0.8,
                  h: 0.18,
                  fill: "#1E293B",
                  opacity: 0.45,
                  radius: 0.03,
                  border: true,
                  anim: "up",
                  start: 0.3,
                })
              }
            />
          )}
          {active === "transitions" && (
            <TransitionsPanel
              value={scene?.transition ?? "none"}
              disabled={!scene}
              onChange={(v) => onUpdateScene({ transition: v })}
            />
          )}
          {active === "effects" && (
            <EffectsPanel scene={scene} onUpdate={onUpdateScene} />
          )}
          {active === "layers" && (
            <LayersPanel
              layerCount={layerCount}
              disabled={!scene}
              onOpenLayersTab={onOpenLayersTab}
            />
          )}
        </div>
      </div>
    </div>
  );
}

// ── Panel: Media (scene list + add) ──

function MediaPanel({
  scenes,
  selectedIndex,
  onSelectScene,
  onAddScene,
}: {
  scenes: Scene[];
  selectedIndex: number;
  onSelectScene: (i: number) => void;
  onAddScene: () => void;
}) {
  const { t } = useTranslation("toolbox");
  return (
    <div className="flex flex-col gap-1">
      {scenes.map((scene, i) => (
        <button
          key={i}
          type="button"
          onClick={() => onSelectScene(i)}
          className={cn(
            "flex min-h-11 items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm transition-colors",
            i === selectedIndex
              ? "bg-primary/15 text-primary"
              : "text-zinc-300 hover:bg-white/5",
          )}
        >
          <span className="w-5 shrink-0 text-center font-mono text-[11px] tabular-nums text-zinc-500">
            {i + 1}
          </span>
          <span className="min-w-0 flex-1 truncate">
            {scene.type === "color"
              ? t("video.type_color")
              : scene.type === "video"
                ? t("video.type_video")
                : t("video.type_image")}
          </span>
          <span className="shrink-0 font-mono text-[11px] tabular-nums text-zinc-500">
            {(Number(scene.duration_sec) || 0).toFixed(1)}s
          </span>
        </button>
      ))}
      <Button
        variant="outline"
        size="sm"
        onClick={onAddScene}
        className="mt-1 min-h-11 justify-start border-dashed border-white/15 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9"
      >
        <Plus className="h-4 w-4" />
        {t("video.add_scene")}
      </Button>
    </div>
  );
}

// ── Panel: Voice (narration shortcut + mute) ──

function VoicePanel({
  scene,
  scenes,
  onToggleMute,
  onEditNarration,
  onSelectScene,
}: {
  scene: Scene | undefined;
  scenes: Scene[];
  onToggleMute: (v: boolean) => void;
  onEditNarration: () => void;
  onSelectScene: (i: number) => void;
}) {
  const { t } = useTranslation("toolbox");
  const narrated = scenes
    .map((s, i) => ({ has: !!s.narration?.trim(), i }))
    .filter((s) => s.has);

  return (
    <div className="flex flex-col gap-3">
      <Button
        variant="outline"
        size="sm"
        onClick={onEditNarration}
        disabled={!scene}
        className="min-h-11 justify-start border-white/15 text-zinc-200 hover:bg-white/5 hover:text-white sm:min-h-9"
      >
        <Mic className="h-4 w-4" />
        {t("video.studio.rail.edit_narration")}
      </Button>

      <div className="flex items-center justify-between gap-2 rounded-lg bg-white/[0.04] px-2.5 py-2">
        <Label className="text-xs text-zinc-300">{t("video.mute")}</Label>
        <Switch
          checked={scene?.mute ?? false}
          disabled={!scene}
          onCheckedChange={onToggleMute}
          aria-label={t("video.mute")}
        />
      </div>

      <div className="flex flex-col gap-1">
        <p className="text-[11px] uppercase tracking-wider text-zinc-500">
          {t("video.studio.rail.narrated_scenes", { n: narrated.length })}
        </p>
        {narrated.length === 0 ? (
          <p className="text-xs text-zinc-500">{t("video.studio.rail.no_narration")}</p>
        ) : (
          narrated.map(({ i }) => (
            <button
              key={i}
              type="button"
              onClick={() => onSelectScene(i)}
              className="flex min-h-11 items-center gap-2 rounded-md px-2 text-left text-xs text-zinc-300 hover:bg-white/5 sm:min-h-9"
            >
              <Mic className="h-3 w-3 shrink-0 text-zinc-500" />
              {t("video.scene_n", { n: i + 1 })}
            </button>
          ))
        )}
      </div>
    </div>
  );
}

// ── Panel: Text (layer quick-insert) ──

function TextPanel({
  disabled,
  onAddText,
  onAddCard,
}: {
  disabled: boolean;
  onAddText: () => void;
  onAddCard: () => void;
}) {
  const { t } = useTranslation("toolbox");
  return (
    <div className="flex flex-col gap-2">
      <Button
        variant="outline"
        size="sm"
        onClick={onAddText}
        disabled={disabled}
        className="min-h-11 justify-start border-white/15 text-zinc-200 hover:bg-white/5 hover:text-white sm:min-h-9"
      >
        <Type className="h-4 w-4" />
        {t("video.studio.rail.add_text_layer")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        onClick={onAddCard}
        disabled={disabled}
        className="min-h-11 justify-start border-white/15 text-zinc-200 hover:bg-white/5 hover:text-white sm:min-h-9"
      >
        <Layers className="h-4 w-4" />
        {t("video.studio.rail.add_card_layer")}
      </Button>
      {disabled && (
        <p className="text-xs text-zinc-500">{t("video.studio.rail.layers_full")}</p>
      )}
      <p className="text-xs text-zinc-500">{t("video.studio.rail.text_hint")}</p>
    </div>
  );
}

// ── Panel: Transitions (selected scene enter transition) ──

function TransitionsPanel({
  value,
  disabled,
  onChange,
}: {
  value: SceneTransition;
  disabled: boolean;
  onChange: (v: SceneTransition | undefined) => void;
}) {
  const { t } = useTranslation("toolbox");
  return (
    <div className="flex flex-col gap-1">
      {TRANSITION_TYPES.map((tt) => (
        <button
          key={tt}
          type="button"
          disabled={disabled}
          onClick={() => onChange(tt === "none" ? undefined : tt)}
          className={cn(
            "flex min-h-11 items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-sm transition-colors disabled:opacity-50 sm:min-h-9",
            value === tt
              ? "bg-primary/15 text-primary"
              : "text-zinc-300 hover:bg-white/5",
          )}
        >
          <ArrowLeftRight className="h-3.5 w-3.5 shrink-0 text-zinc-500" />
          {t(`video.transition_${tt}`)}
        </button>
      ))}
      <p className="mt-1 text-xs text-zinc-500">{t("video.studio.rail.transitions_hint")}</p>
    </div>
  );
}

// ── Panel: Effects (per-scene visual toggles; scoping mirrors SceneCard) ──

function EffectsPanel({
  scene,
  onUpdate,
}: {
  scene: Scene | undefined;
  onUpdate: (patch: Partial<Scene>) => void;
}) {
  const { t } = useTranslation("toolbox");
  if (!scene) {
    return <p className="text-xs text-zinc-500">{t("video.studio.rail.no_scene")}</p>;
  }
  const isColor = scene.type === "color";
  return (
    <div className="flex flex-col gap-2">
      {!isColor && (
        <ToggleRow
          label={t("video.kb_enabled")}
          checked={scene.ken_burns !== undefined}
          onCheckedChange={(v) =>
            onUpdate({
              ken_burns: v ? { zoom_from: 1.0, zoom_to: 1.12, pan: "none" } : undefined,
            })
          }
        />
      )}
      {isColor && (
        <>
          <ToggleRow
            label={t("video.vignette")}
            checked={scene.vignette ?? false}
            onCheckedChange={(v) => onUpdate({ vignette: v || undefined })}
          />
          <ToggleRow
            label={t("video.grain")}
            checked={scene.grain ?? false}
            onCheckedChange={(v) => onUpdate({ grain: v || undefined })}
          />
        </>
      )}
      <ToggleRow
        label={isColor ? t("video.grid") : t("video.mute")}
        checked={isColor ? (scene.grid ?? false) : (scene.mute ?? false)}
        onCheckedChange={(v) =>
          isColor ? onUpdate({ grid: v || undefined }) : onUpdate({ mute: v })
        }
      />
      <p className="mt-1 text-xs text-zinc-500">{t("video.studio.rail.effects_hint")}</p>
    </div>
  );
}

function ToggleRow({
  label,
  checked,
  onCheckedChange,
}: {
  label: string;
  checked: boolean;
  onCheckedChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-lg bg-white/[0.04] px-2.5 py-2">
      <Label className="text-xs text-zinc-300">{label}</Label>
      <Switch checked={checked} onCheckedChange={onCheckedChange} aria-label={label} />
    </div>
  );
}

// ── Panel: Layers (nav to the inspector Layers tab) ──

function LayersPanel({
  layerCount,
  disabled,
  onOpenLayersTab,
}: {
  layerCount: number;
  disabled: boolean;
  onOpenLayersTab: () => void;
}) {
  const { t } = useTranslation("toolbox");
  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm text-zinc-300">
        {t("video.studio.rail.layer_count", { n: layerCount })}
      </p>
      <Button
        variant="outline"
        size="sm"
        onClick={onOpenLayersTab}
        disabled={disabled}
        className="min-h-11 justify-start border-white/15 text-zinc-200 hover:bg-white/5 hover:text-white sm:min-h-9"
      >
        <Layers className="h-4 w-4" />
        {t("video.studio.rail.open_layers")}
      </Button>
    </div>
  );
}
