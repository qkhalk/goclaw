import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { useCanvasPlayer } from "../hooks/use-canvas-player";

// ── Types ──

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
interface Scene {
  type: "image" | "video" | "color";
  source?: string;
  color?: string;
  duration_sec: number;
  fit?: "cover" | "contain";
  mute?: boolean;
  ken_burns?: KenBurns;
  caption?: Caption;
  narration?: string;
}
interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

interface CanvasPlayerProps {
  storyboard: Storyboard;
}

// ── Time formatting ──

function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

// ── Component ──

export function CanvasPlayer({ storyboard }: CanvasPlayerProps) {
  const { t } = useTranslation("toolbox");
  const player = useCanvasPlayer(storyboard);
  const containerRef = useRef<HTMLDivElement>(null);

  // Auto-resize canvas to container
  const handleResize = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const { width: sbW, height: sbH } = storyboard.canvas;
    const containerW = rect.width;
    const containerH = rect.height;

    // Fit inside container maintaining aspect ratio
    const scale = Math.min(containerW / sbW, containerH / sbH, 1);
    const displayW = Math.round(sbW * scale);
    const displayH = Math.round(sbH * scale);

    player.resize(displayW, displayH);
  }, [storyboard.canvas, player]);

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
          player.state.isPlaying ? player.pause() : player.play();
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
      {/* Canvas container */}
      <div
        ref={containerRef}
        className="flex items-center justify-center rounded-lg border bg-black/90 p-2"
        style={{ minHeight: 200 }}
      >
        <canvas
          ref={player.canvasRef}
          className="block rounded"
          style={{ maxWidth: "100%", maxHeight: "60vh" }}
        />
      </div>

      {/* Controls */}
      <div className="flex items-center gap-2 px-1">
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.seek(0)}
          aria-label="Go to start"
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          <SkipBack className="h-4 w-4" />
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.stepFrame(-1)}
          aria-label="Previous frame"
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
          aria-label="Next frame"
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.seek(totalDuration)}
          aria-label="Go to end"
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

      {/* Scene indicator */}
      <div className="flex items-center gap-2 px-1 text-xs text-muted-foreground">
        <span>
          {t("video.scene_n", { n: currentSceneIndex + 1 })} / {storyboard.scenes.length}
        </span>
      </div>
    </div>
  );
}
