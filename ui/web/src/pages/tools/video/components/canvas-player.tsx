import { useCallback, useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  ChevronLeft,
  ChevronRight,
  Volume2,
  Loader2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useTtsCapabilities } from "@/api/tts-capabilities";
import { useCanvasPlayer } from "../hooks/use-canvas-player";
import type { NarrationAudioController } from "../hooks/use-narration-audio";
import { FALLBACK_EDGE_VOICES } from "../hooks/use-narration-audio";
import type { Scene } from "../hooks/use-timeline";

// ── Types (Scene is the canonical model from use-timeline) ──

interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

interface CanvasPlayerProps {
  storyboard: Storyboard;
  narration?: NarrationAudioController;
  /** Storyboard-level default TTS voice (edge-tts id), applied at submit. */
  defaultVoice?: string;
  onDefaultVoiceChange?: (voice: string) => void;
}

// ── Time formatting ──

function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

// ── Component ──

export function CanvasPlayer({ storyboard, narration, defaultVoice, onDefaultVoiceChange }: CanvasPlayerProps) {
  const { t } = useTranslation("toolbox");
  const player = useCanvasPlayer(storyboard, narration);
  const containerRef = useRef<HTMLDivElement>(null);
  // The resize effect must not depend on the player's identity: player.state
  // changes every frame during playback, which would tear down and rebuild
  // the ResizeObserver per frame.
  const playerRef = useRef(player);
  playerRef.current = player;

  const { data: capabilities } = useTtsCapabilities();
  const edgeVoices = useMemo(() => {
    const fromApi = capabilities?.find((p) => p.provider === "edge")?.voices ?? [];
    return fromApi.length > 0 ? fromApi : FALLBACK_EDGE_VOICES;
  }, [capabilities]);

  // Auto-resize canvas to container (16px = the container's p-2 padding)
  const handleResize = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    const { width: sbW, height: sbH } = storyboard.canvas;
    const containerW = container.clientWidth - 16;
    const containerH = container.clientHeight - 16;
    if (containerW <= 0 || containerH <= 0) return;

    // Fit inside container maintaining aspect ratio
    const scale = Math.min(containerW / sbW, containerH / sbH, 1);
    const displayW = Math.round(sbW * scale);
    const displayH = Math.round(sbH * scale);

    playerRef.current.resize(displayW, displayH);
  }, [storyboard.canvas]);

  useEffect(() => {
    handleResize();
    const observer = new ResizeObserver(handleResize);
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }
    return () => observer.disconnect();
  }, [handleResize]);

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      switch (e.key) {
        case " ":
          e.preventDefault();
          if (player.state.isPlaying) player.pause();
          else player.play();
          break;
        case "ArrowLeft":
          e.preventDefault();
          player.stepFrame(-1);
          break;
        case "ArrowRight":
          e.preventDefault();
          player.stepFrame(1);
          break;
        case "Home":
          e.preventDefault();
          player.seek(0);
          break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [player]);

  const { isPlaying, currentTime, totalDuration, currentSceneIndex } = player.state;

  return (
    <div className="flex flex-col gap-3">
      {/* Canvas container — fixed height so the ResizeObserver → resize →
          layout loop cannot feed back into itself (that feedback half-painted
          the bitmap and hid captions). */}
      <div
        ref={containerRef}
        className="flex items-center justify-center rounded-lg border bg-black/90 p-2"
        style={{ height: "min(60vh, 560px)" }}
      >
        <canvas
          ref={player.canvasRef}
          className="block rounded"
          style={{ maxWidth: "100%", maxHeight: "100%" }}
        />
      </div>

      {/* Controls */}
      <div className="flex items-center gap-2 px-1">
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.seek(0)}
          aria-label={t("video.canvas.start")}
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          <SkipBack className="h-4 w-4" />
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.stepFrame(-1)}
          aria-label={t("video.canvas.prev_frame")}
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>

        <Button
          variant="outline"
          size="icon"
          onClick={() => (isPlaying ? player.pause() : player.play())}
          aria-label={isPlaying ? t("video.canvas.pause") : t("video.canvas.play")}
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          {isPlaying ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.stepFrame(1)}
          aria-label={t("video.canvas.next_frame")}
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.seek(totalDuration)}
          aria-label={t("video.canvas.end")}
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          <SkipForward className="h-4 w-4" />
        </Button>

        {/* Seek bar */}
        <div className="relative mx-2 flex-1">
          <input
            type="range"
            min={0}
            max={totalDuration * 100 || 1}
            value={currentTime * 100}
            onChange={(e) => player.seek(Number(e.target.value) / 100)}
            className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-secondary accent-primary [&::-webkit-slider-thumb]:h-3.5 [&::-webkit-slider-thumb]:w-3.5 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary"
          />
        </div>

        {/* Time display */}
        <span className="whitespace-nowrap font-mono text-xs text-muted-foreground">
          {formatTime(currentTime)} / {formatTime(totalDuration)}
        </span>
      </div>

      {/* Scene indicator + default voice + narration synth status */}
      <div className="flex flex-wrap items-center gap-2 px-1 text-xs text-muted-foreground">
        <span>
          {t("video.scene_n", { n: currentSceneIndex + 1 })} / {storyboard.scenes.length}
        </span>
        {narration && narration.pendingCount > 0 && (
          <span className="inline-flex items-center gap-1 text-primary">
            <Loader2 className="h-3 w-3 animate-spin" />
            {t("video.tts_loading")}
          </span>
        )}
        {onDefaultVoiceChange && (
          <div className="ml-auto flex items-center gap-1.5">
            <Volume2 className="h-3.5 w-3.5 shrink-0" />
            <Select value={defaultVoice ?? ""} onValueChange={onDefaultVoiceChange}>
              <SelectTrigger
                className="h-8 w-auto max-w-[220px] text-xs"
                aria-label={t("video.voice_global")}
              >
                <SelectValue placeholder={t("video.voice_global")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="auto" className="text-xs">
                  {t("video.voice_auto")}
                </SelectItem>
                {edgeVoices.map((v) => (
                  <SelectItem key={v.voice_id} value={v.voice_id} className="text-xs">
                    {v.name || v.voice_id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
      </div>
    </div>
  );
}
