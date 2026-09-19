import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useCanvasPlayer } from "../hooks/use-canvas-player";
import type { NarrationAudioController } from "../hooks/use-narration-audio";
import type { Scene } from "../hooks/use-timeline";
import { TransportBar } from "./studio/transport-bar";

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

// ── Component ──

/**
 * Studio preview stage: the canvas centered on a dark surface with a
 * Filmora-style transport strip underneath. Playback logic is unchanged —
 * this component owns only chrome (stage sizing + fullscreen).
 */
export function CanvasPlayer({ storyboard, narration, defaultVoice, onDefaultVoiceChange }: CanvasPlayerProps) {
  const { t } = useTranslation("toolbox");
  const player = useCanvasPlayer(storyboard, narration);
  const containerRef = useRef<HTMLDivElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  // The resize effect must not depend on the player's identity: player.state
  // changes every frame during playback, which would tear down and rebuild
  // the ResizeObserver per frame.
  const playerRef = useRef(player);
  playerRef.current = player;

  // Fullscreen is pure player chrome: the ResizeObserver below re-fits the
  // canvas when the stage enters/leaves fullscreen. The fullscreen element is
  // the component ROOT (stage + transport bar) — promoting only the stage
  // would hide the transport controls behind the fullscreen layer and leave
  // no way to leave fullscreen except Esc.
  const [isFullscreen, setIsFullscreen] = useState(false);
  useEffect(() => {
    const onFsChange = () => setIsFullscreen(document.fullscreenElement === rootRef.current);
    document.addEventListener("fullscreenchange", onFsChange);
    return () => document.removeEventListener("fullscreenchange", onFsChange);
  }, []);

  const toggleFullscreen = useCallback(() => {
    const el = rootRef.current;
    if (!el) return;
    if (document.fullscreenElement) {
      void document.exitFullscreen();
    } else if (typeof el.requestFullscreen === "function") {
      void el.requestFullscreen();
    }
  }, []);

  // Auto-resize canvas to container. Padding is handled by the container's
  // box (the stage centers the sized canvas), so the observer can use the
  // full box and clamp via maxWidth/maxHeight.
  const handleResize = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    const { width: sbW, height: sbH } = storyboard.canvas;
    const containerW = container.clientWidth;
    const containerH = container.clientHeight;
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

  // Keyboard shortcuts. The handler reads the player through playerRef so
  // the listener is bound once — the hook returns a new controller object
  // every render (per frame during playback), which would otherwise
  // re-subscribe this listener at frame rate.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      const player = playerRef.current;
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
  }, []);

  // iOS Safari lacks element requestFullscreen — gate the button on the
  // function existing, not just the (absent) fullscreenEnabled flag.
  const [fullscreenSupported, setFullscreenSupported] = useState(false);
  useEffect(() => {
    setFullscreenSupported(
      document.fullscreenEnabled !== false &&
        typeof rootRef.current?.requestFullscreen === "function",
    );
  }, []);

  return (
    <div ref={rootRef} className="flex h-full min-h-0 flex-col gap-3 p-3 sm:p-4">
      {/* Canvas stage — centered on the dark surface. Fixed height on mobile
          so the ResizeObserver → resize → layout loop cannot feed back into
          itself; fills the remaining editor height on desktop and in
          fullscreen (that feedback half-painted the bitmap and hid
          captions). */}
      <div className="flex min-h-0 flex-1 items-center justify-center">
        <div
          ref={containerRef}
          className="flex items-center justify-center rounded-xl bg-black ring-1 ring-white/10 max-lg:h-[min(60vh,560px)] max-lg:w-full lg:h-full lg:w-full"
        >
          <canvas
            ref={player.canvasRef}
            className="block"
            style={{ maxWidth: "100%", maxHeight: "100%" }}
            aria-label={t("video.title")}
          />
        </div>
      </div>

      {/* Transport controls */}
      <TransportBar
        player={player}
        sceneCount={storyboard.scenes.length}
        narration={narration}
        defaultVoice={defaultVoice}
        onDefaultVoiceChange={onDefaultVoiceChange}
        isFullscreen={isFullscreen}
        onToggleFullscreen={() => {
          if (fullscreenSupported) toggleFullscreen();
        }}
      />
    </div>
  );
}
