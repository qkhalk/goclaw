import { useCallback, useRef, useState } from "react";
import { useHttp } from "@/hooks/use-ws";
import { submitRenderJob, type VideoRenderJob } from "./use-video";
import { renderSceneWithTransition, type SceneTransition } from "../components/scene-transition";

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

// ── MediaRecorder container support ──

/** Best container the browser can actually record. MP4 first (plays
 * everywhere) with a WebM fallback chain; "" lets the browser pick. */
function pickMimeType(): string {
  if (typeof MediaRecorder === "undefined") return "";
  const candidates = [
    "video/mp4;codecs=avc1.42E01E",
    "video/mp4",
    "video/webm;codecs=vp9",
    "video/webm;codecs=vp8",
    "video/webm",
  ];
  for (const mime of candidates) {
    if (MediaRecorder.isTypeSupported(mime)) return mime;
  }
  return "";
}

/** File extension matching the recorded container. */
export function exportExtension(blob: Blob): string {
  return blob.type.includes("mp4") ? "mp4" : "webm";
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

// ── Client-side export (single-pass MediaRecorder, wall-clock paced) ──

/** Render the storyboard to a video Blob entirely in the browser.
 *
 * MediaRecorder captures a canvas stream in REAL time, so frames are drawn
 * against the wall clock: drawing frame for elapsed time t keeps the output
 * duration equal to the storyboard duration no matter the display refresh
 * rate. (WebCodecs would allow faster-than-realtime encoding but there is no
 * muxer in the bundle to containerize the raw H.264 chunks.) */
async function exportWithMediaRecorder(
  storyboard: Storyboard,
  onProgress?: (pct: number) => void,
  signal?: AbortSignal,
): Promise<Blob> {
  if (typeof MediaRecorder === "undefined") {
    throw new Error("MediaRecorder is not supported in this browser");
  }

  const { width, height, fps } = storyboard.canvas;
  const totalSec = totalDuration(storyboard.scenes);
  if (totalSec <= 0) throw new Error("Storyboard has no duration");

  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Canvas 2D context unavailable");

  // Reusable offscreen buffers for transition compositing.
  const scratchA = document.createElement("canvas");
  scratchA.width = width;
  scratchA.height = height;
  const scratchB = document.createElement("canvas");
  scratchB.width = width;
  scratchB.height = height;

  const drawFrame = (time: number) => {
    const { index, localTime } = sceneAtTime(storyboard.scenes, time);
    renderSceneWithTransition(
      ctx,
      canvas,
      storyboard.scenes,
      index,
      localTime,
      imageCache,
      renderSceneToCanvas,
      scratchA,
      scratchB,
    );
  };

  // Preload all images (video scenes fall back to the dark placeholder —
  // same as the preview player).
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

  const stream = canvas.captureStream(fps);
  const mimeType = pickMimeType();
  const recorder = new MediaRecorder(
    stream,
    mimeType ? { mimeType, videoBitsPerSecond: 2_000_000 } : { videoBitsPerSecond: 2_000_000 },
  );

  const chunks: Blob[] = [];
  recorder.ondataavailable = (e) => {
    if (e.data.size > 0) chunks.push(e.data);
  };

  const finished = new Promise<Blob>((resolve, reject) => {
    recorder.onstop = () => {
      stream.getTracks().forEach((track) => track.stop());
      if (chunks.length === 0) {
        reject(new Error("No frames recorded"));
        return;
      }
      resolve(new Blob(chunks, { type: recorder.mimeType || "video/webm" }));
    };
    recorder.onerror = () => reject(new Error("MediaRecorder error"));
  });

  recorder.start(250);

  // Wall-clock draw loop: one rAF per display frame, scene chosen by elapsed
  // time so the recording always lasts exactly totalSec.
  const startMs = performance.now();
  const totalMs = totalSec * 1000;

  await new Promise<void>((resolve) => {
    const step = () => {
      const elapsedMs = performance.now() - startMs;
      if (signal?.aborted || elapsedMs >= totalMs) {
        resolve();
        return;
      }
      drawFrame(elapsedMs / 1000);
      if (onProgress) onProgress(Math.min(99, Math.round((elapsedMs / totalMs) * 100)));
      requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  });

  // Let the encoder flush the last drawn frame before closing the container.
  await new Promise((r) => setTimeout(r, 150));
  if (recorder.state !== "inactive") recorder.stop();

  if (signal?.aborted) throw new Error("Export cancelled");
  if (onProgress) onProgress(100);
  return finished;
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
        const blob = await exportWithMediaRecorder(
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
