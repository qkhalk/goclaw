import { useCallback, useEffect, useRef, useState } from "react";

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

function renderScene(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scene: Scene,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
) {
  const { width, height } = canvas;

  // Clear
  ctx.clearRect(0, 0, width, height);

  if (scene.type === "color") {
    ctx.fillStyle = scene.color || "#000000";
    ctx.fillRect(0, 0, width, height);
    return;
  }

  // For image/video scenes, draw the image
  const imgSrc = scene.source;
  if (!imgSrc) {
    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, width, height);
    ctx.fillStyle = "#666";
    ctx.font = `${Math.min(width, height) * 0.04}px sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillText("No source", width / 2, height / 2);
    return;
  }

  const img = imageCache.get(imgSrc);
  if (!img) {
    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, width, height);
    return;
  }

  // Ken Burns effect
  let scale = 1;
  let offsetX = 0;
  let offsetY = 0;

  if (scene.ken_burns) {
    const progress = scene.duration_sec > 0 ? localTime / scene.duration_sec : 0;
    const kb = scene.ken_burns;
    scale = kb.zoom_from + (kb.zoom_to - kb.zoom_from) * progress;

    // Pan offset
    const panAmount = (scale - 1) * Math.min(width, height) * 0.5;
    switch (kb.pan) {
      case "left":
        offsetX = panAmount * progress;
        break;
      case "right":
        offsetX = -panAmount * progress;
        break;
      case "up":
        offsetY = panAmount * progress;
        break;
      case "down":
        offsetY = -panAmount * progress;
        break;
    }
  }

  // Fit image (cover or contain)
  const imgAspect = img.naturalWidth / img.naturalHeight;
  const canvasAspect = width / height;
  let drawW: number;
  let drawH: number;

  if (scene.fit === "contain") {
    if (imgAspect > canvasAspect) {
      drawW = width * scale;
      drawH = (width / imgAspect) * scale;
    } else {
      drawH = height * scale;
      drawW = (height * imgAspect) * scale;
    }
  } else {
    // cover (default)
    if (imgAspect > canvasAspect) {
      drawH = height * scale;
      drawW = (height * imgAspect) * scale;
    } else {
      drawW = width * scale;
      drawH = (width / imgAspect) * scale;
    }
  }

  const x = (width - drawW) / 2 + offsetX;
  const y = (height - drawH) / 2 + offsetY;

  ctx.drawImage(img, x, y, drawW, drawH);

  // Caption
  if (scene.caption?.text) {
    const fontSize = scene.caption.font_size || Math.round(Math.min(width, height) * 0.035);
    ctx.font = `bold ${fontSize}px sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";

    // Text shadow for readability
    ctx.shadowColor = "rgba(0,0,0,0.7)";
    ctx.shadowBlur = fontSize * 0.25;
    ctx.shadowOffsetX = 0;
    ctx.shadowOffsetY = fontSize * 0.05;

    const text = scene.caption.text;
    const lines = wrapText(ctx, text, width * 0.85);
    const lineHeight = fontSize * 1.3;
    const totalTextH = lines.length * lineHeight;

    let baseY: number;
    switch (scene.caption.position) {
      case "top":
        baseY = totalTextH / 2 + fontSize;
        break;
      case "center":
        baseY = height / 2;
        break;
      default:
        baseY = height - totalTextH / 2 - fontSize;
        break;
    }

    ctx.fillStyle = "#fff";
    lines.forEach((line, li) => {
      ctx.fillText(line, width / 2, baseY + (li - (lines.length - 1) / 2) * lineHeight);
    });

    ctx.shadowColor = "transparent";
    ctx.shadowBlur = 0;
  }
}

function wrapText(ctx: CanvasRenderingContext2D, text: string, maxWidth: number): string[] {
  const words = text.split(/\s+/);
  const lines: string[] = [];
  let current = "";

  for (const word of words) {
    const test = current ? `${current} ${word}` : word;
    if (ctx.measureText(test).width > maxWidth && current) {
      lines.push(current);
      current = word;
    } else {
      current = test;
    }
  }
  if (current) lines.push(current);
  return lines.length > 0 ? lines : [""];
}

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

    renderScene(ctx, canvas, scene, localTime, imageCacheRef.current);

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
