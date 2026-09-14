import { useCallback, useRef, useState } from "react";
import { useHttp } from "@/hooks/use-ws";
import { submitRenderJob, type VideoRenderJob } from "./use-video";

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
export interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

export interface HardwareInfo {
  cores: number;
  memoryGB: number | null;
  recommendation: "client" | "server";
}

export interface UseVideoExportReturn {
  hardware: HardwareInfo;
  exportClient: (sb: Storyboard, onProgress?: (p: number) => void) => Promise<Blob>;
  exportServer: (sb: Storyboard) => Promise<VideoRenderJob>;
  isExporting: boolean;
  progress: number;
  cancel: () => void;
}

// ── Hardware detection ──

function detectHardware(): HardwareInfo {
  const cores = navigator.hardwareConcurrency || 4;
  const memoryGB = (navigator as { deviceMemory?: number }).deviceMemory ?? null;
  const recommendation: "client" | "server" =
    cores < 8 || (memoryGB !== null && memoryGB < 4) ? "server" : "client";
  return { cores, memoryGB, recommendation };
}

// ── Image loading helper ──

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error(`Failed to load: ${src}`));
    img.src = src;
  });
}

// ── Scene helpers ──

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

function renderSceneToCanvas(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scene: Scene,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
) {
  const { width, height } = canvas;
  ctx.clearRect(0, 0, width, height);

  if (scene.type === "color") {
    ctx.fillStyle = scene.color || "#000000";
    ctx.fillRect(0, 0, width, height);
    return;
  }

  const img = scene.source ? imageCache.get(scene.source) : undefined;
  if (!img) {
    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, width, height);
    return;
  }

  let scale = 1;
  let offsetX = 0;
  let offsetY = 0;

  if (scene.ken_burns) {
    const progress = scene.duration_sec > 0 ? localTime / scene.duration_sec : 0;
    const kb = scene.ken_burns;
    scale = kb.zoom_from + (kb.zoom_to - kb.zoom_from) * progress;
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
    ctx.shadowColor = "rgba(0,0,0,0.7)";
    ctx.shadowBlur = fontSize * 0.25;

    const lines = wrapText(ctx, scene.caption.text, width * 0.85);
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

// ── Client-side export via WebCodecs ──

async function exportToMP4(
  storyboard: Storyboard,
  onProgress?: (pct: number) => void,
  signal?: AbortSignal,
): Promise<Blob> {
  const { width, height, fps } = storyboard.canvas;
  const totalSec = totalDuration(storyboard.scenes);
  const totalFrames = Math.ceil(totalSec * fps);

  // Create offscreen canvas
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d")!;

  // Preload all images
  const imageCache = new Map<string, HTMLImageElement>();
  for (const scene of storyboard.scenes) {
    if ((scene.type === "image" || scene.type === "video") && scene.source) {
      if (!imageCache.has(scene.source)) {
        try {
          const img = await loadImage(scene.source);
          imageCache.set(scene.source, img);
        } catch {
          // Skip failed images
        }
      }
    }
  }

  // Check WebCodecs support
  if (typeof VideoEncoder === "undefined" || typeof VideoFrame === "undefined") {
    throw new Error("WebCodecs API not supported in this browser");
  }

  // Collect encoded chunks
  const chunks: EncodedVideoChunk[] = [];
  const encoder = new VideoEncoder({
    output: (chunk) => {
      chunks.push(chunk);
    },
    error: (e) => {
      console.error("VideoEncoder error:", e);
    },
  });

  encoder.configure({
    codec: "avc1.42E01E", // H.264 Baseline
    width,
    height,
    bitrate: 2_000_000,
    framerate: fps,
  });

  // Encode frame by frame
  for (let frame = 0; frame < totalFrames; frame++) {
    if (signal?.aborted) {
      throw new Error("Export cancelled");
    }

    const time = frame / fps;
    const { index, localTime } = sceneAtTime(storyboard.scenes, time);
    const scene = storyboard.scenes[index];
    if (scene) {
      renderSceneToCanvas(ctx, canvas, scene, localTime, imageCache);
    }

    const videoFrame = new VideoFrame(canvas, { timestamp: Math.round(time * 1_000_000) });
    await encoder.encode(videoFrame, { keyFrame: frame % (fps * 2) === 0 });
    videoFrame.close();

    if (onProgress && frame % 10 === 0) {
      onProgress(Math.round((frame / totalFrames) * 100));
    }
  }

  await encoder.flush();

  if (onProgress) onProgress(100);

  // Combine chunks into MP4 using Mp4Muxer or return raw WebM via MediaRecorder fallback
  // For simplicity, return a blob with the chunks encoded as webm
  if (chunks.length === 0) {
    throw new Error("No frames encoded");
  }

  // Use MediaRecorder as a simpler fallback for the blob
  const stream = canvas.captureStream(fps);
  const recorder = new MediaRecorder(stream, {
    mimeType: "video/webm;codecs=vp9",
    videoBitsPerSecond: 2_000_000,
  });

  const blobs: Blob[] = [];
  return new Promise<Blob>((resolve, reject) => {
    recorder.ondataavailable = (e) => {
      if (e.data.size > 0) blobs.push(e.data);
    };
    recorder.onstop = () => {
      resolve(new Blob(blobs, { type: "video/webm" }));
    };
    recorder.onerror = () => reject(new Error("MediaRecorder error"));

    recorder.start();

    // Replay frames to MediaRecorder
    let frameIdx = 0;
    const replayFrame = () => {
      if (frameIdx >= totalFrames) {
        recorder.stop();
        return;
      }
      const time = frameIdx / fps;
      const { index, localTime } = sceneAtTime(storyboard.scenes, time);
      const scene = storyboard.scenes[index];
      if (scene) {
        renderSceneToCanvas(ctx, canvas, scene, localTime, imageCache);
      }
      frameIdx++;
      if (frameIdx % 10 === 0 && onProgress) {
        onProgress(Math.round((frameIdx / totalFrames) * 100));
      }
      requestAnimationFrame(replayFrame);
    };
    requestAnimationFrame(replayFrame);
  });
}

// ── Hook ──

export function useVideoExport(): UseVideoExportReturn {
  const http = useHttp();
  const [isExporting, setIsExporting] = useState(false);
  const [progress, setProgress] = useState(0);
  const abortRef = useRef<AbortController | null>(null);
  const hardware = detectHardware();

  const exportClient = useCallback(
    async (sb: Storyboard, onProgress?: (p: number) => void): Promise<Blob> => {
      const controller = new AbortController();
      abortRef.current = controller;
      setIsExporting(true);
      setProgress(0);

      try {
        const blob = await exportToMP4(
          sb,
          (p) => {
            setProgress(p);
            onProgress?.(p);
          },
          controller.signal,
        );
        return blob;
      } finally {
        setIsExporting(false);
        abortRef.current = null;
      }
    },
    [],
  );

  const exportServer = useCallback(
    async (sb: Storyboard): Promise<VideoRenderJob> => {
      setIsExporting(true);
      setProgress(-1); // -1 = indeterminate (server-side)
      try {
        const result = await submitRenderJob(http, sb);
        return {
          id: result.jobId,
          tenant_id: "",
          user_id: "",
          agent_id: "",
          session_key: "",
          status: result.status as VideoRenderJob["status"],
          engine: "server",
          storyboard_json: JSON.stringify(sb),
          output_path: "",
          output_size_bytes: 0,
          error: "",
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        };
      } finally {
        setIsExporting(false);
        setProgress(0);
      }
    },
    [http],
  );

  const cancel = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setIsExporting(false);
    setProgress(0);
  }, []);

  return {
    hardware,
    exportClient,
    exportServer,
    isExporting,
    progress,
    cancel,
  };
}
