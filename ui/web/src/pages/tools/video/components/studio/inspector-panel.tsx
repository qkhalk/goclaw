import { useTranslation } from "react-i18next";
import { Layers as LayersIcon, Plus, Trash2, type LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { LayerTimeline } from "../layer-timeline";
import type { Layer, Scene } from "../../hooks/use-timeline";

// ── Inspector chrome (tabs) ──

export type InspectorTab = "scene" | "layers" | "render";

interface InspectorPanelProps {
  tab: InspectorTab;
  onTabChange: (tab: InspectorTab) => void;
  /** Existing SceneCard editor, composed by the page. */
  sceneTab: React.ReactNode;
  /** Layer timing quick editor for the selected scene. */
  layersTab: React.ReactNode;
  /** Existing RenderPanel content, composed by the page. */
  renderTab: React.ReactNode;
}

/**
 * Filmora-style right inspector: Scene | Layers | Render tabs inside a
 * scrollable panel. The page composes each tab's content from the existing
 * editors; this component only draws the chrome.
 */
export function InspectorPanel({
  tab,
  onTabChange,
  sceneTab,
  layersTab,
  renderTab,
}: InspectorPanelProps) {
  const { t } = useTranslation("toolbox");
  return (
    <Tabs
      value={tab}
      onValueChange={(v) => onTabChange(v as InspectorTab)}
      className="flex min-h-0 flex-1 flex-col gap-0"
    >
      <TabsList className="h-10 shrink-0 justify-start gap-1 rounded-none border-b border-white/[0.06] bg-[#1b1d23] p-1">
        <TabsTrigger
          value="scene"
          className="min-h-9 rounded-md px-3 text-xs data-[state=active]:bg-white/10 data-[state=active]:text-white"
        >
          {t("video.studio.inspector.scene")}
        </TabsTrigger>
        <TabsTrigger
          value="layers"
          className="min-h-9 rounded-md px-3 text-xs data-[state=active]:bg-white/10 data-[state=active]:text-white"
        >
          {t("video.studio.inspector.layers")}
        </TabsTrigger>
        <TabsTrigger
          value="render"
          className="min-h-9 rounded-md px-3 text-xs data-[state=active]:bg-white/10 data-[state=active]:text-white"
        >
          {t("video.studio.inspector.render")}
        </TabsTrigger>
      </TabsList>
      <TabsContent
        value="scene"
        className="min-h-0 flex-1 lg:overflow-y-auto lg:overscroll-contain"
      >
        {sceneTab}
      </TabsContent>
      <TabsContent
        value="layers"
        className="min-h-0 flex-1 lg:overflow-y-auto lg:overscroll-contain"
      >
        {layersTab}
      </TabsContent>
      <TabsContent
        value="render"
        className="min-h-0 flex-1 lg:overflow-y-auto lg:overscroll-contain"
      >
        {renderTab}
      </TabsContent>
    </Tabs>
  );
}

// ── Layers tab content ──

const MAX_LAYERS = 8;

const LAYER_KINDS: Array<{ kind: Layer["kind"]; icon: LucideIcon; labelKey: string }> = [
  { kind: "text", icon: LayersIcon, labelKey: "video.layer.kind_text" },
  { kind: "shape", icon: LayersIcon, labelKey: "video.layer.kind_shape" },
  { kind: "image", icon: LayersIcon, labelKey: "video.layer.kind_image" },
  { kind: "icon", icon: LayersIcon, labelKey: "video.layer.kind_icon" },
  { kind: "card", icon: LayersIcon, labelKey: "video.layer.kind_card" },
];

/** Default layer shapes mirror SceneCard's addLayer() defaults. */
function defaultLayer(kind: Layer["kind"], newText: string): Layer {
  const layer: Layer = { kind };
  if (kind === "text") Object.assign(layer, { text: newText, y: 0.2, font_size: 64 });
  if (kind === "shape") Object.assign(layer, { y: 0.15, h: 0.18, fill: "#000000", opacity: 0.5 });
  if (kind === "image") Object.assign(layer, { y: 0.55, w: 0.3 });
  if (kind === "icon") Object.assign(layer, { icon: "check", w: 0.12, y: 0.3, fill: "#22C55E", chip: true, anim: "pop" as const, start: 0.4 });
  if (kind === "card") Object.assign(layer, { y: 0.3, w: 0.8, h: 0.18, fill: "#1E293B", opacity: 0.45, radius: 0.03, border: true, anim: "up" as const, start: 0.3 });
  return layer;
}

interface LayersQuickPanelProps {
  scene: Scene | undefined;
  selectedLayer: number;
  onSelectLayer: (index: number) => void;
  onAddLayer: (layer: Layer) => void;
  onUpdateLayerTiming: (
    layerIndex: number,
    timing: { start?: number; duration?: number },
  ) => void;
  onRemoveLayer: (layerIndex: number) => void;
  /** Default caption for new text layers (i18n, mirrors SceneCard). */
  defaultText: string;
}

/**
 * Layer timing strip for the SELECTED scene, built on the existing
 * LayerTimeline component plus the timeline's add/update/remove layer
 * handlers. Full per-layer property editing stays in the Scene tab.
 */
export function LayersQuickPanel({
  scene,
  selectedLayer,
  onSelectLayer,
  onAddLayer,
  onUpdateLayerTiming,
  onRemoveLayer,
  defaultText,
}: LayersQuickPanelProps) {
  const { t } = useTranslation("toolbox");
  const layers = scene?.layers ?? [];
  const active = layers[selectedLayer];
  const clampedSelected = Math.min(selectedLayer, Math.max(0, layers.length - 1));

  return (
    <div className="flex flex-col gap-3 p-3">
      <div className="flex items-center gap-2">
        <LayersIcon className="h-4 w-4 shrink-0 text-zinc-500" />
        <span className="text-xs uppercase tracking-wider text-zinc-500">
          {t("video.studio.inspector.layers_of", { n: layers.length })}
        </span>
      </div>

      {layers.length === 0 ? (
        <p className="rounded-lg bg-white/[0.04] p-3 text-xs text-zinc-400">
          {t("video.studio.inspector.no_layers")}
        </p>
      ) : (
        <>
          <LayerTimeline
            layers={layers}
            sceneSec={scene?.duration_sec || 1}
            selected={clampedSelected}
            onSelect={onSelectLayer}
            onChange={onUpdateLayerTiming}
          />
          {active && (
            <div className="flex items-center gap-2 rounded-lg bg-white/[0.04] px-2.5 py-2">
              <span className="text-xs font-medium uppercase tracking-wide text-zinc-300">
                {t(`video.layer.kind_${active.kind}`)}
              </span>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => onRemoveLayer(clampedSelected)}
                aria-label={t("video.studio.inspector.remove_layer")}
                title={t("video.studio.inspector.remove_layer")}
                className="ml-auto min-h-9 min-w-9 text-destructive hover:text-destructive"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          )}
        </>
      )}

      <div className="flex flex-col gap-1.5">
        <p className="text-[11px] uppercase tracking-wider text-zinc-500">
          {t("video.studio.inspector.add_layer")}
        </p>
        <div className="flex flex-wrap gap-1.5">
          {LAYER_KINDS.map(({ kind, labelKey }) => (
            <Button
              key={kind}
              variant="outline"
              size="xs"
              disabled={layers.length >= MAX_LAYERS}
              onClick={() => onAddLayer(defaultLayer(kind, defaultText))}
              className="min-h-11 border-white/15 text-xs text-zinc-200 hover:bg-white/5 hover:text-white sm:min-h-9"
            >
              <Plus className="h-3 w-3" />
              {t(labelKey)}
            </Button>
          ))}
        </div>
        {layers.length >= MAX_LAYERS && (
          <p className="text-xs text-zinc-500">{t("video.studio.rail.layers_full")}</p>
        )}
        <p className="text-xs text-zinc-500">{t("video.studio.inspector.layers_hint")}</p>
      </div>
    </div>
  );
}
