import { useCallback, useEffect, useRef, useState } from "react";
import type { SceneTransition } from "../components/scene-transition";
import { drawStoryboardFrame } from "../components/render-shared";

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
  transition?: SceneTransition;
}
interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

export interface CanvasPlayerState {
  isPlaying: boolean;
  currentTime: number;
  currentSceneIndex: number;
  fps: number;
  totalDuration: number;
}

export interface UseCanvasPlayerReturn {
  state: CanvasPlayerState;
  canvasRef: React.RefObject<HTMLCanvasElement | null>;
  play: () => void;
  pause: () => void;
  seek: (time: number) => void;
  stepFrame: (delta: number) => void;
  resize: (width: number, height: number) => void;
}

// ── Helpers ──

function totalDuration(scenes: Scene[]): number {
  return scenes.reduce((acc, s) => acc + (Number(s.duration_sec) || 0), 0);
}

function sceneAtTime(scenes: Scene[], time: number): { index: number; localTime: number } {
  let elapsed = 0;
  for (let i = 0; i < scenes.length; i++) {
    const dur = Number(scenes[i]?.duration_sec) || 0;
    if (time < elapsed + dur || i === scenes.length - 1) {
      return { index: i, localTime: time - elapsed };
    }
    elapsed += dur;
  }
  return { index: 0, localTime: 0 };
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error(`Failed to load image: ${src}`));
    img.src = src;
  });
}

// ── Canvas rendering ──

// ── Hook ──

export function useCanvasPlayer(storyboard: Storyboard): UseCanvasPlayerReturn {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [state, setState] = useState<CanvasPlayerState>({
    isPlaying: false,
    currentTime: 0,
    currentSceneIndex: 0,
    fps: storyboard.canvas.fps,
    totalDuration: totalDuration(storyboard.scenes),
  });

  const isPlayingRef = useRef(false);
  const currentTimeRef = useRef(0);
  const rafRef = useRef<number>(0);
  const lastFrameTimeRef = useRef(0);
  const imageCacheRef = useRef(new Map<string, HTMLImageElement>());
  const loadingImagesRef = useRef(new Set<string>());
  // Reusable offscreen buffers for transition compositing.
  const scratchARef = useRef<HTMLCanvasElement | null>(null);
  const scratchBRef = useRef<HTMLCanvasElement | null>(null);

  // Preload images
  useEffect(() => {
    for (const scene of storyboard.scenes) {
      if ((scene.type === "image" || scene.type === "video") && scene.source) {
        if (!imageCacheRef.current.has(scene.source) && !loadingImagesRef.current.has(scene.source)) {
          loadingImagesRef.current.add(scene.source);
          loadImage(scene.source)
            .then((img) => {
              imageCacheRef.current.set(scene.source!, img);
              loadingImagesRef.current.delete(scene.source!);
              // Force re-render after image loads
              if (!isPlayingRef.current) {
                renderCurrentFrame();
              }
            })
            .catch(() => {
              loadingImagesRef.current.delete(scene.source!);
            });
        }
      }
    }
  }, [storyboard.scenes]);

  const renderCurrentFrame = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const time = currentTimeRef.current;
    const { index, localTime } = sceneAtTime(storyboard.scenes, time);
    const scene = storyboard.scenes[index];
    if (!scene) return;

    if (!scratchARef.current) scratchARef.current = document.createElement("canvas");
    if (!scratchBRef.current) scratchBRef.current = document.createElement("canvas");
    drawStoryboardFrame(
      ctx,
      canvas,
      storyboard.scenes,
      index,
      localTime,
      imageCacheRef.current,
      scratchARef.current,
      scratchBRef.current,
    );

    setState((prev) => ({
      ...prev,
      currentTime: time,
      currentSceneIndex: index,
      fps: storyboard.canvas.fps,
      totalDuration: totalDuration(storyboard.scenes),
    }));
  }, [storyboard]);

  const frameLoop = useCallback(
    (timestamp: number) => {
      if (!isPlayingRef.current) return;

      if (lastFrameTimeRef.current === 0) {
        lastFrameTimeRef.current = timestamp;
      }

      const delta = (timestamp - lastFrameTimeRef.current) / 1000;
      lastFrameTimeRef.current = timestamp;

      const total = totalDuration(storyboard.scenes);
      currentTimeRef.current = Math.min(currentTimeRef.current + delta, total);

      if (currentTimeRef.current >= total) {
        // Loop back to start
        currentTimeRef.current = 0;
      }

      renderCurrentFrame();
      rafRef.current = requestAnimationFrame(frameLoop);
    },
    [storyboard.scenes, renderCurrentFrame],
  );

  const play = useCallback(() => {
    isPlayingRef.current = true;
    lastFrameTimeRef.current = 0;
    setState((prev) => ({ ...prev, isPlaying: true }));
    rafRef.current = requestAnimationFrame(frameLoop);
  }, [frameLoop]);

  const pause = useCallback(() => {
    isPlayingRef.current = false;
    cancelAnimationFrame(rafRef.current);
    setState((prev) => ({ ...prev, isPlaying: false }));
  }, []);

  const seek = useCallback(
    (time: number) => {
      currentTimeRef.current = Math.max(0, Math.min(time, totalDuration(storyboard.scenes)));
      renderCurrentFrame();
    },
    [storyboard.scenes, renderCurrentFrame],
  );

  const stepFrame = useCallback(
    (delta: number) => {
      const frameDuration = 1 / storyboard.canvas.fps;
      currentTimeRef.current = Math.max(
        0,
        Math.min(currentTimeRef.current + delta * frameDuration, totalDuration(storyboard.scenes)),
      );
      renderCurrentFrame();
    },
    [storyboard.canvas.fps, storyboard.scenes, renderCurrentFrame],
  );

  const resize = useCallback(
    (width: number, height: number) => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      canvas.width = width;
      canvas.height = height;
      renderCurrentFrame();
    },
    [renderCurrentFrame],
  );

  // Initial render
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas && canvas.width > 0 && canvas.height > 0) {
      renderCurrentFrame();
    }
  }, [storyboard, renderCurrentFrame]);

  // Cleanup
  useEffect(() => {
    return () => {
      cancelAnimationFrame(rafRef.current);
    };
  }, []);

  return {
    state,
    canvasRef,
    play,
    pause,
    seek,
    stepFrame,
    resize,
  };
}
