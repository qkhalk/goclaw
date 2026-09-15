import { useTranslation } from "react-i18next";
import {
  Monitor,
  Cloud,
  Zap,
  CheckCircle2,
  AlertTriangle,
  Loader2,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import type { HardwareInfo } from "../hooks/use-video-export";
import type { ExportMethod } from "../hooks/use-webcodecs-export";
import type { Scene } from "../hooks/use-timeline";

// ── Types ──

interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

interface RenderPanelProps {
  storyboard: Storyboard;
  onStoryboardChange: (sb: Storyboard) => void;
  hardware: HardwareInfo;
  isExporting: boolean;
  progress: number;
  onExportClient: () => void;
  onExportServer: () => void;
  onCancel: () => void;
  webCodecsSupported: boolean;
  exportMethod: ExportMethod;
  onExportMethodChange: (m: ExportMethod) => void;
  onExportWebCodecs: () => void;
  isOverloaded: boolean;
}

const ASPECTS = {
  "9:16": { width: 1080, height: 1920 },
  "16:9": { width: 1920, height: 1080 },
  "1:1": { width: 1080, height: 1080 },
} as const;
type Aspect = keyof typeof ASPECTS;

function aspectOf(canvas: Storyboard["canvas"]): Aspect | "custom" {
  for (const [key, dim] of Object.entries(ASPECTS)) {
    if (canvas.width === dim.width && canvas.height === dim.height) return key as Aspect;
  }
  return "custom";
}

export function RenderPanel({
  storyboard,
  onStoryboardChange,
  hardware,
  isExporting,
  progress,
  onExportClient,
  onExportServer,
  onCancel,
  webCodecsSupported,
  exportMethod,
  onExportMethodChange,
  onExportWebCodecs,
  isOverloaded,
}: RenderPanelProps) {
  const { t } = useTranslation("toolbox");

  const totalSec = storyboard.scenes.reduce(
    (acc, s) => acc + (Number(s.duration_sec) || 0),
    0,
  );

  return (
    <div className="flex flex-col gap-4">
      {/* Render Settings */}
      <div className="rounded-lg border p-4">
        <p className="mb-3 text-sm font-medium">{t("video.render_panel.title")}</p>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">{t("video.aspect")}</Label>
            <Select
              value={aspectOf(storyboard.canvas)}
              onValueChange={(v) => {
                if (v !== "custom" && ASPECTS[v as Aspect]) {
                  onStoryboardChange({
                    ...storyboard,
                    canvas: { ...storyboard.canvas, ...ASPECTS[v as Aspect] },
                  });
                }
              }}
            >
              <SelectTrigger className="text-base md:text-sm" aria-label={t("video.aspect")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(Object.keys(ASPECTS) as Aspect[]).map((a) => (
                  <SelectItem key={a} value={a}>
                    {a} ({ASPECTS[a].width}x{ASPECTS[a].height})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">{t("video.fps")}</Label>
            <Input
              type="number"
              min={1}
              max={60}
              value={storyboard.canvas.fps}
              onChange={(e) =>
                onStoryboardChange({
                  ...storyboard,
                  canvas: {
                    ...storyboard.canvas,
                    fps: Number(e.target.value) || 30,
                  },
                })
              }
              className="text-base md:text-sm"
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">{t("video.output_height")}</Label>
            <Select
              value={String(storyboard.output?.height ?? 720)}
              onValueChange={(v) =>
                onStoryboardChange({
                  ...storyboard,
                  output: { ...storyboard.output, height: Number(v) },
                })
              }
            >
              <SelectTrigger className="text-base md:text-sm" aria-label={t("video.output_height")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[480, 720, 1080].map((h) => (
                  <SelectItem key={h} value={String(h)}>
                    {h}p
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>

      {/* Hardware Detection */}
      <div className="rounded-lg border p-4">
        <p className="mb-2 text-sm font-medium">{t("video.render_panel.hardware")}</p>
        <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
          <span>{t("video.render_panel.cores", { n: hardware.cores })}</span>
          {hardware.memoryGB !== null && (
            <>
              <span>|</span>
              <span>{t("video.render_panel.ram", { n: hardware.memoryGB })}</span>
            </>
          )}
        </div>
        <div className="mt-2">
          <Badge
            variant={hardware.recommendation === "client" ? "success" : "warning"}
            className="gap-1"
          >
            {hardware.recommendation === "client" ? (
              <>
                <CheckCircle2 className="h-3 w-3" />
                {t("video.render_panel.client_ok")}
              </>
            ) : (
              <>
                <AlertTriangle className="h-3 w-3" />
                {t("video.render_panel.server_recommended")}
              </>
            )}
          </Badge>
        </div>
      </div>

      {/* Export Actions */}
      <div className="rounded-lg border p-4">
        {!isExporting ? (
          <div className="flex flex-col gap-2">
            {webCodecsSupported && (
              <div className="flex flex-col gap-1.5">
                <Label className="text-xs">{t("video.render_panel.export_method")}</Label>
                <Select
                  value={exportMethod}
                  onValueChange={(v) => onExportMethodChange(v as ExportMethod)}
                >
                  <SelectTrigger className="text-base md:text-sm" aria-label={t("video.render_panel.export_method")}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="browser-fast">
                      {t("video.render_panel.method_browser_fast")}
                    </SelectItem>
                    <SelectItem value="browser-quality">
                      {t("video.render_panel.method_browser_quality")}
                    </SelectItem>
                    <SelectItem value="server">
                      {t("video.render_panel.server_method")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              {exportMethod !== "server" && (
                <Button
                  onClick={onExportWebCodecs}
                  disabled={totalSec <= 0}
                  className="min-h-11 sm:min-h-9"
                >
                  <Zap className="mr-2 h-4 w-4" />
                  {t("video.render_panel.webcodecs_method")}
                </Button>
              )}
              {exportMethod === "browser-quality" && (
                <Button
                  variant="outline"
                  onClick={onExportClient}
                  disabled={totalSec <= 0}
                  className="min-h-11 sm:min-h-9"
                >
                  <Monitor className="mr-2 h-4 w-4" />
                  {t("video.render_panel.client_method")}
                </Button>
              )}
              {exportMethod === "server" && (
                <Button
                  onClick={onExportServer}
                  disabled={totalSec <= 0}
                  className="min-h-11 sm:min-h-9"
                >
                  <Cloud className="mr-2 h-4 w-4" />
                  {t("video.render_panel.server_method")}
                </Button>
              )}
              {!webCodecsSupported && exportMethod !== "server" && (
                <Button
                  onClick={onExportClient}
                  disabled={totalSec <= 0}
                  className="min-h-11 sm:min-h-9"
                >
                  <Monitor className="mr-2 h-4 w-4" />
                  {t("video.render_panel.client_method")}
                </Button>
              )}
              {!webCodecsSupported && (
                <Button
                  variant="outline"
                  onClick={onExportServer}
                  disabled={totalSec <= 0}
                  className="min-h-11 sm:min-h-9"
                >
                  <Cloud className="mr-2 h-4 w-4" />
                  {t("video.render_panel.server_method")}
                </Button>
              )}
            </div>
            {isOverloaded && (
              <Badge variant="warning" className="gap-1 w-fit">
                <AlertTriangle className="h-3 w-3" />
                {t("video.render_panel.overloaded_warning")}
              </Badge>
            )}
            <p className="text-xs text-muted-foreground">
              {t("video.render_panel.client_hint")}
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <Loader2 className="h-4 w-4 animate-spin text-primary" />
              <span className="text-sm font-medium">
                {t("video.render_panel.render_button")}...
              </span>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={onCancel}
                className="ml-auto min-h-9 min-w-9 text-destructive hover:text-destructive"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
            {progress >= 0 && (
              <>
                <Progress value={progress} className="h-2" />
                <p className="text-xs text-muted-foreground">
                  {t("video.render_panel.percent", { pct: progress })}
                </p>
              </>
            )}
            {progress < 0 && (
              <p className="text-xs text-muted-foreground">
                {t("video.render_panel.server_processing")}
              </p>
            )}
          </div>
        )}
      </div>

      {/* Audio settings */}
      <div className="rounded-lg border p-4">
        <p className="mb-3 text-sm font-medium">{t("video.audio_section")}</p>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">{t("video.bgm_path")}</Label>
            <Input
              value={storyboard.audio?.bgm_path ?? ""}
              onChange={(e) =>
                onStoryboardChange({
                  ...storyboard,
                  audio: e.target.value
                    ? {
                        bgm_path: e.target.value,
                        bgm_volume: storyboard.audio?.bgm_volume,
                      }
                    : undefined,
                })
              }
              placeholder="audio/bgm.mp3"
              className="text-base md:text-sm"
            />
          </div>
          {storyboard.audio && (
            <div className="flex flex-col gap-1.5">
              <Label className="text-xs">{t("video.bgm_volume")}</Label>
              <Input
                type="number"
                step={0.05}
                min={0}
                max={1}
                value={storyboard.audio.bgm_volume ?? 0.2}
                onChange={(e) =>
                  onStoryboardChange({
                    ...storyboard,
                    audio: {
                      ...storyboard.audio!,
                      bgm_volume: Number(e.target.value),
                    },
                  })
                }
                className="text-base md:text-sm"
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
