import { useTranslation } from "react-i18next";
import {
  Trash2,
  ArrowUp,
  ArrowDown,
  Volume2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { Scene } from "../hooks/use-timeline";

interface KenBurns {
  zoom_from: number;
  zoom_to: number;
  pan: "none" | "left" | "right" | "up" | "down";
}
interface Caption {
  text: string;
  position?: "top" | "center" | "bottom";
  font_size?: number;
}

function previewTTS(text: string, voice?: string) {
  if (!text) return;
  const utterance = new SpeechSynthesisUtterance(text);
  if (voice) {
    const voices = speechSynthesis.getVoices();
    const match = voices.find(
      (v) => v.name.includes(voice) || v.lang.startsWith(voice),
    );
    if (match) utterance.voice = match;
  }
  speechSynthesis.cancel();
  speechSynthesis.speak(utterance);
}

interface SceneCardProps {
  scene: Scene;
  index: number;
  total: number;
  onUpdate: (patch: Partial<Scene>) => void;
  onRemove: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
}

export function SceneCard({
  scene,
  index,
  total,
  onUpdate,
  onRemove,
  onMoveUp,
  onMoveDown,
}: SceneCardProps) {
  const { t } = useTranslation("toolbox");

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
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">{t("video.color")}</Label>
            <Input value={scene.color ?? "#000000"} onChange={(e) => onUpdate({ color: e.target.value })} placeholder="#1D4ED8" className="text-base md:text-sm" />
          </div>
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
        <div className="flex flex-col gap-1.5 sm:col-span-3">
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
      </div>

      {/* Narration (TTS) */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
        <div className="flex flex-col gap-1.5 sm:col-span-3">
          <Label className="text-xs">{t("video.scene.narration")}</Label>
          <Textarea value={scene.narration ?? ""} onChange={(e) => onUpdate({ narration: e.target.value || undefined })} rows={2} placeholder={t("video.narration_placeholder")} className="text-base md:text-sm" />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("video.scene.voice")}</Label>
          <Button variant="outline" size="sm" onClick={() => previewTTS(scene.narration || "")} disabled={!scene.narration?.trim()} className="min-h-11 sm:min-h-9">
            <Volume2 className="mr-2 h-3.5 w-3.5" />
            {t("video.scene.preview_tts")}
          </Button>
        </div>
      </div>
    </div>
  );
}
