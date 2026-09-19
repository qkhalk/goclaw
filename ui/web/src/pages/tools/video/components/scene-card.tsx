import { useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Trash2,
  ArrowUp,
  ArrowDown,
  Volume2,
  Square,
  Type,
  ImagePlus,
  Loader2,
  StopCircle,
  Zap,
  PanelTop,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { CloneVoiceDialog } from "./clone-voice-dialog";
import { useTtsCapabilities } from "@/api/tts-capabilities";
import { toast } from "@/stores/use-toast-store";
import { TRANSITION_TYPES } from "./scene-transition";
import { LayerTimeline } from "./layer-timeline";
import { cn } from "@/lib/utils";
import type { NarrationAudioController } from "../hooks/use-narration-audio";
import { FALLBACK_EDGE_VOICES } from "../hooks/use-narration-audio";
import type { Layer, Scene } from "../hooks/use-timeline";
import { FEATHER_ICONS } from "../hooks/use-timeline";

interface KenBurns {
  zoom_from: number;
  zoom_to: number;
  pan: "none" | "left" | "right" | "up" | "down";
}
interface Caption {
  text?: string;
  position?: "top" | "center" | "bottom";
  font_size?: number;
  style?: "" | "chip" | "mono";
}

/** Color-scene backdrop presets (gradient c0→c2, optional grid + glow orbs)
 * tuned after the reference "developer dark-mode" shorts style. */
const COLOR_PRESETS: { key: string; color: string; color2: string; grid: boolean; glow?: string }[] = [
  { key: "tech_dark", color: "#0D1117", color2: "#1E293B", grid: true, glow: "#38BDF8" },
  { key: "deep_ocean", color: "#0B1220", color2: "#1E3A8A", grid: false, glow: "#0EA5E9" },
  { key: "ember", color: "#450A0A", color2: "#B45309", grid: false, glow: "#F97316" },
  { key: "violet_night", color: "#1E1B2E", color2: "#6D28D9", grid: false, glow: "#8B5CF6" },
  { key: "plain", color: "#000000", color2: "#000000", grid: false },
];

interface SceneCardProps {
  scene: Scene;
  index: number;
  total: number;
  onUpdate: (patch: Partial<Scene>) => void;
  onRemove: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  /** Shared narration-audio controller (cache + synth). */
  narration?: NarrationAudioController;
}

export function SceneCard({
  scene,
  index,
  total,
  onUpdate,
  onRemove,
  onMoveUp,
  onMoveDown,
  narration,
}: SceneCardProps) {
  const { t } = useTranslation("toolbox");
  const [selLayer, setSelLayer] = useState(0);
  const [previewing, setPreviewing] = useState(false);
  const [cloneDialogOpen, setCloneDialogOpen] = useState(false);
  const previewElRef = useRef<HTMLAudioElement | null>(null);

  const { data: capabilities } = useTtsCapabilities();
  const edgeVoices = useMemo(() => {
    const fromApi = capabilities?.find((p) => p.provider === "edge")?.voices ?? [];
    return fromApi.length > 0 ? fromApi : FALLBACK_EDGE_VOICES;
  }, [capabilities]);
  const cloneVoices = useMemo(
    () => capabilities?.find((p) => p.provider === "clone")?.voices ?? [],
    [capabilities],
  );

  /** Play the scene's narration through real TTS (shared cache with the
   * player preview); second click stops. */
  async function toggleNarrationPreview() {
    const text = scene.narration?.trim();
    if (!text || !narration) return;
    const el = previewElRef.current;
    if (previewing && el && !el.paused) {
      el.pause();
      setPreviewing(false);
      return;
    }
    setPreviewing(true);
    try {
      const got = (await narration.prepare(text, scene.narration_voice)) ?? undefined;
      if (!got) return;
      previewElRef.current = got;
      got.onended = () => setPreviewing(false);
      got.currentTime = 0;
      await got.play();
    } catch {
      toast.error(t("video.tts_preview_failed"));
      setPreviewing(false);
    }
  }

  // Layer editing rides updateScene's undo history — every change goes
  // through onUpdate({ layers: [...] }).
  const layers = scene.layers ?? [];
  const setLayers = (next: Layer[]) => onUpdate({ layers: next.length > 0 ? next : undefined });
  const activeIdx = Math.min(selLayer, Math.max(0, layers.length - 1));
  const activeLayer = layers.length > 0 ? layers[activeIdx] : undefined;

  function addLayer(kind: Layer["kind"]) {
    const layer: Layer = { kind };
    if (kind === "text") Object.assign(layer, { text: t("video.layer.new_text"), y: 0.2, font_size: 64 });
    if (kind === "shape") Object.assign(layer, { y: 0.15, h: 0.18, fill: "#000000", opacity: 0.5 });
    if (kind === "image") Object.assign(layer, { y: 0.55, w: 0.3 });
    if (kind === "icon") Object.assign(layer, { icon: "check", w: 0.12, y: 0.3, fill: "#22C55E", chip: true, anim: "pop", start: 0.4 });
    if (kind === "card") Object.assign(layer, { y: 0.3, w: 0.8, h: 0.18, fill: "#1E293B", opacity: 0.45, radius: 0.03, border: true, anim: "up", start: 0.3 });
    setLayers([...layers, layer]);
    setSelLayer(layers.length);
  }

  const patchLayer = (patch: Partial<Layer>) =>
    setLayers(layers.map((l, j) => (j === activeIdx ? { ...l, ...patch } : l)));

  const patchLayerAt = (i: number, patch: Partial<Layer>) =>
    setLayers(layers.map((l, j) => (j === i ? { ...l, ...patch } : l)));

  const num = (v: string, fallback: number) => (v === "" ? fallback : Number(v) || fallback);

  return (
    <div className="flex flex-col gap-3 rounded-md border p-3">
      {/* Header row */}
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium">
          {t("video.scene_n", { n: index + 1 })}
        </span>
        <div className="ml-auto flex items-center gap-1">
          <Button variant="ghost" size="icon-sm" aria-label={t("video.move_up")} disabled={index === 0} onClick={onMoveUp}>
            <ArrowUp className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={t("video.move_down")} disabled={index === total - 1} onClick={onMoveDown}>
            <ArrowDown className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={t("video.remove_scene")} disabled={total === 1} onClick={onRemove} className="text-destructive hover:text-destructive">
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Type + Source + Duration */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("video.type")}</Label>
          <Select value={scene.type} onValueChange={(v) => onUpdate({ type: v as Scene["type"] })}>
            <SelectTrigger className="text-base md:text-sm" aria-label={t("video.type")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="image">{t("video.type_image")}</SelectItem>
              <SelectItem value="video">{t("video.type_video")}</SelectItem>
              <SelectItem value="color">{t("video.type_color")}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {scene.type === "color" ? (
          <>
            <div className="flex flex-col gap-1.5">
              <Label className="text-xs">{t("video.color")}</Label>
              <Input value={scene.color ?? "#000000"} onChange={(e) => onUpdate({ color: e.target.value })} placeholder="#1D4ED8" className="text-base md:text-sm" />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label className="text-xs">{t("video.color2")}</Label>
              <Input value={scene.color2 ?? ""} onChange={(e) => onUpdate({ color2: e.target.value || undefined })} placeholder={t("video.color2_hint")} className="text-base md:text-sm" />
            </div>
          </>
        ) : (
          <div className="flex flex-col gap-1.5 sm:col-span-2">
            <Label className="text-xs">{t("video.source")}</Label>
            <Input value={scene.source ?? ""} onChange={(e) => onUpdate({ source: e.target.value })} placeholder={t("video.source_hint")} className={cn("text-base md:text-sm", !scene.source?.trim() && "border-destructive/60 focus-visible:ring-destructive/30")} />
          </div>
        )}

        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("video.scene.duration")}</Label>
          <Input type="number" min={1} max={30} value={scene.duration_sec} onChange={(e) => onUpdate({ duration_sec: Number(e.target.value) || 1 })} className="text-base md:text-sm" />
        </div>
      </div>

      {/* Color-scene backdrop: animated-gradient presets + blueprint grid.
          Mirrors the worker's lavfi gradients (c0→c1) + drawgrid overlay. */}
      {scene.type === "color" && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted-foreground">{t("video.color_presets")}</span>
          {COLOR_PRESETS.map((p) => (
            <button
              key={p.key}
              type="button"
              title={t(`video.preset_${p.key}`)}
              onClick={() =>
                onUpdate({
                  color: p.color,
                  ...(p.key === "plain"
                    ? { color2: undefined, grid: undefined, glow: undefined, vignette: undefined }
                    : { color2: p.color2, grid: p.grid, glow: p.glow, vignette: true }),
                })
              }
              className={cn(
                "h-7 w-12 rounded border transition-transform hover:scale-105",
                scene.color?.toLowerCase() === p.color.toLowerCase() &&
                  scene.color2?.toLowerCase() === p.color2.toLowerCase() &&
                  !!scene.grid === p.grid
                  ? "border-primary ring-2 ring-primary/40"
                  : "border-border",
              )}
              style={{ background: `linear-gradient(135deg, ${p.color}, ${p.color2})` }}
              aria-label={t(`video.preset_${p.key}`)}
            />
          ))}
          <div className="ml-2 flex items-center gap-2">
            <Switch
              id={`grid-${index}`}
              checked={scene.grid ?? false}
              onCheckedChange={(v) => onUpdate({ grid: v || undefined })}
            />
            <Label htmlFor={`grid-${index}`} className="text-xs">
              {t("video.grid")}
            </Label>
          </div>
        </div>
      )}

      {/* Visual v2 (color scenes): glow-orb tint, vignette, film grain —
          painted by the browser preview and burned in by the server worker. */}
      {scene.type === "color" && (
        <div className="flex flex-wrap items-end gap-4">
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">{t("video.glow")}</Label>
            <div className="flex items-center gap-1.5">
              <input
                type="color"
                aria-label={t("video.glow")}
                value={scene.glow ?? "#38BDF8"}
                onChange={(e) => onUpdate({ glow: e.target.value.toUpperCase() })}
                className="h-8 w-10 cursor-pointer rounded border bg-transparent p-0.5"
              />
              <Button
                variant="ghost"
                size="sm"
                className="min-h-9 px-2 text-xs"
                onClick={() => onUpdate({ glow: undefined })}
              >
                {t("video.glow_off")}
              </Button>
            </div>
          </div>
          <div className="flex items-center gap-2 pb-1.5">
            <Switch
              id={`vig-${index}`}
              checked={scene.vignette ?? false}
              onCheckedChange={(v) => onUpdate({ vignette: v || undefined })}
            />
            <Label htmlFor={`vig-${index}`} className="text-xs">
              {t("video.vignette")}
            </Label>
          </div>
          <div className="flex items-center gap-2 pb-1.5">
            <Switch
              id={`grain-${index}`}
              checked={scene.grain ?? false}
              onCheckedChange={(v) => onUpdate({ grain: v || undefined })}
            />
            <Label htmlFor={`grain-${index}`} className="text-xs">
              {t("video.grain")}
            </Label>
          </div>
        </div>
      )}

      {/* Ken Burns + Mute toggles */}
      {scene.type !== "color" && (
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex items-center gap-2">
            <Switch id={`kb-${index}`} checked={scene.ken_burns !== undefined} onCheckedChange={(v) => onUpdate({ ken_burns: v ? { zoom_from: 1.0, zoom_to: 1.12, pan: "none" } : undefined })} />
            <Label htmlFor={`kb-${index}`} className="text-xs">{t("video.kb_enabled")}</Label>
          </div>
          <div className="flex items-center gap-2">
            <Switch id={`mute-${index}`} checked={scene.mute ?? false} onCheckedChange={(v) => onUpdate({ mute: v })} />
            <Label htmlFor={`mute-${index}`} className="text-xs">{t("video.mute")}</Label>
          </div>
          {scene.ken_burns && (
            <>
              <div className="flex items-center gap-1.5">
                <Label className="text-xs">{t("video.kb_zoom_from")}</Label>
                <Input type="number" step={0.01} min={1} max={2} value={scene.ken_burns.zoom_from} onChange={(e) => onUpdate({ ken_burns: { ...scene.ken_burns!, zoom_from: Number(e.target.value) || 1 } })} className="h-8 w-20 text-base md:text-sm" />
                <Label className="text-xs">{t("video.kb_zoom_to")}</Label>
                <Input type="number" step={0.01} min={1} max={2} value={scene.ken_burns.zoom_to} onChange={(e) => onUpdate({ ken_burns: { ...scene.ken_burns!, zoom_to: Number(e.target.value) || 1 } })} className="h-8 w-20 text-base md:text-sm" />
              </div>
              <div className="flex items-center gap-1.5">
                <Label className="text-xs">{t("video.kb_pan")}</Label>
                <Select value={scene.ken_burns.pan} onValueChange={(v) => onUpdate({ ken_burns: { ...scene.ken_burns!, pan: v as KenBurns["pan"] } })}>
                  <SelectTrigger className="h-8 text-base md:text-sm" aria-label={t("video.kb_pan")}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(["none", "left", "right", "up", "down"] as const).map((p) => (
                      <SelectItem key={p} value={p}>{t(`video.kb_pan_${p}`)}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </>
          )}
        </div>
      )}

      {/* Caption */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
        <div className="flex flex-col gap-1.5 sm:col-span-2">
          <Label className="text-xs">{t("video.caption_text")}</Label>
          <Input value={scene.caption?.text ?? ""} onChange={(e) => onUpdate({ caption: e.target.value ? { ...(scene.caption ?? { position: "bottom" as const }), text: e.target.value } : undefined })} className="text-base md:text-sm" />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("video.caption_position")}</Label>
          <Select value={scene.caption?.position ?? "bottom"} onValueChange={(v) => onUpdate({ caption: scene.caption ? { ...scene.caption, position: v as Caption["position"] } : undefined })}>
            <SelectTrigger className="text-base md:text-sm" aria-label={t("video.caption_position")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="top">{t("video.pos_top")}</SelectItem>
              <SelectItem value="center">{t("video.pos_center")}</SelectItem>
              <SelectItem value="bottom">{t("video.pos_bottom")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("video.caption_style")}</Label>
          <Select
            value={scene.caption?.style || "plain"}
            onValueChange={(v) =>
              onUpdate({
                caption: scene.caption
                  ? { ...scene.caption, style: v === "plain" ? undefined : (v as "chip" | "mono") }
                  : undefined,
              })
            }
          >
            <SelectTrigger className="text-base md:text-sm" aria-label={t("video.caption_style")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="plain">{t("video.caption_style_plain")}</SelectItem>
              <SelectItem value="chip">{t("video.caption_style_chip")}</SelectItem>
              <SelectItem value="mono">{t("video.caption_style_mono")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* OpenCut-style transform + color grading (image/video scenes only) */}
      {(scene.type === "image" || scene.type === "video") && (
        <details className="rounded-md border p-3">
          <summary className="cursor-pointer text-sm font-medium">{t("video.advanced")}</summary>
          <div className="mt-3 grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
            {([
              ["video.transform_scale", "scale", 0.3, 3, 0.05, scene.transform?.scale ?? 1],
              ["video.transform_pos_x", "x", -50, 50, 1, scene.transform?.x ?? 0],
              ["video.transform_pos_y", "y", -50, 50, 1, scene.transform?.y ?? 0],
              ["video.transform_rotate", "rotate", -180, 180, 1, scene.transform?.rotate ?? 0],
              ["video.transform_opacity", "opacity", 0, 1, 0.05, scene.transform?.opacity ?? 1],
              ["video.filter_brightness", "brightness", 0.2, 2, 0.05, scene.filter?.brightness ?? 1],
              ["video.filter_contrast", "contrast", 0.2, 2, 0.05, scene.filter?.contrast ?? 1],
              ["video.filter_saturate", "saturate", 0, 2, 0.05, scene.filter?.saturate ?? 1],
              ["video.filter_blur", "blur", 0, 20, 0.5, scene.filter?.blur ?? 0],
            ] as const).map(([labelKey, key, min, max, step, value]) => (
              <div key={key} className="flex flex-col gap-1">
                <div className="flex items-center justify-between">
                  <Label className="text-xs">{t(labelKey)}</Label>
                  <span className="text-xs tabular-nums text-muted-foreground">{value}</span>
                </div>
                <input
                  type="range"
                  min={min}
                  max={max}
                  step={step}
                  value={value}
                  onChange={(e) => {
                    const v = +e.target.value;
                    if (key === "brightness" || key === "contrast" || key === "saturate" || key === "blur") {
                      const next = { ...scene.filter };
                      if (v === (key === "blur" ? 0 : 1)) delete next[key];
                      else next[key] = v;
                      const empty = Object.keys(next).length === 0;
                      onUpdate({ filter: empty ? undefined : next });
                    } else {
                      const next = { ...scene.transform };
                      if (v === (key === "opacity" ? 1 : key === "scale" ? 1 : 0)) delete next[key as "scale" | "x" | "y" | "rotate" | "opacity"];
                      else next[key as "scale" | "x" | "y" | "rotate" | "opacity"] = v;
                      const empty = Object.keys(next).length === 0;
                      onUpdate({ transform: empty ? undefined : next });
                    }
                  }}
                  className="w-full accent-primary"
                />
              </div>
            ))}
          </div>
          <p className="mt-2 text-xs text-muted-foreground">{t("video.advanced_hint")}</p>
        </details>
      )}

      {/* Enter transition — honored by the browser preview, client export,
          and the server render (xfade). */}
      <div className="flex flex-col gap-1.5 sm:max-w-xs">
        <Label className="text-xs">{t("video.transition")}</Label>
        <Select
          value={scene.transition ?? "none"}
          onValueChange={(v) => onUpdate({ transition: v === "none" ? undefined : (v as Scene["transition"]) })}
        >
          <SelectTrigger className="text-base md:text-sm" aria-label={t("video.transition")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TRANSITION_TYPES.map((type) => (
              <SelectItem key={type} value={type}>
                {t("video.transition_" + type)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Timed overlay layers — drawn by the browser preview AND the server
          renderer (drawtext/drawbox/overlay with enable windows). */}
      <details className="rounded-md border p-3" open={layers.length > 0}>
        <summary className="cursor-pointer text-sm font-medium">
          {t("video.layers_title")}
          {layers.length > 0 && (
            <span className="ml-1.5 text-xs tabular-nums text-muted-foreground">({layers.length})</span>
          )}
        </summary>
        <div className="mt-3 flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => addLayer("text")} disabled={layers.length >= 8}>
              <Type className="mr-1.5 h-3.5 w-3.5" />
              {t("video.layers_add_text")}
            </Button>
            <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => addLayer("shape")} disabled={layers.length >= 8}>
              <Square className="mr-1.5 h-3.5 w-3.5" />
              {t("video.layers_add_shape")}
            </Button>
            <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => addLayer("image")} disabled={layers.length >= 8}>
              <ImagePlus className="mr-1.5 h-3.5 w-3.5" />
              {t("video.layers_add_image")}
            </Button>
            <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => addLayer("icon")} disabled={layers.length >= 8}>
              <Zap className="mr-1.5 h-3.5 w-3.5" />
              {t("video.layers_add_icon")}
            </Button>
            <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => addLayer("card")} disabled={layers.length >= 8}>
              <PanelTop className="mr-1.5 h-3.5 w-3.5" />
              {t("video.layers_add_card")}
            </Button>
          </div>

          <LayerTimeline
            layers={layers}
            sceneSec={scene.duration_sec}
            selected={activeIdx}
            onSelect={setSelLayer}
            onChange={(i, timing) => patchLayerAt(i, timing)}
          />

          {activeLayer && (
            <div className="flex flex-col gap-3 rounded-md border bg-muted/20 p-2.5">
              <div className="flex items-center gap-1.5">
                <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {t(`video.layer.kind_${activeLayer.kind}`)}
                </span>
                <div className="ml-auto flex items-center gap-0.5">
                  <Button variant="ghost" size="icon-sm" aria-label={t("video.layer_move_up")} disabled={activeIdx === 0} onClick={() => {
                    const to = activeIdx - 1;
                    const next = [...layers];
                    const [m] = next.splice(activeIdx, 1);
                    if (!m) return;
                    next.splice(to, 0, m);
                    setLayers(next);
                    setSelLayer(to);
                  }}>
                    <ArrowUp className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label={t("video.layer_move_down")} disabled={activeIdx === layers.length - 1} onClick={() => {
                    const to = activeIdx + 1;
                    const next = [...layers];
                    const [m] = next.splice(activeIdx, 1);
                    if (!m) return;
                    next.splice(to, 0, m);
                    setLayers(next);
                    setSelLayer(to);
                  }}>
                    <ArrowDown className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label={t("video.layer_delete")} onClick={() => {
                    setLayers(layers.filter((_, j) => j !== activeIdx));
                    setSelLayer(0);
                  }} className="text-destructive hover:text-destructive">
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>

              {activeLayer.kind === "text" && (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-xs">{t("video.layer.text")}</Label>
                  <Textarea rows={2} value={activeLayer.text ?? ""} onChange={(e) => patchLayer({ text: e.target.value })} className="text-base md:text-sm" />
                </div>
              )}
              {activeLayer.kind === "image" && (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-xs">{t("video.layer.source")}</Label>
                  <Input value={activeLayer.source ?? ""} onChange={(e) => patchLayer({ source: e.target.value })} placeholder="media/logo.png | https://..." className="text-base md:text-sm" />
                </div>
              )}
              {activeLayer.kind === "icon" && (
                <div className="flex flex-wrap items-center gap-4">
                  <div className="flex items-center gap-1.5">
                    <Label className="text-xs">{t("video.layer.icon")}</Label>
                    <Select value={activeLayer.icon ?? "check"} onValueChange={(v) => patchLayer({ icon: v })}>
                      <SelectTrigger className="h-8 w-40 text-base md:text-sm" aria-label={t("video.layer.icon")}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {FEATHER_ICONS.map((name) => (
                          <SelectItem key={name} value={name}>
                            {name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch id={`chip-${activeIdx}`} checked={activeLayer.chip ?? false} onCheckedChange={(v) => patchLayer({ chip: v || undefined })} />
                    <Label htmlFor={`chip-${activeIdx}`} className="text-xs">{t("video.layer.chip")}</Label>
                  </div>
                </div>
              )}
              {activeLayer.kind === "card" && (
                <div className="flex flex-wrap items-center gap-4">
                  <div className="flex items-center gap-1.5">
                    <Label className="text-xs">{t("video.layer.radius")}</Label>
                    <Input type="number" min={0} max={0.2} step={0.005} value={activeLayer.radius ?? 0.018} onChange={(e) => patchLayer({ radius: Math.max(0, Math.min(0.2, Number(e.target.value) || 0)) })} className="h-8 w-24 text-base md:text-sm" />
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch id={`border-${activeIdx}`} checked={activeLayer.border ?? false} onCheckedChange={(v) => patchLayer({ border: v || undefined })} />
                    <Label htmlFor={`border-${activeIdx}`} className="text-xs">{t("video.layer.border")}</Label>
                  </div>
                </div>
              )}

              <div className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
                {([
                  ["video.layer.start", "start", activeLayer.start ?? 0],
                  ["video.layer.duration", "duration", activeLayer.duration ?? 0],
                  ["video.layer.x", "x", activeLayer.x ?? 0.1],
                  ["video.layer.y", "y", activeLayer.y ?? 0.1],
                  ...(activeLayer.kind === "shape" || activeLayer.kind === "card" ? [["video.layer.w", "w", activeLayer.w ?? 0.8], ["video.layer.h", "h", activeLayer.h ?? 0.3]] : []),
                  ...(activeLayer.kind === "icon" ? [["video.layer.w", "w", activeLayer.w ?? 0.12]] : []),
                ] as [string, "start" | "duration" | "x" | "y" | "w" | "h", number][]).map(([labelKey, key, value]) => (
                  <div key={key} className="flex items-center gap-1.5">
                    <Label className="w-16 shrink-0 text-xs">{t(labelKey)}</Label>
                    <Input
                      type="number"
                      min={0}
                      step={0.1}
                      value={value}
                      onChange={(e) => patchLayer({ [key]: num(e.target.value, value) } as Partial<Layer>)}
                      className="h-8 text-base md:text-sm"
                    />
                  </div>
                ))}
                {activeLayer.kind !== "image" && (
                  <div className="flex items-center gap-1.5">
                    <Label className="w-16 shrink-0 text-xs">{t("video.layer.fill")}</Label>
                    <Input value={activeLayer.fill ?? "#FFFFFF"} onChange={(e) => patchLayer({ fill: e.target.value })} placeholder="#FFFFFF" className="h-8 text-base md:text-sm" />
                  </div>
                )}
                {activeLayer.kind === "text" && (
                  <>
                    <div className="flex items-center gap-1.5">
                      <Label className="w-16 shrink-0 text-xs">{t("video.layer.font_size")}</Label>
                      <Input type="number" min={8} max={300} value={activeLayer.font_size ?? 48} onChange={(e) => patchLayer({ font_size: Number(e.target.value) || 48 })} className="h-8 text-base md:text-sm" />
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Label className="w-16 shrink-0 text-xs">{t("video.layer.align")}</Label>
                      <Select value={activeLayer.align ?? "center"} onValueChange={(v) => patchLayer({ align: v as Layer["align"] })}>
                        <SelectTrigger className="h-8 text-base md:text-sm" aria-label={t("video.layer.align")}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="left">{t("video.layer.align_left")}</SelectItem>
                          <SelectItem value="center">{t("video.layer.align_center")}</SelectItem>
                          <SelectItem value="right">{t("video.layer.align_right")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Label className="w-16 shrink-0 text-xs">{t("video.layer.font")}</Label>
                      <Select value={activeLayer.font || "body"} onValueChange={(v) => patchLayer({ font: v === "body" ? undefined : (v as Layer["font"]) })}>
                        <SelectTrigger className="h-8 text-base md:text-sm" aria-label={t("video.layer.font")}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="body">{t("video.layer.font_body")}</SelectItem>
                          <SelectItem value="display">{t("video.layer.font_display")}</SelectItem>
                          <SelectItem value="mono">{t("video.layer.font_mono")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </>
                )}
                {activeLayer.kind !== "image" && (
                  <div className="flex items-center gap-1.5">
                    <Label className="w-16 shrink-0 text-xs">{t("video.layer.anim")}</Label>
                    <Select value={activeLayer.anim || "none"} onValueChange={(v) => patchLayer({ anim: v === "none" ? undefined : (v as Layer["anim"]) })}>
                      <SelectTrigger className="h-8 text-base md:text-sm" aria-label={t("video.layer.anim")}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">{t("video.layer.anim_none")}</SelectItem>
                        <SelectItem value="fade">{t("video.layer.anim_fade")}</SelectItem>
                        <SelectItem value="up">{t("video.layer.anim_up")}</SelectItem>
                        <SelectItem value="down">{t("video.layer.anim_down")}</SelectItem>
                        <SelectItem value="left">{t("video.layer.anim_left")}</SelectItem>
                        <SelectItem value="right">{t("video.layer.anim_right")}</SelectItem>
                        <SelectItem value="pop">{t("video.layer.anim_pop")}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                )}
                <div className="flex items-center gap-1.5">
                  <Label className="w-16 shrink-0 text-xs">{t("video.layer.opacity")}</Label>
                  <Input
                    type="number"
                    min={0}
                    max={1}
                    step={0.05}
                    value={activeLayer.opacity ?? 1}
                    onChange={(e) => patchLayer({ opacity: Math.max(0, Math.min(1, Number(e.target.value))) })}
                    className="h-8 text-base md:text-sm"
                  />
                </div>
              </div>
              {activeLayer.kind === "text" && (
                <div className="flex items-center gap-1.5">
                  <Label className="w-16 shrink-0 text-xs">{t("video.layer.w")}</Label>
                  <Input
                    type="number"
                    min={0.01}
                    max={1}
                    step={0.01}
                    value={activeLayer.w ?? 0.8}
                    onChange={(e) => patchLayer({ w: num(e.target.value, 0.8) })}
                    className="h-8 w-24 text-base md:text-sm"
                  />
                </div>
              )}
            </div>
          )}
          <p className="text-xs text-muted-foreground">{t("video.layers_hint")}</p>
        </div>
      </details>

      {/* Narration pacing fit. With a synthesized clip we know the REAL
          audio duration; before that, estimate ~2.3 words/sec (VN-normalized).
          A narration longer than the scene is what makes voice/text feel
          mismatched — surface it and offer one-tap fit. (The server also
          auto-extends scenes to fit narration on render.) */}
      {(() => {
        const text = (scene.narration ?? "").trim();
        if (!text) return null;
        const audioDur = narration?.durationOf(text, scene.narration_voice);
        const words = text.split(/\s+/).filter(Boolean).length;
        const est = audioDur ?? words / 2.3;
        const dur = Number(scene.duration_sec) || 0;
        if (est <= dur + 0.5) return null;
        return (
          <div className="flex flex-wrap items-center gap-2 rounded-md border border-amber-500/40 bg-amber-500/5 px-2.5 py-1.5 text-xs text-amber-600 dark:text-amber-400">
            <span>
              {audioDur !== undefined
                ? t("video.narration_fit_measured", { sec: audioDur.toFixed(1), dur })
                : t("video.narration_fit_hint", { est: Math.ceil(est), dur })}
            </span>
            <button
              type="button"
              onClick={() =>
                audioDur !== undefined
                  ? onUpdate({ duration_sec: Math.ceil(audioDur + 0.35) })
                  : onUpdate({ duration_sec: Math.ceil(est) })
              }
              className="ml-auto rounded border border-amber-500/50 px-1.5 py-0.5 font-medium transition-colors hover:bg-amber-500/10"
            >
              {t("video.narration_fit_apply", { est: Math.ceil(est) })}
            </button>
          </div>
        );
      })()}

      {/* Narration (TTS) */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
        <div className="flex flex-col gap-1.5 sm:col-span-2">
          <Label className="text-xs">{t("video.scene.narration")}</Label>
          <Textarea value={scene.narration ?? ""} onChange={(e) => onUpdate({ narration: e.target.value || undefined })} rows={2} placeholder={t("video.narration_placeholder")} className="text-base md:text-sm" />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("video.scene.voice")}</Label>
          <Select
            value={scene.narration_voice ?? "default"}
            onValueChange={(v) => {
              // The register entry is an action, not a voice — open the
              // dialog and keep the current selection.
              if (v === "__register__") {
                setCloneDialogOpen(true);
                return;
              }
              onUpdate({ narration_voice: v === "default" ? undefined : v });
            }}
          >
            <SelectTrigger className="text-base md:text-sm" aria-label={t("video.scene.voice")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="default">{t("video.voice_default")}</SelectItem>
              {edgeVoices.map((v) => (
                <SelectItem key={v.voice_id} value={v.voice_id}>
                  {v.name || v.voice_id}
                </SelectItem>
              ))}
              {cloneVoices.length > 0 && (
                <>
                  <SelectSeparator />
                  <SelectGroup>
                    <SelectLabel>{t("video.clone_voice.group")}</SelectLabel>
                    {cloneVoices.map((v) => (
                      <SelectItem key={v.voice_id} value={`clone:${v.voice_id}`}>
                        {v.name || v.voice_id}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </>
              )}
              <SelectSeparator />
              <SelectItem value="__register__" className="text-primary">
                {t("video.clone_voice.add_item")}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("video.scene.preview_tts")}</Label>
          <Button
            variant="outline"
            size="sm"
            onClick={toggleNarrationPreview}
            disabled={!scene.narration?.trim() || !narration || (narration.isLoading(scene.narration ?? "", scene.narration_voice) && !previewing)}
            className="min-h-11 sm:min-h-9"
          >
            {narration?.isLoading(scene.narration ?? "", scene.narration_voice) && !previewing ? (
              <>
                <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                {t("video.tts_loading")}
              </>
            ) : previewing ? (
              <>
                <StopCircle className="mr-2 h-3.5 w-3.5" />
                {t("video.preview_stop")}
              </>
            ) : (
              <>
                <Volume2 className="mr-2 h-3.5 w-3.5" />
                {t("video.scene.preview_tts")}
              </>
            )}
          </Button>
          {scene.narration && scene.narration.trim().length > 500 && (
            <p className="text-xs text-muted-foreground">{t("video.tts_truncated")}</p>
          )}
        </div>
      </div>
      <CloneVoiceDialog
        open={cloneDialogOpen}
        onOpenChange={setCloneDialogOpen}
        onRegistered={(voiceId) => onUpdate({ narration_voice: voiceId })}
      />
    </div>
  );
}
